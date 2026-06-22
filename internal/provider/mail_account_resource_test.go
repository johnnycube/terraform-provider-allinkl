// Copyright (c) 2026 Johannes Küber
// SPDX-License-Identifier: MPL-2.0
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccMailAccount_lifecycle(t *testing.T) {
	backend := startFakeKAS(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create
			{
				Config: testAccProviderConfig + `
resource "allinkl_mail_account" "info" {
  local_part     = "info"
  domain         = "example.com"
  password       = "first-Passw0rd"
  copy_addresses = ["archive@example.org"]
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("allinkl_mail_account.info", "id"),
					resource.TestCheckResourceAttr("allinkl_mail_account.info", "address", "info@example.com"),
					resource.TestCheckResourceAttr("allinkl_mail_account.info", "copy_addresses.#", "1"),
					func(_ *terraform.State) error {
						if got := backend.passwordOf(backend.firstAccountLogin()); got != "first-Passw0rd" {
							return fmt.Errorf("API received password %q", got)
						}
						return nil
					},
				),
			},
			// Update password and copy addresses in place (no replacement)
			{
				Config: testAccProviderConfig + `
resource "allinkl_mail_account" "info" {
  local_part     = "info"
  domain         = "example.com"
  password       = "second-Passw0rd"
  copy_addresses = ["archive@example.org", "backup@example.org"]
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("allinkl_mail_account.info", "copy_addresses.#", "2"),
					func(_ *terraform.State) error {
						if backend.accountCount() != 1 {
							return fmt.Errorf("expected in-place update, got %d accounts", backend.accountCount())
						}
						if got := backend.passwordOf(backend.firstAccountLogin()); got != "second-Passw0rd" {
							return fmt.Errorf("API did not receive new password, has %q", got)
						}
						return nil
					},
				),
			},
			// Import: password cannot be read back, so it is excluded from verify
			{
				ResourceName:            "allinkl_mail_account.info",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password"},
			},
		},
		CheckDestroy: func(_ *terraform.State) error {
			if n := backend.accountCount(); n != 0 {
				return fmt.Errorf("expected all accounts destroyed, %d left", n)
			}
			return nil
		},
	})
}
