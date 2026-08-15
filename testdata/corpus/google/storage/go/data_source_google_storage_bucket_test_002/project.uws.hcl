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
          name = "google_storage_bucket.foo.name"
          project = "\\\"ramen-corpus-project\\\""
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
          address = "data.google_storage_bucket.bar"
          kind = "data_source"
          name = "bar"
          type = "google_storage_bucket"
        }
        version = "ramen.terraform.provenance.v1"
      }
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
      query {
        project = "\\\"ramen-corpus-project\\\""
      }
      x-ramen-credential-bindings = [
        "google_oauth2"
      ]
      x-ramen-terraform {
        attributes {
          location = "\\\"US\\\""
          name = "\\\"ramen-corpus-bucket\\\""
          project = "\\\"ramen-corpus-project\\\""
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
          address = "google_storage_bucket.foo"
          kind = "resource"
          name = "foo"
          type = "google_storage_bucket"
        }
        version = "ramen.terraform.provenance.v1"
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
      api_sources = [
        {
          id = "storage"
          kind = "google-discovery"
          path = "google-discovery/storage.json"
        }
      ]
      metadata {
        action = "create"
        config_dir = "testdata/corpus/google/storage/go/data_source_google_storage_bucket_test_002/input"
        source = "ramen convert tf"
      }
      redaction {

      }
      resources = [
        {
          address = "data.google_storage_bucket.bar"
          attributes = {
            name = "google_storage_bucket.foo.name"
            project = "\\\"ramen-corpus-project\\\""
          }
          credential_bindings = [
            "google_oauth2"
          ]
          dependencies = [
            "google_storage_bucket.foo"
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
          kind = "data_source"
          lifecycle = {

          }
          metadata = {
            terraform_address = "data.google_storage_bucket.bar"
          }
          name = "bar"
          operations = {
            read = {
              credential_bindings = [
                "google_oauth2"
              ]
              operation_id = "storage.buckets.get"
              purpose = "read"
              source_id = "storage"
              source_kind = "google-discovery"
              source_path = "google-discovery/storage.json"
            }
          }
          redaction = {

          }
          type = "google_storage_bucket"
        },
        {
          address = "google_storage_bucket.foo"
          attributes = {
            location = "\\\"US\\\""
            name = "\\\"ramen-corpus-bucket\\\""
            project = "\\\"ramen-corpus-project\\\""
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
            terraform_address = "google_storage_bucket.foo"
          }
          name = "foo"
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