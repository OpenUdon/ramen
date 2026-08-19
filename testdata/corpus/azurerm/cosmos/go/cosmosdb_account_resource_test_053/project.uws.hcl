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
          kind = "\"GlobalDocumentDB\""
          location = "\"eastus\""
          properties {
            databaseAccountOfferType = "\"Standard\""
          }
        }
        resourceGroupName = "\"ramen-corpus-rg\""
        updateParameters {
          kind = "\"GlobalDocumentDB\""
          location = "\"eastus\""
        }
      }
      path {
        accountName = "\"acctest-ca-1\""
      }
      x-ramen-terraform {
        attributes {
          consistency_policy {
            consistency_level = "\"Session\""
          }
          default_identity_type = "ramen-corpus"
          geo_location {
            failover_priority = "0"
            location = "\"eastus\""
          }
          identity {
            type = "\"SystemAssigned\""
          }
          kind = "\"GlobalDocumentDB\""
          location = "\"eastus\""
          name = "\"acctest-ca-1\""
          offer_type = "\"Standard\""
          resource_group_name = "\"ramen-corpus-rg\""
        }
        identity_attributes = [
          {
            name = "account_name"
            request_keys = [
              "accountName"
            ]
            required = true
            response_paths = [
              "id",
              "name"
            ]
            terraform_path = "name"
          },
          {
            name = "resource_group_name"
            request_keys = [
              "resourceGroupName"
            ]
            required = true
            terraform_path = "resource_group_name"
          }
        ]
        object {
          address = "azurerm_cosmosdb_account.test"
          kind = "resource"
          name = "test"
          type = "azurerm_cosmosdb_account"
        }
        version = "ramen.terraform.provenance.v1"
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
      api_sources = [
        {
          id = "cosmos"
          kind = "openapi"
          path = "openapi/cosmos.json"
        }
      ]
      metadata {
        action = "create"
        config_dir = "testdata/corpus/azurerm/cosmos/go/cosmosdb_account_resource_test_053/input"
        source = "ramen convert tf"
      }
      redaction {

      }
      resources = [
        {
          address = "azurerm_cosmosdb_account.test"
          attributes = {
            consistency_policy = {
              consistency_level = "\"Session\""
            }
            default_identity_type = "ramen-corpus"
            geo_location = {
              failover_priority = "0"
              location = "\"eastus\""
            }
            identity = {
              type = "\"SystemAssigned\""
            }
            kind = "\"GlobalDocumentDB\""
            location = "\"eastus\""
            name = "\"acctest-ca-1\""
            offer_type = "\"Standard\""
            resource_group_name = "\"ramen-corpus-rg\""
          }
          identity_attributes = [
            {
              name = "account_name"
              path = "name"
              request_keys = [
                "accountName"
              ]
              required = true
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
          kind = "resource"
          lifecycle = {

          }
          metadata = {
            terraform_address = "azurerm_cosmosdb_account.test"
          }
          name = "test"
          operations = {
            create = {
              operation_id = "DatabaseAccounts_CreateOrUpdate"
              purpose = "create"
              source_id = "cosmos"
              source_kind = "openapi"
              source_path = "openapi/cosmos.json"
            }
          }
          redaction = {

          }
          type = "azurerm_cosmosdb_account"
        }
      ]
      version = "ramen.project.v1"
    }
  }