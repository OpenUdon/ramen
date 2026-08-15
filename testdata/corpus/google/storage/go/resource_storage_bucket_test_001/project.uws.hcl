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
          hierarchical_namespace {
            enabled = "ramen-corpus"
          }
          location = "\\\"US\\\""
          name = "\\\"ramen-corpus\\\""
          uniform_bucket_level_access = "true"
        }
        identity_attributes = [
          {
            name = "bucket_name"
            request_keys = [
              "name"
            ]
            required = true
            response_paths = [
              "id",
              "name"
            ]
            terraform_path = "name"
          }
        ]
        object {
          address = "google_storage_bucket.bucket"
          kind = "resource"
          name = "bucket"
          type = "google_storage_bucket"
        }
        version = "ramen.terraform.provenance.v1"
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
      api_sources = [
        {
          id = "storage"
          kind = "google-discovery"
          path = "google-discovery/storage.json"
        }
      ]
      metadata {
        action = "create"
        config_dir = "testdata/corpus/google/storage/go/resource_storage_bucket_test_001/input"
        source = "ramen convert tf"
      }
      redaction {

      }
      resources = [
        {
          address = "google_storage_bucket.bucket"
          attributes = {
            hierarchical_namespace = {
              enabled = "ramen-corpus"
            }
            location = "\\\"US\\\""
            name = "\\\"ramen-corpus\\\""
            uniform_bucket_level_access = "true"
          }
          credential_bindings = [
            "google_oauth2"
          ]
          identity_attributes = [
            {
              name = "bucket_name"
              path = "name"
              request_keys = [
                "name"
              ]
              required = true
              response_paths = [
                "name",
                "id"
              ]
            }
          ]
          kind = "resource"
          lifecycle = {

          }
          metadata = {
            terraform_address = "google_storage_bucket.bucket"
          }
          name = "bucket"
          operations = {
            create = {
              credential_bindings = [
                "google_oauth2"
              ]
              operation_id = "storage.buckets.insert"
              purpose = "create"
              source_id = "storage"
              source_kind = "google-discovery"
              source_path = "google-discovery/storage.json"
            }
          }
          redaction = {

          }
          type = "google_storage_bucket"
        }
      ]
      version = "ramen.project.v1"
    }
  }