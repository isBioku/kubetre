# KubeTRE on Azure: infrastructure

Terraform for one KubeTRE environment: AKS, its network, Azure Firewall, ACR, Log Analytics,
the Crossplane identity, and Flux. Flux then installs Crossplane and KubeTRE from Git.

## What it creates

| Area | Resources | Security properties |
|---|---|---|
| Network | VNet with node, pod, gateway-pod, shared and private-endpoint subnets, plus a reserved range for workspace VM subnets | No default outbound access; every subnet routes 0.0.0.0/0 to the firewall |
| Egress | Azure Firewall (Standard) with a policy | Threat intelligence in deny mode; platform rules only; workspaces add their own allowlists through Crossplane |
| Cluster | AKS with system, work and gateway node pools | Entra ID only (local accounts off), Azure RBAC, API server limited to operator IPs, Cilium with FQDN filtering, workload identity, no node public IPs, run-command off, Defender, Azure Policy, audit logs |
| Registry | ACR Premium with a private endpoint | Deny by default, operator IPs allowed, no admin user or anonymous pull, ACR Tasks may build |
| Identity | Control-plane, kubelet and Crossplane identities | Crossplane holds only custom roles scoped to the workspace resource group, VNet subnets, route table and firewall policy |
| Ingress | One firewall DNAT rule: TCP 443 to the access gateway's internal load balancer | No other inbound path; source ranges configurable |
| GitOps | AKS Flux extension and configuration | Environment values are passed from Terraform outputs, never copied by hand |

`tests/security.tftest.hcl` asserts these properties against a mocked provider.

## Prerequisites

- Azure CLI signed in with rights to create resources, custom roles and role assignments
  (Owner, or Contributor plus User Access Administrator).
- `az feature register --namespace Microsoft.Compute --name EncryptionAtHost`, or set
  `host_encryption_enabled = false` and `encryptionAtHost: false`.
- An Entra ID group for cluster administrators.

## Deploy

1. **Create the state store** once per subscription. Shared keys are disabled; Terraform uses Entra ID.

   ```sh
   az group create -n rg-kubetre-tfstate -l uksouth
   az storage account create -n <stateaccount> -g rg-kubetre-tfstate --sku Standard_ZRS \
     --min-tls-version TLS1_2 --allow-shared-key-access false --allow-blob-public-access false
   az storage container create -n tfstate --account-name <stateaccount> --auth-mode login
   az role assignment create --role "Storage Blob Data Contributor" --assignee <your-object-id> \
     --scope $(az storage account show -n <stateaccount> --query id -o tsv)
   ```

2. **Configure.** Copy `terraform.tfvars.example` to `terraform.tfvars`, and create `backend.hcl`:

   ```hcl
   resource_group_name  = "rg-kubetre-tfstate"
   storage_account_name = "<stateaccount>"
   container_name       = "tfstate"
   key                  = "kubetredev.tfstate"
   use_azuread_auth     = true
   ```

3. **Apply.**

   ```sh
   terraform init -backend-config=backend.hcl
   terraform apply
   ```

4. **Publish images and charts** into the new registry. KubeTRE's images are built inside
   Azure by ACR Tasks; Guacamole and Envoy Gateway are copied from Docker Hub once.

   ```sh
   make -C ../.. acr-build acr-mirror publish-charts ACR_NAME=$(terraform output -raw acr_name)
   ```

5. **Prepare the access gateway.**
   - Register an app for the gateway with redirect URI `https://<gateway_hostname>/callback`
     and set `gateway_oidc_client_id`. If it is a confidential client, create the Secret
     `kubetre-gateway-oidc` (key `client-secret`) in namespace `kubetre-gateway`.
   - Create a DNS record for `gateway_hostname` pointing at the `firewall_public_ip` output.
   - Create the TLS Secret `kubetre-gateway-tls` in namespace `kubetre-gateway`.
6. **Turn on GitOps.** Push this repository, set `gitops_repository_url`, and apply again.
   Flux installs Crossplane and its packages, Envoy Gateway, KubeTRE, then the access gateway.

7. **Verify.**

   ```sh
   $(terraform output -raw get_credentials)
   kubectl get kustomizations -n flux-system
   kubectl get providers,functions
   ```

   Check the firewall logs in Log Analytics for denied FQDNs and add any the platform needs to
   `platform_egress_fqdns`.

## Known limits

- The Flux configuration expects a public repository. A private one needs credentials wired
  into `azurerm_kubernetes_flux_configuration`.
- Crossplane may write any subnet in the VNet, including the cluster's: Azure RBAC cannot
  scope subnet writes to subnets that do not exist yet. The custom role grants nothing else.
- DaemonSets that tolerate all taints also run on gateway nodes and get gateway-pod addresses,
  which workspace VM NSGs trust. Restrict their egress with network policy (ADR 8).
