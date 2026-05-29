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
  operation "google_storage_bucket_website_create" {
    sourceDescription = "storage"
    sourceOperationId = "storage.buckets.insert"
    description       = "Review create create for Terraform resource google_storage_bucket.website"
    request {
      x-ramen-credential-bindings = [
        "google_oauth2"
      ]
      x-ramen-terraform {
        attributes {
          storage_class = "\\\"STANDARD\\\""
          force_destroy = "true"
          location = "\\\"US\\\""
          name = "\\\"ramen-corpus.gcp.tfacc.hashicorptest.com\\\""
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
          address = "google_storage_bucket.website"
          kind = "resource"
          name = "website"
          type = "google_storage_bucket"
        }
      }
      body {
        location = "\\\"US\\\""
        name = "\\\"ramen-corpus.gcp.tfacc.hashicorptest.com\\\""
      }
    }
  }
  workflow "main" {
    type        = "sequence"
    description = "Review mapped Terraform/OpenTofu objects as API source operations."
    step "google_storage_bucket_website_create" {
      operationRef = "google_storage_bucket_website_create"
      body {
        action = "create"
        purpose = "create"
        terraform_address = "google_storage_bucket.website"
        terraform_type = "google_storage_bucket"
      }
    }
  }
  extensions {
    x-ramen-desired-state {
      version = "ramen.project.v1"
      api_sources = [
        {
          path = "google-discovery/storage.json"
          kind = "google-discovery"
          id = "storage"
        }
      ]
      resources = [
        {
          kind = "resource"
          attributes = {
            force_destroy = "true"
            location = "\\\"US\\\""
            name = "\\\"ramen-corpus.gcp.tfacc.hashicorptest.com\\\""
            storage_class = "\\\"STANDARD\\\""
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
          redaction = {

          }
          address = "google_storage_bucket.website"
          type = "google_storage_bucket"
          name = "website"
          lifecycle = {

          }
          metadata = {
            terraform_address = "google_storage_bucket.website"
          }
        }
      ]
      redaction {

      }
      metadata {
        source = "ramen convert tf"
        action = "create"
        config_dir = "testdata/corpus/google/storage/go/resource_storage_bucket_test_044/input"
      }
    }
  }