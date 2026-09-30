#!/usr/bin/env bash
# Smoke test: ./run.sh <domain> [write]
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
RUN_DIR="$SCRIPT_DIR/.run"
BIN_DIR="$RUN_DIR/bin"
DOMAIN="${1:?usage: $0 <domain> [write]}"
WRITE="${2:-}"

: "${KAS_LOGIN:?set KAS_LOGIN}"
: "${KAS_PASSWORD:?set KAS_PASSWORD}"
TOFU="$(command -v tofu || command -v terraform || true)"
[ -n "$TOFU" ] || { echo "need 'tofu' or 'terraform' on PATH" >&2; exit 1; }

mkdir -p "$BIN_DIR"
echo "==> Building provider from $REPO_ROOT"
( cd "$REPO_ROOT" && GOWORK=off go build -o "$BIN_DIR/terraform-provider-allinkl" . )

cat > "$RUN_DIR/dev.tofurc" <<CFG
provider_installation {
  dev_overrides {
    "registry.terraform.io/johnnycube/allinkl" = "$BIN_DIR"
  }
  direct {}
}
CFG
export TF_CLI_CONFIG_FILE="$RUN_DIR/dev.tofurc"
export TF_DATA_DIR="$RUN_DIR/.terraform"
export TF_VAR_domain="$DOMAIN"
STATE="$RUN_DIR/terraform.tfstate"

cd "$SCRIPT_DIR"
if [ "$WRITE" != "write" ]; then
  echo "==> Phase 1: read only"
  "$TOFU" plan -state="$STATE" -var write=false
  "$TOFU" apply -state="$STATE" -var write=false -auto-approve
  "$TOFU" output -state="$STATE" -json read
  exit 0
fi

echo "==> Phase 2: disposable objects under label 'tfsmoke' in $DOMAIN"
"$TOFU" plan -state="$STATE" -var write=true
read -r -p "Apply? [y/N] " answer
[ "$answer" = "y" ] || exit 0
"$TOFU" apply -state="$STATE" -var write=true -auto-approve
"$TOFU" output -state="$STATE" -json written
echo "==> Plan again: a second plan must show no changes"
"$TOFU" plan -state="$STATE" -var write=true -detailed-exitcode || echo "!! second plan shows changes: a field is read back differently than it was written"
read -r -p "Destroy the disposable objects? [y/N] " answer
[ "$answer" = "y" ] || exit 0
"$TOFU" destroy -state="$STATE" -var write=true -auto-approve
echo "==> Done. Check the KAS panel: nothing named tfsmoke should remain."
