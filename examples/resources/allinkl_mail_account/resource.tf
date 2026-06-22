resource "allinkl_mail_account" "info" {
  local_part     = "info"
  domain         = "example.com"
  password       = var.mailbox_password
  copy_addresses = ["archive@example.org"]
}
