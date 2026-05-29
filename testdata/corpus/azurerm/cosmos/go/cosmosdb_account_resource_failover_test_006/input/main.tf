provider "azurerm" {
  features {}
}

resource "azurerm_cosmosdb_account" "test" {
  name                = "acctest-1"
  location            = "eastus"
  resource_group_name = "ramen-corpus-rg"
  offer_type          = "Standard"

  consistency_policy {
    consistency_level = "Strong"
  }

  geo_location {
    location          = "eastus"
    failover_priority = 0
  }
}
