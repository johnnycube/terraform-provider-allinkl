resource "allinkl_database" "shop" {
  comment       = "shop"
  password      = var.database_password
  allowed_hosts = ["203.0.113.7"]
}

output "shop_database" {
  value = allinkl_database.shop.name
}
