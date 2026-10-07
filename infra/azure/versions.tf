terraform {
  required_version = ">= 1.9"

  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 5.8"
    }
    dns = {
      source  = "hashicorp/dns"
      version = "~> 3.4"
    }
  }

  # Remote state in Azure Storage. Configure with:
  #   terraform init -backend-config=backend.hcl
  # where backend.hcl sets resource_group_name, storage_account_name, container_name, key
  # and use_azuread_auth = true. See README.md for bootstrapping the state account.
  backend "azurerm" {}
}

provider "azurerm" {
  features {}
  # Use Entra ID for storage data-plane calls; no storage account keys.
  storage_use_azuread = true
}

data "azurerm_client_config" "current" {}
