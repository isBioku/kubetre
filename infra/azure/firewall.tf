resource "azurerm_public_ip" "firewall" {
  name                = "pip-${var.name}-fw"
  location            = var.location
  resource_group_name = azurerm_resource_group.core.name
  allocation_method   = "Static"
  sku                 = "Standard"
  zones               = var.zones
  tags                = local.tags
}

resource "azurerm_firewall_policy" "this" {
  name                     = "fwp-${var.name}"
  location                 = var.location
  resource_group_name      = azurerm_resource_group.core.name
  sku                      = var.firewall_sku_tier
  threat_intelligence_mode = "Deny"
  tags                     = local.tags
}

resource "azurerm_firewall" "this" {
  name                = "fw-${var.name}"
  location            = var.location
  resource_group_name = azurerm_resource_group.core.name
  sku_name            = "AZFW_VNet"
  sku_tier            = var.firewall_sku_tier
  firewall_policy_id  = azurerm_firewall_policy.this.id
  threat_intel_mode   = "Deny"
  zones               = var.zones
  tags                = local.tags

  ip_configuration {
    name                 = "ipconfig"
    subnet_id            = azurerm_subnet.firewall.id
    public_ip_address_id = azurerm_public_ip.firewall.id
  }
}

# Platform rules. Workspace allowlists are added by Crossplane as rule collection groups
# with priorities from 1000 upwards, so they never interleave with these.
resource "azurerm_firewall_policy_rule_collection_group" "platform" {
  name               = "rcg-platform"
  firewall_policy_id = azurerm_firewall_policy.this.id
  priority           = 200

  # The only inbound path: HTTPS on the firewall's public IP to the access gateway's internal
  # load balancer. The firewall source-NATs, so return traffic is symmetric.
  nat_rule_collection {
    name     = "gateway-ingress"
    priority = 100
    action   = "Dnat"
    rule {
      name                = "https-to-gateway"
      protocols           = ["TCP"]
      source_addresses    = var.gateway_source_ranges
      destination_address = azurerm_public_ip.firewall.ip_address
      destination_ports   = ["443"]
      translated_address  = local.gateway_ilb_ip
      translated_port     = "443"
    }
  }

  network_rule_collection {
    name     = "aks-required"
    priority = 110
    action   = "Allow"
    rule {
      name                  = "aks-tunnel-udp"
      protocols             = ["UDP"]
      source_addresses      = local.cluster_prefixes
      destination_addresses = ["AzureCloud.${var.location}"]
      destination_ports     = ["1194"]
    }
    rule {
      name                  = "aks-tunnel-tcp"
      protocols             = ["TCP"]
      source_addresses      = local.cluster_prefixes
      destination_addresses = ["AzureCloud.${var.location}"]
      destination_ports     = ["9000"]
    }
  }

  # Windows activation for pay-as-you-go images: Azure KMS endpoints (documented fixed IPs).
  network_rule_collection {
    name     = "vm-windows-activation"
    priority = 120
    action   = "Allow"
    rule {
      name                  = "azure-kms"
      protocols             = ["TCP"]
      source_addresses      = [var.vm_address_pool]
      destination_addresses = ["20.118.99.224", "40.83.235.53", "23.102.135.246"]
      destination_ports     = ["1688"]
    }
  }

  application_rule_collection {
    name     = "cluster-egress"
    priority = 200
    action   = "Allow"
    rule {
      name                  = "aks-fqdn-tag"
      source_addresses      = local.cluster_prefixes
      destination_fqdn_tags = ["AzureKubernetesService"]
      protocols {
        type = "Https"
        port = 443
      }
      protocols {
        type = "Http"
        port = 80
      }
    }
    rule {
      name              = "platform-packages"
      source_addresses  = local.cluster_prefixes
      destination_fqdns = concat(var.platform_egress_fqdns, local.idp_host == "" ? [] : [local.idp_host])
      protocols {
        type = "Https"
        port = 443
      }
    }
  }
}

resource "azurerm_monitor_diagnostic_setting" "firewall" {
  name                           = "diag-firewall"
  target_resource_id             = azurerm_firewall.this.id
  log_analytics_workspace_id     = azurerm_log_analytics_workspace.this.id
  log_analytics_destination_type = "Dedicated"
  enabled_log {
    category_group = "allLogs"
  }
}
