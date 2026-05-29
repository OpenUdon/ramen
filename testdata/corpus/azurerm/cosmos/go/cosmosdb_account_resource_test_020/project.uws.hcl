  uws = "1.4.0"
  info {
    title       = "ramen_native_project"
    description = "Native UWS/Ramen desired-state project generated from static Terraform/OpenTofu configuration."
    version     = "1.0.0"
  }
  sourceDescription "cosmos" {
    url  = "openapi/cosmos.json"
    type = "openapi"
  }
  operation "azurerm_cosmosdb_account_test_create" {
    sourceDescription = "cosmos"
    sourceOperationId = "DatabaseAccounts_CreateOrUpdate"
    description       = "Review create create for Terraform resource azurerm_cosmosdb_account.test"
    request {
      body {
        accountName = "\\\"acctest-ca-1\\\""
        createUpdateParameters {
          kind = "\\\"MongoDB\\\""
          location = "\\\"eastus\\\""
          properties {
            databaseAccountOfferType = "\\\"Standard\\\""
          }
        }
        resourceGroupName = "\\\"ramen-corpus-rg\\\""
        updateParameters {
          kind = "\\\"MongoDB\\\""
          location = "\\\"eastus\\\""
        }
      }
      x-ramen-terraform {
        attributes {
          location = "\\\"eastus\\\""
          name = "\\\"acctest-ca-1\\\""
          resource_group_name = "\\\"ramen-corpus-rg\\\""
          automatic_failover_enabled = "true"
          capabilities {
            name = "\\\"EnableMongo\\\""
          }
          multiple_write_locations_enabled = "true"
          offer_type = "\\\"Standard\\\""
          kind = "\\\"MongoDB\\\""
          dynamic "geo_location" {
            content {
              failover_priority = "geo_location.value.failover_priority"
              location = "geo_location.value.location"
              zone_redundant = "geo_location.value.zone_redundant"
            }
            for_each = "var.geo_location"
          }
          consistency_policy {
            consistency_level = "\\\"Session\\\""
          }
        }
        identity_attributes = [
          {
            name = "account_name"
            terraform_path = "name"
            request_keys = [
              "accountName"
            ]
            response_paths = [
              "name",
              "id"
            ]
            required = true
          },
          {
            name = "resource_group_name"
            terraform_path = "resource_group_name"
            request_keys = [
              "resourceGroupName"
            ]
            required = true
          }
        ]
        object {
          type = "azurerm_cosmosdb_account"
          address = "azurerm_cosmosdb_account.test"
          kind = "resource"
          name = "test"
        }
      }
    }
  }
  workflow "main" {
    type        = "sequence"
    description = "Review mapped Terraform/OpenTofu objects as API source operations."
    step "azurerm_cosmosdb_account_test_create" {
      operationRef = "azurerm_cosmosdb_account_test_create"
      body {
        purpose = "create"
        terraform_address = "azurerm_cosmosdb_account.test"
        terraform_type = "azurerm_cosmosdb_account"
        action = "create"
      }
    }
  }
  extensions {
    x-ramen-desired-state {
      metadata {
        action = "create"
        config_dir = "testdata/corpus/azurerm/cosmos/go/cosmosdb_account_resource_test_020/input"
        source = "ramen convert tf"
      }
      version = "ramen.project.v1"
      api_sources = [
        {
          path = "openapi/cosmos.json"
          kind = "openapi"
          id = "cosmos"
        }
      ]
      resources = [
        {
          redaction = {

          }
          lifecycle = {

          }
          metadata = {
            terraform_address = "azurerm_cosmosdb_account.test"
          }
          address = "azurerm_cosmosdb_account.test"
          kind = "resource"
          name = "test"
          attributes = {
            location = "\\\"eastus\\\""
            multiple_write_locations_enabled = "true"
            name = "\\\"acctest-ca-1\\\""
            capabilities = {
              name = "\\\"EnableMongo\\\""
            }
            consistency_policy = {
              consistency_level = "\\\"Session\\\""
            }
            resource_group_name = "\\\"ramen-corpus-rg\\\""
            kind = "\\\"MongoDB\\\""
            offer_type = "\\\"Standard\\\""
            automatic_failover_enabled = "true"
            dynamic = {
              geo_location = {
                content = {
                  zone_redundant = "geo_location.value.zone_redundant"
                  failover_priority = "geo_location.value.failover_priority"
                  location = "geo_location.value.location"
                }
                for_each = "var.geo_location"
              }
            }
          }
          operations = {
            create = {
              source_path = "openapi/cosmos.json"
              operation_id = "DatabaseAccounts_CreateOrUpdate"
              purpose = "create"
              source_kind = "openapi"
              source_id = "cosmos"
            }
          }
          type = "azurerm_cosmosdb_account"
          identity_attributes = [
            {
              request_keys = [
                "accountName"
              ]
              response_paths = [
                "name",
                "id"
              ]
              required = true
              name = "account_name"
              path = "name"
            },
            {
              path = "resource_group_name"
              request_keys = [
                "resourceGroupName"
              ]
              required = true
              name = "resource_group_name"
            }
          ]
        }
      ]
      redaction {

      }
    }
  }