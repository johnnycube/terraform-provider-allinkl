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
	backend.SeedDNSRecord("www", "A", "203.0.113.10", "0")
	backend.SeedDNSRecord("", "MX", "mail.example.com.", "10")

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
	backend.SeedDomain("example.com", "/web/")
	backend.SeedDomain("example.org", "/org/")

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
					resource.TestCheckResourceAttr("data.allinkl_domains.all", "domains.0.php_version", "8.4"),
					resource.TestCheckResourceAttr("data.allinkl_domains.all", "domains.0.active", "true"),
					resource.TestCheckResourceAttr("data.allinkl_domains.all", "domains.0.redirect_status", "0"),
					resource.TestCheckResourceAttr("data.allinkl_domains.all", "domains.0.tls.lets_encrypt", "false"),
					resource.TestCheckResourceAttrSet("data.allinkl_domains.all", "domains.0.dkim_selector"),
					resource.TestCheckResourceAttr("data.allinkl_domains.all", "domains.1.name", "example.org"),
				),
			},
		},
	})
}

func TestAccListDataSources(t *testing.T) {
	backend := startFakeKAS(t)
	backend.SeedDomain("example.com", "/web/")
	backend.SeedFTPUser("w0123456", "/", "account login", true)
	backend.SeedFTPUser("f0000001", "/logs/", "log reader", false)
	backend.SeedDatabase("d0000001", "shop", "203.0.113.7")
	backend.SeedCronjob("example.com/cron.php", "nightly")
	backend.SeedDDNSUser("example.com", "home", "203.0.113.4")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccProviderConfig + `
resource "allinkl_subdomain" "blog" {
  name   = "blog"
  domain = "example.com"
  path   = "/blog/"
}
data "allinkl_subdomains" "all" {
  depends_on = [allinkl_subdomain.blog]
}
data "allinkl_ftp_users" "all" {}
data "allinkl_databases" "all" {}
data "allinkl_cronjobs" "all" {}
data "allinkl_ddns_users" "all" {}
data "allinkl_mail_filters" "all" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.allinkl_subdomains.all", "subdomains.#", "1"),
					resource.TestCheckResourceAttr("data.allinkl_subdomains.all", "subdomains.0.fqdn", "blog.example.com"),
					resource.TestCheckResourceAttr("data.allinkl_subdomains.all", "subdomains.0.php_version", "8.4"),
					resource.TestCheckResourceAttr("data.allinkl_subdomains.all", "subdomains.0.tls.active", "false"),
					resource.TestCheckResourceAttr("data.allinkl_ftp_users.all", "users.#", "2"),
					resource.TestCheckResourceAttr("data.allinkl_ftp_users.all", "users.0.login", "f0000001"),
					resource.TestCheckResourceAttr("data.allinkl_ftp_users.all", "users.0.path", "/logs/"),
					resource.TestCheckResourceAttr("data.allinkl_ftp_users.all", "users.1.main_user", "true"),
					resource.TestCheckResourceAttr("data.allinkl_databases.all", "databases.#", "1"),
					resource.TestCheckResourceAttr("data.allinkl_databases.all", "databases.0.allowed_hosts.0", "203.0.113.7"),
					resource.TestCheckResourceAttr("data.allinkl_cronjobs.all", "cronjobs.#", "1"),
					resource.TestCheckResourceAttr("data.allinkl_cronjobs.all", "cronjobs.0.url", "example.com/cron.php"),
					resource.TestCheckResourceAttr("data.allinkl_cronjobs.all", "cronjobs.0.active", "true"),
					resource.TestCheckResourceAttr("data.allinkl_ddns_users.all", "users.#", "1"),
					resource.TestCheckResourceAttr("data.allinkl_ddns_users.all", "users.0.host", "home.example.com"),
					resource.TestCheckResourceAttr("data.allinkl_ddns_users.all", "users.0.current_ip", "203.0.113.4"),
					resource.TestCheckResourceAttr("data.allinkl_mail_filters.all", "filters.#", "3"),
					resource.TestCheckResourceAttr("data.allinkl_mail_filters.all", "filters.0.name", "rspamd"),
					resource.TestCheckResourceAttr("data.allinkl_mail_filters.all", "filters.0.recommended", "true"),
				),
			},
		},
	})
}

func TestAccListDataSources_empty(t *testing.T) {
	// KAS answers an account without DDNS users with the fault empty_list.
	startFakeKAS(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccProviderConfig + `
data "allinkl_ddns_users" "all" {}
data "allinkl_ftp_users" "all" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.allinkl_ddns_users.all", "users.#", "0"),
					resource.TestCheckResourceAttr("data.allinkl_ftp_users.all", "users.#", "0"),
				),
			},
		},
	})
}
