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
          force_destroy = "\\\"true\\\""
          lifecycle_rule "action" {
            type = "\\\"Delete\\\""
          }
          lifecycle_rule "condition" {
            age = "0"
            send_age_if_zero = "true"
          }
          location = "\\\"EU\\\""
          name = "\\\"ramen-corpus\\\""
        }
        identity_attributes = [
          {
            terraform_path = "name"
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
        object {
          type = "google_storage_bucket"
          address = "google_storage_bucket.bucket"
          kind = "resource"
          name = "bucket"
        }
      }
      body {
        location = "\\\"EU\\\""
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
        source = "ramen convert tf"
        action = "create"
        config_dir = "testdata/corpus/google/storage/go/resource_storage_bucket_test_013/input"
      }
      version = "ramen.project.v1"
      api_sources = [
        {
          id = "storage"
          path = "google-discovery/storage.json"
          kind = "google-discovery"
        }
      ]
      resources = [
        {
          address = "google_storage_bucket.bucket"
          lifecycle = {

          }
          credential_bindings = [
            "google_oauth2"
          ]
          kind = "resource"
          attributes = {
            name = "\\\"ramen-corpus\\\""
            force_destroy = "\\\"true\\\""
            lifecycle_rule = {
              action = {
                type = "\\\"Delete\\\""
              }
              condition = {
                send_age_if_zero = "true"
                age = "0"
              }
            }
            location = "\\\"EU\\\""
          }
          redaction = {

          }
          metadata = {
            terraform_address = "google_storage_bucket.bucket"
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
          type = "google_storage_bucket"
          name = "bucket"
          operations = {
            create = {
              operation_id = "storage.buckets.insert"
              credential_bindings = [
                "google_oauth2"
              ]
              purpose = "create"
              source_kind = "google-discovery"
              source_id = "storage"
              source_path = "google-discovery/storage.json"
            }
          }
        }
      ]
      redaction {

      }
    }
  }