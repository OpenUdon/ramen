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
      x-ramen-terraform {
        attributes {
          create_mode = "\\\"Default\\\""
          offer_type = "\\\"Standard\\\""
          consistency_policy {
            consistency_level = "\\\"Session\\\""
          }
          kind = "\\\"GlobalDocumentDB\\\""
          name = "\\\"acctest-ca-1\\\""
          resource_group_name = "\\\"ramen-corpus-rg\\\""
          backup {
            type = "\\\"Continuous\\\""
          }
          geo_location {
            failover_priority = "0"
            location = "\\\"eastus\\\""
          }
          location = "\\\"eastus\\\""
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
            terraform_path = "resource_group_name"
            request_keys = [
              "resourceGroupName"
            ]
            required = true
            name = "resource_group_name"
          }
        ]
        object {
          name = "test"
          type = "azurerm_cosmosdb_account"
          address = "azurerm_cosmosdb_account.test"
          kind = "resource"
        }
      }
      body {
        accountName = "\\\"acctest-ca-1\\\""
        createUpdateParameters {
          location = "\\\"eastus\\\""
          properties {
            databaseAccountOfferType = "\\\"Standard\\\""
          }
          kind = "\\\"GlobalDocumentDB\\\""
        }
        resourceGroupName = "\\\"ramen-corpus-rg\\\""
        updateParameters {
          kind = "\\\"GlobalDocumentDB\\\""
          location = "\\\"eastus\\\""
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
        terraform_address = "azurerm_cosmosdb_account.test"
        terraform_type = "azurerm_cosmosdb_account"
        action = "create"
        purpose = "create"
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
          type = "azurerm_cosmosdb_account"
          operations = {
            create = {
              source_path = "openapi/cosmos.json"
              operation_id = "DatabaseAccounts_CreateOrUpdate"
              purpose = "create"
              source_kind = "openapi"
              source_id = "cosmos"
            }
          }
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
              path = "resource_group_name"
              request_keys = [
                "resourceGroupName"
              ]
              required = true
              name = "resource_group_name"
            }
          ]
          redaction = {

          }
          address = "azurerm_cosmosdb_account.test"
          kind = "resource"
          attributes = {
            consistency_policy = {
              consistency_level = "\\\"Session\\\""
            }
            resource_group_name = "\\\"ramen-corpus-rg\\\""
            geo_location = {
              failover_priority = "0"
              location = "\\\"eastus\\\""
            }
            offer_type = "\\\"Standard\\\""
            backup = {
              type = "\\\"Continuous\\\""
            }
            create_mode = "\\\"Default\\\""
            location = "\\\"eastus\\\""
            name = "\\\"acctest-ca-1\\\""
            kind = "\\\"GlobalDocumentDB\\\""
          }
          name = "test"
          lifecycle = {

          }
          metadata = {
            terraform_address = "azurerm_cosmosdb_account.test"
          }
        }
      ]
      redaction {

      }
      metadata {
        config_dir = "testdata/corpus/azurerm/cosmos/go/cosmosdb_account_resource_test_055/input"
        source = "ramen convert tf"
        action = "create"
      }
      version = "ramen.project.v1"
    }
  }