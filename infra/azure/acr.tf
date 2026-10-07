# Registry for images and Helm charts. Public access is denied except for operator IPs;
# AKS pulls through a private endpoint, and ACR Tasks may bypass the rules to build images.
resource "azurerm_container_registry" "this" {
  name                                  = "acr${var.name}"
  location                              = var.location
  resource_group_name                   = azurerm_resource_group.core.name
  sku                                   = "Premium"
  admin_enabled                         = false
  anonymous_pull_enabled                = false
  public_network_access_enabled         = true
  network_rule_bypass_option            = "AzureServices"
  network_rule_bypass_for_tasks_enabled = true
  zone_redundancy_enabled               = length(var.zones) > 0
  tags                                  = local.tags

  network_rule_set = [{
    default_action = "Deny"
    ip_rule        = [for cidr in var.operator_ip_ranges : { action = "Allow", ip_range = cidr }]
  }]
}

resource "azurerm_private_dns_zone" "acr" {
  name                = "privatelink.azurecr.io"
  resource_group_name = azurerm_resource_group.core.name
  tags                = local.tags
}

resource "azurerm_private_dns_zone_virtual_network_link" "acr" {
  name                = "acr"
  private_dns_zone_id = azurerm_private_dns_zone.acr.id
  virtual_network_id  = azurerm_virtual_network.this.id
  tags                = local.tags
}

resource "azurerm_private_endpoint" "acr" {
  name                = "pe-${var.name}-acr"
  location            = var.location
  resource_group_name = azurerm_resource_group.core.name
  subnet_id           = azurerm_subnet.endpoints.id
  tags                = local.tags

  private_service_connection {
    name                           = "acr"
    private_connection_resource_id = azurerm_container_registry.this.id
    subresource_names              = ["registry"]
    is_manual_connection           = false
  }

  private_dns_zone_group {
    name                 = "acr"
    private_dns_zone_ids = [azurerm_private_dns_zone.acr.id]
  }
}
