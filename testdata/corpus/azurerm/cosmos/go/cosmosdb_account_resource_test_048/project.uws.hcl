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
        attributes {
          offer_type = "\\\"Standard\\\""
          resource_group_name = "\\\"ramen-corpus-rg\\\""
          consistency_policy {
            consistency_level = "\\\"Session\\\""
          }
          geo_location {
            failover_priority = "0"
            location = "\\\"eastus\\\""
          }
          kind = "\\\"GlobalDocumentDB\\\""
          local_authentication_disabled = "true"
          location = "\\\"eastus\\\""
          name = "\\\"acctest-ca-1\\\""
        }
      }
      body {
        accountName = "\\\"acctest-ca-1\\\""
        createUpdateParameters {
          properties {
            databaseAccountOfferType = "\\\"Standard\\\""
          }
          kind = "\\\"GlobalDocumentDB\\\""
          location = "\\\"eastus\\\""
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
        terraform_type = "azurerm_cosmosdb_account"
        action = "create"
        purpose = "create"
        terraform_address = "azurerm_cosmosdb_account.test"
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
          attributes = {
            offer_type = "\\\"Standard\\\""
            resource_group_name = "\\\"ramen-corpus-rg\\\""
            consistency_policy = {
              consistency_level = "\\\"Session\\\""
            }
            geo_location = {
              location = "\\\"eastus\\\""
              failover_priority = "0"
            }
            kind = "\\\"GlobalDocumentDB\\\""
            local_authentication_disabled = "true"
            location = "\\\"eastus\\\""
            name = "\\\"acctest-ca-1\\\""
          }
          lifecycle = {

          }
          redaction = {

          }
          metadata = {
            terraform_address = "azurerm_cosmosdb_account.test"
          }
          type = "azurerm_cosmosdb_account"
          name = "test"
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
              request_keys = [
                "resourceGroupName"
              ]
              required = true
              name = "resource_group_name"
              path = "resource_group_name"
            }
          ]
          address = "azurerm_cosmosdb_account.test"
          operations = {
            create = {
              source_path = "openapi/cosmos.json"
              operation_id = "DatabaseAccounts_CreateOrUpdate"
              purpose = "create"
              source_kind = "openapi"
              source_id = "cosmos"
            }
          }
          kind = "resource"
        }
      ]
      redaction {

      }
      metadata {
        source = "ramen convert tf"
        action = "create"
        config_dir = "testdata/corpus/azurerm/cosmos/go/cosmosdb_account_resource_test_048/input"
      }
    }
  }