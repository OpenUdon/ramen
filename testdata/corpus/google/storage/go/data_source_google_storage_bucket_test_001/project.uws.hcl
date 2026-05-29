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
  operation "data_google_storage_bucket_bar_read" {
    sourceDescription = "storage"
    sourceOperationId = "storage.buckets.get"
    description       = "Review read read for Terraform data_source data.google_storage_bucket.bar"
    request {
      x-ramen-terraform {
        attributes {
          name = "google_storage_bucket.foo.name"
        }
        identity_attributes = [
          {
            name = "bucket_name"
            terraform_path = "name"
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
        object {
          kind = "data_source"
          name = "bar"
          type = "google_storage_bucket"
          address = "data.google_storage_bucket.bar"
        }
      }
      path {
        bucket = "google_storage_bucket.foo.name"
      }
      x-ramen-credential-bindings = [
        "google_oauth2"
      ]
    }
  }
  operation "google_storage_bucket_foo_create" {
    sourceDescription = "storage"
    sourceOperationId = "storage.buckets.insert"
    description       = "Review create create for Terraform resource google_storage_bucket.foo"
    request {
      body {
        location = "\\\"US\\\""
        name = "\\\"ramen-corpus-bucket\\\""
      }
      x-ramen-credential-bindings = [
        "google_oauth2"
      ]
      x-ramen-terraform {
        attributes {
          location = "\\\"US\\\""
          name = "\\\"ramen-corpus-bucket\\\""
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
        object {
          type = "google_storage_bucket"
          address = "google_storage_bucket.foo"
          kind = "resource"
          name = "foo"
        }
      }
    }
  }
  workflow "main" {
    type        = "sequence"
    description = "Review mapped Terraform/OpenTofu objects as API source operations."
    step "data_google_storage_bucket_bar_read" {
      operationRef = "data_google_storage_bucket_bar_read"
      body {
        action = "read"
        purpose = "read"
        terraform_address = "data.google_storage_bucket.bar"
        terraform_type = "google_storage_bucket"
      }
    }
    step "google_storage_bucket_foo_create" {
      operationRef = "google_storage_bucket_foo_create"
      body {
        action = "create"
        purpose = "create"
        terraform_address = "google_storage_bucket.foo"
        terraform_type = "google_storage_bucket"
      }
    }
  }
  extensions {
    x-ramen-desired-state {
      resources = [
        {
          metadata = {
            terraform_address = "data.google_storage_bucket.bar"
          }
          address = "data.google_storage_bucket.bar"
          kind = "data_source"
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
          credential_bindings = [
            "google_oauth2"
          ]
          name = "bar"
          redaction = {

          }
          operations = {
            read = {
              operation_id = "storage.buckets.get"
              credential_bindings = [
                "google_oauth2"
              ]
              purpose = "read"
              source_kind = "google-discovery"
              source_id = "storage"
              source_path = "google-discovery/storage.json"
            }
          }
          type = "google_storage_bucket"
          attributes = {
            name = "google_storage_bucket.foo.name"
          }
          lifecycle = {

          }
          dependencies = [
            "google_storage_bucket.foo"
          ]
        },
        {
          address = "google_storage_bucket.foo"
          type = "google_storage_bucket"
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
          attributes = {
            name = "\\\"ramen-corpus-bucket\\\""
            location = "\\\"US\\\""
          }
          lifecycle = {

          }
          redaction = {

          }
          name = "foo"
          credential_bindings = [
            "google_oauth2"
          ]
          metadata = {
            terraform_address = "google_storage_bucket.foo"
          }
          kind = "resource"
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
        source = "ramen convert tf"
        action = "create"
        config_dir = "testdata/corpus/google/storage/go/data_source_google_storage_bucket_test_001/input"
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