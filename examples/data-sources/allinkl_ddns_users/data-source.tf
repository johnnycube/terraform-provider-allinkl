data "allinkl_ddns_users" "all" {}

output "dynamic_hosts" {
  value = { for u in data.allinkl_ddns_users.all.users : u.host => u.current_ip }
}
