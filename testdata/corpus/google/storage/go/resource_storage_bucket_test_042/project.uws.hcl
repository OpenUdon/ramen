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
        attributes {
          name = "\\\"ramen-corpus\\\""
          soft_delete_policy {
            retention_duration_seconds = "1"
          }
          force_destroy = "true"
          location = "\\\"US\\\""
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
        config_dir = "testdata/corpus/google/storage/go/resource_storage_bucket_test_042/input"
        source = "ramen convert tf"
        action = "create"
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
          name = "bucket"
          lifecycle = {

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
          attributes = {
            soft_delete_policy = {
              retention_duration_seconds = "1"
            }
            force_destroy = "true"
            location = "\\\"US\\\""
            name = "\\\"ramen-corpus\\\""
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
          kind = "resource"
          credential_bindings = [
            "google_oauth2"
          ]
          redaction = {

          }
          metadata = {
            terraform_address = "google_storage_bucket.bucket"
          }
          type = "google_storage_bucket"
          address = "google_storage_bucket.bucket"
        }
      ]
    }
  }