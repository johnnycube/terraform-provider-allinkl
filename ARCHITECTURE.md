# Architecture

terraform-provider-allinkl maps Terraform resources onto the all-inkl.com KAS
SOAP API. It is a thin Terraform layer over the
[kasapi](https://github.com/johnnycube/kasapi) library; this document covers
how the pieces fit and the decisions behind them.

## Components

- **`main.go`** — serves the provider over the plugin protocol
  (`providerserver.Serve`) under `registry.terraform.io/johnnycube/allinkl`.
- **`internal/provider`** — the Terraform layer, built on
  terraform-plugin-framework. `provider.go` reads the credentials (attributes or
  `KAS_*` environment variables), builds one `kasapi.Client` and hands it to
  every resource and data source through `Configure`. One file per resource or
  data source; `host.go` holds the host settings and TLS state that domains and
  subdomains share, `helpers.go` the list and diagnostic conversions.
- **kasapi** (separate module) — transport, authentication, flood protection
  and the typed services (`Client.DNS`, `Client.Mail`, `Client.Subdomains`, ...).
  It maps KAS faults to errors such as `kasapi.ErrNotFound`.
- **`internal/fakekas`** — an in-memory KAS: one method per supported action
  behind `Backend.Handle`, which kasapi's `kasapitest` server calls with the
  decoded SOAP parameters. It reproduces the KAS behaviors the provider depends
  on, such as the `nothing_to_do` fault for an update that changes nothing.
- **`examples/`** — the snippets tfplugindocs embeds in `docs/`, plus
  `examples/local` (a full plan/apply/destroy against `fakekas`) and
  `examples/smoke` (a read-only or write check against a real account).

## Data flow

Terraform calls a resource's CRUD method with the plan or state. The resource
converts the model into a kasapi struct, calls the typed service, and reads the
object back so the state holds what KAS reports rather than what was sent.
kasapi encodes the call as a SOAP request, waits out the flood delay of the
previous response, and decodes the reply. Update methods send only the
attributes that changed — KAS answers an unchanged update with a fault.

```
terraform/tofu ──plugin protocol──▶ internal/provider ──typed calls──▶ kasapi ──SOAP──▶ KAS
                                                                          │
                                         acceptance tests: kasapitest + internal/fakekas
```

## Decisions

- **API logic lives in kasapi, not here.** Field names, fault codes and the
  wire format belong to the library, which kascli and other consumers share;
  this repository only translates between Terraform models and typed calls. A
  new object type is a kasapi service first, then a resource here.
- **Acceptance tests run against a fake KAS.** A real account cannot be reset
  between runs and enforces flood delays, so CI drives a real terraform or tofu
  binary against `fakekas` instead. The endpoint override is environment-only
  (`KAS_API_ENDPOINT`, `KAS_AUTH_ENDPOINT`) so a configuration cannot redirect
  credentials. Field names are verified against a real account separately
  (`examples/smoke`).
- **Write-only values stay in the state as configured.** KAS never returns
  passwords or private keys, so the provider keeps the planned value and does
  not detect drift on them; an import sets them on the next apply.
