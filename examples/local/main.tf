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

# ── Hosts ──────────────────────────────────────────────────────────────────

resource "allinkl_subdomain" "blog" {
  name        = "blog"
  domain      = "example.com"
  path        = "/blog/"
  php_version = "8.4"
}

resource "allinkl_subdomain" "shop" {
  name            = "shop"
  domain          = "example.com"
  path            = "https://shop.example.org"
  redirect_status = 301
}

resource "allinkl_domain_settings" "main" {
  domain      = "example.com"
  php_version = "8.4"
}

# ── FTP, database, cronjob, dynamic DNS ────────────────────────────────────

resource "allinkl_ftp_user" "logs" {
  path     = "/logs/"
  comment  = "log reader"
  password = "fake-ftp-pw"
  write    = false
}

resource "allinkl_database" "shop" {
  comment       = "shop"
  password      = "fake-db-pw"
  allowed_hosts = ["203.0.113.7"]
}

resource "allinkl_cronjob" "nightly" {
  comment = "nightly import"
  url     = "example.com/cron.php"
  minute  = "30"
  hour    = "3"
}

resource "allinkl_ddns_user" "home" {
  zone      = "example.com"
  label     = "home"
  comment   = "at home"
  password  = "fake-ddns-pw"
  target_ip = "203.0.113.4"
}

# ── Read-only data sources (the fake server seeds two domains) ──────────────

data "allinkl_domains" "all" {}

data "allinkl_ftp_users" "all" {
  depends_on = [allinkl_ftp_user.logs]
}

data "allinkl_mail_filters" "all" {}

output "hosted_domains" {
  value = data.allinkl_domains.all.domains[*].name
}

output "ftp_logins" {
  value = data.allinkl_ftp_users.all.users[*].login
}

output "recommended_mail_filters" {
  value = [for f in data.allinkl_mail_filters.all.filters : f.name if f.recommended]
}

output "mailbox_login" {
  value = allinkl_mail_account.info.id # KAS login, e.g. m1000000
}
