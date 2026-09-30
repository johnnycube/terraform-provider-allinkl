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
  sender_aliases = ["contact@example.com"]
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("allinkl_mail_account.info", "id"),
					resource.TestCheckResourceAttr("allinkl_mail_account.info", "address", "info@example.com"),
					resource.TestCheckResourceAttr("allinkl_mail_account.info", "copy_addresses.#", "1"),
					resource.TestCheckResourceAttr("allinkl_mail_account.info", "sender_aliases.0", "contact@example.com"),
					func(_ *terraform.State) error {
						if got := backend.PasswordOf(backend.FirstAccountLogin()); got != "first-Passw0rd" {
							return fmt.Errorf("API received password %q", got)
						}
						return nil
					},
				),
			},
			// Update password, copy addresses and sender aliases in place (no replacement)
			{
				Config: testAccProviderConfig + `
resource "allinkl_mail_account" "info" {
  local_part     = "info"
  domain         = "example.com"
  password       = "second-Passw0rd"
  copy_addresses = ["archive@example.org", "backup@example.org"]
  sender_aliases = ["contact@example.com", "hello@example.com"]
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("allinkl_mail_account.info", "copy_addresses.#", "2"),
					resource.TestCheckResourceAttr("allinkl_mail_account.info", "sender_aliases.#", "2"),
					func(_ *terraform.State) error {
						if backend.Count("accounts") != 1 {
							return fmt.Errorf("expected in-place update, got %d accounts", backend.Count("accounts"))
						}
						if got := backend.PasswordOf(backend.FirstAccountLogin()); got != "second-Passw0rd" {
							return fmt.Errorf("API did not receive new password, has %q", got)
						}
						return nil
					},
				),
			},
			// Responder, state, allowed clients, autologin and filters
			{
				Config: testAccProviderConfig + `
resource "allinkl_mail_account" "info" {
  local_part        = "info"
  domain            = "example.com"
  password          = "second-Passw0rd"
  copy_addresses    = ["archive@example.org", "backup@example.org"]
  sender_aliases    = ["contact@example.com", "hello@example.com"]
  state             = "receive_disabled"
  allowed_clients   = ["203.0.113.0/24", "webmail"]
  webmail_autologin = false
  filters           = ["greyl", "spamc_move:move=Junk"]
  responder = {
    text         = "Back in February."
    content_type = "html"
    display_name = "Info"
    start        = "2026-01-01T00:00:00Z"
    end          = "2026-02-01T00:00:00Z"
  }
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("allinkl_mail_account.info", "state", "receive_disabled"),
					resource.TestCheckResourceAttr("allinkl_mail_account.info", "allowed_clients.#", "2"),
					resource.TestCheckResourceAttr("allinkl_mail_account.info", "webmail_autologin", "false"),
					resource.TestCheckResourceAttr("allinkl_mail_account.info", "responder.text", "Back in February."),
					resource.TestCheckResourceAttr("allinkl_mail_account.info", "responder.start", "2026-01-01T00:00:00Z"),
					resource.TestCheckResourceAttr("allinkl_mail_account.info", "spam_filters.#", "2"),
					resource.TestCheckResourceAttr("allinkl_mail_account.info", "spam_filters.1", "spamc_move:move=Junk"),
				),
			},
			// Everything back to the defaults: responder off, filters removed
			{
				Config: testAccProviderConfig + `
resource "allinkl_mail_account" "info" {
  local_part     = "info"
  domain         = "example.com"
  password       = "second-Passw0rd"
  copy_addresses = ["archive@example.org", "backup@example.org"]
  sender_aliases = ["contact@example.com", "hello@example.com"]
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("allinkl_mail_account.info", "state", "active"),
					resource.TestCheckResourceAttr("allinkl_mail_account.info", "webmail_autologin", "true"),
					resource.TestCheckNoResourceAttr("allinkl_mail_account.info", "responder.text"),
					resource.TestCheckResourceAttr("allinkl_mail_account.info", "spam_filters.#", "0"),
				),
			},
			// Import: password and filters cannot be read back
			{
				ResourceName:            "allinkl_mail_account.info",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password", "filters"},
			},
		},
		CheckDestroy: func(_ *terraform.State) error {
			if n := backend.Count("accounts"); n != 0 {
				return fmt.Errorf("expected all accounts destroyed, %d left", n)
			}
			return nil
		},
	})
}
