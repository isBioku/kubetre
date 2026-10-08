variable "name" {
  description = "Short environment name used in every resource name, for example kubetredev."
  type        = string
  validation {
    condition     = can(regex("^[a-z][a-z0-9]{2,11}$", var.name))
    error_message = "name must be 3-12 lowercase letters or digits, starting with a letter (it is used in the ACR name)."
  }
}

variable "location" {
  description = "Azure region."
  type        = string
  default     = "uksouth"
}

variable "zones" {
  description = "Availability zones for AKS nodes and the firewall. Use [] in regions without zones."
  type        = list(string)
  default     = ["1", "2", "3"]
}

variable "operator_ip_ranges" {
  description = "Public CIDRs of operators and CI. Only these reach the AKS API server and the container registry."
  type        = list(string)
  validation {
    condition     = length(var.operator_ip_ranges) > 0 && alltrue([for c in var.operator_ip_ranges : can(cidrhost(c, 0))])
    error_message = "Provide at least one valid CIDR; an open API server is not allowed."
  }
}

variable "admin_group_object_ids" {
  description = "Entra ID groups whose members are cluster administrators. Local Kubernetes accounts are disabled."
  type        = list(string)
  validation {
    condition     = length(var.admin_group_object_ids) > 0
    error_message = "At least one admin group is required because local accounts are disabled."
  }
}

variable "vnet_address_space" {
  description = "Address space for AKS, the firewall and shared subnets. Must be a /16."
  type        = string
  default     = "10.224.0.0/16"
}

variable "vm_address_pool" {
  description = "Address range for workspace VM subnets, allocated by the KubeTRE controller. Must not overlap vnet_address_space."
  type        = string
  default     = "10.240.0.0/16"
}

variable "service_cidr" {
  description = "Kubernetes service CIDR. Must not overlap the VNet."
  type        = string
  default     = "172.16.0.0/16"
}

variable "kubernetes_version" {
  description = "AKS version; null uses the region default."
  type        = string
  default     = null
}

variable "system_vm_size" {
  type    = string
  default = "Standard_D4s_v5"
}

variable "system_node_count" {
  description = "Autoscaling bounds for the system node pool."
  type        = object({ min = number, max = number })
  default     = { min = 2, max = 4 }
  validation {
    condition     = var.system_node_count.min >= 1 && var.system_node_count.min <= var.system_node_count.max
    error_message = "system_node_count needs 1 <= min <= max."
  }
}

variable "work_vm_size" {
  type    = string
  default = "Standard_D8s_v5"
}

variable "work_node_count" {
  description = "Autoscaling bounds for the pool that runs workspace containers."
  type        = object({ min = number, max = number })
  default     = { min = 1, max = 10 }
}

variable "gateway_vm_size" {
  type    = string
  default = "Standard_D4s_v5"
}

variable "gateway_node_count" {
  description = "Autoscaling bounds for the access-gateway node pool."
  type        = object({ min = number, max = number })
  default     = { min = 2, max = 3 }
  validation {
    condition     = var.gateway_node_count.min >= 1 && var.gateway_node_count.min <= var.gateway_node_count.max
    error_message = "gateway_node_count needs 1 <= min <= max."
  }
}

variable "research_vm_sizes" {
  description = "Azure VM size behind each research VM size a researcher can pick."
  type        = object({ small = string, medium = string, large = string })
  default = {
    small  = "Standard_D2s_v5"
    medium = "Standard_D4s_v5"
    large  = "Standard_D8s_v5"
  }
}

variable "host_encryption_enabled" {
  description = "Encrypt node and VM temp disks and caches at host. Requires the EncryptionAtHost feature on the subscription."
  type        = bool
  default     = true
}

variable "aks_sku_tier" {
  description = "Free or Standard (financially backed API server SLA)."
  type        = string
  default     = "Standard"
}

variable "fqdn_filtering_enabled" {
  description = "Advanced Container Networking Services security, which enforces FQDN egress policies for pods. Paid add-on."
  type        = bool
  default     = true
}

variable "defender_enabled" {
  description = "Microsoft Defender for Containers."
  type        = bool
  default     = true
}

variable "firewall_sku_tier" {
  type    = string
  default = "Standard"
  validation {
    condition     = contains(["Standard", "Premium"], var.firewall_sku_tier)
    error_message = "Standard or Premium; Basic cannot do FQDN application rules at the needed scale."
  }
}

variable "platform_egress_fqdns" {
  description = "Extra HTTPS destinations the cluster itself needs, beyond the AzureKubernetesService FQDN tag. Check firewall logs for denials on first deployment."
  type        = list(string)
  default = [
    "xpkg.upbound.io",
    # Upbound's registry serves package layers from this CloudFront distribution. Allow the
    # exact host, never *.cloudfront.net, which would open every CloudFront site.
    "d3qrbvrml4iuq4.cloudfront.net",
    "xpkg.crossplane.io",
    "*.blob.core.windows.net",
    "charts.crossplane.io",
    "ghcr.io",
    "pkg-containers.githubusercontent.com",
    "github.com",
    "gcr.io",
    "storage.googleapis.com",
  ]
}

variable "log_retention_days" {
  type    = number
  default = 90
}

variable "gitops_repository_url" {
  description = "HTTPS URL of the Git repository Flux syncs. Empty installs Flux without a configuration."
  type        = string
  default     = ""
}

variable "gitops_branch" {
  type    = string
  default = "main"
}

variable "gitops_path" {
  description = "Path inside the repository to the deploy/azure directory."
  type        = string
  default     = "deploy/azure"
}

variable "kubetre_version" {
  description = "Image tag of KubeTRE's images in ACR. Keep it equal to the VERSION file, which `make acr-build` uses."
  type        = string
  default     = "0.2.1"
}

variable "oidc_issuer" {
  description = "OIDC issuer the KubeTRE API trusts, for example https://login.microsoftonline.com/<tenant>/v2.0."
  type        = string
  default     = ""
}

variable "oidc_audience" {
  description = "Audience (client ID) of the KubeTRE API's app registration."
  type        = string
  default     = ""
}

variable "roles_claim" {
  type    = string
  default = "roles"
}

variable "firewall_dns_label" {
  description = "Optional DNS label for the firewall's public IP. It yields <label>.<location>.cloudapp.azure.com, a free hostname for the gateway when you have no domain."
  type        = string
  default     = ""
  validation {
    condition     = var.firewall_dns_label == "" || can(regex("^[a-z][a-z0-9-]{1,61}[a-z0-9]$", var.firewall_dns_label))
    error_message = "firewall_dns_label must be 3-63 lowercase letters, digits or hyphens, starting with a letter."
  }
}

variable "gateway_hostname" {
  description = "Public DNS name of the access gateway, for example gateway.tre.example.org. Point it at the firewall's public IP."
  type        = string
  default     = ""
}

variable "gateway_oidc_client_id" {
  description = "Client ID of the access gateway's app registration (redirect URI https://<gateway_hostname>/gateway/callback)."
  type        = string
  default     = ""
}

variable "ui_client_id" {
  description = "Client ID of the UI's single-page app registration (redirect URI https://<gateway_hostname>), pre-authorised for the API's user_impersonation scope."
  type        = string
  default     = ""
}

variable "gateway_source_ranges" {
  description = "Public CIDRs allowed to reach the access gateway on HTTPS. Narrow this to your institution's ranges where possible."
  type        = list(string)
  default     = ["*"]
}

variable "tags" {
  type    = map(string)
  default = {}
}
