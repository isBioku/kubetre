# Crossplane's Azure identity, federated to its provider service accounts. Permissions are
# custom roles scoped to exactly what the KubeTRE compositions create; nothing is
# subscription-wide.
resource "azurerm_user_assigned_identity" "crossplane" {
  name                = "id-${var.name}-crossplane"
  location            = var.location
  resource_group_name = azurerm_resource_group.core.name
  tags                = local.tags
}

locals {
  crossplane_service_accounts = ["provider-azure-network", "provider-azure-compute"]
}

resource "azurerm_federated_identity_credential" "crossplane" {
  for_each                  = toset(local.crossplane_service_accounts)
  name                      = each.value
  user_assigned_identity_id = azurerm_user_assigned_identity.crossplane.id
  issuer                    = azurerm_kubernetes_cluster.this.oidc_issuer_url
  subject                   = "system:serviceaccount:crossplane-system:${each.value}"
  audience                  = ["api://AzureADTokenExchange"]
}

# Workspace NICs, NSGs, disks and VMs live in their own resource group.
resource "azurerm_role_assignment" "crossplane_workspaces" {
  for_each             = toset(["Network Contributor", "Virtual Machine Contributor"])
  scope                = azurerm_resource_group.workspaces.id
  role_definition_name = each.value
  principal_id         = azurerm_user_assigned_identity.crossplane.principal_id
}

resource "azurerm_role_definition" "crossplane_subnets" {
  name        = "kubetre-${var.name}-workspace-subnets"
  scope       = azurerm_virtual_network.this.id
  description = "Create and delete workspace subnets in the KubeTRE VNet."
  permissions {
    actions = [
      "Microsoft.Network/virtualNetworks/read",
      "Microsoft.Network/virtualNetworks/subnets/read",
      "Microsoft.Network/virtualNetworks/subnets/write",
      "Microsoft.Network/virtualNetworks/subnets/delete",
      "Microsoft.Network/virtualNetworks/subnets/join/action",
    ]
  }
  assignable_scopes = [azurerm_virtual_network.this.id]
}

resource "azurerm_role_assignment" "crossplane_subnets" {
  scope              = azurerm_virtual_network.this.id
  role_definition_id = azurerm_role_definition.crossplane_subnets.role_definition_resource_id
  principal_id       = azurerm_user_assigned_identity.crossplane.principal_id
}

resource "azurerm_role_definition" "crossplane_route_join" {
  name        = "kubetre-${var.name}-route-join"
  scope       = azurerm_route_table.egress.id
  description = "Associate workspace subnets with the egress route table."
  permissions {
    actions = [
      "Microsoft.Network/routeTables/read",
      "Microsoft.Network/routeTables/join/action",
    ]
  }
  assignable_scopes = [azurerm_route_table.egress.id]
}

resource "azurerm_role_assignment" "crossplane_route_join" {
  scope              = azurerm_route_table.egress.id
  role_definition_id = azurerm_role_definition.crossplane_route_join.role_definition_resource_id
  principal_id       = azurerm_user_assigned_identity.crossplane.principal_id
}

resource "azurerm_role_definition" "crossplane_firewall_rules" {
  name        = "kubetre-${var.name}-firewall-rules"
  scope       = azurerm_firewall_policy.this.id
  description = "Manage per-workspace rule collection groups on the KubeTRE firewall policy."
  permissions {
    actions = [
      "Microsoft.Network/firewallPolicies/read",
      "Microsoft.Network/firewallPolicies/join/action",
      "Microsoft.Network/firewallPolicies/ruleCollectionGroups/read",
      "Microsoft.Network/firewallPolicies/ruleCollectionGroups/write",
      "Microsoft.Network/firewallPolicies/ruleCollectionGroups/delete",
    ]
  }
  assignable_scopes = [azurerm_firewall_policy.this.id]
}

resource "azurerm_role_assignment" "crossplane_firewall_rules" {
  scope              = azurerm_firewall_policy.this.id
  role_definition_id = azurerm_role_definition.crossplane_firewall_rules.role_definition_resource_id
  principal_id       = azurerm_user_assigned_identity.crossplane.principal_id
}
