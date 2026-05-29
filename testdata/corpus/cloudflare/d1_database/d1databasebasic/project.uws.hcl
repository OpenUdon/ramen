  uws = "1.4.0"
  info {
    title       = "ramen_native_project"
    description = "Native UWS/Ramen desired-state project generated from static Terraform/OpenTofu configuration."
    version     = "1.0.0"
  }
  sourceDescription "d1_database" {
    url  = "openapi/d1_database.json"
    type = "openapi"
  }
  operation "cloudflare_d1_database_ramen_corpus_create" {
    sourceDescription = "d1_database"
    sourceOperationId = "d1-create-database"
    description       = "Review create create for Terraform resource cloudflare_d1_database.ramen_corpus"
    request {
      path {
        account_id = "\\\"023e105f4ecef8ad9ca31a8372d0c353\\\""
      }
      x-ramen-terraform {
        attributes {
          account_id = "\\\"023e105f4ecef8ad9ca31a8372d0c353\\\""
          name = "\\\"ramen_corpus\\\""
        }
        identity_attributes = [
          {
            name = "account_id"
            terraform_path = "account_id"
            request_keys = [
              "account_id"
            ]
            required = true
          },
          {
            request_keys = [
              "name",
              "database_id"
            ]
            response_paths = [
              "result.name",
              "result.uuid"
            ]
            required = true
            name = "database_name"
            terraform_path = "name"
          }
        ]
        object {
          name = "ramen_corpus"
          type = "cloudflare_d1_database"
          address = "cloudflare_d1_database.ramen_corpus"
          kind = "resource"
        }
      }
      body {
        name = "\\\"ramen_corpus\\\""
      }
    }
  }
  workflow "main" {
    type        = "sequence"
    description = "Review mapped Terraform/OpenTofu objects as API source operations."
    step "cloudflare_d1_database_ramen_corpus_create" {
      operationRef = "cloudflare_d1_database_ramen_corpus_create"
      body {
        terraform_address = "cloudflare_d1_database.ramen_corpus"
        terraform_type = "cloudflare_d1_database"
        action = "create"
        purpose = "create"
      }
    }
  }
  extensions {
    x-ramen-desired-state {
      metadata {
        action = "create"
        config_dir = "testdata/corpus/cloudflare/d1_database/d1databasebasic/input"
        source = "ramen convert tf"
      }
      version = "ramen.project.v1"
      api_sources = [
        {
          kind = "openapi"
          id = "d1_database"
          path = "openapi/d1_database.json"
        }
      ]
      resources = [
        {
          kind = "resource"
          type = "cloudflare_d1_database"
          identity_attributes = [
            {
              request_keys = [
                "account_id"
              ]
              required = true
              name = "account_id"
              path = "account_id"
            },
            {
              path = "name"
              request_keys = [
                "name",
                "database_id"
              ]
              response_paths = [
                "result.name",
                "result.uuid"
              ]
              required = true
              name = "database_name"
            }
          ]
          address = "cloudflare_d1_database.ramen_corpus"
          attributes = {
            account_id = "\\\"023e105f4ecef8ad9ca31a8372d0c353\\\""
            name = "\\\"ramen_corpus\\\""
          }
          metadata = {
            terraform_address = "cloudflare_d1_database.ramen_corpus"
          }
          name = "ramen_corpus"
          lifecycle = {

          }
          redaction = {

          }
          operations = {
            create = {
              source_kind = "openapi"
              source_id = "d1_database"
              source_path = "openapi/d1_database.json"
              operation_id = "d1-create-database"
              purpose = "create"
            }
          }
        }
      ]
      redaction {

      }
    }
  }