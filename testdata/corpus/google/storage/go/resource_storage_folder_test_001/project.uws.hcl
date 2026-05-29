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
        location = "\\\"EU\\\""
        name = "\\\"ramen-corpus\\\""
      }
      x-ramen-credential-bindings = [
        "google_oauth2"
      ]
      x-ramen-terraform {
        object {
          address = "google_storage_bucket.bucket"
          kind = "resource"
          name = "bucket"
          type = "google_storage_bucket"
        }
        attributes {
          force_destroy = "ramen-corpus"
          hierarchical_namespace {
            enabled = "ramen-corpus"
          }
          location = "\\\"EU\\\""
          name = "\\\"ramen-corpus\\\""
          uniform_bucket_level_access = "true"
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
          kind = "resource"
          attributes = {
            hierarchical_namespace = {
              enabled = "ramen-corpus"
            }
            location = "\\\"EU\\\""
            name = "\\\"ramen-corpus\\\""
            uniform_bucket_level_access = "true"
            force_destroy = "ramen-corpus"
          }
          lifecycle = {

          }
          credential_bindings = [
            "google_oauth2"
          ]
          type = "google_storage_bucket"
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
          name = "bucket"
          metadata = {
            terraform_address = "google_storage_bucket.bucket"
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
          address = "google_storage_bucket.bucket"
        }
      ]
      redaction {

      }
      metadata {
        config_dir = "testdata/corpus/google/storage/go/resource_storage_folder_test_001/input"
        source = "ramen convert tf"
        action = "create"
      }
    }
  }