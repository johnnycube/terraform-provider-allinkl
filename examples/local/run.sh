#!/usr/bin/env bash
# Build the provider, start a local fake KAS server, and run a full
# plan/apply/destroy against it - no real all-inkl.com account required.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
RUN_DIR="$SCRIPT_DIR/.run"
BIN_DIR="$RUN_DIR/bin"
ADDR="127.0.0.1:8511"

TOFU="$(command -v tofu || command -v terraform || true)"
[ -n "$TOFU" ] || { echo "need 'tofu' or 'terraform' on PATH" >&2; exit 1; }

mkdir -p "$BIN_DIR"

echo "==> Building provider (against published kasapi v0.1.0)"
( cd "$REPO_ROOT" && GOWORK=off go build -o "$BIN_DIR/terraform-provider-allinkl" . )

echo "==> Building + starting fake KAS server on $ADDR"
( cd "$REPO_ROOT" && GOWORK=off go build -o "$BIN_DIR/fakekas" ./examples/local/fakekas )
"$BIN_DIR/fakekas" -addr "$ADDR" &
FAKE_PID=$!

cleanup() {
  echo "==> Stopping fake KAS server"
  kill "$FAKE_PID" 2>/dev/null || true
  wait "$FAKE_PID" 2>/dev/null || true
}
trap cleanup EXIT

# Wait for the server to accept connections.
for _ in $(seq 1 50); do
  if (exec 3<>/dev/tcp/127.0.0.1/8511) 2>/dev/null; then exec 3>&- 3<&-; break; fi
  sleep 0.1
done

# dev_overrides loads the freshly-built provider straight from disk, so there
# is no registry download and no `tofu init`.
cat > "$RUN_DIR/dev.tofurc" <<EOF
provider_installation {
  dev_overrides {
    "registry.terraform.io/johnnycube/allinkl" = "$BIN_DIR"
  }
  direct {}
}
EOF

export TF_CLI_CONFIG_FILE="$RUN_DIR/dev.tofurc"
export KAS_LOGIN="w0123456"
export KAS_PASSWORD="secret" # the password the fake server expects
export KAS_AUTH_ENDPOINT="http://$ADDR/KasAuth.php"
export KAS_API_ENDPOINT="http://$ADDR/KasApi.php"

cd "$SCRIPT_DIR"
rm -f terraform.tfstate terraform.tfstate.backup

echo "==> tofu plan"
"$TOFU" plan

echo "==> tofu apply"
"$TOFU" apply -auto-approve

echo "==> outputs"
"$TOFU" output

echo "==> tofu destroy"
"$TOFU" destroy -auto-approve

echo "==> Done."
