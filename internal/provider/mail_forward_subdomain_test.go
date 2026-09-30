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
			if n := backend.Count("forwards"); n != 0 {
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
					resource.TestCheckResourceAttr("allinkl_subdomain.blog", "redirect_status", "0"),
					resource.TestCheckResourceAttr("allinkl_subdomain.blog", "active", "true"),
					// KAS chose the PHP version; the fake answers 8.4.
					resource.TestCheckResourceAttr("allinkl_subdomain.blog", "php_version", "8.4"),
					resource.TestCheckResourceAttr("allinkl_subdomain.blog", "tls.active", "false"),
					resource.TestCheckResourceAttr("allinkl_subdomain.blog", "tls.hsts_max_age", "-1"),
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
			// Redirect, PHP version and deactivation
			{
				Config: testAccProviderConfig + `
resource "allinkl_subdomain" "blog" {
  name            = "blog"
  domain          = "example.com"
  path            = "https://example.org/blog"
  redirect_status = 301
  php_version     = "8.3"
  active          = false
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("allinkl_subdomain.blog", "path", "https://example.org/blog"),
					resource.TestCheckResourceAttr("allinkl_subdomain.blog", "redirect_status", "301"),
					resource.TestCheckResourceAttr("allinkl_subdomain.blog", "php_version", "8.3"),
					resource.TestCheckResourceAttr("allinkl_subdomain.blog", "active", "false"),
				),
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
			if n := backend.Count("subdomains"); n != 0 {
				return fmt.Errorf("expected all subdomains destroyed, %d left", n)
			}
			return nil
		},
	})
}

func TestAccSubdomain_createInactive(t *testing.T) {
	// KAS refuses the active flag on create; the resource applies it in a
	// second call.
	startFakeKAS(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccProviderConfig + `
resource "allinkl_subdomain" "parked" {
  name   = "parked"
  domain = "example.com"
  active = false
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("allinkl_subdomain.parked", "active", "false"),
					resource.TestCheckResourceAttr("allinkl_subdomain.parked", "path", "/"),
				),
			},
		},
	})
}
