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
  operation "google_storage_bucket_test_create" {
    sourceDescription = "storage"
    sourceOperationId = "storage.buckets.insert"
    description       = "Review create create for Terraform resource google_storage_bucket.test"
    request {
      body {
        location = "\\\"US\\\""
        name = "ramen-corpus"
      }
      query {
        project = "ramen-corpus"
      }
      x-ramen-credential-bindings = [
        "google_oauth2"
      ]
      x-ramen-terraform {
        object {
          address = "google_storage_bucket.test"
          kind = "resource"
          name = "test"
          type = "google_storage_bucket"
        }
        attributes {
          location = "\\\"US\\\""
          name = "ramen-corpus"
          project = "ramen-corpus"
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
    step "google_storage_bucket_test_create" {
      operationRef = "google_storage_bucket_test_create"
      body {
        action = "create"
        purpose = "create"
        terraform_address = "google_storage_bucket.test"
        terraform_type = "google_storage_bucket"
      }
    }
  }
  extensions {
    x-ramen-desired-state {
      redaction {

      }
      metadata {
        source = "ramen convert tf"
        action = "create"
        config_dir = "testdata/corpus/google/storage/go/list_google_storage_bucket_test_001/input"
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
          type = "google_storage_bucket"
          attributes = {
            location = "\\\"US\\\""
            name = "ramen-corpus"
            project = "ramen-corpus"
          }
          redaction = {

          }
          name = "test"
          lifecycle = {

          }
          kind = "resource"
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
          metadata = {
            terraform_address = "google_storage_bucket.test"
          }
          address = "google_storage_bucket.test"
          credential_bindings = [
            "google_oauth2"
          ]
        }
      ]
    }
  }