# Azure resources

What a KubeTRE environment creates in Azure, named for an environment called `<env>`.
Compare AzureTRE's resource breakdown, which also has App Service, Cosmos DB, Service Bus,
Key Vault, Bastion and per-workspace VNets, storage accounts and Key Vaults.

## State resource group (created by hand)

| Resource | Purpose |
|---|---|
| Storage account and container `tfstate` | Terraform state, with shared-key access disabled |

## Core resource group `rg-<env>`

| Resource | Name | Purpose |
|---|---|---|
| AKS cluster | `aks-<env>` | runs everything; Entra ID authentication, Azure RBAC, workload identity, local accounts disabled |
| Container registry | `acr<env>` | KubeTRE's images, Helm charts and mirrored third-party images; private endpoint, public access limited to `operator_ip_ranges` |
| Azure Firewall | `fw-<env>` | the only way in (DNAT 443) and out |
| Firewall policy | `fwp-<env>` | platform rules, plus one rule collection group per VM workspace added by Crossplane |
| Public IP | `pip-<env>-fw` | the TRE's public address; optional DNS label |
| Virtual network | `vnet-<env>` | all subnets; workspace VM subnets are added inside it |
| Route table | `rt-<env>-egress` | sends all traffic to the firewall |
| Log Analytics workspace | `log-<env>` | AKS and firewall logs |
| Private endpoint and DNS zone | `pe-<env>-acr`, `privatelink.azurecr.io` | private access to the registry |
| Managed identities | `id-<env>-aks`, `-kubelet`, `-crossplane`, `-flux` | the cluster, image pulls, Crossplane's Azure access, Flux's registry access |

AKS also creates its node resource group `MC_rg-<env>_aks-<env>_<region>` with the node scale
sets, disks and the internal load balancer.

## Workspace resource group `rg-<env>-workspaces`

Crossplane creates these when researchers use VM workspaces:

| Resource | Name | Per |
|---|---|---|
| Network security group | `nsg-ws-<id>` | VM workspace |
| Subnet | `snet-ws-<id>`, in `vnet-<env>` | VM workspace |
| Firewall rule collection group | `rcg-ws-<id>`, in `fwp-<env>` | VM workspace |
| Virtual machine, OS disk, network card | generated names | Azure VM |

KubeVirt VM disks are Azure managed disks in the node resource group, created through
persistent volume claims.

## Permissions

Crossplane's identity holds only what it needs: rights to create resources in the workspace
resource group, custom roles to add subnets to the VNet, join the route table and manage rule
collection groups on the firewall policy. It cannot change the core network or the firewall
itself. Flux's identity can only pull from the registry.

## What KubeTRE does not create

No App Service, Cosmos DB, Service Bus, Key Vault, Application Gateway, Bastion, jump box,
MySQL or storage accounts, and no per-workspace VNet, storage account or Key Vault.
