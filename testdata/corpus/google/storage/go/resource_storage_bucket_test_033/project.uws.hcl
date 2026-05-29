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
        object {
          kind = "resource"
          name = "bucket"
          type = "google_storage_bucket"
          address = "google_storage_bucket.bucket"
        }
        attributes {
          force_destroy = "true"
          lifecycle_rule "action" {
            type = "\\\"Delete\\\""
          }
          lifecycle_rule "condition" {
            with_state = "\\\"ANY\\\""
            age = "10"
          }
          location = "\\\"US\\\""
          name = "\\\"ramen-corpus\\\""
        }
        identity_attributes = [
          {
            response_paths = [
              "name",
              "id"
            ]
            required = true
            name = "bucket_name"
            terraform_path = "name"
            request_keys = [
              "name"
            ]
          }
        ]
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
          address = "google_storage_bucket.bucket"
          attributes = {
            force_destroy = "true"
            lifecycle_rule = {
              action = {
                type = "\\\"Delete\\\""
              }
              condition = {
                age = "10"
                with_state = "\\\"ANY\\\""
              }
            }
            location = "\\\"US\\\""
            name = "\\\"ramen-corpus\\\""
          }
          redaction = {

          }
          type = "google_storage_bucket"
          name = "bucket"
          operations = {
            create = {
              purpose = "create"
              source_kind = "google-discovery"
              source_id = "storage"
              source_path = "google-discovery/storage.json"
              operation_id = "storage.buckets.insert"
              credential_bindings = [
                "google_oauth2"
              ]
            }
          }
          metadata = {
            terraform_address = "google_storage_bucket.bucket"
          }
          lifecycle = {

          }
          kind = "resource"
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
        }
      ]
      redaction {

      }
      metadata {
        action = "create"
        config_dir = "testdata/corpus/google/storage/go/resource_storage_bucket_test_033/input"
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