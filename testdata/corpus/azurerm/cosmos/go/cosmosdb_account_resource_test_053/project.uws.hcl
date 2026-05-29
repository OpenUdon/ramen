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
          kind = "\\\"GlobalDocumentDB\\\""
          location = "\\\"eastus\\\""
          properties {
            databaseAccountOfferType = "\\\"Standard\\\""
          }
        }
        resourceGroupName = "\\\"ramen-corpus-rg\\\""
        updateParameters {
          kind = "\\\"GlobalDocumentDB\\\""
          location = "\\\"eastus\\\""
        }
      }
      x-ramen-terraform {
        attributes {
          identity {
            type = "\\\"SystemAssigned\\\""
          }
          kind = "\\\"GlobalDocumentDB\\\""
          offer_type = "\\\"Standard\\\""
          name = "\\\"acctest-ca-1\\\""
          resource_group_name = "\\\"ramen-corpus-rg\\\""
          consistency_policy {
            consistency_level = "\\\"Session\\\""
          }
          geo_location {
            failover_priority = "0"
            location = "\\\"eastus\\\""
          }
          location = "\\\"eastus\\\""
          default_identity_type = "ramen-corpus"
        }
        identity_attributes = [
          {
            required = true
            name = "account_name"
            terraform_path = "name"
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
            terraform_path = "resource_group_name"
            request_keys = [
              "resourceGroupName"
            ]
            required = true
          }
        ]
        object {
          kind = "resource"
          name = "test"
          type = "azurerm_cosmosdb_account"
          address = "azurerm_cosmosdb_account.test"
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
      metadata {
        action = "create"
        config_dir = "testdata/corpus/azurerm/cosmos/go/cosmosdb_account_resource_test_053/input"
        source = "ramen convert tf"
      }
      version = "ramen.project.v1"
      api_sources = [
        {
          kind = "openapi"
          id = "cosmos"
          path = "openapi/cosmos.json"
        }
      ]
      resources = [
        {
          redaction = {

          }
          metadata = {
            terraform_address = "azurerm_cosmosdb_account.test"
          }
          type = "azurerm_cosmosdb_account"
          name = "test"
          lifecycle = {

          }
          address = "azurerm_cosmosdb_account.test"
          identity_attributes = [
            {
              path = "name"
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
              path = "resource_group_name"
              request_keys = [
                "resourceGroupName"
              ]
            }
          ]
          kind = "resource"
          attributes = {
            location = "\\\"eastus\\\""
            offer_type = "\\\"Standard\\\""
            consistency_policy = {
              consistency_level = "\\\"Session\\\""
            }
            default_identity_type = "ramen-corpus"
            geo_location = {
              failover_priority = "0"
              location = "\\\"eastus\\\""
            }
            name = "\\\"acctest-ca-1\\\""
            resource_group_name = "\\\"ramen-corpus-rg\\\""
            identity = {
              type = "\\\"SystemAssigned\\\""
            }
            kind = "\\\"GlobalDocumentDB\\\""
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
        }
      ]
      redaction {

      }
    }
  }