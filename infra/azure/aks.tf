# User-assigned control-plane identity, so its network permissions exist before the cluster.
resource "azurerm_user_assigned_identity" "aks" {
  name                = "id-${var.name}-aks"
  location            = var.location
  resource_group_name = azurerm_resource_group.core.name
  tags                = local.tags
}

resource "azurerm_role_assignment" "aks_network" {
  for_each = {
    vnet  = azurerm_virtual_network.this.id
    route = azurerm_route_table.egress.id
  }
  scope                = each.value
  role_definition_name = "Network Contributor"
  principal_id         = azurerm_user_assigned_identity.aks.principal_id
}

# Explicit kubelet identity: AcrPull is granted before the cluster exists and survives rebuilds.
resource "azurerm_user_assigned_identity" "kubelet" {
  name                = "id-${var.name}-kubelet"
  location            = var.location
  resource_group_name = azurerm_resource_group.core.name
  tags                = local.tags
}

# The control plane must be able to assign the kubelet identity to nodes.
resource "azurerm_role_assignment" "aks_kubelet_operator" {
  scope                = azurerm_user_assigned_identity.kubelet.id
  role_definition_name = "Managed Identity Operator"
  principal_id         = azurerm_user_assigned_identity.aks.principal_id
}

resource "azurerm_role_assignment" "kubelet_acr_pull" {
  scope                = azurerm_container_registry.this.id
  role_definition_name = "AcrPull"
  principal_id         = azurerm_user_assigned_identity.kubelet.principal_id
}

resource "azurerm_kubernetes_cluster" "this" {
  name                = "aks-${var.name}"
  location            = var.location
  resource_group_name = azurerm_resource_group.core.name
  dns_prefix          = var.name
  kubernetes_version  = var.kubernetes_version
  sku_tier            = var.aks_sku_tier
  tags                = local.tags

  # Entra ID only: no static admin credentials, Kubernetes RBAC via Azure RBAC.
  local_account_disabled            = true
  role_based_access_control_enabled = true
  azure_active_directory_role_based_access_control {
    tenant_id              = data.azurerm_client_config.current.tenant_id
    admin_group_object_ids = var.admin_group_object_ids
    azure_rbac_enabled     = true
  }

  oidc_issuer_enabled          = true
  workload_identity_enabled    = true
  run_command_enabled          = false
  azure_policy_enabled         = true
  image_cleaner_enabled        = true
  image_cleaner_interval_hours = 48

  automatic_upgrade_channel = "patch"
  node_os_upgrade_channel   = "NodeImage"

  # The firewall's address is included because node traffic to the API server leaves through it.
  api_server_access_profile {
    authorized_ip_ranges = concat(var.operator_ip_ranges, ["${azurerm_public_ip.firewall.ip_address}/32"])
  }

  identity {
    type         = "UserAssigned"
    identity_ids = [azurerm_user_assigned_identity.aks.id]
  }

  kubelet_identity {
    user_assigned_identity_id = azurerm_user_assigned_identity.kubelet.id
    client_id                 = azurerm_user_assigned_identity.kubelet.client_id
    object_id                 = azurerm_user_assigned_identity.kubelet.principal_id
  }

  default_node_pool {
    name                         = "system"
    vm_size                      = var.system_vm_size
    vnet_subnet_id               = azurerm_subnet.nodes.id
    pod_subnet_id                = azurerm_subnet.pods.id
    zones                        = var.zones
    os_sku                       = "AzureLinux"
    only_critical_addons_enabled = true
    auto_scaling_enabled         = true
    min_count                    = var.system_node_count.min
    max_count                    = var.system_node_count.max
    node_public_ip_enabled       = false
    host_encryption_enabled      = var.host_encryption_enabled
    temporary_name_for_rotation  = "systemtmp"
    upgrade_settings {
      max_surge                     = "33%"
      node_soak_duration_in_minutes = 0
    }
  }

  network_profile {
    network_plugin     = "azure"
    network_data_plane = "cilium"
    network_policy     = "cilium"
    outbound_type      = "userDefinedRouting"
    load_balancer_sku  = "standard"
    service_cidr       = var.service_cidr
    dns_service_ip     = cidrhost(var.service_cidr, 10)
    advanced_networking {
      security_enabled      = var.fqdn_filtering_enabled
      observability_enabled = false
    }
  }

  # Node pools are declared explicitly below; no automatic (Karpenter-style) provisioning.
  node_provisioning_profile {
    mode               = "Manual"
    default_node_pools = "None"
  }

  oms_agent {
    log_analytics_workspace_id      = azurerm_log_analytics_workspace.this.id
    msi_auth_for_monitoring_enabled = true
  }

  dynamic "microsoft_defender" {
    for_each = var.defender_enabled ? [1] : []
    content {
      log_analytics_workspace_id = azurerm_log_analytics_workspace.this.id
    }
  }

  lifecycle {
    ignore_changes = [default_node_pool[0].node_count]
  }

  depends_on = [
    azurerm_role_assignment.aks_network,
    azurerm_role_assignment.aks_kubelet_operator,
    azurerm_role_assignment.kubelet_acr_pull,
    azurerm_subnet_route_table_association.cluster,
    azurerm_firewall_policy_rule_collection_group.platform,
  ]
}

