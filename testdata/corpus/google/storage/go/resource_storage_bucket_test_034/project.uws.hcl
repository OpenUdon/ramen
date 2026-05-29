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
          labels = "{\\\"my-label\\\":\\\"my-label-value\\\"}"
          location = "\\\"US\\\""
          name = "\\\"ramen-corpus\\\""
          force_destroy = "true"
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
            terraform_path = "name"
          }
        ]
        object {
          name = "bucket"
          type = "google_storage_bucket"
          address = "google_storage_bucket.bucket"
          kind = "resource"
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
        action = "create"
        purpose = "create"
        terraform_address = "google_storage_bucket.bucket"
        terraform_type = "google_storage_bucket"
      }
    }
  }
  extensions {
    x-ramen-desired-state {
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
          redaction = {

          }
          kind = "resource"
          attributes = {
            force_destroy = "true"
            labels = "{\\\"my-label\\\":\\\"my-label-value\\\"}"
            location = "\\\"US\\\""
            name = "\\\"ramen-corpus\\\""
          }
          lifecycle = {

          }
          address = "google_storage_bucket.bucket"
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
          name = "bucket"
          credential_bindings = [
            "google_oauth2"
          ]
          metadata = {
            terraform_address = "google_storage_bucket.bucket"
          }
          type = "google_storage_bucket"
        }
      ]
      redaction {

      }
      metadata {
        source = "ramen convert tf"
        action = "create"
        config_dir = "testdata/corpus/google/storage/go/resource_storage_bucket_test_034/input"
      }
    }
  }