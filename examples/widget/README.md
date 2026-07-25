# Local Widget Example

This example exercises Ramen's native desired-state lifecycle using a local
OpenAPI document and the public mock executor. It performs no network calls and
requires no credentials.

From the Ramen repository root:

```bash
go run ./cmd/ramen init \
  --project ./examples/widget \
  --state /tmp/ramen-widget-state.db

go run ./cmd/ramen validate --project ./examples/widget
go run ./cmd/ramen graph --project ./examples/widget

go run ./cmd/ramen plan \
  --project ./examples/widget \
  --state /tmp/ramen-widget-state.db \
  --out /tmp/ramen-widget-plan.json

go run ./cmd/ramen apply \
  --plan /tmp/ramen-widget-plan.json \
  --state /tmp/ramen-widget-state.db \
  --auto-approve \
  --mock \
  --out /tmp/ramen-widget-apply

go run ./cmd/ramen state list \
  --state /tmp/ramen-widget-state.db
```

Use a task-specific temporary path instead of `/tmp/ramen-widget-state.db`
when running more than one copy concurrently.
