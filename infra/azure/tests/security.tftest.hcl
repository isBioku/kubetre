# Security baseline tests. They run against a mocked azurerm provider, so they need no Azure
# account: `terraform test` in infra/azure.

mock_provider "azurerm" {
  mock_data "azurerm_client_config" {
    defaults = {
      tenant_id       = "11111111-1111-1111-1111-111111111111"
      subscription_id = "22222222-2222-2222-2222-222222222222"
      object_id       = "33333333-3333-3333-3333-333333333333"
    }
  }
  mock_resource "azurerm_resource_group" {
    defaults = { id = "/subscriptions/22222222-2222-2222-2222-222222222222/resourceGroups/rg-kubetretest" }
  }
  mock_resource "azurerm_virtual_network" {
    defaults = { id = "/subscriptions/22222222-2222-2222-2222-222222222222/resourceGroups/rg-kubetretest/providers/Microsoft.Network/virtualNetworks/vnet-kubetretest" }
  }
  mock_resource "azurerm_subnet" {
    defaults = { id = "/subscriptions/22222222-2222-2222-2222-222222222222/resourceGroups/rg-kubetretest/providers/Microsoft.Network/virtualNetworks/vnet-kubetretest/subnets/default" }
  }
  mock_resource "azurerm_firewall_policy" {
    defaults = { id = "/subscriptions/22222222-2222-2222-2222-222222222222/resourceGroups/rg-kubetretest/providers/Microsoft.Network/firewallPolicies/fwp-kubetretest" }
  }
  mock_resource "azurerm_route_table" {
    defaults = { id = "/subscriptions/22222222-2222-2222-2222-222222222222/resourceGroups/rg-kubetretest/providers/Microsoft.Network/routeTables/rt-kubetretest-egress" }
  }
  mock_resource "azurerm_private_dns_zone" {
    defaults = { id = "/subscriptions/22222222-2222-2222-2222-222222222222/resourceGroups/rg-kubetretest/providers/Microsoft.Network/privateDnsZones/privatelink.azurecr.io" }
  }
  mock_resource "azurerm_container_registry" {
    defaults = { id = "/subscriptions/22222222-2222-2222-2222-222222222222/resourceGroups/rg-kubetretest/providers/Microsoft.ContainerRegistry/registries/acrkubetretest" }
  }
  mock_resource "azurerm_log_analytics_workspace" {
    defaults = { id = "/subscriptions/22222222-2222-2222-2222-222222222222/resourceGroups/rg-kubetretest/providers/Microsoft.OperationalInsights/workspaces/log-kubetretest" }
  }
  mock_resource "azurerm_public_ip" {
    defaults = { ip_address = "20.50.60.70", id = "/subscriptions/22222222-2222-2222-2222-222222222222/resourceGroups/rg-kubetretest/providers/Microsoft.Network/publicIPAddresses/pip-kubetretest-fw" }
  }
  mock_resource "azurerm_firewall" {
    defaults = {
      id               = "/subscriptions/22222222-2222-2222-2222-222222222222/resourceGroups/rg-kubetretest/providers/Microsoft.Network/azureFirewalls/fw-kubetretest"
      ip_configuration = { private_ip_address = "10.224.10.4" }
    }
  }
  mock_resource "azurerm_kubernetes_cluster" {
    defaults = {
      id              = "/subscriptions/22222222-2222-2222-2222-222222222222/resourceGroups/rg-kubetretest/providers/Microsoft.ContainerService/managedClusters/aks-kubetretest"
      oidc_issuer_url = "https://oidc.example.com/issuer/"
    }
  }
  mock_resource "azurerm_user_assigned_identity" {
    defaults = {
      id           = "/subscriptions/22222222-2222-2222-2222-222222222222/resourceGroups/rg-kubetretest/providers/Microsoft.ManagedIdentity/userAssignedIdentities/id-kubetretest"
      principal_id = "55555555-5555-5555-5555-555555555555"
      client_id    = "66666666-6666-6666-6666-666666666666"
    }
  }
  mock_resource "azurerm_role_definition" {
    defaults = { role_definition_resource_id = "/subscriptions/22222222-2222-2222-2222-222222222222/providers/Microsoft.Authorization/roleDefinitions/77777777-7777-7777-7777-777777777777" }
  }
}

override_resource {
  target = azurerm_subnet.nodes
  values = { id = "/subscriptions/22222222-2222-2222-2222-222222222222/resourceGroups/rg-kubetretest/providers/Microsoft.Network/virtualNetworks/vnet-kubetretest/subnets/snet-aks-nodes" }
}

override_resource {
  target = azurerm_subnet.pods
  values = { id = "/subscriptions/22222222-2222-2222-2222-222222222222/resourceGroups/rg-kubetretest/providers/Microsoft.Network/virtualNetworks/vnet-kubetretest/subnets/snet-aks-pods" }
}

