# The domain exists in the account already; the resource adopts its settings.
resource "allinkl_domain_settings" "main" {
  domain      = "example.com"
  path        = "/example.com/"
  php_version = "8.4"
}

output "example_com_uses_lets_encrypt" {
  value = allinkl_domain_settings.main.tls.lets_encrypt
}
