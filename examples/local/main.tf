terraform {
  required_providers {
    allinkl = {
      source = "registry.terraform.io/johnnycube/allinkl"
    }
  }
}

# Credentials are read from KAS_LOGIN / KAS_PASSWORD, and the endpoints from
# KAS_API_ENDPOINT / KAS_AUTH_ENDPOINT - run.sh points all four at the local
# fake KAS server, so no real all-inkl.com account is involved.
provider "allinkl" {}

# ── DNS records ────────────────────────────────────────────────────────────

resource "allinkl_dns_record" "www" {
  zone = "example.com"
  name = "www"
  type = "A"
  data = "203.0.113.10"
}

resource "allinkl_dns_record" "mx" {
  zone = "example.com"
  name = ""
  type = "MX"
  data = "mail.example.com."
  aux  = 10
}

# ── Email ──────────────────────────────────────────────────────────────────

resource "allinkl_mail_account" "info" {
  local_part     = "info"
  domain         = "example.com"
  password       = "fake-mailbox-pw"
  copy_addresses = ["archive@example.org"]
}

resource "allinkl_mail_forward" "sales" {
  local_part = "sales"
  domain     = "example.com"
  targets    = ["alice@example.org", "bob@example.org"]
}

# ── Subdomain ──────────────────────────────────────────────────────────────

resource "allinkl_subdomain" "blog" {
  name   = "blog"
  domain = "example.com"
  path   = "/blog/"
}

# ── Read-only data sources (the fake server seeds two domains) ──────────────

data "allinkl_domains" "all" {}

output "hosted_domains" {
  value = data.allinkl_domains.all.domains[*].name
}

output "mailbox_login" {
  value = allinkl_mail_account.info.id # KAS login, e.g. m1000000
}
