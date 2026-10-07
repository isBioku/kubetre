output "cluster_name" {
  value = azurerm_kubernetes_cluster.this.name
}

output "resource_group" {
  value = azurerm_resource_group.core.name
}

output "acr_name" {
  description = "Use with `make acr-build ACR_NAME=...` and `make publish-charts`."
  value       = azurerm_container_registry.this.name
}

output "acr_login_server" {
  value = azurerm_container_registry.this.login_server
}

output "firewall_public_ip" {
  value = azurerm_public_ip.firewall.ip_address
}

output "gitops_substitutions" {
  description = "Values Flux substitutes into deploy/azure."
  value       = local.gitops_substitutions
}

output "get_credentials" {
  value = "az aks get-credentials --resource-group ${azurerm_resource_group.core.name} --name ${azurerm_kubernetes_cluster.this.name}"
}

output "gateway_url" {
  description = "Create a DNS record for this host pointing at firewall_public_ip."
  value       = var.gateway_hostname == "" ? "" : "https://${var.gateway_hostname}"
}
