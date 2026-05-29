  uws = "1.4.0"
  info {
    title       = "ramen_native_project"
    description = "Native UWS/Ramen desired-state project generated from static Terraform/OpenTofu configuration."
    version     = "1.0.0"
  }
  sourceDescription "r2_bucket" {
    url  = "openapi/r2_bucket.json"
    type = "openapi"
  }
  operation "cloudflare_r2_bucket_ramen_corpus_create" {
    sourceDescription = "r2_bucket"
    sourceOperationId = "r2-create-bucket"
    description       = "Review create create for Terraform resource cloudflare_r2_bucket.ramen_corpus"
    request {
      x-ramen-terraform {
        attributes {
          name = "\\\"ramen_corpus\\\""
          account_id = "\\\"023e105f4ecef8ad9ca31a8372d0c353\\\""
          location = "\\\"ENAM\\\""
        }
        identity_attributes = [
          {
            terraform_path = "account_id"
            request_keys = [
              "account_id"
            ]
            required = true
            name = "account_id"
          },
          {
            response_paths = [
              "result.name"
            ]
            required = true
            name = "bucket_name"
            terraform_path = "name"
            request_keys = [
              "name",
              "bucket_name"
            ]
          }
        ]
        object {
          type = "cloudflare_r2_bucket"
          address = "cloudflare_r2_bucket.ramen_corpus"
          kind = "resource"
          name = "ramen_corpus"
        }
      }
      body {
        locationHint = "\\\"ENAM\\\""
        name = "\\\"ramen_corpus\\\""
      }
      path {
        account_id = "\\\"023e105f4ecef8ad9ca31a8372d0c353\\\""
      }
    }
  }
  workflow "main" {
    type        = "sequence"
    description = "Review mapped Terraform/OpenTofu objects as API source operations."
    step "cloudflare_r2_bucket_ramen_corpus_create" {
      operationRef = "cloudflare_r2_bucket_ramen_corpus_create"
      body {
        action = "create"
        purpose = "create"
        terraform_address = "cloudflare_r2_bucket.ramen_corpus"
        terraform_type = "cloudflare_r2_bucket"
      }
    }
  }
  extensions {
    x-ramen-desired-state {
      metadata {
        source = "ramen convert tf"
        action = "create"
        config_dir = "testdata/corpus/cloudflare/r2_bucket/location/input"
      }
      version = "ramen.project.v1"
      api_sources = [
        {
          kind = "openapi"
          id = "r2_bucket"
          path = "openapi/r2_bucket.json"
        }
      ]
      resources = [
        {
          type = "cloudflare_r2_bucket"
          name = "ramen_corpus"
          lifecycle = {

          }
          redaction = {

          }
          attributes = {
            account_id = "\\\"023e105f4ecef8ad9ca31a8372d0c353\\\""
            location = "\\\"ENAM\\\""
            name = "\\\"ramen_corpus\\\""
          }
          operations = {
            create = {
              operation_id = "r2-create-bucket"
              purpose = "create"
              source_kind = "openapi"
              source_id = "r2_bucket"
              source_path = "openapi/r2_bucket.json"
            }
          }
          kind = "resource"
          metadata = {
            terraform_address = "cloudflare_r2_bucket.ramen_corpus"
          }
          address = "cloudflare_r2_bucket.ramen_corpus"
          identity_attributes = [
            {
              name = "account_id"
              path = "account_id"
              request_keys = [
                "account_id"
              ]
              required = true
            },
            {
              name = "bucket_name"
              path = "name"
              request_keys = [
                "name",
                "bucket_name"
              ]
              response_paths = [
                "result.name"
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