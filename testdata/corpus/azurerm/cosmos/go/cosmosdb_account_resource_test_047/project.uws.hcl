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
        object {
          address = "azurerm_cosmosdb_account.test"
          kind = "resource"
          name = "test"
          type = "azurerm_cosmosdb_account"
        }
        attributes {
          kind = "\\\"MongoDB\\\""
          mongo_server_version = "\\\"ramen-corpus\\\""
          name = "\\\"acctest-ca-1\\\""
          resource_group_name = "\\\"ramen-corpus-rg\\\""
          capabilities {
            name = "\\\"EnableMongo\\\""
          }
          location = "\\\"eastus\\\""
          offer_type = "\\\"Standard\\\""
          consistency_policy {
            consistency_level = "\\\"Session\\\""
          }
          geo_location {
            location = "\\\"eastus\\\""
            failover_priority = "0"
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
            required = true
            name = "resource_group_name"
            terraform_path = "resource_group_name"
            request_keys = [
              "resourceGroupName"
            ]
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
      resources = [
        {
          kind = "resource"
          type = "azurerm_cosmosdb_account"
          lifecycle = {

          }
          metadata = {
            terraform_address = "azurerm_cosmosdb_account.test"
          }
          address = "azurerm_cosmosdb_account.test"
          attributes = {
            location = "\\\"eastus\\\""
            mongo_server_version = "\\\"ramen-corpus\\\""
            kind = "\\\"MongoDB\\\""
            consistency_policy = {
              consistency_level = "\\\"Session\\\""
            }
            name = "\\\"acctest-ca-1\\\""
            offer_type = "\\\"Standard\\\""
            resource_group_name = "\\\"ramen-corpus-rg\\\""
            capabilities = {
              name = "\\\"EnableMongo\\\""
            }
            geo_location = {
              failover_priority = "0"
              location = "\\\"eastus\\\""
            }
          }
          operations = {
            create = {
              source_id = "cosmos"
              source_path = "openapi/cosmos.json"
              operation_id = "DatabaseAccounts_CreateOrUpdate"
              purpose = "create"
              source_kind = "openapi"
            }
          }
          redaction = {

          }
          name = "test"
          identity_attributes = [
            {
              response_paths = [
                "name",
                "id"
              ]
              required = true
              name = "account_name"
              path = "name"
              request_keys = [
                "accountName"
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
        }
      ]
      redaction {

      }
      metadata {
        action = "create"
        config_dir = "testdata/corpus/azurerm/cosmos/go/cosmosdb_account_resource_test_047/input"
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
    }
  }