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
      body {
        project = "\\\"ramen-corpus-project\\\""
      }
      path {
        bucket = "google_storage_bucket.foo.name"
      }
      x-ramen-credential-bindings = [
        "google_oauth2"
      ]
      x-ramen-terraform {
        attributes {
          project = "\\\"ramen-corpus-project\\\""
          name = "google_storage_bucket.foo.name"
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
          address = "data.google_storage_bucket.bar"
          kind = "data_source"
          name = "bar"
          type = "google_storage_bucket"
        }
      }
    }
  }
  operation "google_storage_bucket_foo_create" {
    sourceDescription = "storage"
    sourceOperationId = "storage.buckets.insert"
    description       = "Review create create for Terraform resource google_storage_bucket.foo"
    request {
      x-ramen-terraform {
        object {
          kind = "resource"
          name = "foo"
          type = "google_storage_bucket"
          address = "google_storage_bucket.foo"
        }
        attributes {
          project = "\\\"ramen-corpus-project\\\""
          location = "\\\"US\\\""
          name = "\\\"ramen-corpus-bucket\\\""
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
      }
      body {
        location = "\\\"US\\\""
        name = "\\\"ramen-corpus-bucket\\\""
      }
      query {
        project = "\\\"ramen-corpus-project\\\""
      }
      x-ramen-credential-bindings = [
        "google_oauth2"
      ]
    }
  }
  workflow "main" {
    type        = "sequence"
    description = "Review mapped Terraform/OpenTofu objects as API source operations."
    step "data_google_storage_bucket_bar_read" {
      operationRef = "data_google_storage_bucket_bar_read"
      body {
        purpose = "read"
        terraform_address = "data.google_storage_bucket.bar"
        terraform_type = "google_storage_bucket"
        action = "read"
      }
    }
    step "google_storage_bucket_foo_create" {
      operationRef = "google_storage_bucket_foo_create"
      body {
        purpose = "create"
        terraform_address = "google_storage_bucket.foo"
        terraform_type = "google_storage_bucket"
        action = "create"
      }
    }
  }
  extensions {
    x-ramen-desired-state {
      api_sources = [
        {
          kind = "google-discovery"
          id = "storage"
          path = "google-discovery/storage.json"
        }
      ]
      resources = [
        {
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
          metadata = {
            terraform_address = "data.google_storage_bucket.bar"
          }
          kind = "data_source"
          lifecycle = {

          }
          operations = {
            read = {
              source_kind = "google-discovery"
              source_id = "storage"
              source_path = "google-discovery/storage.json"
              operation_id = "storage.buckets.get"
              credential_bindings = [
                "google_oauth2"
              ]
              purpose = "read"
            }
          }
          redaction = {

          }
          type = "google_storage_bucket"
          name = "bar"
          address = "data.google_storage_bucket.bar"
          attributes = {
            project = "\\\"ramen-corpus-project\\\""
            name = "google_storage_bucket.foo.name"
          }
          dependencies = [
            "google_storage_bucket.foo"
          ]
        },
        {
          metadata = {
            terraform_address = "google_storage_bucket.foo"
          }
          attributes = {
            location = "\\\"US\\\""
            name = "\\\"ramen-corpus-bucket\\\""
            project = "\\\"ramen-corpus-project\\\""
          }
          redaction = {

          }
          address = "google_storage_bucket.foo"
          name = "foo"
          kind = "resource"
          type = "google_storage_bucket"
          operations = {
            create = {
              credential_bindings = [
                "google_oauth2"
              ]
              purpose = "create"
              source_kind = "google-discovery"
              source_id = "storage"
              source_path = "google-discovery/storage.json"
              operation_id = "storage.buckets.insert"
            }
          }
          credential_bindings = [
            "google_oauth2"
          ]
          lifecycle = {

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
        }
      ]
      redaction {

      }
      metadata {
        action = "create"
        config_dir = "testdata/corpus/google/storage/go/data_source_google_storage_bucket_test_002/input"
        source = "ramen convert tf"
      }
      version = "ramen.project.v1"
    }
  }