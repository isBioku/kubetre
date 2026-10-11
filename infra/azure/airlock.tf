# The airlock: the only sanctioned way to move files into or out of a workspace.
#
# Two storage accounts, with each request's stage kept by KubeTRE rather than in a storage
# account per stage (the design AzureTRE has also moved to):
#   - external: import drafts uploaded from outside, and approved exports downloaded from
#     outside. Reachable from the internet, but only with short-lived, container-scoped links
#     signed with Entra ID (shared keys are disabled).
#   - internal: export drafts uploaded from workspace machines, and approved imports downloaded
#     inside workspaces. No public access; one private endpoint at a fixed address, which
#     workspace VM security groups allow on HTTPS.
# The KubeTRE API and controller use the airlock identity, through workload identity, to create
# containers, sign links and move files between stages.

locals {
  airlock_external_account = "stalext${var.name}"
  airlock_internal_account = "stalint${var.name}"
  airlock_internal_pe_ip   = cidrhost(local.subnets.endpoints, 20)
}

resource "azurerm_storage_account" "airlock" {
  for_each                          = { external = local.airlock_external_account, internal = local.airlock_internal_account }
  name                              = each.value
  resource_group_name               = azurerm_resource_group.core.name
  location                          = var.location
  account_tier                      = "Standard"
  account_replication_type          = "ZRS"
  account_kind                      = "StorageV2"
  min_tls_version                   = "TLS1_2"
  shared_access_key_enabled         = false
  default_to_oauth_authentication   = true
  allow_nested_items_to_be_public   = false
  infrastructure_encryption_enabled = true
  public_network_access             = each.key == "external" ? "Enabled" : "Disabled"
  tags                              = local.tags

  dynamic "network_rules" {
    for_each = each.key == "external" && !contains(var.gateway_source_ranges, "*") ? [1] : []
    content {
      default_action = "Deny"
      # Users' networks, and the firewall that the cluster and workspace VMs leave through.
      ip_rules = concat([for r in var.gateway_source_ranges : trimsuffix(r, "/32")], [azurerm_public_ip.firewall.ip_address])
      bypass   = ["AzureServices"]
    }
  }

  blob_properties {
    delete_retention_policy {
      days = 7
    }
    container_delete_retention_policy {
      days = 7
    }
  }
}

resource "azurerm_private_dns_zone" "blob" {
  name                = "privatelink.blob.core.windows.net"
  resource_group_name = azurerm_resource_group.core.name
  tags                = local.tags
}

resource "azurerm_private_dns_zone_virtual_network_link" "blob" {
  name                = "blob"
  private_dns_zone_id = azurerm_private_dns_zone.blob.id
  virtual_network_id  = azurerm_virtual_network.this.id
  tags                = local.tags
}

resource "azurerm_private_endpoint" "airlock_internal" {
  name                = "pe-${var.name}-airlock"
  location            = var.location
  resource_group_name = azurerm_resource_group.core.name
  subnet_id           = azurerm_subnet.endpoints.id
  tags                = local.tags

  private_service_connection {
    name                           = "airlock-internal"
    private_connection_resource_id = azurerm_storage_account.airlock["internal"].id
    subresource_names              = ["blob"]
    is_manual_connection           = false
  }

  # A fixed address, so workspace VM security groups can allow exactly this endpoint.
  ip_configuration {
    name               = "blob"
    private_ip_address = local.airlock_internal_pe_ip
    subresource_name   = "blob"
    member_name        = "blob"
  }

  private_dns_zone_group {
    name                 = "blob"
    private_dns_zone_ids = [azurerm_private_dns_zone.blob.id]
  }
}

resource "azurerm_user_assigned_identity" "airlock" {
  name                = "id-${var.name}-airlock"
  location            = var.location
  resource_group_name = azurerm_resource_group.core.name
  tags                = local.tags
}

resource "azurerm_federated_identity_credential" "airlock" {
  for_each                  = toset(["kubetre-api", "kubetre-controller"])
  name                      = each.value
  user_assigned_identity_id = azurerm_user_assigned_identity.airlock.id
  issuer                    = azurerm_kubernetes_cluster.this.oidc_issuer_url
  subject                   = "system:serviceaccount:kubetre-system:${each.value}"
  audience                  = ["api://AzureADTokenExchange"]
}

# Data access on the two airlock accounts only, including signing user delegation links.
resource "azurerm_role_assignment" "airlock_data" {
  for_each             = azurerm_storage_account.airlock
  scope                = each.value.id
  role_definition_name = "Storage Blob Data Contributor"
  principal_id         = azurerm_user_assigned_identity.airlock.principal_id
}

# Optional malware scanning of every uploaded file. KubeTRE reads the scan result from the
# blob's index tags before a request reaches review.
resource "azurerm_security_center_storage_defender" "airlock" {
  for_each                                    = var.airlock_malware_scanning_enabled ? azurerm_storage_account.airlock : {}
  storage_account_id                          = each.value.id
  malware_scanning_on_upload_enabled          = true
  malware_scanning_on_upload_cap_gb_per_month = var.airlock_malware_scanning_cap_gb
  sensitive_data_discovery_enabled            = false
}
