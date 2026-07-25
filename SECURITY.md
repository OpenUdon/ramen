# Security Policy

## Reporting

Report suspected vulnerabilities through GitHub's private security-advisory
feature for the OpenUdon/ramen repository. Do not include credentials, access
tokens, live response bodies, customer identifiers, or exploitable details in
a public issue.

If private reporting is unavailable, open a public issue containing only a
request for a private contact channel.

## Supported Versions

The latest v0.1.x release receives security fixes on a best-effort basis.
Unreleased commits and older pre-1.0 lines are not maintained as separate
security branches.

## Security Boundary

Ramen treats project files, API descriptions, converted input, executor
results, and state files as untrusted. Default public builds perform no live API
execution. Credentials belong to executor-owned configuration and must never be
placed in Ramen projects, plans, state, examples, diagnostics, or issue
reports.

Ramen is pre-1.0 software and is provided without an operational response-time
or support SLA.
