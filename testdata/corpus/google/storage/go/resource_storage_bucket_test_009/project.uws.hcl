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
        name = "\\\"ramen-corpus\\\""
        location = "\\\"ASIA\\\""
      }
      x-ramen-credential-bindings = [
        "google_oauth2"
      ]
      x-ramen-terraform {
        attributes {
          custom_placement_config {
            data_locations = "[\\\"asia-east1\\\",\\\"asia-southeast1\\\"]"
          }
          force_destroy = "true"
          location = "\\\"ASIA\\\""
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
          address = "google_storage_bucket.bucket"
          kind = "resource"
          name = "bucket"
          type = "google_storage_bucket"
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
        action = "create"
        purpose = "create"
        terraform_address = "google_storage_bucket.bucket"
        terraform_type = "google_storage_bucket"
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
          name = "bucket"
          redaction = {

          }
          metadata = {
            terraform_address = "google_storage_bucket.bucket"
          }
          address = "google_storage_bucket.bucket"
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
          credential_bindings = [
            "google_oauth2"
          ]
          type = "google_storage_bucket"
          attributes = {
            custom_placement_config = {
              data_locations = "[\\\"asia-east1\\\",\\\"asia-southeast1\\\"]"
            }
            force_destroy = "true"
            location = "\\\"ASIA\\\""
            name = "\\\"ramen-corpus\\\""
          }
          kind = "resource"
          lifecycle = {

          }
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
        }
      ]
      redaction {

      }
      metadata {
        config_dir = "testdata/corpus/google/storage/go/resource_storage_bucket_test_009/input"
        source = "ramen convert tf"
        action = "create"
      }
    }
  }