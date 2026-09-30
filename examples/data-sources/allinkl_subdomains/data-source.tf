data "allinkl_subdomains" "all" {}

output "subdomains_without_tls" {
  value = [for s in data.allinkl_subdomains.all.subdomains : s.fqdn if !s.tls.active]
}
