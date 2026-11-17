#!/bin/sh
# The Flutter setup action downloads a large SDK via curl. Its default HTTP/2
# transport has failed with curl(92) on hosted runners. Scope HTTP/1.1 to that
# action only; do not change the user's/global curl config or TLS verification.
# Keep the action's existing retry behavior: retry-all-errors with redirected
# stdout could concatenate a partial archive and a retried archive.
set -eu
: "${RUNNER_TEMP:?RUNNER_TEMP is required}"
: "${GITHUB_ENV:?GITHUB_ENV is required}"
curl_home="$RUNNER_TEMP/clashforge-flutter-curl"
mkdir -p "$curl_home"
printf '%s\n' 'http1.1' > "$curl_home/.curlrc"
printf 'FLUTTER_CURL_HOME=%s\n' "$curl_home" >> "$GITHUB_ENV"
