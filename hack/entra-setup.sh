#!/usr/bin/env bash
# Creates the Entra ID objects a KubeTRE environment needs, then prints the Terraform
# variables to use. Run it once per environment with the Azure CLI signed in as someone
# allowed to create app registrations and groups.
#
#   hack/entra-setup.sh <environment-name> <gateway-hostname>
#   hack/entra-setup.sh kubetredev gateway.tre.example.org
#
# It creates:
#   - group   kubetre-<env>-admins             AKS cluster administrators (you are added)
#   - app     kubetre-<env>-api                the API: app roles TREAdmin and TREUser,
#                                              v2 tokens, scope user_impersonation that the
#                                              Azure CLI may request without a consent prompt
#   - app     kubetre-<env>-gateway            the access gateway: redirect /callback, secret
# and assigns you the TREAdmin role.
set -euo pipefail

env_name="${1:?usage: $0 <environment-name> <gateway-hostname>}"
gateway_host="${2:?usage: $0 <environment-name> <gateway-hostname>}"
graph="https://graph.microsoft.com/v1.0"
azure_cli_app_id="04b07795-8ddb-461a-bbee-02f9e1bf7b46" # Microsoft Azure CLI, first-party

uuid() { python3 -c 'import uuid; print(uuid.uuid4())'; }
tenant_id="$(az account show --query tenantId -o tsv)"
me="$(az ad signed-in-user show --query id -o tsv)"

existing_app() { az ad app list --display-name "$1" --query '[0].appId' -o tsv; }

# --- admin group -------------------------------------------------------------------------
group_name="kubetre-${env_name}-admins"
group_id="$(az ad group list --display-name "$group_name" --query '[0].id' -o tsv)"
if [[ -z "$group_id" ]]; then
  group_id="$(az ad group create --display-name "$group_name" --mail-nickname "$group_name" --query id -o tsv)"
  echo "created group $group_name"
fi
az ad group member check --group "$group_id" --member-id "$me" --query value -o tsv | grep -q true \
  || az ad group member add --group "$group_id" --member-id "$me"

# --- API app -----------------------------------------------------------------------------
api_name="kubetre-${env_name}-api"
api_app_id="$(existing_app "$api_name")"
if [[ -z "$api_app_id" ]]; then
  admin_role_id="$(uuid)"; user_role_id="$(uuid)"
  roles="$(mktemp)"; trap 'rm -f "$roles"' EXIT
  cat > "$roles" <<JSON
[
  {"id": "$admin_role_id", "allowedMemberTypes": ["User"], "isEnabled": true, "value": "TREAdmin",
   "displayName": "TRE administrator", "description": "Creates and deletes workspaces."},
  {"id": "$user_role_id", "allowedMemberTypes": ["User"], "isEnabled": true, "value": "TREUser",
   "displayName": "TRE user", "description": "Uses the workspaces they are a member of."}
]
JSON
  api_app_id="$(az ad app create --display-name "$api_name" --sign-in-audience AzureADMyOrg \
    --app-roles @"$roles" --query appId -o tsv)"
  echo "created app $api_name ($api_app_id)"
fi
api_object_id="$(az ad app show --id "$api_app_id" --query id -o tsv)"
az ad app update --id "$api_app_id" --identifier-uris "api://${api_app_id}"

# Tokens must be v2 so their issuer matches https://login.microsoftonline.com/<tenant>/v2.0.
scope_id="$(az ad app show --id "$api_app_id" --query "api.oauth2PermissionScopes[?value=='user_impersonation'].id | [0]" -o tsv)"
[[ -n "$scope_id" ]] || scope_id="$(uuid)"
scope_json="{\"id\":\"$scope_id\",\"value\":\"user_impersonation\",\"type\":\"User\",\"isEnabled\":true,
  \"adminConsentDisplayName\":\"Access KubeTRE\",\"adminConsentDescription\":\"Call the KubeTRE API as the signed-in user.\",
  \"userConsentDisplayName\":\"Access KubeTRE\",\"userConsentDescription\":\"Call the KubeTRE API as you.\"}"
