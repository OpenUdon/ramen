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
          name = "\\\"acctest-1\\\""
          offer_type = "\\\"Standard\\\""
          resource_group_name = "\\\"ramen-corpus-rg\\\""
          consistency_policy {
            consistency_level = "\\\"BoundedStaleness\\\""
          }
          geo_location {
            failover_priority = "0"
            location = "\\\"eastus\\\""
          }
          location = "\\\"eastus\\\""
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
            request_keys = [
              "resourceGroupName"
            ]
            required = true
            name = "resource_group_name"
            terraform_path = "resource_group_name"
          }
        ]
        object {
          kind = "resource"
          name = "test"
          type = "azurerm_cosmosdb_account"
          address = "azurerm_cosmosdb_account.test"
        }
      }
      body {
        accountName = "\\\"acctest-1\\\""
        createUpdateParameters {
          properties {
            databaseAccountOfferType = "\\\"Standard\\\""
          }
          location = "\\\"eastus\\\""
        }
        resourceGroupName = "\\\"ramen-corpus-rg\\\""
        updateParameters {
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
        terraform_type = "azurerm_cosmosdb_account"
        action = "create"
        purpose = "create"
        terraform_address = "azurerm_cosmosdb_account.test"
      }
    }
  }
  extensions {
    x-ramen-desired-state {
      resources = [
        {
          operations = {
            create = {
              purpose = "create"
              source_kind = "openapi"
              source_id = "cosmos"
              source_path = "openapi/cosmos.json"
              operation_id = "DatabaseAccounts_CreateOrUpdate"
            }
          }
          type = "azurerm_cosmosdb_account"
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
          metadata = {
            terraform_address = "azurerm_cosmosdb_account.test"
          }
          attributes = {
            resource_group_name = "\\\"ramen-corpus-rg\\\""
            consistency_policy = {
              consistency_level = "\\\"BoundedStaleness\\\""
            }
            geo_location = {
              failover_priority = "0"
              location = "\\\"eastus\\\""
            }
            location = "\\\"eastus\\\""
            name = "\\\"acctest-1\\\""
            offer_type = "\\\"Standard\\\""
          }
          redaction = {

          }
          address = "azurerm_cosmosdb_account.test"
          kind = "resource"
          name = "test"
          lifecycle = {

          }
        }
      ]
      redaction {

      }
      metadata {
        action = "create"
        config_dir = "testdata/corpus/azurerm/cosmos/go/cosmosdb_account_resource_failover_test_001/input"
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
    }
  }