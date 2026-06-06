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

variable "subscription_id" {
  type    = string
  default = "00000000-0000-0000-0000-000000000000"
}

variable "resource_group_name" {
  type    = string
  default = "ramen-parity-z01"
}

variable "server_name" {
  type    = string
  default = "ramen-parity-z01"
}

variable "database_name" {
  type    = string
  default = "ramen-parity-z01-static"
}

resource "azurerm_mssql_database" "z01_sql_database" {
  name      = var.database_name
  server_id = "/subscriptions/${var.subscription_id}/resourceGroups/${var.resource_group_name}/providers/Microsoft.Sql/servers/${var.server_name}"
  sku_name  = "Basic"
}
