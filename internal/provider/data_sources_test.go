// Copyright (c) 2026 Johannes Küber
// SPDX-License-Identifier: MPL-2.0
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccDNSRecordsDataSource(t *testing.T) {
	backend := startFakeKAS(t)
	backend.seedDNSRecord("www", "A", "203.0.113.10", "0")
	backend.seedDNSRecord("", "MX", "mail.example.com.", "10")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccProviderConfig + `
data "allinkl_dns_records" "all" {
  zone = "example.com"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.allinkl_dns_records.all", "records.#", "2"),
					resource.TestCheckResourceAttr("data.allinkl_dns_records.all", "records.0.name", "www"),
					resource.TestCheckResourceAttr("data.allinkl_dns_records.all", "records.0.type", "A"),
					resource.TestCheckResourceAttr("data.allinkl_dns_records.all", "records.1.type", "MX"),
					resource.TestCheckResourceAttr("data.allinkl_dns_records.all", "records.1.aux", "10"),
					resource.TestCheckResourceAttr("data.allinkl_dns_records.all", "records.1.changeable", "true"),
				),
			},
		},
	})
}

func TestAccDomainsDataSource(t *testing.T) {
	backend := startFakeKAS(t)
	backend.seedDomain("example.com", "/web/")
	backend.seedDomain("example.org", "/org/")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccProviderConfig + `
data "allinkl_domains" "all" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.allinkl_domains.all", "domains.#", "2"),
					resource.TestCheckResourceAttr("data.allinkl_domains.all", "domains.0.name", "example.com"),
					resource.TestCheckResourceAttr("data.allinkl_domains.all", "domains.0.path", "/web/"),
					resource.TestCheckResourceAttr("data.allinkl_domains.all", "domains.1.name", "example.org"),
				),
			},
		},
	})
}
