data "allinkl_databases" "all" {}

output "database_names" {
  value = data.allinkl_databases.all.databases[*].name
}
