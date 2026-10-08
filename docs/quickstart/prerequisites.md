# 1. Prerequisites

## Tools

| Tool | Version |
|---|---|
| Azure CLI (`az`) | 2.60 or later |
| Terraform | 1.9 or later |
| `kubectl` and `kubelogin` | `az aks install-cli` installs both |
| GNU Make, Python 3, Git | any recent |
| GitHub CLI (`gh`) | optional, to fork and push |
| Go | 1.27, only to run the tests |

Unlike AzureTRE, there is no dev container and no separate deployment repository: clone (or
fork) the KubeTRE repository and work in it.

```sh
git clone https://github.com/isBioku/kubetre.git
cd kubetre
```

Flux deploys from a Git repository you choose. Use your fork if you will change templates or
manifests.

## Azure permissions

- **Subscription:** Owner, or Contributor plus User Access Administrator. Terraform creates
  role assignments and custom roles.
- **Entra ID:** permission to create app registrations and groups. If you lack it, ask your
  Entra ID administrator to run [step 2](identity.md) for you.

## Register providers and features

Once per subscription:

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

If you cannot register `EncryptionAtHost`, set `host_encryption_enabled = false` in
[step 3](infrastructure.md).

## Quota and sizes

New pay-as-you-go subscriptions usually allow only 10 vCPUs per region, and Azure may withhold
some sizes or zones from them. The default cluster needs about 24 vCPUs of the Dsv5 family
before any research VM; the small profile needs 8.

```sh
az vm list-usage --location uksouth -o table | grep -E "Total Regional|DSv5"
az vm list-skus --location uksouth --size Standard_D4s_v5 --all \
  --query "[].restrictions[].{type:type, reason:reasonCode, zones:restrictionInfo.zones}" -o table
```

Request more with `az quota update` (extension `quota`) or in the portal under Quotas; small
Dsv5 increases are usually approved within minutes. Note any restricted zones for `zones` in
step 3.

## Decide

- **Environment name:** 3 to 12 lowercase letters or digits, for example `kubetredev`. It is
  part of every resource name, including the registry, so it must be globally unique.
- **Region:** for example `uksouth`.
- **Operator addresses:** the public IP ranges you and any automation work from. Only these
  reach the cluster's API server and the registry.
- **Hostname:** where users will reach the TRE, for example `tre.example.org`, in a DNS zone
  you control, and how you will get a TLS certificate for it. Without a domain, Azure can give
  you `<label>.<region>.cloudapp.azure.com`; see [step 5](gateway.md).
- **Profile:** the full defaults, or the small profile for a short, low-cost test.
