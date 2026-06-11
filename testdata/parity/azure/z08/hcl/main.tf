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
  default = "ramen-parity-z08-static"
}

variable "location" {
  type    = string
  default = "eastus"
}

resource "azurerm_resource_group" "z08_group" {
  name     = var.resource_group_name
  location = var.location
}
