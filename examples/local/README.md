# Local example (no all-inkl.com account)

Runs the provider end to end on your machine against an in-memory **fake KAS
server**, so you can see a real `plan`/`apply`/`destroy` without credentials or
a registry.

## Run

```sh
./run.sh
```

That script:

1. builds the provider from this repo into `.run/bin/`;
2. builds and starts `fakekas/` (the fake KAS SOAP API) on `127.0.0.1:8511`;
3. writes a `dev_overrides` CLI config so OpenTofu/Terraform loads the
   freshly-built provider straight from disk (no `init`, no download);
4. points the provider at the fake server with `KAS_LOGIN`, `KAS_PASSWORD`,
   `KAS_AUTH_ENDPOINT` and `KAS_API_ENDPOINT`;
5. runs `plan`, `apply`, prints outputs, then `destroy`, and stops the server.

Requires `go` and a `tofu` (or `terraform`) binary on `PATH`. Generated
artifacts land in `.run/` and are git-ignored.

## How it works

The provider reads its endpoints from `KAS_API_ENDPOINT` / `KAS_AUTH_ENDPOINT`
(see `internal/provider/provider.go`), which normally default to the real KAS
SOAP service. `fakekas` speaks the same wire protocol as the
`kasapitest` server used by the acceptance tests - it issues a session token
(password `secret`, any login) and keeps DNS, mail and subdomain state in
memory. `main.tf` then exercises every resource and both data sources.

To drive it by hand instead of via `run.sh`, start the server
(`go run ./fakekas`), export the four `KAS_*` variables plus a `dev_overrides`
`TF_CLI_CONFIG_FILE`, and run `tofu` in this directory.
