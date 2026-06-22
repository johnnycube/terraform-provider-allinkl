// Copyright (c) 2026 Johannes Küber
// SPDX-License-Identifier: MPL-2.0
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// These tests assert that bad input is rejected with a clear error instead of
// reaching the API, and that a pre-existing mailbox is reported with an import
// hint rather than a raw KAS fault.

func TestAccDNSRecord_invalidType(t *testing.T) {
	startFakeKAS(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccProviderConfig + `
resource "allinkl_dns_record" "bad" {
  zone = "example.com"
  name = "www"
  type = "not a type"
  data = "203.0.113.10"
}`,
				ExpectError: regexp.MustCompile("must be a DNS record type"),
			},
		},
	})
}

func TestAccMailAccount_shortPassword(t *testing.T) {
	startFakeKAS(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccProviderConfig + `
resource "allinkl_mail_account" "info" {
  local_part = "info"
  domain     = "example.com"
  password   = "short"
}`,
				ExpectError: regexp.MustCompile("at least 8"),
			},
		},
	})
}

func TestAccMailForward_noTargets(t *testing.T) {
	startFakeKAS(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccProviderConfig + `
resource "allinkl_mail_forward" "sales" {
  local_part = "sales"
  domain     = "example.com"
  targets    = []
}`,
				ExpectError: regexp.MustCompile("at least 1"),
			},
		},
	})
}

func TestAccMailAccount_alreadyExists(t *testing.T) {
	backend := startFakeKAS(t)
	backend.seedMailAccount("info", "example.com")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccProviderConfig + `
resource "allinkl_mail_account" "info" {
  local_part = "info"
  domain     = "example.com"
  password   = "valid-Passw0rd"
}`,
				ExpectError: regexp.MustCompile("already exists"),
			},
		},
	})
}