override_resource {
  target = azurerm_subnet.gateway_pods
  values = { id = "/subscriptions/22222222-2222-2222-2222-222222222222/resourceGroups/rg-kubetretest/providers/Microsoft.Network/virtualNetworks/vnet-kubetretest/subnets/snet-gateway-pods" }
}

override_resource {
  target = azurerm_subnet.shared_ilb
  values = { id = "/subscriptions/22222222-2222-2222-2222-222222222222/resourceGroups/rg-kubetretest/providers/Microsoft.Network/virtualNetworks/vnet-kubetretest/subnets/snet-shared-ilb" }
}

override_resource {
  target = azurerm_subnet.firewall
  values = { id = "/subscriptions/22222222-2222-2222-2222-222222222222/resourceGroups/rg-kubetretest/providers/Microsoft.Network/virtualNetworks/vnet-kubetretest/subnets/AzureFirewallSubnet" }
}

override_resource {
  target = azurerm_subnet.endpoints
  values = { id = "/subscriptions/22222222-2222-2222-2222-222222222222/resourceGroups/rg-kubetretest/providers/Microsoft.Network/virtualNetworks/vnet-kubetretest/subnets/snet-private-endpoints" }
}

mock_provider "dns" {
  mock_data "dns_a_record_set" {
    defaults = { addrs = ["4.158.91.146"] }
  }
}

variables {
  name                   = "kubetretest"
  operator_ip_ranges     = ["203.0.113.0/24"]
  admin_group_object_ids = ["88888888-8888-8888-8888-888888888888"]
  gitops_repository_url  = "https://github.com/example/kubetre"
}

run "cluster_access_is_entra_only_and_restricted" {
  command = apply

  assert {
    condition     = azurerm_kubernetes_cluster.this.local_account_disabled && !azurerm_kubernetes_cluster.this.run_command_enabled
    error_message = "Local accounts and run-command must be disabled."
  }
  assert {
    condition     = azurerm_kubernetes_cluster.this.azure_active_directory_role_based_access_control[0].azure_rbac_enabled
    error_message = "Kubernetes authorization must use Azure RBAC."
  }
  assert {
    condition = (
      !contains(azurerm_kubernetes_cluster.this.api_server_access_profile[0].authorized_ip_ranges, "0.0.0.0/0") &&
      contains(azurerm_kubernetes_cluster.this.api_server_access_profile[0].authorized_ip_ranges, "203.0.113.0/24") &&
      contains(azurerm_kubernetes_cluster.this.api_server_access_profile[0].authorized_ip_ranges, "20.50.60.70/32")
    )
    error_message = "The API server must admit only operators and the firewall's egress address."
  }
  assert {
    condition     = azurerm_kubernetes_cluster.this.oidc_issuer_enabled && azurerm_kubernetes_cluster.this.workload_identity_enabled
    error_message = "Workload identity is required so no Azure secrets live in the cluster."
  }
}

run "egress_is_forced_through_the_firewall" {
  command = apply

  assert {
    condition     = azurerm_kubernetes_cluster.this.network_profile[0].outbound_type == "userDefinedRouting"
    error_message = "Cluster egress must use the firewall route, not a public load balancer."
  }
  assert {
    condition = (
      azurerm_kubernetes_cluster.this.network_profile[0].network_policy == "cilium" &&
      azurerm_kubernetes_cluster.this.network_profile[0].advanced_networking[0].security_enabled
    )
    error_message = "Cilium with FQDN filtering must be enabled for workspace egress policies."
  }
  assert {
    condition = (
      one(azurerm_route_table.egress.route).address_prefix == "0.0.0.0/0" &&
      one(azurerm_route_table.egress.route).next_hop_type == "VirtualAppliance" &&
      one(azurerm_route_table.egress.route).next_hop_in_ip_address == "10.224.10.4"
    )
    error_message = "The default route must point at the firewall."
  }
  assert {
    condition     = length(azurerm_subnet_route_table_association.cluster) == 4
    error_message = "Node, pod, gateway-pod and shared subnets must all use the egress route."
  }
  assert {
    condition     = azurerm_firewall_policy.this.threat_intelligence_mode == "Deny" && azurerm_firewall.this.threat_intel_mode == "Deny"
    error_message = "Threat intelligence must block, not just alert."
  }
}

