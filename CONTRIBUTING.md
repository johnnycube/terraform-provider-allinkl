# Contributing to terraform-provider-allinkl

Contributions are welcome. A few rules keep the licensing and quality clear.

## License of contributions

The project is MPL-2.0. By submitting a contribution you agree it is licensed
under the same terms. There is no CLA.

All commits must be signed off (`git commit -s`), which adds a
`Signed-off-by: Your Name <you@example.com>` trailer and asserts the
[Developer Certificate of Origin](https://developercertificate.org/): that you
have the right to submit the change under the project license.

## File headers

Every Go file carries the MPL-2.0 Exhibit A header; add it to new files:

```go
// Copyright (c) 2026 Johannes Küber
// SPDX-License-Identifier: MPL-2.0
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
```

## Working rules

- API logic belongs in the [kasapi library](https://github.com/johnnycube/kasapi)
  (Apache-2.0); this repository is only the Terraform layer. A new use case is a
  typed service in the library plus a resource here, the matching actions in
  `internal/provider/fake_backend_test.go`, and a lifecycle acceptance test.
- `gofmt`, `go vet ./...` and `go test -race ./...` must pass. CI enforces all
  three.
- Acceptance tests run with `TF_ACC=1 go test ./internal/provider/` and need a
  `terraform` or `tofu` binary (`TF_ACC_TERRAFORM_PATH=tofu`), but no real KAS
  credentials — they use the in-process fake server.

## Prose

Documentation follows [STYLE.md](../STYLE.md): declarative, on point, no
marketing words, explain the why.
