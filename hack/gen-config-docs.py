#!/usr/bin/env python3
"""Regenerates docs/admin/configuration.md from infra/azure/variables.tf."""
import os
import re

root = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..")
src = open(os.path.join(root, "infra/azure/variables.tf")).read()

groups = [
    ("Environment", ["name", "location", "zones", "tags"]),
    ("Access", ["operator_ip_ranges", "admin_group_object_ids", "gateway_source_ranges"]),
    ("Network", ["vnet_address_space", "vm_address_pool", "service_cidr", "firewall_sku_tier",
                 "platform_egress_fqdns", "mirror_upstream_fqdns", "firewall_dns_label"]),
    ("Cluster", ["kubernetes_version", "aks_sku_tier", "system_vm_size", "system_node_count",
                 "work_vm_size", "work_node_count", "gateway_vm_size", "gateway_node_count",
                 "fqdn_filtering_enabled", "defender_enabled", "host_encryption_enabled",
                 "log_retention_days"]),
    ("Research VMs", ["research_vm_sizes", "kubevirt_enabled", "kubevirt_vm_size", "kubevirt_node_count"]),
    ("Airlock", ["airlock_malware_scanning_enabled", "airlock_malware_scanning_cap_gb"]),
    ("Identity", ["oidc_issuer", "oidc_audience", "roles_claim", "gateway_hostname",
                  "gateway_oidc_client_id", "ui_client_id"]),
    ("GitOps and version", ["gitops_repository_url", "gitops_branch", "gitops_path", "kubetre_version"]),
]
extra = {
    "system_vm_size": "Node size of the system pool.",
    "work_vm_size": "Node size of the pool for workspace containers.",
    "gateway_vm_size": "Node size of the access-gateway pool.",
    "firewall_sku_tier": "Standard or Premium. Basic cannot apply FQDN rules at the scale needed.",
    "log_retention_days": "Log Analytics retention.",
    "gitops_branch": "Branch Flux follows.",
    "roles_claim": "Dot-separated claim path holding TRE roles in the token.",
    "tags": "Tags added to every Azure resource.",
    "platform_egress_fqdns": "Extra HTTPS hosts the cluster itself may reach, beyond the AKS FQDN tag.",
}

variables = {}
for m in re.finditer(r'variable "([^"]+)" \{(.*?)\n\}', src, re.S):
    body = m.group(2)
    desc = re.search(r'description\s*=\s*"((?:[^"\\]|\\.)*)"', body)
    default = re.search(r'\n\s*default\s*=\s*(\{[^}]*\}|\[[^\]]*\]|[^\n]+)', body, re.S)
    if default:
        value = re.sub(r"\s+", " ", re.sub(r"#[^\n]*", "", default.group(1))).strip()
        value = "see `variables.tf`" if len(value) > 45 else "`" + value + "`"
    else:
        value = "required"
    text = desc.group(1) if desc else extra.get(m.group(1), "")
    variables[m.group(1)] = (value, text.replace("|", "\\|"))

listed = {n for _, names in groups for n in names}
missing = [n for n in variables if n not in listed]
if missing:
    raise SystemExit(f"add these variables to a group in {__file__}: {missing}")

out = [
    "# Configuration", "",
    "KubeTRE is configured with Terraform variables in `infra/azure/terraform.tfvars`, plus",
    "`backend.hcl` for state. There is no `config.yaml` and no environment variables file.",
    "Terraform passes environment values to Flux as substitutions (`gitops_substitutions` in",
    "`gitops.tf`), so nothing environment-specific is committed to Git.", "",
    "`profiles/small.tfvars` overrides node counts, sizes and VM sizes for short, low-cost tests.",
    "Pass it with `-var-file=profiles/small.tfvars`.", "",
    "This reference is generated from `infra/azure/variables.tf` by `hack/gen-config-docs.py`.", "",
]
for group, names in groups:
    out += ["## " + group, "", "| Variable | Default | Description |", "|---|---|---|"]
    out += [f"| `{n}` | {variables[n][0]} | {variables[n][1]} |" for n in names]
    out.append("")
out += [
    "## Coming from AzureTRE", "",
    "| AzureTRE (`config.yaml`) | KubeTRE |", "|---|---|",
    "| `TRE_ID` | `name` |", "| `LOCATION` | `location` |",
    "| `MGMT_RESOURCE_GROUP_NAME`, `MGMT_STORAGE_ACCOUNT_NAME`, `TERRAFORM_STATE_CONTAINER_NAME` | `backend.hcl` |",
    "| `ACR_NAME` | derived: `acr<name>` |",
    "| `CORE_ADDRESS_SPACE`, `TRE_ADDRESS_SPACE` | `vnet_address_space`, `vm_address_pool`, `service_cidr` |",
    "| `PUBLIC_DEPLOYMENT_IP_ADDRESS`, `PRIVATE_AGENT_SUBNET_ID` | `operator_ip_ranges` |",
    "| `AAD_TENANT_ID`, `API_CLIENT_ID` | `oidc_issuer`, `oidc_audience` |",
    "| `SWAGGER_UI_CLIENT_ID` | `ui_client_id` (no Swagger UI) |",
    "| `CUSTOM_DOMAIN` | `gateway_hostname`, optionally `firewall_dns_label` |",
    "| `FIREWALL_SKU` | `firewall_sku_tier` (no Basic) |",
    "| `RESOURCE_PROCESSOR_*`, `*_APP_SERVICE_PLAN_SKU` | none; node pools: `system_*`, `work_*`, `gateway_*`, `kubevirt_*` |",
    "| `APP_GATEWAY_SKU`, `DEPLOY_BASTION`, `BASTION_SKU` | none: Envoy Gateway behind the firewall, no Bastion |",
    "| `UI_SITE_NAME`, `UI_FOOTER_TEXT` | none: the UI keeps Microsoft's defaults (\"Azure TRE\") |",
    "| `API_CLIENT_SECRET`, `APPLICATION_ADMIN_*`, `TEST_ACCOUNT_*`, `WORKSPACE_API_*`, `AUTO_*` | none; see [Identities](identities.md) |",
    "| `ENABLE_CMK_ENCRYPTION`, `enable_dns_policy`, `firewall_force_tunnel_ip`, `USER_MANAGEMENT_ENABLED` | not supported yet |",
    "",
]
open(os.path.join(root, "docs/admin/configuration.md"), "w").write("\n".join(out))
print(f"documented {len(variables)} variables")
