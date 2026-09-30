# home.example.com follows the address a router reports with this login.
resource "allinkl_ddns_user" "home" {
  zone       = "example.com"
  label      = "home"
  comment    = "at home"
  password   = var.ddns_password
  target_ip  = "203.0.113.4"
  dual_stack = true
}
