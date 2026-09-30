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

func TestAccFTPUser_lifecycle(t *testing.T) {
	backend := startFakeKAS(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccProviderConfig + `
resource "allinkl_ftp_user" "logs" {
  path     = "/logs/"
  comment  = "log reader"
  password = "first-Passw0rd"
  write    = false
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestMatchResourceAttr("allinkl_ftp_user.logs", "id", regexp.MustCompile(`^f\d+$`)),
					resource.TestCheckResourceAttr("allinkl_ftp_user.logs", "read", "true"),
					resource.TestCheckResourceAttr("allinkl_ftp_user.logs", "write", "false"),
					resource.TestCheckResourceAttr("allinkl_ftp_user.logs", "list", "true"),
					resource.TestCheckResourceAttr("allinkl_ftp_user.logs", "virus_scan", "true"),
					checkPassword(backend, "allinkl_ftp_user.logs", "first-Passw0rd"),
				),
			},
			{
				Config: testAccProviderConfig + `
resource "allinkl_ftp_user" "logs" {
  path       = "/logs/"
  comment    = "log reader (read only)"
  password   = "second-Passw0rd"
  write      = false
  virus_scan = false
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("allinkl_ftp_user.logs", "comment", "log reader (read only)"),
					resource.TestCheckResourceAttr("allinkl_ftp_user.logs", "virus_scan", "false"),
					checkPassword(backend, "allinkl_ftp_user.logs", "second-Passw0rd"),
					checkCount(backend, "ftp", 1),
				),
			},
			{
				ResourceName:            "allinkl_ftp_user.logs",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password"},
			},
		},
		CheckDestroy: checkCount(backend, "ftp", 0),
	})
}
