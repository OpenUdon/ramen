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
        attributes {
          force_destroy = "true"
          location = "\\\"us-central1\\\""
          name = "\\\"ramen-corpus\\\""
          uniform_bucket_level_access = "true"
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
          kind = "resource"
          name = "bucket"
          type = "google_storage_bucket"
          address = "google_storage_bucket.bucket"
        }
      }
      body {
        name = "\\\"ramen-corpus\\\""
        location = "\\\"us-central1\\\""
      }
    }
  }
  workflow "main" {
    type        = "sequence"
    description = "Review mapped Terraform/OpenTofu objects as API source operations."
    step "google_storage_bucket_bucket_create" {
      operationRef = "google_storage_bucket_bucket_create"
      body {
        terraform_address = "google_storage_bucket.bucket"
        terraform_type = "google_storage_bucket"
        action = "create"
        purpose = "create"
      }
    }
  }
  extensions {
    x-ramen-desired-state {
      resources = [
        {
          redaction = {

          }
          identity_attributes = [
            {
              required = true
              name = "bucket_name"
              path = "name"
              request_keys = [
                "name"
              ]
              response_paths = [
                "name",
                "id"
              ]
            }
          ]
          metadata = {
            terraform_address = "google_storage_bucket.bucket"
          }
          kind = "resource"
          name = "bucket"
          attributes = {
            name = "\\\"ramen-corpus\\\""
            uniform_bucket_level_access = "true"
            force_destroy = "true"
            location = "\\\"us-central1\\\""
          }
          credential_bindings = [
            "google_oauth2"
          ]
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
          address = "google_storage_bucket.bucket"
          type = "google_storage_bucket"
          lifecycle = {

          }
        }
      ]
      redaction {

      }
      metadata {
        action = "create"
        config_dir = "testdata/corpus/google/storage/go/resource_storage_bucket_test_051/input"
        source = "ramen convert tf"
      }
      version = "ramen.project.v1"
      api_sources = [
        {
          id = "storage"
          path = "google-discovery/storage.json"
          kind = "google-discovery"
        }
      ]
    }
  }