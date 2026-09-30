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

func TestAccCronjob_lifecycle(t *testing.T) {
	backend := startFakeKAS(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccProviderConfig + `
resource "allinkl_cronjob" "nightly" {
  comment = "nightly import"
  url     = "example.com/cron.php"
  minute  = "30"
  hour    = "3"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestMatchResourceAttr("allinkl_cronjob.nightly", "id", regexp.MustCompile(`^\d+$`)),
					resource.TestCheckResourceAttr("allinkl_cronjob.nightly", "protocol", "https"),
					resource.TestCheckResourceAttr("allinkl_cronjob.nightly", "day_of_month", "*"),
					resource.TestCheckResourceAttr("allinkl_cronjob.nightly", "active", "true"),
				),
			},
			{
				Config: testAccProviderConfig + `
resource "allinkl_cronjob" "nightly" {
  comment       = "nightly import"
  url           = "example.com/cron.php"
  minute        = "*/15"
  hour          = "*"
  http_user     = "cron"
  http_password = "basic-auth"
  mail_address  = "ops@example.com"
  mail_subject  = "comment"
  active        = false
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("allinkl_cronjob.nightly", "minute", "*/15"),
					resource.TestCheckResourceAttr("allinkl_cronjob.nightly", "http_user", "cron"),
					resource.TestCheckResourceAttr("allinkl_cronjob.nightly", "mail_address", "ops@example.com"),
					resource.TestCheckResourceAttr("allinkl_cronjob.nightly", "active", "false"),
					checkCount(backend, "cronjobs", 1),
				),
			},
			{
				ResourceName:            "allinkl_cronjob.nightly",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"http_password"},
			},
		},
		CheckDestroy: checkCount(backend, "cronjobs", 0),
	})
}

func TestAccCronjob_validation(t *testing.T) {
	startFakeKAS(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccProviderConfig + `
resource "allinkl_cronjob" "bad" {
  comment = "bad"
  url     = "https://example.com/cron.php"
}`,
				ExpectError: regexp.MustCompile(`must not carry a protocol`),
			},
			{
				Config: testAccProviderConfig + `
resource "allinkl_cronjob" "bad" {
  comment = "bad"
  url     = "example.com/cron.php"
  minute  = "every five"
}`,
				ExpectError: regexp.MustCompile(`must be cron syntax`),
			},
		},
	})
}
