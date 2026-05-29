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
      x-ramen-terraform {
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
        attributes {
          force_destroy = "true"
          location = "\\\"US\\\""
          name = "\\\"ramen-corpus\\\""
          requester_pays = "ramen-corpus"
        }
      }
      body {
        location = "\\\"US\\\""
        name = "\\\"ramen-corpus\\\""
      }
      x-ramen-credential-bindings = [
        "google_oauth2"
      ]
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
      api_sources = [
        {
          id = "storage"
          path = "google-discovery/storage.json"
          kind = "google-discovery"
        }
      ]
      resources = [
        {
          redaction = {

          }
          metadata = {
            terraform_address = "google_storage_bucket.bucket"
          }
          lifecycle = {

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
          address = "google_storage_bucket.bucket"
          type = "google_storage_bucket"
          kind = "resource"
          attributes = {
            name = "\\\"ramen-corpus\\\""
            requester_pays = "ramen-corpus"
            force_destroy = "true"
            location = "\\\"US\\\""
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
          name = "bucket"
        }
      ]
      redaction {

      }
      metadata {
        source = "ramen convert tf"
        action = "create"
        config_dir = "testdata/corpus/google/storage/go/resource_storage_bucket_test_006/input"
      }
      version = "ramen.project.v1"
    }
  }