# Changelog

## v0.2.2 (2026-09-29)

Built on [kasapi](https://github.com/johnnycube/kasapi) v0.3.1. No changes to
the schemas of resources or data sources.

Changed (via kasapi v0.3):

- An update that matches the current state succeeds. KAS answers such an
  update with the fault `nothing_to_do`, which surfaced as an error.
- A subdomain that no longer exists is recognized on update and delete. KAS
  reports it as `subdomain_doenst_exist`, which was not read as "not found".
- Reads ask KAS for the one object instead of the account's full list.

## v0.2.1 (2026-09-29)

No changes to resources or data sources. This release rebuilds the provider
on patched dependencies.

Dependencies:

- Go 1.26.8, fixing three standard-library advisories present in 1.26.5
  ([GO-2026-6218](https://pkg.go.dev/vuln/GO-2026-6218) net/url,
  [GO-2026-6091](https://pkg.go.dev/vuln/GO-2026-6091) html/template,
  [GO-2026-6090](https://pkg.go.dev/vuln/GO-2026-6090) crypto/tls).
- google.golang.org/grpc 1.83.2
  ([GO-2026-6061](https://pkg.go.dev/vuln/GO-2026-6061),
  GHSA-2v4p-qf9q-27wj) and golang.org/x/text 0.41.0
  ([GO-2026-5970](https://pkg.go.dev/vuln/GO-2026-5970)). Both are indirect
  dependencies of the provider framework.
- terraform-plugin-log 0.11.0.
- GitHub Actions in CI and the release workflow are on their current major
  versions.

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
