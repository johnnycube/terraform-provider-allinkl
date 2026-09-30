resource "allinkl_subdomain" "blog" {
  name        = "blog"
  domain      = "example.com"
  path        = "/blog/"
  php_version = "8.4"
}

# A subdomain that redirects: the path holds the target URL.
resource "allinkl_subdomain" "shop" {
  name            = "shop"
  domain          = "example.com"
  path            = "https://shop.example.org"
  redirect_status = 301
}
