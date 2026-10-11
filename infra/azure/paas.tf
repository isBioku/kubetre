# The optional Azure PaaS workspace services (crossplane/paas): Azure SQL, MySQL, Azure OpenAI,
# Health Data Services, Azure ML and Databricks. Crossplane creates them per workspace in the
# workspaces resource group, with private endpoints in the workspace's subnet. Terraform adds
# what they share: private DNS zones linked to the VNet, Crossplane's federated credentials for
# the extra providers, and the permissions to create the services and grant members roles.

locals {
  paas_dns_zones = var.paas_services_enabled ? {
    mysql             = "privatelink.mysql.database.azure.com"
    sql               = "privatelink.database.windows.net"
    openai            = "privatelink.openai.azure.com"
    cognitiveservices = "privatelink.cognitiveservices.azure.com"
    healthcare        = "privatelink.azurehealthcareapis.com"
    dicom             = "privatelink.dicom.azurehealthcareapis.com"
    azureml           = "privatelink.api.azureml.ms"
    notebooks         = "privatelink.notebooks.azure.net"
    file              = "privatelink.file.core.windows.net"
    vault             = "privatelink.vaultcore.azure.net"
    databricks        = "privatelink.azuredatabricks.net"
  } : {}

  paas_providers = var.paas_services_enabled ? [for p in [
    "dbformysql", "sql", "cognitiveservices", "healthcareapis", "machinelearningservices",
    "databricks", "storage", "keyvault", "insights", "authorization",
  ] : "provider-azure-${p}"] : []

  # The only roles Crossplane may grant, and only inside the workspaces resource group: the
  # data roles members get, and what Azure ML grants its own identity on its storage and vault.
  paas_assignable_roles = [
    "5e0bd9bd-7b93-4f28-af87-19fc36ad61bd", # Cognitive Services OpenAI User
    "f6c7c914-8db3-469d-8ca1-694a8f32e121", # AzureML Data Scientist
    "5a1fc7df-4bf1-4951-a576-89034ee01acd", # FHIR Data Contributor
    "58a3b984-7adf-4c20-983a-32417c86fbc8", # DICOM Data Owner
    "ba92f5b4-2d11-453d-a403-e96b0029c9fe", # Storage Blob Data Contributor
    "69566ab7-960f-475b-8e7c-b3118f30c6bd", # Storage File Data Privileged Contributor
    "00482a5a-887f-4fb3-b363-3b7fe8e74483", # Key Vault Administrator
    "b24988ac-6180-42a0-ab88-20f7382dd24c", # Contributor (Databricks admins, Azure ML's identity)
  ]
  paas_role_condition = <<-EOT
    (
     (
      !(ActionMatches{'Microsoft.Authorization/roleAssignments/write'})
     )
     OR
     (
      @Request[Microsoft.Authorization/roleAssignments:RoleDefinitionId] ForAnyOfAnyValues:GuidEquals {${join(", ", local.paas_assignable_roles)}}
     )
    )
    AND
    (
     (
      !(ActionMatches{'Microsoft.Authorization/roleAssignments/delete'})
     )
     OR
     (
      @Resource[Microsoft.Authorization/roleAssignments:RoleDefinitionId] ForAnyOfAnyValues:GuidEquals {${join(", ", local.paas_assignable_roles)}}
     )
    )
  EOT

  paas_private_dns_zone_ids = merge(
    { for k, z in azurerm_private_dns_zone.paas : k => z.id },
    { blob = azurerm_private_dns_zone.blob.id },
  )
}

resource "azurerm_private_dns_zone" "paas" {
  for_each            = local.paas_dns_zones
  name                = each.value
  resource_group_name = azurerm_resource_group.core.name
  tags                = local.tags
}

resource "azurerm_private_dns_zone_virtual_network_link" "paas" {
  for_each            = local.paas_dns_zones
  name                = each.key
  private_dns_zone_id = azurerm_private_dns_zone.paas[each.key].id
  virtual_network_id  = azurerm_virtual_network.this.id
  tags                = local.tags
}

# Private endpoints register their addresses in these zones.
resource "azurerm_role_assignment" "crossplane_paas_dns" {
  for_each             = var.paas_services_enabled ? local.paas_private_dns_zone_ids : {}
  scope                = each.value
  role_definition_name = "Private DNS Zone Contributor"
  principal_id         = azurerm_user_assigned_identity.crossplane.principal_id
}

resource "azurerm_role_assignment" "crossplane_paas" {
  count                = var.paas_services_enabled ? 1 : 0
  scope                = azurerm_resource_group.workspaces.id
  role_definition_name = "Contributor"
  principal_id         = azurerm_user_assigned_identity.crossplane.principal_id
}

resource "azurerm_role_assignment" "crossplane_paas_roles" {
  count                = var.paas_services_enabled ? 1 : 0
  scope                = azurerm_resource_group.workspaces.id
  role_definition_name = "Role Based Access Control Administrator"
  principal_id         = azurerm_user_assigned_identity.crossplane.principal_id
  condition_version    = "2.0"
  condition            = local.paas_role_condition
}
