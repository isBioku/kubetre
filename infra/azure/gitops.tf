# Flux's source controller pulls charts from ACR with its own workload identity. Without it,
# Flux falls back to the node's managed identity and fails, because nodes carry several
# identities and Azure will not pick one ("Multiple user assigned identities exist").
resource "azurerm_user_assigned_identity" "flux" {
  name                = "id-${var.name}-flux"
  location            = var.location
  resource_group_name = azurerm_resource_group.core.name
  tags                = local.tags
}

resource "azurerm_federated_identity_credential" "flux_source_controller" {
  name                      = "flux-source-controller"
  user_assigned_identity_id = azurerm_user_assigned_identity.flux.id
  issuer                    = azurerm_kubernetes_cluster.this.oidc_issuer_url
  subject                   = "system:serviceaccount:flux-system:source-controller"
  audience                  = ["api://AzureADTokenExchange"]
}

resource "azurerm_role_assignment" "flux_acr_pull" {
  scope                = azurerm_container_registry.this.id
  role_definition_name = "AcrPull"
  principal_id         = azurerm_user_assigned_identity.flux.principal_id
}

resource "azurerm_kubernetes_cluster_extension" "flux" {
  name           = "flux"
  cluster_id     = azurerm_kubernetes_cluster.this.id
  extension_type = "microsoft.flux"
  configuration_settings = {
    "workloadIdentity.enable"        = "true"
    "workloadIdentity.azureClientId" = azurerm_user_assigned_identity.flux.client_id
  }
  # Flux's controllers need the API server rule, or they crash-loop and the extension times out.
  depends_on = [azurerm_kubernetes_cluster_node_pool.work, azurerm_firewall_policy_rule_collection_group.cluster_api]
}

locals {
  # Values Flux substitutes into deploy/azure, so no environment value is copied by hand.
  gitops_substitutions = {
    AZURE_TENANT_ID          = data.azurerm_client_config.current.tenant_id
    AZURE_SUBSCRIPTION_ID    = data.azurerm_client_config.current.subscription_id
    LOCATION                 = var.location
    CROSSPLANE_CLIENT_ID     = azurerm_user_assigned_identity.crossplane.client_id
    WORKSPACE_RESOURCE_GROUP = azurerm_resource_group.workspaces.name
    VNET_NAME                = azurerm_virtual_network.this.name
    VNET_RESOURCE_GROUP      = azurerm_virtual_network.this.resource_group_name
    ROUTE_TABLE_ID           = azurerm_route_table.egress.id
    FIREWALL_POLICY_ID       = azurerm_firewall_policy.this.id
    ACCESS_GATEWAY_PREFIX    = local.subnets.gateway_pods
    SHARED_SERVICES_PREFIX   = local.subnets.shared_ilb
    VM_ADDRESS_POOL          = var.vm_address_pool
    ENCRYPTION_AT_HOST       = tostring(var.host_encryption_enabled)
    ACR_LOGIN_SERVER         = azurerm_container_registry.this.login_server
    KUBETRE_VERSION          = var.kubetre_version
    OIDC_ISSUER              = var.oidc_issuer
    OIDC_AUDIENCE            = var.oidc_audience
    ROLES_CLAIM              = var.roles_claim
    IDP_HOST                 = local.idp_host
    GATEWAY_HOSTNAME         = var.gateway_hostname
    GATEWAY_OIDC_CLIENT_ID   = var.gateway_oidc_client_id
    UI_CLIENT_ID             = var.ui_client_id
    NEXUS_ILB_IP             = local.nexus_ilb_ip
    AIRLOCK_CLIENT_ID        = azurerm_user_assigned_identity.airlock.client_id
    AIRLOCK_EXTERNAL_ACCOUNT = local.airlock_external_account
    AIRLOCK_INTERNAL_ACCOUNT = local.airlock_internal_account
    AIRLOCK_INTERNAL_IP      = local.airlock_internal_pe_ip
    # Quoted, so it stays a string in ConfigMaps after Kustomize drops the quotes.
    AIRLOCK_MALWARE_SCANNING = jsonencode(tostring(var.airlock_malware_scanning_enabled))
    GITEA_ILB_IP             = local.gitea_ilb_ip
    MIRROR_UPSTREAM_FQDNS    = jsonencode([for f in var.mirror_upstream_fqdns : { matchName = f }])
    TRE_ID                   = var.name
    COSTS_CLIENT_ID          = azurerm_user_assigned_identity.costs.client_id
    CORE_RESOURCE_GROUP      = azurerm_resource_group.core.name
    NODE_RESOURCE_GROUP      = azurerm_kubernetes_cluster.this.node_resource_group
    VNET_ID                  = azurerm_virtual_network.this.id
    LOG_ANALYTICS_ID         = azurerm_log_analytics_workspace.this.id
    PRIVATE_DNS_ZONE_IDS     = jsonencode(local.paas_private_dns_zone_ids)
    GATEWAY_ILB_IP           = local.gateway_ilb_ip
    ILB_SUBNET_NAME          = azurerm_subnet.shared_ilb.name
    VM_SIZE_SMALL            = var.research_vm_sizes.small
    VM_SIZE_MEDIUM           = var.research_vm_sizes.medium
    VM_SIZE_LARGE            = var.research_vm_sizes.large
  }
}

