resource "allinkl_mail_forward" "sales" {
  local_part = "sales"
  domain     = "example.com"
  targets    = ["alice@example.org", "bob@example.org"]
}
