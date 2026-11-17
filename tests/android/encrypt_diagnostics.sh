#!/usr/bin/env bash
# Encrypt diagnostic logs before upload; the decryption key stays off GitHub.
# No raw log, subscription endpoint, or credential may be printed/uploaded.
set -euo pipefail
umask 077
: "${RUNNER_TEMP:?RUNNER_TEMP is required}"
: "${GITHUB_WORKSPACE:?GITHUB_WORKSPACE is required}"
recipient="$GITHUB_WORKSPACE/.github/ci-diagnostics-recipient.pem"
output="$RUNNER_TEMP/clashforge-e2e-diagnostics.cms"
files=()
for name in clashforge-e2e-logcat.txt clashforge-e2e-flutter.txt; do
  [ ! -f "$RUNNER_TEMP/$name" ] || files+=("$name")
done
if [ "${#files[@]}" -eq 0 ]; then
  echo 'No native diagnostics were produced; no artifact emitted'
  exit 0
fi
bundle="$(mktemp "$RUNNER_TEMP/clashforge-diagnostics.XXXXXX")"
trap 'rm -f "$bundle"' EXIT
# CMS authenticated encryption. Only the local private RSA key can decrypt.
tar -C "$RUNNER_TEMP" -czf "$bundle" -- "${files[@]}"
openssl cms -encrypt -binary -aes-256-gcm -outform DER \
  -in "$bundle" -out "$output" "$recipient"
echo 'Encrypted diagnostics ready (no plaintext uploaded)'
