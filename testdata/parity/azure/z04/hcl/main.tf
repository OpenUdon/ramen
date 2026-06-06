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
  default = "ramen-parity-z04"
}

variable "account_name" {
  type    = string
  default = "ramenparityz04static"
}

variable "location" {
  type    = string
  default = "eastus"
}

resource "azurerm_storage_account" "z04_account" {
  name                     = var.account_name
  resource_group_name      = var.resource_group_name
  location                 = var.location
  account_tier             = "Standard"
  account_replication_type = "LRS"
}
