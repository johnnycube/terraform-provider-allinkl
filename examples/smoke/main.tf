# Smoke test against a real KAS account. Phase 1 (default) only reads. Phase 2
# (-var write=true) creates disposable objects under a test label and removes
# them again on destroy. Nothing that exists before the test is changed.

terraform {
  required_providers {
    allinkl = {
      source = "registry.terraform.io/johnnycube/allinkl"
    }
  }
}

# Credentials from KAS_LOGIN / KAS_PASSWORD, and KAS_OTP for a 2FA account.
provider "allinkl" {}

variable "domain" {
  description = "A domain of the account to create the test objects under."
  type        = string
}

variable "label" {
  description = "Label for every disposable object, so they are easy to spot in the KAS panel."
  type        = string
  default     = "tfsmoke"
}

variable "write" {
  description = "Phase 2: create the disposable objects."
  type        = bool
  default     = false
}

variable "password" {
  description = "Password for the disposable logins."
  type        = string
  sensitive   = true
  default     = "Smoke-Test-Passw0rd!"
}

# ── Phase 1: read every object type ─────────────────────────────────────────
# The lists read after the phase 2 objects exist, so they show up in `read`.

data "allinkl_domains" "all" {}
data "allinkl_mail_filters" "all" {}
data "allinkl_subdomains" "all" {
  depends_on = [allinkl_subdomain.smoke]
}
data "allinkl_dns_records" "zone" {
  zone       = var.domain
  depends_on = [allinkl_dns_record.smoke]
}
data "allinkl_ftp_users" "all" {
  depends_on = [allinkl_ftp_user.smoke]
}
data "allinkl_databases" "all" {
  depends_on = [allinkl_database.smoke]
}
data "allinkl_cronjobs" "all" {
  depends_on = [allinkl_cronjob.smoke]
}
data "allinkl_ddns_users" "all" {
  depends_on = [allinkl_ddns_user.smoke]
}

output "read" {
  value = {
    domains      = { for d in data.allinkl_domains.all.domains : d.name => { php = d.php_version, active = d.active, tls = d.tls.type } }
    subdomains   = [for s in data.allinkl_subdomains.all.subdomains : s.fqdn]
    dns_records  = length(data.allinkl_dns_records.zone.records)
    ftp_users    = [for u in data.allinkl_ftp_users.all.users : u.login]
    databases    = [for d in data.allinkl_databases.all.databases : d.login]
    cronjobs     = [for j in data.allinkl_cronjobs.all.cronjobs : j.id]
    ddns_users   = [for u in data.allinkl_ddns_users.all.users : u.host]
    mail_filters = [for f in data.allinkl_mail_filters.all.filters : f.name]
  }
}

# ── Phase 2: disposable objects ─────────────────────────────────────────────

resource "allinkl_dns_record" "smoke" {
  count = var.write ? 1 : 0
  zone  = var.domain
  name  = var.label
  type  = "TXT"
  data  = "smoke test, safe to delete"
}

resource "allinkl_subdomain" "smoke" {
  count       = var.write ? 1 : 0
  name        = var.label
  domain      = var.domain
  path        = "/${var.label}/"
  php_version = "8.4"
}

resource "allinkl_mail_account" "smoke" {
  count      = var.write ? 1 : 0
  local_part = var.label
  domain     = var.domain
  password   = var.password
  responder = {
    text = "smoke test"
  }
}

resource "allinkl_mail_forward" "smoke" {
  count      = var.write ? 1 : 0
  local_part = "${var.label}-forward"
  domain     = var.domain
  targets    = ["${var.label}@${var.domain}"]
  depends_on = [allinkl_mail_account.smoke]
}

resource "allinkl_ftp_user" "smoke" {
  count    = var.write ? 1 : 0
  path     = "/${var.label}/"
  comment  = var.label
  password = var.password
  write    = false
}

resource "allinkl_database" "smoke" {
  count    = var.write ? 1 : 0
  comment  = var.label
  password = var.password
}

resource "allinkl_cronjob" "smoke" {
  count   = var.write ? 1 : 0
  comment = var.label
  url     = "${var.domain}/${var.label}"
  minute  = "0"
  hour    = "3"
  active  = false # never runs
}

resource "allinkl_ddns_user" "smoke" {
  count     = var.write ? 1 : 0
  zone      = var.domain
  label     = "${var.label}-ddns"
  comment   = var.label
  password  = var.password
  target_ip = "203.0.113.4"
}

output "written" {
  value = var.write ? {
    subdomain = allinkl_subdomain.smoke[0].id
    mailbox   = allinkl_mail_account.smoke[0].id
    ftp_user  = allinkl_ftp_user.smoke[0].id
    database  = allinkl_database.smoke[0].id
    cronjob   = allinkl_cronjob.smoke[0].id
    ddns_user = allinkl_ddns_user.smoke[0].id
  } : null
}
