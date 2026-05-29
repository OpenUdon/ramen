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
        resourceGroupName = "\\\"ramen-corpus-rg\\\""
        updateParameters {
          kind = "\\\"MongoDB\\\""
          location = "\\\"eastus\\\""
        }
        accountName = "\\\"acctest-1\\\""
        createUpdateParameters {
          kind = "\\\"MongoDB\\\""
          location = "\\\"eastus\\\""
          properties {
            databaseAccountOfferType = "\\\"Standard\\\""
          }
        }
      }
      x-ramen-terraform {
        object {
          address = "azurerm_cosmosdb_account.test"
          kind = "resource"
          name = "test"
          type = "azurerm_cosmosdb_account"
        }
        attributes {
          location = "\\\"eastus\\\""
          name = "\\\"acctest-1\\\""
          offer_type = "\\\"Standard\\\""
          resource_group_name = "\\\"ramen-corpus-rg\\\""
          capabilities {
            name = "\\\"EnableMongo\\\""
          }
          consistency_policy {
            consistency_level = "\\\"BoundedStaleness\\\""
          }
          geo_location {
            failover_priority = "0"
            location = "\\\"eastus\\\""
          }
          kind = "\\\"MongoDB\\\""
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
            terraform_path = "resource_group_name"
            request_keys = [
              "resourceGroupName"
            ]
            required = true
            name = "resource_group_name"
          }
        ]
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
          address = "azurerm_cosmosdb_account.test"
          attributes = {
            capabilities = {
              name = "\\\"EnableMongo\\\""
            }
            consistency_policy = {
              consistency_level = "\\\"BoundedStaleness\\\""
            }
            geo_location = {
              failover_priority = "0"
              location = "\\\"eastus\\\""
            }
            kind = "\\\"MongoDB\\\""
            location = "\\\"eastus\\\""
            name = "\\\"acctest-1\\\""
            offer_type = "\\\"Standard\\\""
            resource_group_name = "\\\"ramen-corpus-rg\\\""
          }
          identity_attributes = [
            {
              required = true
              name = "account_name"
              path = "name"
              request_keys = [
                "accountName"
              ]
              response_paths = [
                "name",
                "id"
              ]
            },
            {
              name = "resource_group_name"
              path = "resource_group_name"
              request_keys = [
                "resourceGroupName"
              ]
              required = true
            }
          ]
          type = "azurerm_cosmosdb_account"
          name = "test"
          lifecycle = {

          }
          operations = {
            create = {
              purpose = "create"
              source_kind = "openapi"
              source_id = "cosmos"
              source_path = "openapi/cosmos.json"
              operation_id = "DatabaseAccounts_CreateOrUpdate"
            }
          }
          metadata = {
            terraform_address = "azurerm_cosmosdb_account.test"
          }
          kind = "resource"
          redaction = {

          }
        }
      ]
      redaction {

      }
      metadata {
        source = "ramen convert tf"
        action = "create"
        config_dir = "testdata/corpus/azurerm/cosmos/go/cosmosdb_account_resource_failover_test_005/input"
      }
    }
  }