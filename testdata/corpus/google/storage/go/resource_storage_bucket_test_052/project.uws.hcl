  uws = "1.4.0"
  info {
    title       = "ramen_native_project"
    description = "Native UWS/Ramen desired-state project generated from static Terraform/OpenTofu configuration."
    version     = "1.0.0"
  }
  sourceDescription "storage" {
    url  = "google-discovery/storage.json"
    type = "google-discovery"
  }
  operation "google_storage_bucket_bucket_create" {
    sourceDescription = "storage"
    sourceOperationId = "storage.buckets.insert"
    description       = "Review create create for Terraform resource google_storage_bucket.bucket"
    request {
      body {
        name = "\\\"ramen-corpus\\\""
        location = "\\\"US\\\""
      }
      x-ramen-credential-bindings = [
        "google_oauth2"
      ]
      x-ramen-terraform {
        attributes {
          force_destroy = "true"
          location = "\\\"US\\\""
          name = "\\\"ramen-corpus\\\""
          uniform_bucket_level_access = "true"
          encryption "customer_supplied_encryption_enforcement_config" {
            restriction_mode = "\\\"ramen-corpus\\\""
          }
        }
        identity_attributes = [
          {
            required = true
            name = "bucket_name"
            terraform_path = "name"
            request_keys = [
              "name"
            ]
            response_paths = [
              "name",
              "id"
            ]
          }
        ]
        object {
          address = "google_storage_bucket.bucket"
          kind = "resource"
          name = "bucket"
          type = "google_storage_bucket"
        }
      }
    }
  }
  workflow "main" {
    type        = "sequence"
    description = "Review mapped Terraform/OpenTofu objects as API source operations."
    step "google_storage_bucket_bucket_create" {
      operationRef = "google_storage_bucket_bucket_create"
      body {
        purpose = "create"
        terraform_address = "google_storage_bucket.bucket"
        terraform_type = "google_storage_bucket"
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
        config_dir = "testdata/corpus/google/storage/go/resource_storage_bucket_test_052/input"
      }
      version = "ramen.project.v1"
      api_sources = [
        {
          kind = "google-discovery"
          id = "storage"
          path = "google-discovery/storage.json"
        }
      ]
      resources = [
        {
          attributes = {
            encryption = {
              customer_supplied_encryption_enforcement_config = {
                restriction_mode = "\\\"ramen-corpus\\\""
              }
            }
            force_destroy = "true"
            location = "\\\"US\\\""
            name = "\\\"ramen-corpus\\\""
            uniform_bucket_level_access = "true"
          }
          lifecycle = {

          }
          redaction = {

          }
          operations = {
            create = {
              source_id = "storage"
              source_path = "google-discovery/storage.json"
              operation_id = "storage.buckets.insert"
              credential_bindings = [
                "google_oauth2"
              ]
              purpose = "create"
              source_kind = "google-discovery"
            }
          }
          identity_attributes = [
            {
              request_keys = [
                "name"
              ]
              response_paths = [
                "name",
                "id"
              ]
              required = true
              name = "bucket_name"
              path = "name"
            }
          ]
          credential_bindings = [
            "google_oauth2"
          ]
          metadata = {
            terraform_address = "google_storage_bucket.bucket"
          }
          kind = "resource"
          type = "google_storage_bucket"
          name = "bucket"
          address = "google_storage_bucket.bucket"
        }
      ]
    }
  }