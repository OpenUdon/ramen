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
        }
        resourceGroupName = "\\\"ramen-corpus-rg\\\""
        updateParameters {
          location = "\\\"eastus\\\""
        }
        accountName = "\\\"acctest-1\\\""
      }
      x-ramen-terraform {
        attributes {
          consistency_policy {
            consistency_level = "\\\"Eventual\\\""
          }
          geo_location {
            failover_priority = "0"
            location = "\\\"eastus\\\""
          }
          location = "\\\"eastus\\\""
          name = "\\\"acctest-1\\\""
          offer_type = "\\\"Standard\\\""
          resource_group_name = "\\\"ramen-corpus-rg\\\""
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
        source = "ramen convert tf"
        action = "create"
        config_dir = "testdata/corpus/azurerm/cosmos/go/cosmosdb_account_resource_failover_test_003/input"
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
          name = "test"
          redaction = {

          }
          address = "azurerm_cosmosdb_account.test"
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
              path = "resource_group_name"
              request_keys = [
                "resourceGroupName"
              ]
              required = true
              name = "resource_group_name"
            }
          ]
          attributes = {
            location = "\\\"eastus\\\""
            name = "\\\"acctest-1\\\""
            offer_type = "\\\"Standard\\\""
            resource_group_name = "\\\"ramen-corpus-rg\\\""
            consistency_policy = {
              consistency_level = "\\\"Eventual\\\""
            }
            geo_location = {
              failover_priority = "0"
              location = "\\\"eastus\\\""
            }
          }
          lifecycle = {

          }
          type = "azurerm_cosmosdb_account"
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
        }
      ]
      redaction {

      }
    }
  }