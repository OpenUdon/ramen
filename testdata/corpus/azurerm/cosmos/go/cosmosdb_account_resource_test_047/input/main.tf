provider "azurerm" {
  features {}
}

resource "azurerm_cosmosdb_account" "test" {
  name                 = "acctest-ca-1"
  location             = "eastus"
  resource_group_name  = "ramen-corpus-rg"
  offer_type           = "Standard"
  kind                 = "MongoDB"
  mongo_server_version = "ramen-corpus"

  capabilities {
    name = "EnableMongo"
  }

  consistency_policy {
    consistency_level = "Session"
  }

  geo_location {
    location          = "eastus"
    failover_priority = 0
  }
}
