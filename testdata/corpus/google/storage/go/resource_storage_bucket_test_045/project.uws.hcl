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
      body {
        location = "\\\"US\\\""
        name = "\\\"ramen-corpus.gcp.tfacc.hashicorptest.com\\\""
      }
      x-ramen-credential-bindings = [
        "google_oauth2"
      ]
      x-ramen-terraform {
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
          address = "google_storage_bucket.website"
          kind = "resource"
          name = "website"
          type = "google_storage_bucket"
        }
        attributes {
          force_destroy = "true"
          location = "\\\"US\\\""
          name = "\\\"ramen-corpus.gcp.tfacc.hashicorptest.com\\\""
          storage_class = "\\\"STANDARD\\\""
          website {
            main_page_suffix = "\\\"index.html\\\""
          }
        }
      }
    }
  }
  workflow "main" {
    type        = "sequence"
    description = "Review mapped Terraform/OpenTofu objects as API source operations."
    step "google_storage_bucket_website_create" {
      operationRef = "google_storage_bucket_website_create"
      body {
        terraform_address = "google_storage_bucket.website"
        terraform_type = "google_storage_bucket"
        action = "create"
        purpose = "create"
      }
    }
  }
  extensions {
    x-ramen-desired-state {
      redaction {

      }
      metadata {
        action = "create"
        config_dir = "testdata/corpus/google/storage/go/resource_storage_bucket_test_045/input"
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
      resources = [
        {
          kind = "resource"
          type = "google_storage_bucket"
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
          redaction = {

          }
          address = "google_storage_bucket.website"
          attributes = {
            force_destroy = "true"
            location = "\\\"US\\\""
            name = "\\\"ramen-corpus.gcp.tfacc.hashicorptest.com\\\""
            storage_class = "\\\"STANDARD\\\""
            website = {
              main_page_suffix = "\\\"index.html\\\""
            }
          }
          metadata = {
            terraform_address = "google_storage_bucket.website"
          }
          name = "website"
          lifecycle = {

          }
        }
      ]
    }
  }