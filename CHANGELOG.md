# Changelog

## v0.2.0 (2026-07-16)

Built on [kasapi](https://github.com/johnnycube/kasapi) v0.2.0.

New:

- `allinkl_mail_account.sender_aliases` — the addresses a mailbox may use in
  the FROM header when sending. KAS has no standalone alias objects: sender
  aliases are a mailbox property, receiving aliases are `allinkl_mail_forward`
  resources.

Fixed (via kasapi v0.2.0):

- Copy addresses are sent as the single comma-separated `copy_adress`
  parameter the API expects, not `copy_adress_0..N`.
- Forward targets are sent as `target_0..target_9` (0-indexed), not
  `target_1..N`, which silently dropped one of ten targets.

Dependencies:

- terraform-plugin-framework 1.19.0, -framework-validators 0.19.0,
  -go 0.31.0, -log 0.10.0, -testing 1.16.0.
- Go 1.26.5, fixing the crypto/tls Encrypted Client Hello privacy leak
  ([GO-2026-5856](https://pkg.go.dev/vuln/GO-2026-5856)) flagged by
  govulncheck.
- GitHub Actions in CI are pinned to commit digests.

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
