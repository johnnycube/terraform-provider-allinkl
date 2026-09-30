data "allinkl_mail_filters" "all" {}

# Filter names go into allinkl_mail_account.filters.
output "recommended_mail_filters" {
  value = [for f in data.allinkl_mail_filters.all.filters : f.name if f.recommended]
}
