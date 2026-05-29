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
          location = "\\\"eastus\\\""
          kind = "\\\"GlobalDocumentDB\\\""
        }
        accountName = "\\\"acctest-ca-1\\\""
        createUpdateParameters {
          kind = "\\\"GlobalDocumentDB\\\""
          location = "\\\"eastus\\\""
          properties {
            databaseAccountOfferType = "\\\"Standard\\\""
          }
        }
      }
      x-ramen-terraform {
        attributes {
          kind = "\\\"GlobalDocumentDB\\\""
          offer_type = "\\\"Standard\\\""
          analytical_storage_enabled = "false"
          capacity {
            total_throughput_limit = "1"
          }
          geo_location {
            failover_priority = "0"
            location = "\\\"eastus\\\""
          }
          consistency_policy {
            consistency_level = "\\\"Session\\\""
          }
          resource_group_name = "\\\"ramen-corpus-rg\\\""
          location = "\\\"eastus\\\""
          name = "\\\"acctest-ca-1\\\""
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
            terraform_path = "resource_group_name"
            request_keys = [
              "resourceGroupName"
            ]
            required = true
            name = "resource_group_name"
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
        terraform_type = "azurerm_cosmosdb_account"
        action = "create"
        purpose = "create"
        terraform_address = "azurerm_cosmosdb_account.test"
      }
    }
  }
  extensions {
    x-ramen-desired-state {
      metadata {
        action = "create"
        config_dir = "testdata/corpus/azurerm/cosmos/go/cosmosdb_account_resource_test_051/input"
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
          address = "azurerm_cosmosdb_account.test"
          kind = "resource"
          attributes = {
            offer_type = "\\\"Standard\\\""
            analytical_storage_enabled = "false"
            consistency_policy = {
              consistency_level = "\\\"Session\\\""
            }
            resource_group_name = "\\\"ramen-corpus-rg\\\""
            kind = "\\\"GlobalDocumentDB\\\""
            capacity = {
              total_throughput_limit = "1"
            }
            geo_location = {
              failover_priority = "0"
              location = "\\\"eastus\\\""
            }
            location = "\\\"eastus\\\""
            name = "\\\"acctest-ca-1\\\""
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
          redaction = {

          }
          name = "test"
          metadata = {
            terraform_address = "azurerm_cosmosdb_account.test"
          }
          type = "azurerm_cosmosdb_account"
          lifecycle = {

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
    }
  }