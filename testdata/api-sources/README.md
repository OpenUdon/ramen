# API source regression fixtures

These immutable, untrusted documents make Ramen's conversion, parity, and
training-data tests hermetic. Tests parse them as metadata only; they never
execute an API operation, resolve credentials, or contact an endpoint.

The files preserve reviewed upstream snapshots:

| File | Upstream source | Revision or version | SHA-256 |
| --- | --- | --- | --- |
| `aws-iam-smithy-model.json` | `aws/api-models-aws` IAM service model | blob `784d7b2e6c8edc68b6127c11b3c3191609fdbb8d` | `c7fa986edb29bdfa1482df283aa7d4879298f3a7dd4a3768176c6b15f163fc3f` |
| `aws-lambda-smithy-model.json` | `aws/api-models-aws` Lambda service model | blob `5da641a9176761c13e03977cdf6fa718cfd4c35a` | `954bfbb8b6d2654ba9844cd5877a546504e8cb22472e220c9ee938dc273ea7e8` |
| `aws-s3-smithy-model.json` | `aws/api-models-aws` S3 service model | blob `70c7bfe0bc2c2dc28040577f994005830cf319a9` | `5b9d8d655ffe78ef9eaa04952cb44a2998d6e8ace77b33161e6b0bd8fb663fd2` |
| `google-cloud-storage-discovery-v1.json` | Google Cloud Storage Discovery document | revision `20260512` | `cc6c7274d25703d0b8a75cc6c3742a4813c853df4038701bdc020252e6272eba` |
| `azure-cosmos-db-resource-manager-openapi.json` | `Azure/azure-rest-api-specs` Cosmos DB Resource Manager Swagger | commit `1a37711b19072f2ed81a4dc1e2763e49d3a0d7c2`, API `2025-10-15` | `96f9252ca2fc20d199c1053f12b870df292725bbbcce69bd1ff10d6a19ace510` |
| `kubernetes-v1-19-2-swagger.json` | `hashicorp/terraform-provider-kubernetes` OpenAPI test fixture | commit `dcdf46c9ca238b671d1159f252ec19c8fe2ed16e`, Kubernetes `v1.19.2` | `9babd139a8141cac6613906b2b7b7962a9d5c88ff4146979e96e3f487cbebdaf` |

AWS API Models are published under Apache-2.0. The Google Developers site
policies, Azure REST API Specs repository license and Microsoft documentation
terms, and terraform-provider-kubernetes repository license apply to their
respective snapshots.
