# AWS Provider Parity Fixtures

This tree is reserved for the `Wxx` AWS provider/runtime parity lane.

AWS parity is OpenTofu-baseline-first for new live work. Historical provider
corpus fixtures may still mention Terraform/OpenTofu conversion input, but new
mutation parity runs use OpenTofu plus Ramen unless an explicit broader review
requires otherwise.

Default tests are credential-free. W01 has an opt-in live harness behind the
`awslive` build tag and explicit environment gates:

```bash
RAMEN_AWS_PARITY=1 \
RAMEN_AWS_PARITY_LANE=w01 \
RAMEN_AWS_TOFU=/path/to/tofu \
AWS_ACCESS_KEY_ID=... \
AWS_SECRET_ACCESS_KEY=... \
go test -tags 'awslive udon' . -run '^TestAWSProviderParityLive$' -count=1 -v
```

`AWS_SESSION_TOKEN` and `RAMEN_AWS_REGION` are optional. Recording updates
require `RAMEN_AWS_PARITY_RECORD_UPDATE=1`; otherwise the live test compares
only if a committed recording already exists. No AWS live recording is
committed by default. The W01 Ramen+udon path relies on udon/soliton AWS
Signature Version 4 handling: the workflow carries provider appendices
(`service`, `region`, and `aws_signing_name`), and the signer reads the
standard AWS environment credentials. The symbolic `aws_hmac` binding remains
fixture/review metadata and is not a custom Ramen signer.

W02-W04 are static-only. W02 is the next AWS live candidate after the W01
recording decision is settled, with a dedicated IAM Role harness and minimal
role scope. W03 and W04 remain parked for live promotion because S3 bucket
names are global and support-bucket cleanup requires a separate approval.
Future live lanes must require explicit environment gates, use the minimum
practical disposable resources, avoid high-cost resources, and verify cleanup
before any sanitized observation is committed.
