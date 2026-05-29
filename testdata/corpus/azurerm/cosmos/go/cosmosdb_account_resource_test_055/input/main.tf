provider "azurerm" {
  features {}
}

resource "azurerm_cosmosdb_account" "test" {
  name                = "acctest-ca-1"
  location            = "eastus"
  resource_group_name = "ramen-corpus-rg"
  offer_type          = "Standard"
  kind = "GlobalDocumentDB"
  create_mode         = "Default"

  consistency_policy {
    consistency_level = "Session"
  }

  backup {
    type = "Continuous"
  }

  geo_location {
    location          = "eastus"
    failover_priority = 0
  }
}
