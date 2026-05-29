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
          kind = "\\\"GlobalDocumentDB\\\""
        }
        resourceGroupName = "\\\"ramen-corpus-rg\\\""
        updateParameters {
          kind = "\\\"GlobalDocumentDB\\\""
          location = "\\\"eastus\\\""
        }
        accountName = "\\\"acctest-ca-1\\\""
      }
      x-ramen-terraform {
        attributes {
          location = "\\\"eastus\\\""
          name = "\\\"acctest-ca-1\\\""
          offer_type = "\\\"Standard\\\""
          resource_group_name = "\\\"ramen-corpus-rg\\\""
          backup {
            interval_in_minutes = "120"
            retention_in_hours = "10"
            type = "\\\"Periodic\\\""
          }
          consistency_policy {
            consistency_level = "\\\"Session\\\""
          }
          geo_location {
            failover_priority = "0"
            location = "\\\"eastus\\\""
          }
          kind = "\\\"GlobalDocumentDB\\\""
        }
        identity_attributes = [
          {
            response_paths = [
              "name",
              "id"
            ]
            required = true
            name = "account_name"
            terraform_path = "name"
            request_keys = [
              "accountName"
            ]
          },
          {
            request_keys = [
              "resourceGroupName"
            ]
            required = true
            name = "resource_group_name"
            terraform_path = "resource_group_name"
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
      version = "ramen.project.v1"
      api_sources = [
        {
          id = "cosmos"
          path = "openapi/cosmos.json"
          kind = "openapi"
        }
      ]
      resources = [
        {
          name = "test"
          type = "azurerm_cosmosdb_account"
          redaction = {

          }
          metadata = {
            terraform_address = "azurerm_cosmosdb_account.test"
          }
          kind = "resource"
          attributes = {
            resource_group_name = "\\\"ramen-corpus-rg\\\""
            backup = {
              type = "\\\"Periodic\\\""
              interval_in_minutes = "120"
              retention_in_hours = "10"
            }
            consistency_policy = {
              consistency_level = "\\\"Session\\\""
            }
            geo_location = {
              failover_priority = "0"
              location = "\\\"eastus\\\""
            }
            kind = "\\\"GlobalDocumentDB\\\""
            location = "\\\"eastus\\\""
            name = "\\\"acctest-ca-1\\\""
            offer_type = "\\\"Standard\\\""
          }
          operations = {
            create = {
              source_kind = "openapi"
              source_id = "cosmos"
              source_path = "openapi/cosmos.json"
              operation_id = "DatabaseAccounts_CreateOrUpdate"
              purpose = "create"
            }
          }
          address = "azurerm_cosmosdb_account.test"
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
              required = true
              name = "resource_group_name"
              path = "resource_group_name"
              request_keys = [
                "resourceGroupName"
              ]
            }
          ]
        }
      ]
      redaction {

      }
      metadata {
        action = "create"
        config_dir = "testdata/corpus/azurerm/cosmos/go/cosmosdb_account_resource_test_036/input"
        source = "ramen convert tf"
      }
    }
  }