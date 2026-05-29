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
      path {
        account_id = "\\\"023e105f4ecef8ad9ca31a8372d0c353\\\""
      }
      x-ramen-terraform {
        attributes {
          location = "\\\"weur\\\""
          name = "\\\"ramen_corpus\\\""
          storage_class = "\\\"InfrequentAccess\\\""
          account_id = "\\\"023e105f4ecef8ad9ca31a8372d0c353\\\""
          jurisdiction = "\\\"eu\\\""
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
            required = true
            name = "bucket_name"
            terraform_path = "name"
            request_keys = [
              "name",
              "bucket_name"
            ]
            response_paths = [
              "result.name"
            ]
          }
        ]
        object {
          address = "cloudflare_r2_bucket.ramen_corpus"
          kind = "resource"
          name = "ramen_corpus"
          type = "cloudflare_r2_bucket"
        }
      }
      body {
        storageClass = "\\\"InfrequentAccess\\\""
        locationHint = "\\\"weur\\\""
        name = "\\\"ramen_corpus\\\""
      }
      header {
        cf-r2-jurisdiction = "\\\"eu\\\""
      }
    }
  }
  workflow "main" {
    type        = "sequence"
    description = "Review mapped Terraform/OpenTofu objects as API source operations."
    step "cloudflare_r2_bucket_ramen_corpus_create" {
      operationRef = "cloudflare_r2_bucket_ramen_corpus_create"
      body {
        purpose = "create"
        terraform_address = "cloudflare_r2_bucket.ramen_corpus"
        terraform_type = "cloudflare_r2_bucket"
        action = "create"
      }
    }
  }
  extensions {
    x-ramen-desired-state {
      api_sources = [
        {
          kind = "openapi"
          id = "r2_bucket"
          path = "openapi/r2_bucket.json"
        }
      ]
      resources = [
        {
          redaction = {

          }
          address = "cloudflare_r2_bucket.ramen_corpus"
          type = "cloudflare_r2_bucket"
          name = "ramen_corpus"
          metadata = {
            terraform_address = "cloudflare_r2_bucket.ramen_corpus"
          }
          attributes = {
            account_id = "\\\"023e105f4ecef8ad9ca31a8372d0c353\\\""
            jurisdiction = "\\\"eu\\\""
            location = "\\\"weur\\\""
            name = "\\\"ramen_corpus\\\""
            storage_class = "\\\"InfrequentAccess\\\""
          }
          lifecycle = {

          }
          operations = {
            create = {
              source_id = "r2_bucket"
              source_path = "openapi/r2_bucket.json"
              operation_id = "r2-create-bucket"
              purpose = "create"
              source_kind = "openapi"
            }
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
              response_paths = [
                "result.name"
              ]
              required = true
              name = "bucket_name"
              path = "name"
              request_keys = [
                "name",
                "bucket_name"
              ]
            }
          ]
          kind = "resource"
        }
      ]
      redaction {

      }
      metadata {
        source = "ramen convert tf"
        action = "create"
        config_dir = "testdata/corpus/cloudflare/r2_bucket/comprehensive/input"
      }
      version = "ramen.project.v1"
    }
  }