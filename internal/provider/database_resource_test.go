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

func TestAccDatabase_lifecycle(t *testing.T) {
	backend := startFakeKAS(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccProviderConfig + `
resource "allinkl_database" "shop" {
  comment  = "shop"
  password = "first-Passw0rd"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestMatchResourceAttr("allinkl_database.shop", "id", regexp.MustCompile(`^d\d+$`)),
					resource.TestCheckResourceAttrPair("allinkl_database.shop", "name", "allinkl_database.shop", "id"),
					resource.TestCheckNoResourceAttr("allinkl_database.shop", "allowed_hosts.#"),
					checkPassword(backend, "allinkl_database.shop", "first-Passw0rd"),
				),
			},
			{
				Config: testAccProviderConfig + `
resource "allinkl_database" "shop" {
  comment       = "shop v2"
  password      = "second-Passw0rd"
  allowed_hosts = ["203.0.113.7", "203.0.113.0/24"]
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("allinkl_database.shop", "comment", "shop v2"),
					resource.TestCheckResourceAttr("allinkl_database.shop", "allowed_hosts.#", "2"),
					checkPassword(backend, "allinkl_database.shop", "second-Passw0rd"),
					checkCount(backend, "databases", 1),
				),
			},
			{
				ResourceName:            "allinkl_database.shop",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password"},
			},
		},
		CheckDestroy: checkCount(backend, "databases", 0),
	})
}