resource "azurerm_kubernetes_flux_configuration" "kubetre" {
  count      = var.gitops_repository_url == "" ? 0 : 1
  name       = "kubetre"
  cluster_id = azurerm_kubernetes_cluster.this.id
  namespace  = "flux-system"
  scope      = "cluster"

  git_repository {
    url                      = var.gitops_repository_url
    reference_type           = "branch"
    reference_value          = var.gitops_branch
    sync_interval_in_seconds = 300
  }

  kustomizations {
    name                      = "crossplane"
    path                      = "${var.gitops_path}/crossplane"
    wait                      = true
    retry_interval_in_seconds = 60
  }

  kustomizations {
    name                      = "crossplane-packages"
    path                      = "${var.gitops_path}/packages"
    depends_on                = ["crossplane"]
    wait                      = true
    retry_interval_in_seconds = 60
    post_build {
      substitute = local.gitops_substitutions
    }
  }

  kustomizations {
    name                      = "edge"
    path                      = "${var.gitops_path}/edge"
    wait                      = true
    retry_interval_in_seconds = 60
    post_build {
      substitute = local.gitops_substitutions
    }
  }

  kustomizations {
    name                      = "platform"
    path                      = "${var.gitops_path}/platform"
    depends_on                = ["crossplane-packages"]
    retry_interval_in_seconds = 60
    post_build {
      substitute = local.gitops_substitutions
    }
  }

  kustomizations {
    name                      = "gateway"
    path                      = "${var.gitops_path}/gateway"
    depends_on                = ["platform", "edge"]
    retry_interval_in_seconds = 60
    post_build {
      substitute = local.gitops_substitutions
    }
  }

  # KubeVirt and CDI, plus the KubeVirt VM template, only when enabled.
  dynamic "kustomizations" {
    for_each = var.kubevirt_enabled ? ["kubevirt"] : []
    content {
      name                      = "kubevirt"
      path                      = "${var.gitops_path}/kubevirt"
      depends_on                = ["platform"]
      retry_interval_in_seconds = 60
      post_build {
        substitute = local.gitops_substitutions
      }
    }
  }

  # Azure PaaS workspace services, only when enabled.
  dynamic "kustomizations" {
    for_each = var.paas_services_enabled ? ["paas"] : []
    content {
      name                      = "paas"
      path                      = "${var.gitops_path}/paas"
      depends_on                = ["crossplane-packages", "platform"]
      retry_interval_in_seconds = 60
      post_build {
        substitute = local.gitops_substitutions
      }
    }
  }

  depends_on = [azurerm_kubernetes_cluster_extension.flux]
}
