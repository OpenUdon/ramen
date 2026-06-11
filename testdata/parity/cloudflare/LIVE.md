# Live Cloudflare Parity

Cloudflare parity is opt-in only. Live runs must set:

```bash
RAMEN_CLOUDFLARE_PARITY=1
RAMEN_CLOUDFLARE_PARITY_LANE=c01
RAMEN_CLOUDFLARE_TOFU=/path/to/tofu
RAMEN_CLOUDFLARE_TERRAFORM=/path/to/terraform
CLOUDFLARE_ACCOUNT_ID=...
CLOUDFLARE_API_TOKEN=...
UDON_CREDENTIAL_CLOUDFLARE_API_TOKEN=...
```

Recording promotion also requires:

```bash
RAMEN_CLOUDFLARE_PARITY_RECORD_UPDATE=1
```

Committed recordings must not include account IDs, API tokens, authorization
headers, raw response bodies, local paths, provider state, or credential names
other than the symbolic `cloudflare_api_token` binding.

## C06 D1 Read-Replication Update

C06 creates one disposable D1 database per runtime using the
`ramen-parity-c06-*` prefix. It creates the database with
`read_replication.mode=disabled`, verifies a no-op, updates the mode to
`auto`, deletes the database, and verifies cleanup absence before preserving a
sanitized recording.

C06 must not run SQL queries, import/export database contents, create tables,
or broaden D1 update claims beyond `read_replication.mode` without a new
focused evidence lane.
