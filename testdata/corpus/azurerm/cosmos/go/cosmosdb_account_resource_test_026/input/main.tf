provider "azurerm" {
  features {}
}

resource "azurerm_cosmosdb_account" "test" {
  name                          = "acctest-ca-1"
  location                      = "eastus"
  resource_group_name           = "ramen-corpus-rg"
  offer_type                    = "Standard"
  kind = "GlobalDocumentDB"
  public_network_access_enabled = true

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
