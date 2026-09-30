data "allinkl_cronjobs" "all" {}

output "inactive_cronjobs" {
  value = [for j in data.allinkl_cronjobs.all.cronjobs : j.comment if !j.active]
}
