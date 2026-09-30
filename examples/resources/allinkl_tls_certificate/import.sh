# KAS returns the certificate but never the key; the next apply re-installs the pair.
terraform import allinkl_tls_certificate.www www.example.com
