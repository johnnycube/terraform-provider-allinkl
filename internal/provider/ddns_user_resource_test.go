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

func TestAccDDNSUser_lifecycle(t *testing.T) {
	backend := startFakeKAS(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccProviderConfig + `
resource "allinkl_ddns_user" "home" {
  zone      = "example.com"
  label     = "home"
  comment   = "at home"
  password  = "first-Passw0rd"
  target_ip = "203.0.113.4"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestMatchResourceAttr("allinkl_ddns_user.home", "id", regexp.MustCompile(`^dyn\d+$`)),
					resource.TestCheckResourceAttr("allinkl_ddns_user.home", "host", "home.example.com"),
					resource.TestCheckResourceAttr("allinkl_ddns_user.home", "current_ip", "203.0.113.4"),
					resource.TestCheckResourceAttr("allinkl_ddns_user.home", "dual_stack", "false"),
					checkPassword(backend, "allinkl_ddns_user.home", "first-Passw0rd"),
				),
			},
			{
				Config: testAccProviderConfig + `
resource "allinkl_ddns_user" "home" {
  zone       = "example.com"
  label      = "home"
  comment    = "at home, dual stack"
  password   = "second-Passw0rd"
  target_ip  = "203.0.113.4"
  dual_stack = true
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("allinkl_ddns_user.home", "dual_stack", "true"),
					checkPassword(backend, "allinkl_ddns_user.home", "second-Passw0rd"),
					checkCount(backend, "ddns", 1),
				),
			},
			{
				ResourceName:            "allinkl_ddns_user.home",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password"},
			},
		},
		CheckDestroy: checkCount(backend, "ddns", 0),
	})
}
