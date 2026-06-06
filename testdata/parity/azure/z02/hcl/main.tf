terraform {
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 4.0"
    }
  }
}

provider "azurerm" {
  features {}
}

variable "resource_group_name" {
  type    = string
  default = "ramen-parity-z02"
}

variable "account_name" {
  type    = string
  default = "ramen-parity-z02-static"
}

variable "location" {
  type    = string
  default = "eastus"
}

resource "azurerm_cosmosdb_account" "z02_account" {
  name                = var.account_name
  location            = var.location
  resource_group_name = var.resource_group_name
  offer_type          = "Standard"
  kind                = "GlobalDocumentDB"

  consistency_policy {
    consistency_level = "Session"
  }

  geo_location {
    location          = var.location
    failover_priority = 0
  }
}
