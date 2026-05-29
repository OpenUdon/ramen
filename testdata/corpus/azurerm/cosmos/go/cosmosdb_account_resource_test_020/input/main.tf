variable "geo_location" {
  type = list(object({
    location          = string
    failover_priority = string
    zone_redundant    = bool
  }))
  default = [
    {
      location          = "ramen-corpus"
      failover_priority = 0
      zone_redundant    = false
    },
    {
      location          = "ramen-corpus"
      failover_priority = 1
      zone_redundant    = true
    }
  ]
}

provider "azurerm" {
  features {}
}

resource "azurerm_cosmosdb_account" "test" {
  name                = "acctest-ca-1"
  location            = "eastus"
  resource_group_name = "ramen-corpus-rg"
  offer_type          = "Standard"
  kind                = "MongoDB"

  capabilities {
    name = "EnableMongo"
  }

  multiple_write_locations_enabled = true
  automatic_failover_enabled       = true

  consistency_policy {
    consistency_level = "Session"
  }

  dynamic "geo_location" {
    for_each = var.geo_location
    content {
      location          = geo_location.value.location
      failover_priority = geo_location.value.failover_priority
      zone_redundant    = geo_location.value.zone_redundant
    }
  }
}
