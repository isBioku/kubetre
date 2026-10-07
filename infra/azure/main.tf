locals {
  tags = merge({ "managed-by" = "terraform", "kubetre-environment" = var.name }, var.tags)

  # Address plan inside the /16 VNet space.
  subnets = {
    nodes        = cidrsubnet(var.vnet_address_space, 6, 0)   # /22 AKS nodes
    pods         = cidrsubnet(var.vnet_address_space, 6, 1)   # /22 pods on system and work pools
    gateway_pods = cidrsubnet(var.vnet_address_space, 8, 8)   # /24 pods on the access-gateway pool only
    shared_ilb   = cidrsubnet(var.vnet_address_space, 8, 9)   # /24 internal load balancers for shared services
    firewall     = cidrsubnet(var.vnet_address_space, 10, 40) # /26 AzureFirewallSubnet
    endpoints    = cidrsubnet(var.vnet_address_space, 8, 11)  # /24 private endpoints
  }

  # Fixed address of the access gateway's internal load balancer; the firewall forwards to it.
  gateway_ilb_ip = cidrhost(local.subnets.shared_ilb, 10)

  # Host of the OIDC issuer, which the API and the gateway broker must reach.
  idp_host = var.oidc_issuer == "" ? "" : regex("^https://([^/:]+)", var.oidc_issuer)[0]

  # Cluster sources allowed through the firewall for platform traffic.
  cluster_prefixes = [local.subnets.nodes, local.subnets.pods, local.subnets.gateway_pods]
}

resource "azurerm_resource_group" "core" {
  name     = "rg-${var.name}"
  location = var.location
  tags     = local.tags
}

# Workspace subnets' NSGs, NICs and VMs live here, created by Crossplane.
resource "azurerm_resource_group" "workspaces" {
  name     = "rg-${var.name}-workspaces"
  location = var.location
  tags     = local.tags
}

resource "azurerm_log_analytics_workspace" "this" {
  name                = "log-${var.name}"
  location            = var.location
  resource_group_name = azurerm_resource_group.core.name
  sku                 = "PerGB2018"
  retention_in_days   = var.log_retention_days
  tags                = local.tags
}
