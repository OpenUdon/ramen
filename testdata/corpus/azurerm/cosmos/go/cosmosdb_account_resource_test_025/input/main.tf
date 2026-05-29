provider "azurerm" {
  features {}
}

resource "azurerm_cosmosdb_account" "test" {
  name                = "acctest-ca-1"
  location            = "eastus"
  resource_group_name = "ramen-corpus-rg"
  offer_type          = "Standard"
  kind                = "MongoDB"

  analytical_storage_enabled = true

  consistency_policy {
    consistency_level = "Session"
  }

  capabilities {
    name = "EnableMongo"
  }

  geo_location {
    location          = "eastus"
    failover_priority = 0
  }
}
