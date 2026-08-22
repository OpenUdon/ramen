# Browser Desired State

Ramen can reconcile a native resource through a reviewed UWS
`browser-profile` when no suitable API contract exists. Prefer OpenAPI, Smithy,
Google Discovery, or another supported API source whenever it can express the
resource: browser profiles are a bounded fallback for stable, accessibility-
addressable UI behavior, not a replacement for an available API.

Ramen owns desired-state mapping, deterministic planning, artifact digests,
state history, and trusted-executor capability checks. UWS owns the portable
browser and authentication contracts. OpenUdon owns authoring and reviewer UI
controls. A trusted runtime owns browsers, credentials, MFA, live sessions,
profile freshness, driver configuration, and exact runtime approval. Ramen
does not perform browser automation.

## Version compatibility

Ramen accepts browser profiles 1.5, 1.6, and 1.7 and browser-authentication
profiles 1.0 and 1.1. Generated action documents use the highest applicable
minimum:

| Contract in use | Minimum UWS |
|---|---:|
| Browser 1.5 without a named session | 1.5 |
| A named or external session, or authentication 1.0 | 1.7 |
| Browser 1.6 or authentication 1.1 | 1.8 |
| Browser 1.7 scalar outputs | 1.9 |

Authentication profile/call versions must pair exactly: authentication 1.0
uses call 1.0, and authentication 1.1 uses call 1.1.

## Exact operation selection

The native API source lists only the browser capability profile:

```yaml
api_sources:
  - kind: browser-profile
    id: member-ui
    path: browser.yaml
```

Each browser lifecycle role names both the action key in that profile and the
exact top-level UWS operation carrying request/session/authentication details:

```yaml
operations:
  read:
    purpose: read
    source_kind: browser-profile
    source_id: member-ui
    operation_id: read_status
    uws_operation_ref: read_status_uws
```

Ramen rejects mismatched source descriptions, paths, action selectors, UWS
version floors, side effects, sessions, authentication dependencies, and
credential slots. Distinct roles may select the same profile action through
different `uws_operation_ref` values; their per-operation requests and sessions
remain distinct in the plan.

Browser action parameters use the UWS operation `request.body`. Ramen request
bindings overlay desired resource values into those direct action parameters.
Browser 1.7 string, integer, number, boolean, and presence outputs become named
`$response.body.<output>` operation and step outputs.

## Sessions and credentials are symbolic

An in-workflow authenticated session uses one direct authentication dependency
and the same `x-uws-browser-session` name on the browser operation. The
authentication call maps each profile slot to a symbolic executor binding:

```yaml
x-uws-browser-authentication:
  profile: authentication.yaml
  flow: login
  session: member_portal
  credentialBindings:
    username: member_username
```

`member_username` is a name, not a username value. Credential values, OTPs,
cookies, tokens, and session handles must never enter the project, plan, state,
generated action document, or `ramen run` result.

For a session established outside the UWS document, omit the authentication
operation and dependency but retain the symbolic session extension:

```yaml
x-uws-browser-session:
  session: operator_session
```

Ramen records that as an external session requirement. The trusted runtime
must resolve it; Ramen never accepts or forwards the live session value.

## Two approval boundaries

Plan approval binds the UWS operation, request, browser/authentication profile
versions, paths and digests, outputs, contexts, side effects, confirmation,
session names, flow, timeout, and symbolic credential bindings. Any change
invalidates the approval.

That approval does not satisfy the browser profile's runtime confirmation.
Before handoff, Ramen requires explicit executor capabilities for browser
contexts, scalar outputs, sessions, authentication, mutation approval, and
authentication approval as applicable. The public mock advertises those
capabilities for offline tests. The opt-in Udon adapter does not, and fails
before execution.

Imperative `ramen run --check` also validates referenced browser artifacts and
returns their digests in `browser_artifacts`. The approval digest binds that
summary, and Ramen revalidates and rehashes the artifacts before every executor
handoff.

## Offline example

[The browser example](../examples/browser/README.md) is a native UWS 1.9
project with browser 1.7 scalar outputs, a frame context, authentication 1.1, a
named session, and symbolic credential references. Its documented commands use
validation, planning, and the mock executor only; they do not open a browser or
resolve credentials.