az rest --method PATCH --uri "$graph/applications/$api_object_id" --headers Content-Type=application/json \
  --body "{\"api\":{\"requestedAccessTokenVersion\":2,\"oauth2PermissionScopes\":[$scope_json]}}"
# Pre-authorise the Azure CLI, so operators can run: az account get-access-token --scope api://<id>/user_impersonation
az rest --method PATCH --uri "$graph/applications/$api_object_id" --headers Content-Type=application/json \
  --body "{\"api\":{\"requestedAccessTokenVersion\":2,\"oauth2PermissionScopes\":[$scope_json],
    \"preAuthorizedApplications\":[{\"appId\":\"$azure_cli_app_id\",\"delegatedPermissionIds\":[\"$scope_id\"]}]}}"
# Put the user's email in tokens; workspace membership is matched on it.
az ad app update --id "$api_app_id" --optional-claims '{"accessToken":[{"name":"email"}],"idToken":[{"name":"email"}]}'

api_sp_id="$(az ad sp show --id "$api_app_id" --query id -o tsv 2>/dev/null || true)"
[[ -n "$api_sp_id" ]] || api_sp_id="$(az ad sp create --id "$api_app_id" --query id -o tsv)"

admin_role_id="$(az ad app show --id "$api_app_id" --query "appRoles[?value=='TREAdmin'].id | [0]" -o tsv)"
assigned="$(az rest --method GET --uri "$graph/servicePrincipals/$api_sp_id/appRoleAssignedTo" \
  --query "value[?principalId=='$me' && appRoleId=='$admin_role_id'] | length(@)" -o tsv)"
if [[ "$assigned" == "0" ]]; then
  az rest --method POST --uri "$graph/servicePrincipals/$api_sp_id/appRoleAssignedTo" --headers Content-Type=application/json \
    --body "{\"principalId\":\"$me\",\"resourceId\":\"$api_sp_id\",\"appRoleId\":\"$admin_role_id\"}" >/dev/null
  echo "assigned you the TREAdmin role"
fi

# --- gateway app -------------------------------------------------------------------------
gw_name="kubetre-${env_name}-gateway"
gw_app_id="$(existing_app "$gw_name")"
if [[ -z "$gw_app_id" ]]; then
  gw_app_id="$(az ad app create --display-name "$gw_name" --sign-in-audience AzureADMyOrg \
    --web-redirect-uris "https://${gateway_host}/callback" --query appId -o tsv)"
  echo "created app $gw_name ($gw_app_id)"
else
  az ad app update --id "$gw_app_id" --web-redirect-uris "https://${gateway_host}/callback"
fi
az ad app update --id "$gw_app_id" --optional-claims '{"idToken":[{"name":"email"}]}'
az ad sp show --id "$gw_app_id" >/dev/null 2>&1 || az ad sp create --id "$gw_app_id" >/dev/null

secret_file="kubetre-${env_name}-gateway-client-secret.txt"
if [[ ! -s "$secret_file" ]]; then
  umask 077
  az ad app credential reset --id "$gw_app_id" --display-name "kubetre-gateway" --years 1 --append \
    --query password -o tsv > "$secret_file"
  echo "wrote the gateway client secret to ./$secret_file (keep it out of Git)"
fi

cat <<OUT

Add these to infra/azure/terraform.tfvars:

admin_group_object_ids = ["$group_id"]
oidc_issuer            = "https://login.microsoftonline.com/${tenant_id}/v2.0"
oidc_audience          = "$api_app_id"
gateway_hostname       = "$gateway_host"
gateway_oidc_client_id = "$gw_app_id"

Get an API token with:
  az account get-access-token --scope api://${api_app_id}/user_impersonation --query accessToken -o tsv
OUT
