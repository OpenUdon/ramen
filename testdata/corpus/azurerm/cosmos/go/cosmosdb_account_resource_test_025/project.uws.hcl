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
        createUpdateParameters {
          location = "\\\"eastus\\\""
          properties {
            databaseAccountOfferType = "\\\"Standard\\\""
          }
          kind = "\\\"MongoDB\\\""
        }
        resourceGroupName = "\\\"ramen-corpus-rg\\\""
        updateParameters {
          location = "\\\"eastus\\\""
          kind = "\\\"MongoDB\\\""
        }
        accountName = "\\\"acctest-ca-1\\\""
      }
      x-ramen-terraform {
        attributes {
          capabilities {
            name = "\\\"EnableMongo\\\""
          }
          consistency_policy {
            consistency_level = "\\\"Session\\\""
          }
          geo_location {
            failover_priority = "0"
            location = "\\\"eastus\\\""
          }
          kind = "\\\"MongoDB\\\""
          resource_group_name = "\\\"ramen-corpus-rg\\\""
          location = "\\\"eastus\\\""
          offer_type = "\\\"Standard\\\""
          analytical_storage_enabled = "true"
          name = "\\\"acctest-ca-1\\\""
        }
        identity_attributes = [
          {
            terraform_path = "name"
            request_keys = [
              "accountName"
            ]
            response_paths = [
              "name",
              "id"
            ]
            required = true
            name = "account_name"
          },
          {
            required = true
            name = "resource_group_name"
            terraform_path = "resource_group_name"
            request_keys = [
              "resourceGroupName"
            ]
          }
        ]
        object {
          address = "azurerm_cosmosdb_account.test"
          kind = "resource"
          name = "test"
          type = "azurerm_cosmosdb_account"
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
        action = "create"
        purpose = "create"
        terraform_address = "azurerm_cosmosdb_account.test"
        terraform_type = "azurerm_cosmosdb_account"
      }
    }
  }
  extensions {
    x-ramen-desired-state {
      api_sources = [
        {
          kind = "openapi"
          id = "cosmos"
          path = "openapi/cosmos.json"
        }
      ]
      resources = [
        {
          operations = {
            create = {
              source_kind = "openapi"
              source_id = "cosmos"
              source_path = "openapi/cosmos.json"
              operation_id = "DatabaseAccounts_CreateOrUpdate"
              purpose = "create"
            }
          }
          lifecycle = {

          }
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
              request_keys = [
                "resourceGroupName"
              ]
              required = true
              name = "resource_group_name"
              path = "resource_group_name"
            }
          ]
          redaction = {

          }
          address = "azurerm_cosmosdb_account.test"
          kind = "resource"
          name = "test"
          type = "azurerm_cosmosdb_account"
          metadata = {
            terraform_address = "azurerm_cosmosdb_account.test"
          }
          attributes = {
            name = "\\\"acctest-ca-1\\\""
            consistency_policy = {
              consistency_level = "\\\"Session\\\""
            }
            analytical_storage_enabled = "true"
            capabilities = {
              name = "\\\"EnableMongo\\\""
            }
            offer_type = "\\\"Standard\\\""
            resource_group_name = "\\\"ramen-corpus-rg\\\""
            geo_location = {
              failover_priority = "0"
              location = "\\\"eastus\\\""
            }
            kind = "\\\"MongoDB\\\""
            location = "\\\"eastus\\\""
          }
        }
      ]
      redaction {

      }
      metadata {
        source = "ramen convert tf"
        action = "create"
        config_dir = "testdata/corpus/azurerm/cosmos/go/cosmosdb_account_resource_test_025/input"
      }
      version = "ramen.project.v1"
    }
  }