# Runs workspace containers.
resource "azurerm_kubernetes_cluster_node_pool" "work" {
  name                        = "work"
  kubernetes_cluster_id       = azurerm_kubernetes_cluster.this.id
  mode                        = "User"
  vm_size                     = var.work_vm_size
  vnet_subnet_id              = azurerm_subnet.nodes.id
  pod_subnet_id               = azurerm_subnet.pods.id
  zones                       = var.zones
  os_sku                      = "AzureLinux"
  auto_scaling_enabled        = true
  min_count                   = var.work_node_count.min
  max_count                   = var.work_node_count.max
  node_public_ip_enabled      = false
  host_encryption_enabled     = var.host_encryption_enabled
  temporary_name_for_rotation = "worktmp"
  # AKS's defaults, stated so plans stay clean (Azure reports the soak time as 0, not unset).
  upgrade_settings {
    max_surge                     = "10%"
    node_soak_duration_in_minutes = 0
  }
  tags = local.tags
  lifecycle {
    ignore_changes = [node_count]
  }
}

# Runs only the access gateway (Guacamole). Its pods draw addresses from snet-gateway-pods,
# the only source workspace VM NSGs accept for RDP and SSH.
resource "azurerm_kubernetes_cluster_node_pool" "gateway" {
  name                        = "gateway"
  kubernetes_cluster_id       = azurerm_kubernetes_cluster.this.id
  mode                        = "User"
  vm_size                     = var.gateway_vm_size
  vnet_subnet_id              = azurerm_subnet.nodes.id
  pod_subnet_id               = azurerm_subnet.gateway_pods.id
  zones                       = var.zones
  os_sku                      = "AzureLinux"
  auto_scaling_enabled        = true
  min_count                   = var.gateway_node_count.min
  max_count                   = var.gateway_node_count.max
  node_public_ip_enabled      = false
  host_encryption_enabled     = var.host_encryption_enabled
  temporary_name_for_rotation = "gatewaytmp"
  # AKS's defaults, stated so plans stay clean (Azure reports the soak time as 0, not unset).
  upgrade_settings {
    max_surge                     = "10%"
    node_soak_duration_in_minutes = 0
  }
  node_labels = { "kubetre.io/node-pool" = "gateway" }
  node_taints = ["kubetre.io/node-pool=gateway:NoSchedule"]
  tags        = local.tags
  lifecycle {
    ignore_changes = [node_count]
  }
}

# Runs only KubeVirt VMs, which need nested virtualization. Tainted, so nothing else lands
# here; the VMs' pods use the ordinary pod subnet and workspace network policies.
resource "azurerm_kubernetes_cluster_node_pool" "kubevirt" {
  count                       = var.kubevirt_enabled ? 1 : 0
  name                        = "kubevirt"
  kubernetes_cluster_id       = azurerm_kubernetes_cluster.this.id
  mode                        = "User"
  vm_size                     = var.kubevirt_vm_size
  vnet_subnet_id              = azurerm_subnet.nodes.id
  pod_subnet_id               = azurerm_subnet.pods.id
  zones                       = var.zones
  os_sku                      = "AzureLinux"
  auto_scaling_enabled        = true
  min_count                   = var.kubevirt_node_count.min
  max_count                   = var.kubevirt_node_count.max
  node_public_ip_enabled      = false
  host_encryption_enabled     = var.host_encryption_enabled
  temporary_name_for_rotation = "kubevirttmp"
  upgrade_settings {
    max_surge                     = "10%"
    node_soak_duration_in_minutes = 0
  }
  node_labels = { "kubetre.io/node-pool" = "kubevirt" }
  node_taints = ["kubetre.io/node-pool=kubevirt:NoSchedule"]
  tags        = local.tags
  lifecycle {
    ignore_changes = [node_count]
  }
}

resource "azurerm_monitor_diagnostic_setting" "aks" {
  name                           = "diag-aks"
  target_resource_id             = azurerm_kubernetes_cluster.this.id
  log_analytics_workspace_id     = azurerm_log_analytics_workspace.this.id
  log_analytics_destination_type = "Dedicated"
  enabled_log {
    category = "kube-audit-admin"
  }
  enabled_log {
    category = "guard"
  }
  enabled_log {
    category = "kube-apiserver"
  }
}
