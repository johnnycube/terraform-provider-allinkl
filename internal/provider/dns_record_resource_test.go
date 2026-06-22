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

func TestAccDNSRecord_lifecycle(t *testing.T) {
	backend := startFakeKAS(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create
			{
				Config: testAccProviderConfig + `
resource "allinkl_dns_record" "www" {
  zone = "example.com"
  name = "www"
  type = "A"
  data = "203.0.113.10"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("allinkl_dns_record.www", "id"),
					resource.TestCheckResourceAttr("allinkl_dns_record.www", "name", "www"),
					resource.TestCheckResourceAttr("allinkl_dns_record.www", "type", "A"),
					resource.TestCheckResourceAttr("allinkl_dns_record.www", "data", "203.0.113.10"),
					resource.TestCheckResourceAttr("allinkl_dns_record.www", "aux", "0"),
				),
			},
			// In-place update of data
			{
				Config: testAccProviderConfig + `
resource "allinkl_dns_record" "www" {
  zone = "example.com"
  name = "www"
  type = "A"
  data = "203.0.113.20"
}`,
				Check: resource.TestCheckResourceAttr("allinkl_dns_record.www", "data", "203.0.113.20"),
			},
			// Changing the type must force a replacement (new id)
			{
				Config: testAccProviderConfig + `
resource "allinkl_dns_record" "www" {
  zone = "example.com"
  name = "www"
  type = "TXT"
  data = "v=spf1 -all"
}`,
				Check: resource.TestCheckResourceAttr("allinkl_dns_record.www", "type", "TXT"),
			},
			// Import round-trip
			{
				ResourceName:      "allinkl_dns_record.www",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs, ok := s.RootModule().Resources["allinkl_dns_record.www"]
					if !ok {
						return "", fmt.Errorf("resource not found in state")
					}
					return "example.com/" + rs.Primary.ID, nil
				},
			},
		},
		CheckDestroy: func(_ *terraform.State) error {
			if n := backend.dnsCount(); n != 0 {
				return fmt.Errorf("expected all records destroyed, %d left", n)
			}
			return nil
		},
	})
}
