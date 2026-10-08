# Configuration

KubeTRE is configured with Terraform variables in `infra/azure/terraform.tfvars`, plus
`backend.hcl` for state. There is no `config.yaml` and no environment variables file.
Terraform passes environment values to Flux as substitutions (`gitops_substitutions` in
`gitops.tf`), so nothing environment-specific is committed to Git.

`profiles/small.tfvars` overrides node counts, sizes and VM sizes for short, low-cost tests.
Pass it with `-var-file=profiles/small.tfvars`.

This reference is generated from `infra/azure/variables.tf`.

## Environment

| Variable | Default | Description |
|---|---|---|
| `name` | required | Short environment name used in every resource name, for example kubetredev. |
| `location` | `"uksouth"` | Azure region. |
| `zones` | `["1", "2", "3"]` | Availability zones for AKS nodes and the firewall. Use [] in regions without zones. |
| `tags` | `{}` | Tags added to every Azure resource. |

## Access

| Variable | Default | Description |
|---|---|---|
| `operator_ip_ranges` | required | Public CIDRs of operators and CI. Only these reach the AKS API server and the container registry. |
| `admin_group_object_ids` | required | Entra ID groups whose members are cluster administrators. Local Kubernetes accounts are disabled. |
| `gateway_source_ranges` | `["*"]` | Public CIDRs allowed to reach the access gateway on HTTPS. Narrow this to your institution's ranges where possible. |

## Network

| Variable | Default | Description |
|---|---|---|
| `vnet_address_space` | `"10.224.0.0/16"` | Address space for AKS, the firewall and shared subnets. Must be a /16. |
| `vm_address_pool` | `"10.240.0.0/16"` | Address range for workspace VM subnets, allocated by the KubeTRE controller. Must not overlap vnet_address_space. |
| `service_cidr` | `"172.16.0.0/16"` | Kubernetes service CIDR. Must not overlap the VNet. |
| `firewall_sku_tier` | `"Standard"` | Standard or Premium. Basic cannot apply FQDN rules at the scale needed. |
| `platform_egress_fqdns` | see `variables.tf` | Extra HTTPS destinations the cluster itself needs, beyond the AzureKubernetesService FQDN tag. Check firewall logs for denials on first deployment. |
| `firewall_dns_label` | `""` | Optional DNS label for the firewall's public IP. It yields <label>.<location>.cloudapp.azure.com, a free hostname for the gateway when you have no domain. |

## Cluster

| Variable | Default | Description |
|---|---|---|
| `kubernetes_version` | `null` | AKS version; null uses the region default. |
| `aks_sku_tier` | `"Standard"` | Free or Standard (financially backed API server SLA). |
| `system_vm_size` | `"Standard_D4s_v5"` | Node size of the system pool. |
| `system_node_count` | `{ min = 2, max = 4 }` | Autoscaling bounds for the system node pool. |
| `work_vm_size` | `"Standard_D8s_v5"` | Node size of the pool for workspace containers. |
| `work_node_count` | `{ min = 1, max = 10 }` | Autoscaling bounds for the pool that runs workspace containers. |
| `gateway_vm_size` | `"Standard_D4s_v5"` | Node size of the access-gateway pool. |
| `gateway_node_count` | `{ min = 2, max = 3 }` | Autoscaling bounds for the access-gateway node pool. |
| `fqdn_filtering_enabled` | `true` | Advanced Container Networking Services security, which enforces FQDN egress policies for pods. Paid add-on. |
| `defender_enabled` | `true` | Microsoft Defender for Containers. |
| `host_encryption_enabled` | `true` | Encrypt node and VM temp disks and caches at host. Requires the EncryptionAtHost feature on the subscription. |
| `log_retention_days` | `90` | Log Analytics retention. |

## Research VMs

