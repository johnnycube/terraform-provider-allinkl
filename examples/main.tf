terraform {
  required_providers {
    allinkl = {
      source = "registry.terraform.io/johnnycube/allinkl"
    }
  }
}

provider "allinkl" {
  # Or set KAS_LOGIN / KAS_PASSWORD environment variables instead.
  login    = var.kas_login
  password = var.kas_password
}

variable "kas_login" { type = string }
variable "kas_password" {
  type      = string
  sensitive = true
}

# A record for www.example.com
resource "allinkl_dns_record" "www" {
  zone = "example.com"
  name = "www"
  type = "A"
  data = "203.0.113.10"
}

# MX record at the zone apex (aux = priority)
resource "allinkl_dns_record" "mx" {
  zone = "example.com"
  name = ""
  type = "MX"
  data = "mail.example.com."
  aux  = 10
}

# TXT record, e.g. SPF
resource "allinkl_dns_record" "spf" {
  zone = "example.com"
  name = ""
  type = "TXT"
  data = "v=spf1 mx -all"
}

# Inspect an existing zone
data "allinkl_dns_records" "all" {
  zone = "example.com"
}

output "existing_records" {
  value = data.allinkl_dns_records.all.records
}

# ── Email ────────────────────────────────────────────────────────────────

variable "mailbox_password" {
  type      = string
  sensitive = true
}

# A mailbox info@example.com. The password is write-only against the API but
# (as with all Terraform secrets) ends up in the state file - protect it.
resource "allinkl_mail_account" "info" {
  local_part = "info"
  domain     = "example.com"
  password   = var.mailbox_password

  copy_addresses = ["archive@example.org"]
}

# Forward sales@example.com to two external addresses.
resource "allinkl_mail_forward" "sales" {
  local_part = "sales"
  domain     = "example.com"
  targets = [
    "alice@example.org",
    "bob@example.org",
  ]
}

output "mailbox_login" {
  value = allinkl_mail_account.info.id # KAS login, e.g. m1234567
}

# ── Subdomains & domains ─────────────────────────────────────────────────

resource "allinkl_subdomain" "blog" {
  name   = "blog"
  domain = "example.com"
  path   = "/blog/"
}

data "allinkl_domains" "all" {}

output "hosted_domains" {
  value = data.allinkl_domains.all.domains[*].name
}
