# A login that may read and list the access logs, nothing else.
resource "allinkl_ftp_user" "logs" {
  path     = "/logs/"
  comment  = "log reader"
  password = var.ftp_password
  write    = false
}
