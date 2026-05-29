provider "azurerm" {
  features {}
}

resource "azurerm_cosmosdb_account" "test" {
  name                = "acctest-1"
  location            = "eastus"
  resource_group_name = "ramen-corpus-rg"
  kind                = "MongoDB"
  offer_type          = "Standard"

  capabilities {
    name = "EnableMongo"
  }

  consistency_policy {
    consistency_level = "BoundedStaleness"
  }

  geo_location {
    location          = "eastus"
    failover_priority = 0
  }
}
