#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_PATH="${UPKG_BIN:-"$ROOT_DIR/bin/upkg"}"
PKG_DIR="${UPKG_PKG_DIR:-"$ROOT_DIR/pkg-test"}"
RESULTS_DIR="${UPKG_RESULTS_DIR:-"$ROOT_DIR/test-results"}"
ALLOW_SYSTEM="${UPKG_E2E_SYSTEM:-0}"

mkdir -p "$RESULTS_DIR"
BASELINE_FILE="$RESULTS_DIR/baseline.txt"
: >"$BASELINE_FILE"

log() {
  printf '%s\n' "$*" | tee -a "$BASELINE_FILE"
}

log "upkg E2E baseline - $(date)"
log "ROOT_DIR=$ROOT_DIR"
log "BIN_PATH=$BIN_PATH"
log "PKG_DIR=$PKG_DIR"
log "ALLOW_SYSTEM=$ALLOW_SYSTEM"

if [[ ! -x "$BIN_PATH" ]]; then
  log "Binary not found, building..."
  (cd "$ROOT_DIR" && make build)
fi

TMP_HOME="$(mktemp -d)"
trap 'rm -rf "$TMP_HOME"' EXIT
export HOME="$TMP_HOME"

run_case() {
  local label="$1"
  local pkg="$2"
  local name="$3"

  log ""
  log "== Case: $label =="
  log "Installing $pkg as name=$name"

  if ! "$BIN_PATH" install "$pkg" --name "$name" --force --skip-icon-fix --timeout 120; then
    log "FAIL install ($label)"
    return 1
  fi

  if ! "$BIN_PATH" uninstall "$name" --timeout 120; then
    log "FAIL uninstall ($label)"
    return 1
  fi

  log "OK ($label)"
}

failures=0

# AppImage
if [[ -f "$PKG_DIR/MiniKeyboard-x86_64.AppImage" ]]; then
  run_case "appimage" "$PKG_DIR/MiniKeyboard-x86_64.AppImage" "e2e-appimage" || failures=$((failures + 1))
else
  log "SKIP appimage: package not found"
fi

# Binary (use local pkgctl or override)
BIN_PKG="${UPKG_BINARY_PKG:-"$ROOT_DIR/pkgctl"}"
if [[ -f "$BIN_PKG" ]]; then
  run_case "binary" "$BIN_PKG" "e2e-binary" || failures=$((failures + 1))
else
  log "SKIP binary: package not found ($BIN_PKG)"
fi

# Tarball
if [[ -f "$PKG_DIR/gitkraken-amd64.tar.gz" ]]; then
  run_case "tarball" "$PKG_DIR/gitkraken-amd64.tar.gz" "e2e-tarball" || failures=$((failures + 1))
else
  log "SKIP tarball: package not found"
fi

# Zip (handled by tarball backend)
if [[ -f "$PKG_DIR/balenaEtcher-linux-x64-2.1.4.zip" ]]; then
  run_case "zip" "$PKG_DIR/balenaEtcher-linux-x64-2.1.4.zip" "e2e-zip" || failures=$((failures + 1))
else
  log "SKIP zip: package not found"
fi

# DEB (system-modifying via debtap/pacman)
if [[ "$ALLOW_SYSTEM" == "1" ]]; then
  if [[ -f "$PKG_DIR/cursor_2.0.34_amd64.deb" ]]; then
    run_case "deb" "$PKG_DIR/cursor_2.0.34_amd64.deb" "e2e-deb" || failures=$((failures + 1))
  else
    log "SKIP deb: package not found"
  fi
else
  log "SKIP deb: set UPKG_E2E_SYSTEM=1 to enable"
fi

# RPM (safe if rpmextract.sh is present; otherwise system-modifying)
if command -v rpmextract.sh >/dev/null 2>&1; then
  if [[ -f "$PKG_DIR/cursor-2.0.34.el8.x86_64.rpm" ]]; then
    run_case "rpm (rpmextract)" "$PKG_DIR/cursor-2.0.34.el8.x86_64.rpm" "e2e-rpm" || failures=$((failures + 1))
  else
    log "SKIP rpm: package not found"
  fi
elif [[ "$ALLOW_SYSTEM" == "1" ]]; then
  if [[ -f "$PKG_DIR/cursor-2.0.34.el8.x86_64.rpm" ]]; then
    run_case "rpm (debtap/pacman)" "$PKG_DIR/cursor-2.0.34.el8.x86_64.rpm" "e2e-rpm" || failures=$((failures + 1))
  else
    log "SKIP rpm: package not found"
  fi
else
  log "SKIP rpm: rpmextract.sh not found; set UPKG_E2E_SYSTEM=1 to enable fallback"
fi

log ""
if [[ "$failures" -eq 0 ]]; then
  log "E2E baseline completed successfully"
  exit 0
fi

log "E2E baseline completed with $failures failure(s)"
exit 1

