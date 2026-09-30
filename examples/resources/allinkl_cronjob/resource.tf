resource "allinkl_cronjob" "nightly" {
  comment = "nightly import"
  url     = "example.com/cron.php"
  minute  = "30"
  hour    = "3"

  http_user     = "cron"
  http_password = var.cron_password
  mail_address  = "ops@example.com"
  mail_subject  = "comment"
}
