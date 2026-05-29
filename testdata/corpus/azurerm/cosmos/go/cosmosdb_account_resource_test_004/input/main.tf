provider "azurerm" {
  features {}
}

resource "azurerm_cosmosdb_account" "test" {
  name                    = "acctest-ca-1"
  location                = "eastus"
  resource_group_name     = "ramen-corpus-rg"
  offer_type              = "Standard"
  kind = "GlobalDocumentDB"
  partition_merge_enabled = ramen-corpus

  consistency_policy {
    consistency_level = "Session"
    max_interval_in_seconds = 1
    max_staleness_prefix    = 1
  }

  geo_location {
    location          = "eastus"
    failover_priority = 0
  }
}
