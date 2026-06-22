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

func TestAccMailForward_lifecycle(t *testing.T) {
	backend := startFakeKAS(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccProviderConfig + `
resource "allinkl_mail_forward" "sales" {
  local_part = "sales"
  domain     = "example.com"
  targets    = ["alice@example.org"]
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("allinkl_mail_forward.sales", "id", "sales@example.com"),
					resource.TestCheckResourceAttr("allinkl_mail_forward.sales", "targets.#", "1"),
				),
			},
			// In-place target update
			{
				Config: testAccProviderConfig + `
resource "allinkl_mail_forward" "sales" {
  local_part = "sales"
  domain     = "example.com"
  targets    = ["alice@example.org", "bob@example.org"]
}`,
				Check: resource.TestCheckResourceAttr("allinkl_mail_forward.sales", "targets.#", "2"),
			},
			// Import by source address
			{
				ResourceName:      "allinkl_mail_forward.sales",
				ImportState:       true,
				ImportStateId:     "sales@example.com",
				ImportStateVerify: true,
			},
		},
		CheckDestroy: func(_ *terraform.State) error {
			if n := backend.forwardCount(); n != 0 {
				return fmt.Errorf("expected all forwards destroyed, %d left", n)
			}
			return nil
		},
	})
}

func TestAccSubdomain_lifecycle(t *testing.T) {
	backend := startFakeKAS(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccProviderConfig + `
resource "allinkl_subdomain" "blog" {
  name   = "blog"
  domain = "example.com"
  path   = "/blog/"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("allinkl_subdomain.blog", "id", "blog.example.com"),
					resource.TestCheckResourceAttr("allinkl_subdomain.blog", "path", "/blog/"),
				),
			},
			// In-place path update
			{
				Config: testAccProviderConfig + `
resource "allinkl_subdomain" "blog" {
  name   = "blog"
  domain = "example.com"
  path   = "/www/blog/"
}`,
				Check: resource.TestCheckResourceAttr("allinkl_subdomain.blog", "path", "/www/blog/"),
			},
			// Import by FQDN
			{
				ResourceName:      "allinkl_subdomain.blog",
				ImportState:       true,
				ImportStateId:     "blog.example.com",
				ImportStateVerify: true,
			},
		},
		CheckDestroy: func(_ *terraform.State) error {
			if n := backend.subdomainCount(); n != 0 {
				return fmt.Errorf("expected all subdomains destroyed, %d left", n)
			}
			return nil
		},
	})
}
