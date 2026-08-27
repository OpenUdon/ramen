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
profiles 1.0 and 1.1. Native projects may use UWS 1.9.1 for the optional
content-trust registry; this does not revise browser profile 1.7. Generated
action documents use the highest applicable minimum:

| Contract in use | Minimum UWS |
|---|---:|
| Browser 1.5 without a named session | 1.5 |
| A named or external session, or authentication 1.0 | 1.7 |
| Browser 1.6 or authentication 1.1 | 1.8 |
| Browser 1.7 scalar outputs | 1.9 |
| Optional root `contentTrust` registry | 1.9.1 |

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

## Advisory content-trust analysis

When a UWS 1.9.1 native project declares `contentTrust`, `ramen validate`
loads each contained browser profile through the same bounded regular-file and
package-containment checks used by browser contract validation, then supplies
Browsertools' resolver to the UWS analyzer. Browser-derived values default to
untrusted. String outputs retain free-text capability; integer, number,
boolean, presence, and inline-enum outputs are constrained scalars; structured
outputs remain composite. Capability narrowing does not make provenance
trusted: an untrusted integer can still produce an advisory warning when it
controls a later step or reaches an authority-bearing channel.

The UWS core expression form for a selected response member is an RFC 6901
fragment such as `$response.body#/count`. Existing runtime-specific forms such
as `$response.body.count` remain project data, but the analyzer reports them as
opaque when it cannot recover their flow. Browser profile semantics are known
through the resolver; unrelated extension-profile semantics remain unknown
unless the core expression grammar is sufficient.

Findings are warnings by default and expose only stable codes, document paths,
and fixed messages. They do not include browser text, request/response values,
credentials, sessions, or profile paths. `--strict` uses Ramen's existing
warning promotion policy. Neither mode changes planning, approval digests,
action lowering, apply/run handoff, state, or executor authority. Malformed
trust declarations are ordinary UWS validation errors rather than advisory
findings.

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
