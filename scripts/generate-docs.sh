#!/usr/bin/env bash
# Regenerate docs/ with tfplugindocs. With a terraform binary on PATH,
# tfplugindocs exports the provider schema itself. Without one it downloads
# Terraform through hc-install, whose 60 s timeout a slow connection exceeds;
# then the schema is exported through tofu instead.
set -euo pipefail
cd "$(dirname "$0")/.."

if command -v terraform >/dev/null; then
  exec go tool tfplugindocs generate --provider-name allinkl --rendered-provider-name allinkl
fi
command -v tofu >/dev/null || { echo "need terraform or tofu on PATH" >&2; exit 1; }

W=$(mktemp -d)
trap 'rm -rf "$W"' EXIT
go build -o "$W/bin/terraform-provider-allinkl" .
# tfplugindocs looks the schema up under registry.terraform.io/hashicorp/<name>.
cat > "$W/main.tf" <<'TF'
terraform {
  required_providers {
    allinkl = { source = "registry.terraform.io/hashicorp/allinkl" }
  }
}
TF
cat > "$W/dev.tofurc" <<RC
provider_installation {
  dev_overrides { "registry.terraform.io/hashicorp/allinkl" = "$W/bin" }
  direct {}
}
RC
( cd "$W" && TF_CLI_CONFIG_FILE="$W/dev.tofurc" tofu providers schema -json > "$W/schema.json" )
go tool tfplugindocs generate --providers-schema "$W/schema.json" --provider-name allinkl --rendered-provider-name allinkl
