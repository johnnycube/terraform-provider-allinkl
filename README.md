# terraform-provider-allinkl

[![CI](https://github.com/johnnycube/terraform-provider-allinkl/actions/workflows/ci.yml/badge.svg)](https://github.com/johnnycube/terraform-provider-allinkl/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-MPL--2.0-blue.svg)](LICENSE)

A Terraform and OpenTofu provider for all-inkl.com (KAS) hosting. It manages
DNS records, mailboxes, mail forwards and subdomains through the KAS SOAP API,
and exposes the account's domains as a data source.

The provider is the Terraform layer over
[kasapi](https://github.com/johnnycube/kasapi), a standalone Go client. The
split is deliberate: kasapi knows nothing about Terraform, and the provider
contains no SOAP or session handling. A new resource type is a typed service
in the library plus a resource here.

> **Unofficial.** Not affiliated with, endorsed by, or supported by
> all-inkl.com (Neue Medien Münnich GmbH). "all-inkl" and "KAS" name the
> service this talks to, nothing more. No warranty — see [LICENSE](LICENSE).

## Using the provider

```terraform
terraform {
  required_providers {
    allinkl = {
      source  = "johnnycube/allinkl"
      version = "~> 0.1"
    }
  }
}

provider "allinkl" {
  # or set KAS_LOGIN / KAS_PASSWORD in the environment
  login    = "w0123456"
  password = var.kas_password
}
```

The same configuration works with OpenTofu — the registry resolves
`johnnycube/allinkl` for both.

## Resources

### `allinkl_dns_record`

```hcl
resource "allinkl_dns_record" "www" {
  zone = "example.com"   # forces replacement when changed
  name = "www"           # "" for the zone apex
  type = "A"             # forces replacement; KAS cannot change a record's type in place
  data = "203.0.113.10"
  aux  = 0               # MX/SRV priority, default 0
}
```

Import with `terraform import allinkl_dns_record.www example.com/12345`. Record
IDs come from the `allinkl_dns_records` data source.

### `allinkl_mail_account` and `allinkl_mail_forward`

```hcl
resource "allinkl_mail_account" "info" {
  local_part     = "info"          # forces replacement
  domain         = "example.com"   # forces replacement
  password       = var.mailbox_password
  copy_addresses = ["archive@example.org"]
}

resource "allinkl_mail_forward" "sales" {
  local_part = "sales"
  domain     = "example.com"
  targets    = ["alice@example.org", "bob@example.org"]   # 1–10 targets
}
```

The account `id` is the KAS-assigned mail login (`m1234567`), which is also the
IMAP/SMTP username. The password is write-only: KAS never returns it, so drift
on the password is not detectable and changing the value updates it. As with
any Terraform secret it is persisted in state — protect the state accordingly.

Import: `terraform import allinkl_mail_account.info m1234567` and
`terraform import allinkl_mail_forward.sales sales@example.com`. After importing
an account, the next apply sets the password to the configured value, because
the API cannot read the existing one.

### `allinkl_subdomain`

```hcl
resource "allinkl_subdomain" "blog" {
  name   = "blog"
  domain = "example.com"
  path   = "/blog/"
}
```

Import with `terraform import allinkl_subdomain.blog blog.example.com`.

## Data sources

`allinkl_dns_records` reads every record of a zone, including system records:

```hcl
data "allinkl_dns_records" "all" {
  zone = "example.com"
}
```

`allinkl_domains` reads every domain in the account. Domain creation and
deletion are deliberately not exposed as a resource — they touch registration
and routing, which is not safe to automate without per-account testing. Use the
data source to reference existing domains, or `kascli exec` from the library for
the raw actions.

```hcl
data "allinkl_domains" "all" {}
```

## Security

- **Credentials** come from the provider block or `KAS_LOGIN` / `KAS_PASSWORD`.
  Prefer the environment variables so credentials stay out of `.tf` files;
  `*.tfvars` is git-ignored. Consider a dedicated KAS API password rather than
  the account password.
- **SHA1 auth by default.** The client sends `sha1(password)` to `KasAuth`
  rather than the plaintext (`auth_type = "plain"` opts out). The session token
  is short-lived, held in memory, and never logged.
- **Transport.** TLS 1.2 is the floor; responses are size-limited.
- **State holds secrets.** Terraform persists every attribute, including
  `password`, in state. Encrypt and access-control it — encrypted remote state
  is the usual answer.
- **Validation on both layers.** Schema validators reject malformed input at
  plan time; the library re-validates (RFC 5322 addresses, non-empty IDs) before
  any request, so non-Terraform consumers get the same guarantees.

## Building

```sh
make build
```

For local development, point Terraform at the binary with a `dev_overrides`
block in `~/.terraformrc` (or `~/.tofurc`). The key must match the provider
`source` written in your configuration:

```hcl
provider_installation {
  dev_overrides {
    "registry.opentofu.org/johnnycube/allinkl" = "/path/to/binary/dir"
  }
  direct {}
}
```

`examples/local/` runs a full plan/apply/destroy against an in-process fake KAS
server, no credentials required — see its `README.md`.

kasapi is a separate module. For side-by-side development use the `go.work`
file shipped with both repositories, or the commented `replace` directive in
`go.mod`.

## Releasing

Releases are tag-driven: pushing a `vX.Y.Z` tag runs GoReleaser
(`.github/workflows/release.yml`), which cross-compiles every platform, writes
and GPG-signs a `SHA256SUMS` file, and publishes a GitHub release with the
registry manifest.

One-time setup: add the `GPG_PRIVATE_KEY` and `GPG_PASSPHRASE` repository
secrets, then register the GPG public key with the target registry.

To cut a release, update `CHANGELOG.md`, confirm `make test` and `make testacc`
pass, then tag and push:

```sh
git tag vX.Y.Z
git push origin vX.Y.Z
```

Submit the provider and GPG public key to the OpenTofu registry
(github.com/opentofu/registry) and/or registry.terraform.io. Versioning follows
semver: patch for fixes, minor for new resources, major for breaking schema
changes.

## Testing

```sh
make test      # unit tests
make testacc   # acceptance tests: a real terraform/tofu binary against the fake KAS server
```

Acceptance tests run a Terraform or OpenTofu binary through full
create/update/replace/import/destroy lifecycles. They point the provider at the
in-process fake KAS server (`kasapitest`) via the `KAS_API_ENDPOINT` /
`KAS_AUTH_ENDPOINT` environment variables — an env-only, test-focused override,
deliberately not a schema attribute so a configuration cannot silently redirect
credentials elsewhere. No real credentials are needed, and CI runs them on every
push. Set `TF_ACC_TERRAFORM_PATH=tofu` to run against OpenTofu.

They do not replace one manual verification of the KAS field names against a
real account — see the notes below.

## Extending

A new object type is a two-repository, two-file change. Email was added exactly
this way; use it as the reference.

**1. API layer.** In the [kasapi library](https://github.com/johnnycube/kasapi),
add a typed service over `Client.Exec` mirroring `dns.go` / `mail.go`:

```go
type FTPService struct{ c *Client }

func (s *FTPService) Create(ctx context.Context, user, password, path string) error {
    _, err := s.c.Exec(ctx, "add_ftpuser", map[string]any{
        "ftp_user":     user,
        "ftp_password": password,
        "ftp_path":     path,
    })
    return err
}
```

Register it in the library's `New()`: `c.FTP = &FTPService{c: c}`.

**2. Terraform layer.** Add `internal/provider/ftp_user_resource.go` here,
following `dns_record_resource.go` / `mail_account_resource.go`, register the
constructor in `provider.go`'s `Resources()`, and add the matching actions to
`fake_backend_test.go` plus a lifecycle acceptance test.

The KAS actions for the common cases already exist:

| Use case        | KAS actions |
|-----------------|-------------|
| FTP users       | `get_ftpusers`, `add_ftpuser`, `update_ftpuser`, `delete_ftpuser` |
| Databases       | `get_databases`, `add_database`, `update_database`, `delete_database` |
| Cronjobs        | `get_cronjobs`, `add_cronjob`, `update_cronjob`, `delete_cronjob` |
| Domains (write) | `add_domain`, `update_domain`, `delete_domain` — deliberately unexposed; see `allinkl_domains` |

Nothing in the transport, auth or flood-protection code changes.

## Notes

- KAS serializes responses as SOAP-encoded PHP structures; the library decodes
  them generically. Verify the exact field names against the action
  documentation in the KAS panel (KAS → Tools → KAS API). The library's `kascli`
  makes this one command per action:

  ```sh
  kascli exec get_mailaccounts
  kascli exec get_dns_settings zone_host=example.com.
  kascli exec get_subdomains
  ```

  This matters in particular for the mail and subdomain actions
  (`mail_adresses`, `copy_adress_N`, `target_N`, `subdomain_path`), whose
  parameter names carry verification notes in the source.
- KAS enforces flood protection between requests. The client serializes calls
  and waits out the announced delay, so large plans apply slowly by design.
- `update_dns_settings` cannot change a record's type; the resource models this
  with `RequiresReplace` on `type` and `zone`.
- Errors surface with the KAS symbolic fault codes (`kas_login_incorrect`,
  `flood_protection`) to keep debugging direct.

## License

Mozilla Public License 2.0 — the Terraform-provider ecosystem convention. See
[LICENSE](LICENSE). The underlying kasapi library is Apache-2.0.
