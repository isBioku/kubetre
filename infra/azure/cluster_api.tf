# Cluster egress that the AzureKubernetesService FQDN tag does not cover, found from the
# firewall's own deny logs on the first deployment.

# Pods reach the Kubernetes API through the kubernetes Service (172.16.0.1), which Cilium
# translates to the API server's public IP. That traffic carries no hostname, so an FQDN rule
# cannot match it and the firewall drops it. AKS's documented egress requirements include
# this rule: TCP 443 to the API server's IP. Without it, Flux and every controller that talks
# to the API crash-loop.
data "dns_a_record_set" "api_server" {
  host = azurerm_kubernetes_cluster.this.fqdn
}

locals {
  # Endpoints for cluster extensions (Flux) and Azure Monitor's agents.
  azure_platform_fqdns = [
    "${var.location}.dp.kubernetesconfiguration.azure.com",
    "global.handler.control.monitor.azure.com",
    "${var.location}.handler.control.monitor.azure.com",
    "*.ods.opinsights.azure.com",
    "*.oms.opinsights.azure.com",
    "*.ingest.monitor.azure.com",
  ]
}

resource "azurerm_firewall_policy_rule_collection_group" "cluster_api" {
  name               = "rcg-cluster-api"
  firewall_policy_id = azurerm_firewall_policy.this.id
  priority           = 300

  network_rule_collection {
    name     = "api-server"
    priority = 100
    action   = "Allow"
    rule {
      name                  = "api-server-https"
      protocols             = ["TCP"]
      source_addresses      = local.cluster_prefixes
      destination_addresses = data.dns_a_record_set.api_server.addrs
      destination_ports     = ["443"]
    }
  }

  application_rule_collection {
    name     = "azure-platform-services"
    priority = 200
    action   = "Allow"
    rule {
      name              = "aks-extensions-and-monitoring"
      source_addresses  = local.cluster_prefixes
      destination_fqdns = local.azure_platform_fqdns
      protocols {
        type = "Https"
        port = 443
      }
    }
  }

  # Managed-disk endpoints, used by the nodes themselves. Allowed from the node subnet only:
  # pods (including workspace pods) share the pod subnets and must not get a blob upload path.
  application_rule_collection {
    name     = "node-managed-disks"
    priority = 210
    action   = "Allow"
    rule {
      name              = "managed-disk-endpoints"
      source_addresses  = [local.subnets.nodes]
      destination_fqdns = ["*.blob.storage.azure.net"]
      protocols {
        type = "Https"
        port = 443
      }
    }
  }
}
