data "allinkl_ftp_users" "all" {}

output "ftp_logins_with_write_access" {
  value = [for u in data.allinkl_ftp_users.all.users : u.login if u.write]
}
