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
          properties {
            databaseAccountOfferType = "\\\"Standard\\\""
          }
          kind = "\\\"GlobalDocumentDB\\\""
          location = "\\\"eastus\\\""
        }
      }
      x-ramen-terraform {
        attributes {
          kind = "\\\"GlobalDocumentDB\\\""
          location = "\\\"eastus\\\""
          name = "\\\"acctest-ca-1\\\""
          offer_type = "\\\"Standard\\\""
          partition_merge_enabled = "ramen-corpus"
          resource_group_name = "\\\"ramen-corpus-rg\\\""
          consistency_policy {
            max_staleness_prefix = "1"
            consistency_level = "\\\"Session\\\""
            max_interval_in_seconds = "1"
          }
          geo_location {
            failover_priority = "0"
            location = "\\\"eastus\\\""
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
            terraform_path = "name"
            request_keys = [
              "accountName"
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
          name = "test"
          type = "azurerm_cosmosdb_account"
          address = "azurerm_cosmosdb_account.test"
          kind = "resource"
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
      redaction {

      }
      metadata {
        source = "ramen convert tf"
        action = "create"
        config_dir = "testdata/corpus/azurerm/cosmos/go/cosmosdb_account_resource_test_004/input"
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
          operations = {
            create = {
              source_id = "cosmos"
              source_path = "openapi/cosmos.json"
              operation_id = "DatabaseAccounts_CreateOrUpdate"
              purpose = "create"
              source_kind = "openapi"
            }
          }
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
              required = true
              name = "resource_group_name"
              path = "resource_group_name"
              request_keys = [
                "resourceGroupName"
              ]
            }
          ]
          lifecycle = {

          }
          redaction = {

          }
          address = "azurerm_cosmosdb_account.test"
          kind = "resource"
          metadata = {
            terraform_address = "azurerm_cosmosdb_account.test"
          }
          type = "azurerm_cosmosdb_account"
          name = "test"
          attributes = {
            consistency_policy = {
              consistency_level = "\\\"Session\\\""
              max_interval_in_seconds = "1"
              max_staleness_prefix = "1"
            }
            geo_location = {
              failover_priority = "0"
              location = "\\\"eastus\\\""
            }
            kind = "\\\"GlobalDocumentDB\\\""
            location = "\\\"eastus\\\""
            name = "\\\"acctest-ca-1\\\""
            offer_type = "\\\"Standard\\\""
            partition_merge_enabled = "ramen-corpus"
            resource_group_name = "\\\"ramen-corpus-rg\\\""
          }
        }
      ]
    }
  }