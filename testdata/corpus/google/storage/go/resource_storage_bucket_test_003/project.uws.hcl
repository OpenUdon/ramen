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
        location = "\\\"US\\\""
        name = "\\\"ramen-corpus\\\""
      }
      x-ramen-credential-bindings = [
        "google_oauth2"
      ]
      x-ramen-terraform {
        attributes {
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
        terraform_type = "google_storage_bucket"
        action = "create"
        purpose = "create"
        terraform_address = "google_storage_bucket.bucket"
      }
    }
  }
  extensions {
    x-ramen-desired-state {
      api_sources = [
        {
          path = "google-discovery/storage.json"
          kind = "google-discovery"
          id = "storage"
        }
      ]
      resources = [
        {
          attributes = {
            location = "\\\"US\\\""
            name = "\\\"ramen-corpus\\\""
          }
          lifecycle = {

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
          credential_bindings = [
            "google_oauth2"
          ]
          kind = "resource"
          name = "bucket"
          redaction = {

          }
          metadata = {
            terraform_address = "google_storage_bucket.bucket"
          }
          address = "google_storage_bucket.bucket"
          type = "google_storage_bucket"
          identity_attributes = [
            {
              response_paths = [
                "name",
                "id"
              ]
              required = true
              name = "bucket_name"
              path = "name"
              request_keys = [
                "name"
              ]
            }
          ]
        }
      ]
      redaction {

      }
      metadata {
        action = "create"
        config_dir = "testdata/corpus/google/storage/go/resource_storage_bucket_test_003/input"
        source = "ramen convert tf"
      }
      version = "ramen.project.v1"
    }
  }