| Variable | Default | Description |
|---|---|---|
| `research_vm_sizes` | see `variables.tf` | Azure VM size behind each research VM size a researcher can pick. |
| `kubevirt_enabled` | `false` | Offer Linux VMs that run inside the cluster on KubeVirt, as well as Azure VMs. Adds a node pool for them and installs KubeVirt and CDI. |
| `kubevirt_vm_size` | `"Standard_D4s_v5"` | Node size for KubeVirt VMs. It must support nested virtualization (Dv5 and Dsv5 do). |
| `kubevirt_node_count` | `{ min = 1, max = 3 }` | Autoscaling bounds for the KubeVirt node pool. |

## Identity

| Variable | Default | Description |
|---|---|---|
| `oidc_issuer` | `""` | OIDC issuer the KubeTRE API trusts, for example https://login.microsoftonline.com/<tenant>/v2.0. |
| `oidc_audience` | `""` | Audience (client ID) of the KubeTRE API's app registration. |
| `roles_claim` | `"roles"` | Dot-separated claim path holding TRE roles in the token. |
| `gateway_hostname` | `""` | Public DNS name of the access gateway, for example gateway.tre.example.org. Point it at the firewall's public IP. |
| `gateway_oidc_client_id` | `""` | Client ID of the access gateway's app registration (redirect URI https://<gateway_hostname>/gateway/callback). |
| `ui_client_id` | `""` | Client ID of the UI's single-page app registration (redirect URI https://<gateway_hostname>), pre-authorised for the API's user_impersonation scope. |

## GitOps and version

| Variable | Default | Description |
|---|---|---|
| `gitops_repository_url` | `""` | HTTPS URL of the Git repository Flux syncs. Empty installs Flux without a configuration. |
| `gitops_branch` | `"main"` | Branch Flux follows. |
| `gitops_path` | `"deploy/azure"` | Path inside the repository to the deploy/azure directory. |
| `kubetre_version` | `"0.2.5"` | Image tag of KubeTRE's images in ACR. Keep it equal to the VERSION file, which `make acr-build` uses. |

## Coming from AzureTRE

| AzureTRE (`config.yaml`) | KubeTRE |
|---|---|
| `TRE_ID` | `name` |
| `LOCATION` | `location` |
| `MGMT_RESOURCE_GROUP_NAME`, `MGMT_STORAGE_ACCOUNT_NAME`, `TERRAFORM_STATE_CONTAINER_NAME` | `backend.hcl` |
| `ACR_NAME` | derived: `acr<name>` |
| `CORE_ADDRESS_SPACE`, `TRE_ADDRESS_SPACE` | `vnet_address_space`, `vm_address_pool`, `service_cidr` |
| `PUBLIC_DEPLOYMENT_IP_ADDRESS`, `PRIVATE_AGENT_SUBNET_ID` | `operator_ip_ranges` |
| `AAD_TENANT_ID`, `API_CLIENT_ID` | `oidc_issuer`, `oidc_audience` |
| `SWAGGER_UI_CLIENT_ID` | `ui_client_id` (no Swagger UI) |
| `CUSTOM_DOMAIN` | `gateway_hostname`, optionally `firewall_dns_label` |
| `FIREWALL_SKU` | `firewall_sku_tier` (no Basic) |
| `RESOURCE_PROCESSOR_*`, `*_APP_SERVICE_PLAN_SKU` | none; node pools: `system_*`, `work_*`, `gateway_*`, `kubevirt_*` |
| `APP_GATEWAY_SKU`, `DEPLOY_BASTION`, `BASTION_SKU` | none: Envoy Gateway behind the firewall, no Bastion |
| `UI_SITE_NAME`, `UI_FOOTER_TEXT` | none: the UI keeps Microsoft's defaults ("Azure TRE") |
| `API_CLIENT_SECRET`, `APPLICATION_ADMIN_*`, `TEST_ACCOUNT_*`, `WORKSPACE_API_*`, `AUTO_*` | none; see [Identities](identities.md) |
| `ENABLE_CMK_ENCRYPTION`, airlock settings, `enable_dns_policy`, `firewall_force_tunnel_ip`, `USER_MANAGEMENT_ENABLED` | not supported yet |
