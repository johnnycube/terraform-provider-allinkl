# A certificate obtained elsewhere, for example by an ACME client that solved
# the DNS-01 challenge through allinkl_dns_record.
resource "allinkl_tls_certificate" "www" {
  host         = "www.example.com"
  certificate  = file("${path.module}/certs/www.example.com.crt")
  private_key  = file("${path.module}/certs/www.example.com.key")
  bundle       = file("${path.module}/certs/chain.pem")
  force_https  = true
  hsts_max_age = 31536000
}
