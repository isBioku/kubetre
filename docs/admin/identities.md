# Identities

KubeTRE needs three Entra ID app registrations and one group, all created by
`hack/entra-setup.sh`. The script is idempotent and short enough to read as the manual
recipe.

| AzureTRE identity | KubeTRE |
|---|---|
| API (`<TRE_ID> API`) | `kubetre-<env>-api` |
| Client UX (`<TRE_ID> UX`) | `kubetre-<env>-ui` |
| none (Guacamole used the workspace app) | `kubetre-<env>-gateway` |
| Application administrator | not needed: KubeTRE never changes Entra ID at runtime |
| Automation admin (tests, CI) | not available yet |
| One app per workspace | not needed: workspace members are kept on the `Workspace` resource |

None of KubeTRE's identities needs Microsoft Graph application permissions or admin consent.

## The API: `kubetre-<env>-api`

Secures the AzureTRE-compatible API at `/api` and KubeTRE's own API at `/api/v1`.

- **Identifier URI:** `api://<app id>`. Its `user_impersonation` scope is what the UI and
  the Azure CLI request.
- **Tokens:** version 2 (`requestedAccessTokenVersion: 2`), so their issuer ends in `/v2.0`.
- **Optional claim:** `email` in access and ID tokens. Workspace membership is matched on it.
- **App roles:**

  | Value | Display name | Grants |
  |---|---|---|
  | `TREAdmin` | TRE administrator | create, update and delete workspaces |
  | `TREUser` | TRE user | use the workspaces you are a member of |

  Roles can be assigned to users only, not to applications.
- **Pre-authorised clients:** the Azure CLI and the UI app, so neither asks for consent.
- **No client secret:** the API only validates tokens.

Terraform settings: `oidc_audience` is the app ID and `oidc_issuer` is
`https://login.microsoftonline.com/<tenant>/v2.0`.

A token for scripts:

```sh
az account get-access-token --scope api://<app id>/user_impersonation --query accessToken -o tsv
```

## The UI: `kubetre-<env>-ui`

A single-page application for AzureTRE's UI.

- **Redirect URIs (SPA):** `https://<host>`, `https://<host>/` and `https://<host>/logout`.
- **Permission:** the API's `user_impersonation` scope, pre-authorised on the API.
- **Configuration:** `ui_client_id`. The UI reads it at runtime from `/config.js`.

## The access gateway: `kubetre-<env>-gateway`

A confidential web application the broker uses to sign users in before it opens a remote
session.

- **Redirect URI:** `https://<host>/gateway/callback`.
- **Optional claim:** `email` in the ID token.
- **Client secret:** valid for one year. It is written to
  `kubetre-<env>-gateway-client-secret.txt` and stored in the cluster as the
  `kubetre-gateway-oidc` secret.
- **Configuration:** `gateway_oidc_client_id`.

To rotate the secret before it expires, delete the local file, rerun the script, and recreate
the Kubernetes secret:

```sh
rm kubetre-<env>-gateway-client-secret.txt
hack/entra-setup.sh <env> <host>
kubectl -n kubetre-gateway create secret generic kubetre-gateway-oidc \
  --from-file=client-secret=kubetre-<env>-gateway-client-secret.txt --dry-run=client -o yaml | kubectl apply -f -
kubectl -n kubetre-gateway rollout restart deploy/broker
```

## The admin group: `kubetre-<env>-admins`

Members are AKS cluster administrators (`admin_group_object_ids`). Local Kubernetes accounts
are disabled, so this group is the way in for operators.

## Workspace roles

Workspace owners, researchers and airlock managers are lists of email addresses on each
`Workspace`, entered in the workspace's form. The API compares them with the email, or the
subject, in the caller's token and returns the caller's roles to the UI. Nothing is created in
Entra ID per workspace, so no consent is needed when a workspace is created.

## Other identity providers

The API and the gateway work with any OpenID Connect provider, such as Keycloak or Dex. Set
`oidc_issuer`, `oidc_audience` and `roles_claim` (a dot-separated path, for example
`realm_access.roles`), and register equivalent clients.
