# Offline Browser Desired-State Example

This native project demonstrates Ramen's UWS browser contract support without
opening a browser, contacting a service, or resolving credentials. It contains:

- UWS 1.9 and a browser 1.7 capability profile;
- string, integer, number, boolean, and presence outputs;
- one portable frame context;
- browser-authentication 1.1 with a popup context;
- a named in-workflow session and the symbolic binding `member_username`;
- a read-only role plus a separately confirmed mutation role.

From the Ramen repository root:

```bash
go run ./cmd/ramen validate --project ./examples/browser
go run ./cmd/ramen graph --project ./examples/browser
go run ./cmd/ramen run ./examples/browser/project.uws.yaml --check --json
go run ./cmd/ramen plan \
  --project ./examples/browser \
  --action read \
  --state /tmp/ramen-browser-state.db \
  --out /tmp/ramen-browser-plan.json
go run ./cmd/ramen apply \
  --plan /tmp/ramen-browser-plan.json \
  --state /tmp/ramen-browser-state.db \
  --auto-approve \
  --mock \
  --out /tmp/ramen-browser-apply
```

These commands are credential-free. `member_username` is only a symbolic name;
the example contains no username, password, token, cookie, OTP, or session
handle. The public mock executor performs no network or browser I/O.

For the external-session form, remove `authenticate_member`, remove each
browser operation's `dependsOn`, and keep its `x-uws-browser-session` with an
operator-chosen symbolic name. A real trusted browser runtime—not Ramen—must
resolve that name to an execution-local session and enforce runtime approval.

See [Browser Desired State](../../docs/browser-desired-state.md) for version
floors, approval behavior, and ownership boundaries.
