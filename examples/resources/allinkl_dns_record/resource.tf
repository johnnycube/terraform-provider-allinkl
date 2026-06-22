resource "allinkl_dns_record" "www" {
  zone = "example.com"
  name = "www"
  type = "A"
  data = "203.0.113.10"
}