run "no_public_node_addresses_and_isolated_gateway_pool" {
  command = apply

  assert {
    condition = (
      !azurerm_kubernetes_cluster.this.default_node_pool[0].node_public_ip_enabled &&
      !azurerm_kubernetes_cluster_node_pool.work.node_public_ip_enabled &&
      !azurerm_kubernetes_cluster_node_pool.gateway.node_public_ip_enabled
    )
    error_message = "No node may have a public IP."
  }
  assert {
    condition = (
      azurerm_kubernetes_cluster_node_pool.gateway.pod_subnet_id == azurerm_subnet.gateway_pods.id &&
      azurerm_kubernetes_cluster_node_pool.work.pod_subnet_id != azurerm_subnet.gateway_pods.id &&
      azurerm_kubernetes_cluster.this.default_node_pool[0].pod_subnet_id != azurerm_subnet.gateway_pods.id
    )
    error_message = "Only the gateway pool may use the gateway pod subnet that VM NSGs trust."
  }
  assert {
    condition     = contains(azurerm_kubernetes_cluster_node_pool.gateway.node_taints, "kubetre.io/node-pool=gateway:NoSchedule")
    error_message = "The gateway pool must be tainted so ordinary workloads cannot land on it."
  }
  assert {
    condition     = output.gitops_substitutions.ACCESS_GATEWAY_PREFIX == one(azurerm_subnet.gateway_pods.address_prefixes)
    error_message = "VM NSGs must be told the gateway pod subnet, not some other range."
  }
}

run "registry_is_locked_down" {
  command = apply

  assert {
    condition     = !azurerm_container_registry.this.admin_enabled && !azurerm_container_registry.this.anonymous_pull_enabled
    error_message = "ACR must not have admin credentials or anonymous pull."
  }
  assert {
    condition     = one(azurerm_container_registry.this.network_rule_set).default_action == "Deny"
    error_message = "ACR must deny public access by default."
  }
}

run "crossplane_has_no_broad_permissions" {
  command = apply

  assert {
    condition = alltrue([
      for ra in concat(
        values(azurerm_role_assignment.crossplane_workspaces),
        [azurerm_role_assignment.crossplane_subnets, azurerm_role_assignment.crossplane_route_join, azurerm_role_assignment.crossplane_firewall_rules],
      ) : !can(regex("^/subscriptions/[^/]+/?$", ra.scope))
    ])
    error_message = "Crossplane must not hold subscription-scoped roles."
  }
  assert {
    condition = alltrue([
      for ra in values(azurerm_role_assignment.crossplane_workspaces) : !contains(["Owner", "Contributor", "User Access Administrator"], ra.role_definition_name)
    ])
    error_message = "Crossplane must not be Owner, Contributor or User Access Administrator."
  }
  assert {
    condition     = toset([for f in azurerm_federated_identity_credential.crossplane : f.subject]) == toset(["system:serviceaccount:crossplane-system:provider-azure-network", "system:serviceaccount:crossplane-system:provider-azure-compute"])
    error_message = "Federated credentials must match the provider service account names in crossplane/install."
  }
}

run "only_https_to_the_gateway_is_published" {
  command = apply
  variables {
    oidc_issuer = "https://login.microsoftonline.com/11111111-1111-1111-1111-111111111111/v2.0"
  }

  assert {
    condition = (
      length(azurerm_firewall_policy_rule_collection_group.platform.nat_rule_collection) == 1 &&
      length(one(azurerm_firewall_policy_rule_collection_group.platform.nat_rule_collection).rule) == 1
    )
    error_message = "Exactly one inbound DNAT rule may exist."
  }
  assert {
    condition = alltrue([
      for r in one(azurerm_firewall_policy_rule_collection_group.platform.nat_rule_collection).rule :
      toset(r.destination_ports) == toset(["443"]) && tostring(r.translated_port) == "443" && r.translated_address == output.gitops_substitutions.GATEWAY_ILB_IP
    ])
    error_message = "Inbound traffic may only be HTTPS to the access gateway's internal load balancer."
  }
  assert {
    condition     = cidrhost(one(azurerm_subnet.shared_ilb.address_prefixes), 10) == output.gitops_substitutions.GATEWAY_ILB_IP
    error_message = "The gateway load balancer address must sit in the shared ILB subnet."
  }
  assert {
    condition     = output.gitops_substitutions.IDP_HOST == "login.microsoftonline.com"
    error_message = "The sign-in provider's host must be derived from the issuer."
  }
}

