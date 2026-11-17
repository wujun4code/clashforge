# Optional HTTP rule-provider last-known-good fix

This directory carries an **external Mihomo source patch**, not a change to the
ClashForge Go module, mobile embedded engine, or default downloaded binary.

- Upstream: `MetaCubeX/mihomo`, tag `v1.19.32`.
- Exact base commit: `88dcbf7f1614a67c3b36b848ee3592dfa92ada36`.
- Patch: `v1.19.32-external-lkg1.patch` (one implementation and two test files).
- Locally built version: `v1.19.32-external-lkg1`. Do not label it an official build.

## Behavior

Reject HTTP rule-provider updates that parse to zero rules before committing
accepted cache, content hash, successful-update timestamp, or runtime rules.
An invalid update must return an error rather than replace last-known-good rules.

This deliberately also rejects an intentionally empty HTTP feed. File/inline
empty feeds remain unchanged. It is **not** strict validation of every YAML line
or protection against a non-empty but semantically incomplete/malicious feed.
Auxiliary request metadata such as ETag/backoff is outside the zero-side-effect
promise.

## Verification and use

Against the exact upstream base, the patch was independently replayed and tested,
including race tests for `rules/provider` and `component/resource` with the
`with_gvisor` build tag. Real AMD64 controller fixtures for empty body, malformed
YAML, and empty payload returned HTTP 503 while retaining the previous rule and
cache; valid recovery returned HTTP 204. ARM64 was cross-built, not executed.

To inspect/apply it in a separate clean upstream checkout:

```sh
git checkout 88dcbf7f1614a67c3b36b848ee3592dfa92ada36
git apply --check /path/to/v1.19.32-external-lkg1.patch
git apply /path/to/v1.19.32-external-lkg1.patch
GOMAXPROCS=2 go test -race -p 2 -tags with_gvisor ./rules/provider ./component/resource
```

No patched binary is committed here. Adoption, versioning, distribution and any
router/mobile installation require a separate explicit decision. ClashForge CI
continues to use the pinned official core rather than silently substituting this
external build.
