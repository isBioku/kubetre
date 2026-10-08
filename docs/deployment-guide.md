# Deploying KubeTRE on Azure

This guide takes you from an empty Azure subscription to a researcher working in a Windows VM
inside an isolated workspace. Follow the steps in order. Expect the first run to take two to
three hours, most of it waiting for Azure.

KubeTRE is early-stage and has not yet been run end to end in Azure. Treat the first
deployment as a test environment and read [Security gaps](#security-gaps) before putting
sensitive data in it.

## What you will build

```text
Researcher's browser
   │ HTTPS (only inbound path)
   ▼
Azure Firewall ── DNAT 443 ──► Envoy Gateway (internal load balancer)
   ▲                              ├── /          AzureTRE's UI (Microsoft, MIT)
   │ all egress                   ├── /api/      AzureTRE-compatible API, and KubeTRE's /api/v1
   │                              ├── /gateway   access gateway broker (OIDC sign-in, Connect)
   │                              └── /guacamole Apache Guacamole ──► guacd ──► VMs and desktops
AKS (system, work and gateway node pools)
   ├── KubeTRE controller ── Crossplane ──► per-workspace subnet, NSG, firewall rules, Azure VMs
   └── Flux ◄── this Git repository
```

## Costs

Several components run all the time, before anyone logs in: Azure Firewall Standard, the AKS
Standard tier, Advanced Container Networking Services (for FQDN filtering), at least six nodes
across the system, work and gateway pools, ACR Premium and Log Analytics ingestion. Check the
Azure pricing calculator for your region before you start, and tear the environment down when
you are not using it (see [Teardown](#teardown)).

## 1. Prerequisites

**Tools** on your workstation:

| Tool | Version |
|---|---|
| Azure CLI (`az`) | 2.60 or later |
| Terraform | 1.9 or later |
| `kubectl` and `kubelogin` | `az aks install-cli` installs both |
| GNU Make, Python 3, Git | any recent |
| Go | 1.27, only if you want to run the tests |

**Azure permissions.** You need Owner on the subscription, or Contributor plus User Access
Administrator, because Terraform creates role assignments and custom roles. In Entra ID you
need permission to create app registrations and groups.

**Register providers and features** once per subscription:

```sh
az login
az account set --subscription <subscription-id>

for p in Microsoft.ContainerService Microsoft.Network Microsoft.Compute \
         Microsoft.ContainerRegistry Microsoft.OperationalInsights Microsoft.ManagedIdentity \
         Microsoft.KubernetesConfiguration Microsoft.Insights Microsoft.Storage; do
  az provider register --namespace "$p"
done

az feature register --namespace Microsoft.Compute --name EncryptionAtHost
az feature show --namespace Microsoft.Compute --name EncryptionAtHost --query properties.state
az provider register --namespace Microsoft.Compute   # once the feature shows "Registered"
```

If you cannot register `EncryptionAtHost`, set `host_encryption_enabled = false` in step 3.

**Check quota and size availability.** New pay-as-you-go subscriptions usually allow only 10
vCPUs per region, and Azure may withhold some sizes or zones from them. The default cluster
needs about 24 vCPUs of the Dsv5 family before any research VM:

```sh
az vm list-usage --location uksouth -o table | grep -E "Total Regional|DSv5"
az vm list-skus --location uksouth --size Standard_D4s_v5 --all \
  --query "[].restrictions[].{type:type, reason:reasonCode, zones:restrictionInfo.zones}" -o table
```

Request more with `az quota update` (extension `quota`) or in the portal under Quotas; small
increases for Dsv5 are usually approved within minutes. Set `zones` in step 3 to the zones
that are not restricted. For short test runs within 10 vCPUs, apply with the small profile
(`-var-file=profiles/small.tfvars`): one node per pool and smaller research VMs.

**Decide** before you start:

- An environment name: 3 to 12 lowercase letters or digits, for example `kubetredev`. It is
  used in every resource name, including the registry, so it must be globally unique.
- An Azure region, for example `uksouth`.
- The public IP ranges you and your CI work from. Only these reach the cluster API and the
  registry.
- A hostname for the gateway, for example `gateway.tre.example.org`, in a DNS zone you control,
  and a way to get a TLS certificate for it.

## 2. Create the Entra ID objects

Clone the repository and run the setup script. It creates an admin group, the API and gateway
app registrations with the right roles, redirect URI and token settings, and makes you a TRE
administrator.

```sh
git clone https://github.com/isBioku/kubetre.git
cd kubetre
hack/entra-setup.sh kubetredev gateway.tre.example.org
```

It prints the Terraform variables for the next step and writes the gateway's client secret to
`kubetre-kubetredev-gateway-client-secret.txt`. That file is ignored by Git; keep it safe.

Users get access in two ways:

- **TRE administrators** need the `TREAdmin` app role on the `kubetre-<env>-api` enterprise
  application. In the Entra admin centre, open Enterprise applications, select the app, then
  Users and groups.
- **Researchers and workspace owners** need no role. A TRE administrator adds their email
  addresses to a workspace.

## 3. Create the infrastructure

**State store.** Terraform keeps its state in Azure Storage with shared keys disabled.

```sh
az group create -n rg-kubetre-tfstate -l uksouth
az storage account create -n <stateaccount> -g rg-kubetre-tfstate --sku Standard_ZRS \
  --min-tls-version TLS1_2 --allow-shared-key-access false --allow-blob-public-access false
az storage container create -n tfstate --account-name <stateaccount> --auth-mode login
az role assignment create --role "Storage Blob Data Contributor" \
  --assignee "$(az ad signed-in-user show --query id -o tsv)" \
  --scope "$(az storage account show -n <stateaccount> --query id -o tsv)"
```

**Configuration.** In `infra/azure`, create `backend.hcl`:

```hcl
resource_group_name  = "rg-kubetre-tfstate"
storage_account_name = "<stateaccount>"
container_name       = "tfstate"
key                  = "kubetredev.tfstate"
use_azuread_auth     = true
```

Copy `terraform.tfvars.example` to `terraform.tfvars`. Fill in `name`, `location` and
`operator_ip_ranges`, paste the lines printed by the setup script, and leave
`gitops_repository_url` empty for now.

**Apply.** Add `-var-file=profiles/small.tfvars` for a short, low-cost test run.

```sh
cd infra/azure
terraform init -backend-config=backend.hcl
terraform apply
```

This takes 20 to 40 minutes. When it finishes, connect to the cluster:

```sh
$(terraform output -raw get_credentials)
kubelogin convert-kubeconfig -l azurecli
kubectl get nodes
```

## 4. Publish images and charts

KubeTRE's own images are built inside Azure by ACR Tasks. Guacamole and Envoy Gateway are
copied from Docker Hub once, so the cluster never pulls from the internet. From the repository
root:

```sh
ACR=$(terraform -chdir=infra/azure output -raw acr_name)
make acr-build acr-mirror publish-charts ACR_NAME=$ACR
```

If `az acr import` hits Docker Hub's rate limit, rerun `make acr-mirror` later, or pass Docker
Hub credentials to `az acr import`.

## 5. Prepare the gateway's DNS, TLS and sign-in secret

**DNS.** Create an A record for your gateway hostname pointing at the firewall:

```sh
terraform -chdir=infra/azure output -raw firewall_public_ip
```

No domain? Set `firewall_dns_label` (for example `kubetredev-gw`) and use
`<label>.<region>.cloudapp.azure.com` as `gateway_hostname`. Azure creates that name on the
firewall's public IP for you; `terraform output firewall_fqdn` shows it.

**Namespace and secrets.** Flux creates the `kubetre-gateway` namespace later, but you can
create it now so the secrets are ready when the gateway starts:

```sh
kubectl create namespace kubetre-gateway
kubectl -n kubetre-gateway create secret tls kubetre-gateway-tls \
  --cert=fullchain.pem --key=privkey.pem
kubectl -n kubetre-gateway create secret generic kubetre-gateway-oidc \
  --from-file=client-secret=kubetre-kubetredev-gateway-client-secret.txt
```

Use a certificate from your organisation's CA, or from Let's Encrypt with a DNS challenge.
Automatic renewal is not built yet, so note the expiry date. For a short test run you can use
a self-signed certificate; browsers warn once and sign-in still works:

```sh
H=<gateway_hostname>
openssl req -x509 -newkey rsa:3072 -sha256 -days 30 -nodes -keyout privkey.pem -out fullchain.pem \
  -subj "/CN=$H" -addext "subjectAltName=DNS:$H" -addext "extendedKeyUsage=serverAuth"
```

## 6. Turn on GitOps

Set `gitops_repository_url = "https://github.com/isBioku/kubetre"` in `terraform.tfvars`
(or your fork) and apply again:

```sh
terraform -chdir=infra/azure apply
```

Flux then installs, in order: Crossplane, its Azure providers and functions, Envoy Gateway,
the KubeTRE platform and templates, and the access gateway. Watch progress:

```sh
kubectl get kustomizations -n flux-system -w
```

All five should reach `Ready=True` within about 15 minutes. If one stays false, see
[Troubleshooting](#troubleshooting).

## 7. Check the platform

```sh
kubectl get providers,functions                     # all HEALTHY=True
kubectl get workspacetemplates,servicetemplates     # 2 workspace and 3 service templates
kubectl -n kubetre-system get pods                  # controller and API running
kubectl -n kubetre-gateway get pods                 # broker, guacamole, guacd running
kubectl -n envoy-gateway-system get svc             # EXTERNAL-IP equals the gateway ILB address
```

Then check the firewall for anything the platform needed but was denied. In the Azure portal,
open the Log Analytics workspace `log-<env>` and run:

```kusto
AZFWApplicationRule
| where TimeGenerated > ago(1h) and Action == "Deny"
| summarize Count = count() by Fqdn, SourceIp
| order by Count desc
```

Add any platform hostnames you trust to `platform_egress_fqdns` and apply again. Denials from
workspace subnets are expected: that is the isolation working.

## 8. Create the first workspace

Open `https://<gateway_hostname>` and sign in. This is AzureTRE's UI. Your account needs the
`TREAdmin` app role on the API app, which the setup script assigned to you. Researchers need
the `TREUser` role:

```sh
az ad app show --id <oidc_audience> --query "appRoles[].{role:value,id:id}" -o table
```

Select **Create new**, choose **Workspace with virtual machines**, and fill in the form:

- **Name** and **Description**.
- **Workspace owners** and **Workspace researchers**, by email address. Owners default to you.
- **Allowed internet domains**, for example `pypi.org`.
- **Cost centre**, which the template requires.

The workspace card shows *deploying* and then *deployed* after a few minutes, once Crossplane
has created its subnet, network security group and firewall rules. The notification panel
follows the operation, as in AzureTRE.

## 9. Give a researcher a Windows VM

1. **Add Virtual Desktops.** A workspace owner opens the workspace, selects **Create new**
   under Workspace Services and chooses **Virtual Desktops**. It is ready in seconds, because
   it installs nothing itself.
2. **Create the VM.** The researcher opens Virtual Desktops, selects **Create new** and
   chooses **Windows VM**, **Linux VM** or **Linux desktop**. A VM is ready in five to ten
   minutes.
3. **Connect.** The researcher selects **Connect** on the VM's card. A new tab opens through
   the gateway at `/gateway/connect/...`, signs in if needed, and hands the session to
   Guacamole.

Researchers see only their own VMs. Workspace owners see every VM in the UI, but only a VM's
owner can open a session to it.

To delete a resource, disable it first and then delete it, as in AzureTRE. KubeTRE's own API at
`/api/v1` still works for scripts:

```sh
TOKEN=$(az account get-access-token --scope api://<oidc_audience>/user_impersonation --query accessToken -o tsv)
curl -s https://<gateway_hostname>/api/v1/me -H "Authorization: Bearer $TOKEN"
```

## 10. Verify the security controls

Do these checks before anyone uses real data:

1. **Clipboard and file transfer.** In the session, copying text out, pasting text in and
   dragging a file in must all fail.
2. **Privacy between researchers.** A second researcher in the same workspace must not see
   Rita's VM in the UI. The workspace owner sees it, but **Connect** must fail for them.
3. **Workspace isolation.** From inside the VM, connecting to another workspace's subnet must
   fail, and browsing to a site not in `allowedFQDNs` must fail.
4. **No public addresses.** `az vm list-ip-addresses -g rg-<env>-workspaces -o table` must show
   no public IPs.
5. **Audit.** The broker logs each session it opens:
   `kubectl -n kubetre-gateway logs deploy/broker | grep "session opened"`.

## Day-two operations

**Upgrade KubeTRE.** Build the new version with `make acr-build ACR_NAME=$ACR IMG_TAG=<version>`,
publish charts if they changed, set `kubetre_version` in `terraform.tfvars` and apply. Flux
rolls the controller, API and broker.

**Add a template.** Add a `ServiceTemplate` to `config/templates`, push its chart with
`make publish-charts`, commit and push. Flux registers it.

**Rotate the gateway's sign-in secret** before it expires after one year: delete the secret
file, rerun `hack/entra-setup.sh`, and recreate the `kubetre-gateway-oidc` secret.

**Renew the TLS certificate** by recreating `kubetre-gateway-tls` with the new files.

## Teardown

Delete in this order, or Azure will refuse to delete the network while VMs still use it:

1. Delete every workspace, from the UI (disable, then delete) or through the API
   (`DELETE /api/v1/workspaces/<name>`). The controller
   removes its namespace, and Crossplane removes its VMs, subnet, NSG and firewall rules.
   Check that `rg-<env>-workspaces` is empty.
2. Set `gitops_repository_url = ""` and `terraform apply`, so Flux stops reconciling.
3. `terraform destroy`.
4. Optionally delete the Entra ID apps and group, and the state storage account.

## Troubleshooting

| Symptom | Likely cause and fix |
|---|---|
| A Flux kustomization stays `Ready=False` | `kubectl describe kustomization <name> -n flux-system` shows the failing object. The platform stage may fail once while Crossplane's CRDs install; it retries every minute. |
| Apply fails with `NotAvailableForSubscription` or a quota error | The size or zone is restricted for your subscription, or the family's vCPU quota is too low. Run the checks in step 1, adjust `zones` or the sizes, or request quota. |
| Pods show `ImagePullBackOff` | `make acr-build` or `make acr-mirror` did not run, or `kubetre_version` does not match the tag you built. |
| A Crossplane provider is not healthy | The firewall denied its package download: run the query in step 7. If the Upbound package needs a subscription, build it from `crossplane-contrib/provider-upjet-azure`. |
| A workspace stays `Pending` with `VMNetworkProvisioning` | `kubectl -n ws-<name> describe workspacenetwork vm-network`, then the composed resources, show the Azure error. Commonly the Crossplane identity lacks a permission or the address pool overlaps something. |
| The gateway page does not load | Check the DNS record, that `kubetre-gateway-tls` exists, `kubectl -n kubetre-gateway get gateway kubetre -o yaml` for listener status, and that the Envoy service's IP equals the gateway ILB address. |
| Sign-in to the UI fails with a redirect URI error | The UI app's single-page redirect URIs must include `https://<gateway_hostname>`. Rerun the setup script with the right hostname. |
| **Connect** fails with a redirect URI error | The gateway app's redirect URI must be exactly `https://<gateway_hostname>/gateway/callback`. Rerun the setup script. |
| The UI says you have no access | Assign the user the `TREUser` or `TREAdmin` role on the API app. |
| The API returns 401 | The token's audience must be the API app ID and its issuer must end in `/v2.0`. Use the `az account get-access-token --scope` command above. |
| `isAdmin` is false | Assign the `TREAdmin` role, then get a new token; roles are read from the token. |
| The VM's Connect button is disabled | It shows the reason: usually "waiting for the virtual machine's address" while Azure is still creating it. |
| Researchers cannot see a workspace | Their email in the workspace must match the email claim in their token. Check `/api/v1/me`. |
| A deleted resource cannot be removed | Disable it first. The UI's delete is only enabled once a resource is disabled. |

## Security gaps

These are known and recorded in the architecture decisions. Close them, or accept them in
writing with your information security officer, before using sensitive data:

- TLS certificates are installed and renewed by hand.
- There is no session recording, and audit is limited to logs.
- There are no package mirrors, so VMs reach the internet allowlist directly.
- There is no airlock yet for moving data in and out.
- Container desktops share a node kernel, unlike VMs.
- The broker can read Secrets across the cluster.
- DaemonSets running on gateway nodes get addresses the VMs trust.

See [ADR 8](adr/0008-vm-security-model.md) and [ADR 9](adr/0009-access-gateway.md).
