// Copyright (c) 2026 Johannes Küber
// SPDX-License-Identifier: MPL-2.0
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package provider

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/johnnycube/kasapi/kasapitest"
)

// Acceptance tests run a real terraform/tofu binary against this provider,
// which talks to an in-process fake KAS server (github.com/johnnycube/kasapi/kasapitest) -
// no real credentials needed. Run with:
//
//	TF_ACC=1 go test ./internal/provider/
//
// Against OpenTofu, point the harness at the tofu binary with an absolute
// path (a bare name is rejected with an ExactBinPath error):
//
//	TF_ACC=1 TF_ACC_TERRAFORM_PATH="$(command -v tofu)" go test ./internal/provider/
//
// TestMain pins TF_ACC_PROVIDER_NAMESPACE for OpenTofu compatibility, so no
// other environment variables are required.

// TestMain pins the reattach provider namespace to "hashicorp". By default the
// plugin-testing harness registers each factory provider under both the legacy
// "-" namespace and "hashicorp" so it works regardless of the CLI's Terraform
// version. OpenTofu >= 1.12 rejects the legacy "-" namespace for the
// registry.terraform.io host and panics while parsing the reattach config, so
// every acceptance test would fail at "init". Forcing a single valid namespace
// makes the suite pass under both terraform and tofu. Respect an explicit
// override if the caller already set one.
func TestMain(m *testing.M) {
	if os.Getenv("TF_ACC_PROVIDER_NAMESPACE") == "" {
		os.Setenv("TF_ACC_PROVIDER_NAMESPACE", "hashicorp")
	}
	// resource.TestMain handles the -sweep flag (see sweep_test.go) and
	// otherwise runs the tests; it calls os.Exit itself.
	resource.TestMain(m)
}

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"allinkl": providerserver.NewProtocol6WithError(New("test")()),
}

const testAccProviderConfig = `
provider "allinkl" {
  login    = "w0123456"
  password = "secret"
}
`

// startFakeKAS starts the fake server with a fresh in-memory backend and
// points the provider at it for the duration of the test via env vars.
func startFakeKAS(t *testing.T) *fakeBackend {
	t.Helper()
	b := newFakeBackend()
	srv := kasapitest.New(t, b.handle)
	t.Setenv("KAS_API_ENDPOINT", srv.APIURL())
	t.Setenv("KAS_AUTH_ENDPOINT", srv.AuthURL())
	return b
}