run "small_profile_keeps_the_security_baseline" {
  command = apply
  variables {
    system_vm_size     = "Standard_D2s_v5"
    system_node_count  = { min = 1, max = 1 }
    work_vm_size       = "Standard_D4s_v5"
    work_node_count    = { min = 1, max = 1 }
    gateway_vm_size    = "Standard_D2s_v5"
    gateway_node_count = { min = 1, max = 1 }
    aks_sku_tier       = "Free"
    defender_enabled   = false
    zones              = ["2", "3"]
  }

  assert {
    condition = (
      azurerm_kubernetes_cluster.this.default_node_pool[0].max_count == 1 &&
      azurerm_kubernetes_cluster_node_pool.work.max_count == 1 &&
      azurerm_kubernetes_cluster_node_pool.gateway.max_count == 1
    )
    error_message = "The small profile must never scale beyond its quota."
  }
  assert {
    condition = (
      azurerm_kubernetes_cluster.this.local_account_disabled &&
      azurerm_kubernetes_cluster.this.network_profile[0].outbound_type == "userDefinedRouting" &&
      azurerm_kubernetes_cluster.this.network_profile[0].advanced_networking[0].security_enabled &&
      azurerm_kubernetes_cluster_node_pool.gateway.pod_subnet_id == azurerm_subnet.gateway_pods.id
    )
    error_message = "Shrinking the cluster must not weaken its security settings."
  }
}

run "pods_reach_only_the_api_server_ip" {
  command = apply

  assert {
    condition = alltrue([
      for r in one(azurerm_firewall_policy_rule_collection_group.cluster_api.network_rule_collection).rule :
      toset(r.destination_addresses) == toset(["4.158.91.146"]) && toset(r.destination_ports) == toset(["443"]) && toset(r.protocols) == toset(["TCP"])
    ])
    error_message = "The API server rule must allow only TCP 443 to the API server's own address."
  }
  assert {
    condition = alltrue([
      for c in azurerm_firewall_policy_rule_collection_group.cluster_api.application_rule_collection : c.name != "node-managed-disks" ||
      alltrue([for r in c.rule : toset(r.source_addresses) == toset(azurerm_subnet.nodes.address_prefixes)])
    ])
    error_message = "Managed-disk endpoints must be reachable from the node subnet only, never from pods."
  }
}

run "flux_pulls_from_acr_with_its_own_identity" {
  command = apply

  assert {
    condition = (
      azurerm_kubernetes_cluster_extension.flux.configuration_settings["workloadIdentity.enable"] == "true" &&
      azurerm_kubernetes_cluster_extension.flux.configuration_settings["workloadIdentity.azureClientId"] == azurerm_user_assigned_identity.flux.client_id
    )
    error_message = "Flux must use workload identity, not the node's managed identities."
  }
  assert {
    condition     = azurerm_role_assignment.flux_acr_pull.scope == azurerm_container_registry.this.id && azurerm_role_assignment.flux_acr_pull.role_definition_name == "AcrPull"
    error_message = "Flux's identity may only pull from this environment's registry."
  }
  assert {
    condition     = azurerm_federated_identity_credential.flux_source_controller.subject == "system:serviceaccount:flux-system:source-controller"
    error_message = "Only Flux's source controller may use the Flux identity."
  }
}

run "inverted_node_bounds_are_rejected" {
  command = plan
  variables {
    gateway_node_count = { min = 3, max = 1 }
  }
  expect_failures = [var.gateway_node_count]
}

run "open_api_server_is_rejected" {
  command = plan
  variables {
    operator_ip_ranges = []
  }
  expect_failures = [var.operator_ip_ranges]
}

run "kubevirt_is_off_by_default" {
  command = apply

  assert {
    condition     = length(azurerm_kubernetes_cluster_node_pool.kubevirt) == 0
    error_message = "No KubeVirt node pool unless kubevirt_enabled is set."
  }
  assert {
    condition     = !contains([for k in azurerm_kubernetes_flux_configuration.kubetre[0].kustomizations : k.name], "kubevirt")
    error_message = "No KubeVirt Flux stage unless kubevirt_enabled is set."
  }
}

run "kubevirt_runs_on_its_own_tainted_pool" {
  command = apply
  variables {
    kubevirt_enabled = true
  }

  assert {
    condition     = contains(azurerm_kubernetes_cluster_node_pool.kubevirt[0].node_taints, "kubetre.io/node-pool=kubevirt:NoSchedule")
    error_message = "The KubeVirt pool must be tainted so only VMs land on it."
  }
  assert {
    condition     = azurerm_kubernetes_cluster_node_pool.kubevirt[0].pod_subnet_id != azurerm_subnet.gateway_pods.id
    error_message = "KubeVirt VMs must not draw addresses from the gateway pod subnet that workspace VM NSGs trust."
  }
  assert {
    condition     = !azurerm_kubernetes_cluster_node_pool.kubevirt[0].node_public_ip_enabled
    error_message = "KubeVirt nodes must have no public IPs."
  }
  assert {
    condition     = one([for k in azurerm_kubernetes_flux_configuration.kubetre[0].kustomizations : k.depends_on if k.name == "kubevirt"]) == tolist(["platform"])
    error_message = "The KubeVirt stage must follow the platform stage, which defines ServiceTemplate."
  }
}
