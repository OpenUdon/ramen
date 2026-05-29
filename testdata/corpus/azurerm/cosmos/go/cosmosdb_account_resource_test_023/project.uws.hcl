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
          kind = "\\\"GlobalDocumentDB\\\""
          location = "\\\"eastus\\\""
        }
        accountName = "\\\"acctest-ca-1\\\""
        createUpdateParameters {
          properties {
            databaseAccountOfferType = "\\\"Standard\\\""
          }
          kind = "\\\"GlobalDocumentDB\\\""
          location = "\\\"eastus\\\""
        }
      }
      x-ramen-terraform {
        attributes {
          geo_location {
            failover_priority = "0"
            location = "\\\"eastus\\\""
          }
          kind = "\\\"GlobalDocumentDB\\\""
          location = "\\\"eastus\\\""
          name = "\\\"acctest-ca-1\\\""
          offer_type = "\\\"Standard\\\""
          resource_group_name = "\\\"ramen-corpus-rg\\\""
          consistency_policy {
            consistency_level = "\\\"Session\\\""
          }
          free_tier_enabled = "true"
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
            terraform_path = "name"
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
        config_dir = "testdata/corpus/azurerm/cosmos/go/cosmosdb_account_resource_test_023/input"
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
          type = "azurerm_cosmosdb_account"
          attributes = {
            location = "\\\"eastus\\\""
            name = "\\\"acctest-ca-1\\\""
            offer_type = "\\\"Standard\\\""
            resource_group_name = "\\\"ramen-corpus-rg\\\""
            consistency_policy = {
              consistency_level = "\\\"Session\\\""
            }
            free_tier_enabled = "true"
            geo_location = {
              failover_priority = "0"
              location = "\\\"eastus\\\""
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
          metadata = {
            terraform_address = "azurerm_cosmosdb_account.test"
          }
          kind = "resource"
          identity_attributes = [
            {
              name = "account_name"
              path = "name"
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
              path = "resource_group_name"
              request_keys = [
                "resourceGroupName"
              ]
              required = true
            }
          ]
          name = "test"
          lifecycle = {

          }
          redaction = {

          }
          address = "azurerm_cosmosdb_account.test"
        }
      ]
      redaction {

      }
    }
  }