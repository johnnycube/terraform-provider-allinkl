// Copyright (c) 2026 Johannes Küber
// SPDX-License-Identifier: MPL-2.0
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/johnnycube/terraform-provider-allinkl/internal/fakekas"
)

// backendDomain fails when the fake no longer knows the domain.
func backendDomain(b *fakekas.Backend, name string) (string, error) {
	out, fault := b.Handle("get_domains", map[string]any{"domain_name": name})
	if fault != "" || out == "" {
		return "", fmt.Errorf("domain %s vanished from the backend (fault %q)", name, fault)
	}
	return out, nil
}

func TestAccDomainSettings_lifecycle(t *testing.T) {
	backend := startFakeKAS(t)
	backend.SeedDomain("example.com", "/web/")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Adopting the domain without settings changes nothing.
			{
				Config: testAccProviderConfig + `
resource "allinkl_domain_settings" "main" {
  domain = "example.com"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("allinkl_domain_settings.main", "id", "example.com"),
					resource.TestCheckResourceAttr("allinkl_domain_settings.main", "path", "/web/"),
					resource.TestCheckResourceAttr("allinkl_domain_settings.main", "php_version", "8.4"),
					resource.TestCheckResourceAttr("allinkl_domain_settings.main", "active", "true"),
					resource.TestCheckResourceAttr("allinkl_domain_settings.main", "tls.active", "false"),
					resource.TestCheckResourceAttrSet("allinkl_domain_settings.main", "dkim_selector"),
				),
			},
			{
				Config: testAccProviderConfig + `
resource "allinkl_domain_settings" "main" {
  domain          = "example.com"
  path            = "https://www.example.com"
  redirect_status = 301
  php_version     = "8.3"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("allinkl_domain_settings.main", "path", "https://www.example.com"),
					resource.TestCheckResourceAttr("allinkl_domain_settings.main", "redirect_status", "301"),
					resource.TestCheckResourceAttr("allinkl_domain_settings.main", "php_version", "8.3"),
				),
			},
			{
				ResourceName:      "allinkl_domain_settings.main",
				ImportState:       true,
				ImportStateId:     "example.com",
				ImportStateVerify: true,
			},
		},
		// Destroy forgets the domain; the fake still has it.
		CheckDestroy: func(_ *terraform.State) error {
			if _, err := backendDomain(backend, "example.com"); err != nil {
				return err
			}
			return nil
		},
	})
}

func TestAccDomainSettings_unknownDomain(t *testing.T) {
	startFakeKAS(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccProviderConfig + `
resource "allinkl_domain_settings" "nope" {
  domain = "missing.example"
}`,
				ExpectError: regexp.MustCompile(`not hosted in this KAS account`),
			},
		},
	})
}
