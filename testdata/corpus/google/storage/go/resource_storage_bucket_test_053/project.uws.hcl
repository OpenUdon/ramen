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
      x-ramen-credential-bindings = [
        "google_oauth2"
      ]
      x-ramen-terraform {
        object {
          name = "bucket"
          type = "google_storage_bucket"
          address = "google_storage_bucket.bucket"
          kind = "resource"
        }
        attributes {
          uniform_bucket_level_access = "true"
          encryption "google_managed_encryption_enforcement_config" {
            restriction_mode = "\\\"ramen-corpus\\\""
          }
          force_destroy = "true"
          location = "\\\"US\\\""
          name = "\\\"ramen-corpus\\\""
        }
        identity_attributes = [
          {
            name = "bucket_name"
            terraform_path = "name"
            request_keys = [
              "name"
            ]
            response_paths = [
              "name",
              "id"
            ]
            required = true
          }
        ]
      }
      body {
        location = "\\\"US\\\""
        name = "\\\"ramen-corpus\\\""
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
      metadata {
        action = "create"
        config_dir = "testdata/corpus/google/storage/go/resource_storage_bucket_test_053/input"
        source = "ramen convert tf"
      }
      version = "ramen.project.v1"
      api_sources = [
        {
          path = "google-discovery/storage.json"
          kind = "google-discovery"
          id = "storage"
        }
      ]
      resources = [
        {
          redaction = {

          }
          metadata = {
            terraform_address = "google_storage_bucket.bucket"
          }
          name = "bucket"
          address = "google_storage_bucket.bucket"
          attributes = {
            uniform_bucket_level_access = "true"
            encryption = {
              google_managed_encryption_enforcement_config = {
                restriction_mode = "\\\"ramen-corpus\\\""
              }
            }
            force_destroy = "true"
            location = "\\\"US\\\""
            name = "\\\"ramen-corpus\\\""
          }
          identity_attributes = [
            {
              name = "bucket_name"
              path = "name"
              request_keys = [
                "name"
              ]
              response_paths = [
                "name",
                "id"
              ]
              required = true
            }
          ]
          credential_bindings = [
            "google_oauth2"
          ]
          kind = "resource"
          type = "google_storage_bucket"
          lifecycle = {

          }
          operations = {
            create = {
              source_path = "google-discovery/storage.json"
              operation_id = "storage.buckets.insert"
              credential_bindings = [
                "google_oauth2"
              ]
              purpose = "create"
              source_kind = "google-discovery"
              source_id = "storage"
            }
          }
        }
      ]
      redaction {

      }
    }
  }