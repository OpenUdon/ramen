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
          labels = "{\\\"a-new-label\\\":\\\"a-new-label-value\\\",\\\"my-label\\\":\\\"my-updated-label-value\\\"}"
          location = "\\\"US\\\""
          name = "\\\"ramen-corpus\\\""
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
        action = "create"
        purpose = "create"
        terraform_address = "google_storage_bucket.bucket"
        terraform_type = "google_storage_bucket"
      }
    }
  }
  extensions {
    x-ramen-desired-state {
      resources = [
        {
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
          address = "google_storage_bucket.bucket"
          attributes = {
            force_destroy = "true"
            labels = "{\\\"a-new-label\\\":\\\"a-new-label-value\\\",\\\"my-label\\\":\\\"my-updated-label-value\\\"}"
            location = "\\\"US\\\""
            name = "\\\"ramen-corpus\\\""
          }
          type = "google_storage_bucket"
          name = "bucket"
          credential_bindings = [
            "google_oauth2"
          ]
          kind = "resource"
          lifecycle = {

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
        }
      ]
      redaction {

      }
      metadata {
        config_dir = "testdata/corpus/google/storage/go/resource_storage_bucket_test_038/input"
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
    }
  }