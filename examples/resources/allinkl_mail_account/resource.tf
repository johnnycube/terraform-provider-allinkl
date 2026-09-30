resource "allinkl_mail_account" "info" {
  local_part     = "info"
  domain         = "example.com"
  password       = var.mailbox_password
  copy_addresses = ["archive@example.org"]

  # Addresses this mailbox may send from. Receiving under an alias
  # is a separate allinkl_mail_forward.
  sender_aliases = ["contact@example.com"]

  # Standard filters: names from data.allinkl_mail_filters, content
  # filters with an action.
  filters = ["rspamd", "spamc_move:move=Junk"]
}

# A mailbox that only the office network and webmail may access, with an
# out-of-office reply for a fixed window.
resource "allinkl_mail_account" "office" {
  local_part      = "office"
  domain          = "example.com"
  password        = var.office_password
  allowed_clients = ["203.0.113.0/24", "webmail"]

  responder = {
    text  = "The office is closed until 4 January."
    start = "2026-12-24T00:00:00+01:00"
    end   = "2027-01-04T00:00:00+01:00"
  }
}
