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
          name = "\\\"ramen-corpus\\\""
          force_destroy = "true"
          location = "\\\"US\\\""
          logging {
            log_bucket = "\\\"ramen-corpus\\\""
            log_object_prefix = "\\\"ramen-corpus\\\""
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
        terraform_address = "google_storage_bucket.bucket"
        terraform_type = "google_storage_bucket"
        action = "create"
        purpose = "create"
      }
    }
  }
  extensions {
    x-ramen-desired-state {
      metadata {
        config_dir = "testdata/corpus/google/storage/go/resource_storage_bucket_test_027/input"
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
          lifecycle = {

          }
          identity_attributes = [
            {
              path = "name"
              request_keys = [
                "name"
              ]
              response_paths = [
                "name",
                "id"
              ]
              required = true
              name = "bucket_name"
            }
          ]
          redaction = {

          }
          kind = "resource"
          type = "google_storage_bucket"
          name = "bucket"
          address = "google_storage_bucket.bucket"
          attributes = {
            force_destroy = "true"
            location = "\\\"US\\\""
            logging = {
              log_object_prefix = "\\\"ramen-corpus\\\""
              log_bucket = "\\\"ramen-corpus\\\""
            }
            name = "\\\"ramen-corpus\\\""
          }
          operations = {
            create = {
              source_kind = "google-discovery"
              source_id = "storage"
              source_path = "google-discovery/storage.json"
              operation_id = "storage.buckets.insert"
              credential_bindings = [
                "google_oauth2"
              ]
              purpose = "create"
            }
          }
          credential_bindings = [
            "google_oauth2"
          ]
          metadata = {
            terraform_address = "google_storage_bucket.bucket"
          }
        }
      ]
      redaction {

      }
    }
  }