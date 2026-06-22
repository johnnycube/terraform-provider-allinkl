# Changelog

## v0.1.0 (2026-06-17)

First release.

Resources:

- `allinkl_dns_record` — DNS records with full CRUD, import, and drift
  detection. `type` and `zone` force replacement.
- `allinkl_mail_account` — mailboxes with write-only password handling.
- `allinkl_mail_forward` — mail redirects with 1–10 targets.
- `allinkl_subdomain` — subdomains with a document-root path.

Data sources:

- `allinkl_dns_records` — every record of a zone.
- `allinkl_domains` — every hosted domain (read-only by design).

Tooling:

- Acceptance test suite against an in-process fake KAS server, no credentials
  required.
- DCO-based contribution policy, security policy, golangci-lint and Renovate in
  CI.

Built on the [kasapi](https://github.com/johnnycube/kasapi) library (Apache-2.0;
this provider is MPL-2.0).
