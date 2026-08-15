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
      body {
        locationHint = "\\\"ENAM\\\""
        name = "\\\"ramen_corpus\\\""
        storageClass = "\\\"Standard\\\""
      }
      path {
        account_id = "\\\"023e105f4ecef8ad9ca31a8372d0c353\\\""
      }
      x-ramen-terraform {
        attributes {
          account_id = "\\\"023e105f4ecef8ad9ca31a8372d0c353\\\""
          location = "\\\"ENAM\\\""
          name = "\\\"ramen_corpus\\\""
          storage_class = "\\\"Standard\\\""
        }
        identity_attributes = [
          {
            name = "account_id"
            request_keys = [
              "account_id"
            ]
            required = true
            terraform_path = "account_id"
          },
          {
            name = "bucket_name"
            request_keys = [
              "bucket_name",
              "name"
            ]
            required = true
            response_paths = [
              "result.name"
            ]
            terraform_path = "name"
          }
        ]
        object {
          address = "cloudflare_r2_bucket.ramen_corpus"
          kind = "resource"
          name = "ramen_corpus"
          type = "cloudflare_r2_bucket"
        }
        version = "ramen.terraform.provenance.v1"
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
      api_sources = [
        {
          id = "r2_bucket"
          kind = "openapi"
          path = "openapi/r2_bucket.json"
        }
      ]
      metadata {
        action = "create"
        config_dir = "testdata/corpus/cloudflare/r2_bucket/r2bucketbasic/input"
        source = "ramen convert tf"
      }
      redaction {

      }
      resources = [
        {
          address = "cloudflare_r2_bucket.ramen_corpus"
          attributes = {
            account_id = "\\\"023e105f4ecef8ad9ca31a8372d0c353\\\""
            location = "\\\"ENAM\\\""
            name = "\\\"ramen_corpus\\\""
            storage_class = "\\\"Standard\\\""
          }
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
              required = true
              response_paths = [
                "result.name"
              ]
            }
          ]
          kind = "resource"
          lifecycle = {

          }
          metadata = {
            terraform_address = "cloudflare_r2_bucket.ramen_corpus"
          }
          name = "ramen_corpus"
          operations = {
            create = {
              operation_id = "r2-create-bucket"
              purpose = "create"
              source_id = "r2_bucket"
              source_kind = "openapi"
              source_path = "openapi/r2_bucket.json"
            }
          }
          redaction = {

          }
          type = "cloudflare_r2_bucket"
        }
      ]
      version = "ramen.project.v1"
    }
  }