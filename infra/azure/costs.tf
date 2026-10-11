# Cost reporting: the KubeTRE API reads Azure Cost Management with its own identity, which
# can read cost data and nothing else. It is federated to the API's service account only.
resource "azurerm_user_assigned_identity" "costs" {
  name                = "id-${var.name}-costs"
  location            = var.location
  resource_group_name = azurerm_resource_group.core.name
  tags                = local.tags
}

resource "azurerm_federated_identity_credential" "costs" {
  name                      = "kubetre-api"
  user_assigned_identity_id = azurerm_user_assigned_identity.costs.id
  issuer                    = azurerm_kubernetes_cluster.this.oidc_issuer_url
  subject                   = "system:serviceaccount:kubetre-system:kubetre-api"
  audience                  = ["api://AzureADTokenExchange"]
}

# Cost Management queries are made at subscription scope, filtered to the TRE's resource
# groups, as AzureTRE does. The role grants cost data only.
resource "azurerm_role_assignment" "costs" {
  scope                = "/subscriptions/${data.azurerm_client_config.current.subscription_id}"
  role_definition_name = "Cost Management Reader"
  principal_id         = azurerm_user_assigned_identity.costs.principal_id
}
