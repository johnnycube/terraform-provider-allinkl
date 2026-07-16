resource "allinkl_mail_account" "info" {
  local_part     = "info"
  domain         = "example.com"
  password       = var.mailbox_password
  copy_addresses = ["archive@example.org"]

  # Addresses this mailbox may send from. Receiving under an alias
  # is a separate allinkl_mail_forward.
  sender_aliases = ["contact@example.com"]
}
