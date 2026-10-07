# The VNet deliberately declares no inline subnets. Crossplane adds one subnet per VM-enabled
# workspace inside var.vm_address_pool; inline subnets would make Terraform delete them.
resource "azurerm_virtual_network" "this" {
  name                = "vnet-${var.name}"
  location            = var.location
  resource_group_name = azurerm_resource_group.core.name
  address_space       = [var.vnet_address_space, var.vm_address_pool]
  tags                = local.tags
}

resource "azurerm_subnet" "nodes" {
  name                            = "snet-aks-nodes"
  resource_group_name             = azurerm_resource_group.core.name
  virtual_network_name            = azurerm_virtual_network.this.name
  address_prefixes                = [local.subnets.nodes]
  default_outbound_access_enabled = false
}

resource "azurerm_subnet" "pods" {
  name                            = "snet-aks-pods"
  resource_group_name             = azurerm_resource_group.core.name
  virtual_network_name            = azurerm_virtual_network.this.name
  address_prefixes                = [local.subnets.pods]
  default_outbound_access_enabled = false
  delegation {
    name = "aks"
    service_delegation {
      name    = "Microsoft.ContainerService/managedClusters"
      actions = ["Microsoft.Network/virtualNetworks/subnets/join/action"]
    }
  }
}

# Only access-gateway pods get addresses here, so workspace VM NSGs can trust this range alone.
resource "azurerm_subnet" "gateway_pods" {
  name                            = "snet-gateway-pods"
  resource_group_name             = azurerm_resource_group.core.name
  virtual_network_name            = azurerm_virtual_network.this.name
  address_prefixes                = [local.subnets.gateway_pods]
  default_outbound_access_enabled = false
  delegation {
    name = "aks"
    service_delegation {
      name    = "Microsoft.ContainerService/managedClusters"
      actions = ["Microsoft.Network/virtualNetworks/subnets/join/action"]
    }
  }
}

resource "azurerm_subnet" "shared_ilb" {
  name                            = "snet-shared-ilb"
  resource_group_name             = azurerm_resource_group.core.name
  virtual_network_name            = azurerm_virtual_network.this.name
  address_prefixes                = [local.subnets.shared_ilb]
  default_outbound_access_enabled = false
}

resource "azurerm_subnet" "firewall" {
  name                 = "AzureFirewallSubnet"
  resource_group_name  = azurerm_resource_group.core.name
  virtual_network_name = azurerm_virtual_network.this.name
  address_prefixes     = [local.subnets.firewall]
}

resource "azurerm_subnet" "endpoints" {
  name                            = "snet-private-endpoints"
  resource_group_name             = azurerm_resource_group.core.name
  virtual_network_name            = azurerm_virtual_network.this.name
  address_prefixes                = [local.subnets.endpoints]
  default_outbound_access_enabled = false
}

# All egress from the cluster and from workspace VM subnets goes to the firewall.
resource "azurerm_route_table" "egress" {
  name                          = "rt-${var.name}-egress"
  location                      = var.location
  resource_group_name           = azurerm_resource_group.core.name
  bgp_route_propagation_enabled = false
  tags                          = local.tags

  route = [{
    name                   = "default-via-firewall"
    address_prefix         = "0.0.0.0/0"
    next_hop_type          = "VirtualAppliance"
    next_hop_in_ip_address = azurerm_firewall.this.ip_configuration[0].private_ip_address
  }]
}

resource "azurerm_subnet_route_table_association" "cluster" {
  for_each = {
    nodes        = azurerm_subnet.nodes.id
    pods         = azurerm_subnet.pods.id
    gateway_pods = azurerm_subnet.gateway_pods.id
    shared_ilb   = azurerm_subnet.shared_ilb.id
  }
  subnet_id      = each.value
  route_table_id = azurerm_route_table.egress.id
}
