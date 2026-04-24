# GitHub PR Auto Merge, Tag, and Release Plan

> **Status:** Proposed roadmap item
> **Scope:** GitHub Actions workflow design
> **Goal:** Implement a stable pipeline for `PR -> CI -> auto merge -> auto tag -> auto release`

---

## Goal

Create a reliable GitHub Actions pipeline so that when a pull request targeting `main` passes CI and satisfies merge conditions, the repository will:

1. automatically merge the PR
2. automatically create the next version tag
3. automatically build and publish a GitHub Release

---

## Current Repository State

The repository already contains these workflow files:

- `.github/workflows/ci.yml`
- `.github/workflows/auto-tag.yml`
- `.github/workflows/release.yml`

Current behavior is already close to the target design:

- `ci.yml` runs on pull requests to `main`
- `auto-tag.yml` listens for CI completion via `workflow_run`
- `auto-tag.yml` merges the PR, computes a tag, pushes the tag, and dispatches `release.yml`
- `release.yml` builds artifacts and creates a GitHub Release

So this roadmap is not a greenfield design. It is a hardening and cleanup plan for the existing implementation.

---

## Recommended End-State Architecture

Recommended control flow:

```text
pull_request
  -> ci.yml
  -> workflow_run(auto-tag.yml)
  -> merge PR
  -> create tag on merged main
  -> workflow_dispatch(release.yml)
  -> build artifacts and publish release
```

### Workflow responsibilities

#### `ci.yml`
Responsible only for validation:
- install dependencies
- build UI
- build Go code
- run tests and optional lint/static checks

#### `auto-tag.yml`
Responsible only for orchestration:
- verify auto-merge conditions
- merge PR
- sync latest `main`
- check idempotency
- compute next tag
- push tag
- dispatch `release.yml`

#### `release.yml`
Responsible only for release build/publish:
- compute version from tag/input
- build UI and binaries
- package artifacts
- generate checksums
- create or update GitHub Release

---

## Recommended Merge Policy

Do **not** merge every green PR automatically.

Recommended auto-merge gate:

- base branch is `main`
- PR is not draft
- CI has succeeded
- PR has label `automerge`

Optional future gate:
- at least one review approval
- PR has no blocking label such as `do-not-merge`

### Why use an `automerge` label

This gives a simple manual approval switch:

- apply `automerge` -> PR may merge automatically after CI passes
- do not apply `automerge` -> PR stays manual

---

## Recommended Versioning Strategy

### Preferred default
Automatically publish **alpha** versions for merged PRs.

Examples:
- `v0.1.0-alpha.1`
- `v0.1.0-alpha.2`
- `v0.1.0-alpha.3`

### Stable releases
Stable versions such as:
- `v0.1.0`
- `v0.1.1`

should be released manually, not on every merged PR.

### Reasoning
The automated pipeline is best treated as continuous delivery of prerelease artifacts. That keeps the semantics clean:

- auto pipeline -> alpha/prerelease
- manual curated release -> stable

---

## Required Hardening Improvements

### 1. Add idempotency protection

**Rule:** one merged `main` commit should map to at most one version tag and one release.

Before generating a new tag in `auto-tag.yml`, check whether `origin/main` HEAD already has a `v*` tag.

If yes:
- skip tag creation
- skip release dispatch
- exit successfully

This prevents duplicate versions when workflows are rerun.

### 2. Add concurrency control

Protect tag generation from races when multiple PRs go green at nearly the same time.

Recommended job-level configuration:

```yaml
concurrency:
  group: merge-tag-release-main
  cancel-in-progress: false
```

This serializes merge/tag orchestration and avoids two jobs computing the same next tag.

### 3. Add stronger PR eligibility checks

Before merging, `auto-tag.yml` should verify:

- PR exists
- PR state is `OPEN`
- PR is not draft
- base branch is `main`
- label `automerge` exists
- optional: review decision is approved

If any check fails, exit successfully without side effects.

### 4. Make release inputs reproducible

Current `release.yml` downloads `MetaCubeX/mihomo/releases/latest`.

This makes release artifacts non-reproducible because the same repository tag could produce different assets later.

Recommended change:
- pin a fixed Mihomo version in repository-controlled config or workflow env

For example:

```yaml
env:
  MIHOMO_VERSION: v1.19.24
```

Then always download that exact version during packaging.

### 5. Improve CI coverage

Current CI validates buildability, but it should more clearly represent release readiness.

Recommended additions to `ci.yml`:
- `go test ./...`
- optional frontend lint
- optional `go vet`

This keeps `CI passed` meaningful when used as the gate for automation.

---

## Detailed Workflow Blueprint

### A. `ci.yml`

#### Purpose
Validation only. No merge, tagging, or publishing side effects.

#### Trigger
```yaml
on:
  pull_request:
    branches:
      - main
```

#### Suggested job design

1. **UI checks**
   - `npm ci`
   - `npm run build`
   - optional `npm run lint`

2. **Go checks**
   - `go test ./...`
   - `go build ./...`
   - optional `go vet ./...`

#### Design principle
`ci.yml` should answer only one question:

> Is this PR technically safe to merge?

---

### B. `auto-tag.yml`

#### Purpose
Orchestrate merge, tag, and release dispatch after CI succeeds.

#### Trigger
```yaml
on:
  workflow_run:
    workflows: ["CI"]
    types: [completed]
```

#### Required conditions
The job should proceed only when:
- `github.event.workflow_run.conclusion == 'success'`
- `github.event.workflow_run.event == 'pull_request'`

#### Recommended step flow

1. Resolve PR number from `workflow_run.pull_requests`
2. Exit if no PR is attached
3. Query PR details
4. Validate automerge policy
5. Exit if PR is already merged or closed
6. Perform squash merge and delete branch
7. Fetch `origin/main` and tags
8. Reset local `main` to `origin/main`
9. Check whether current `HEAD` already has a version tag
10. Exit if tag already exists
11. Compute next alpha tag
12. Push tag
13. Dispatch `release.yml` with `version=<tag>`
14. Write a clear job summary for auditability

#### Recommended merge method
Use squash merge:

```text
squash + delete branch
```

That keeps history cleaner for this type of automated pipeline.

#### Required concurrency control
```yaml
concurrency:
  group: merge-tag-release-main
  cancel-in-progress: false
```

---

### C. `release.yml`

#### Purpose
Build versioned artifacts and publish GitHub Release entries.

#### Trigger
Keep both triggers available:

```yaml
on:
  push:
    tags:
      - 'v*'
  workflow_dispatch:
    inputs:
      version:
        description: 'Version string'
        required: false
        default: 'dev'
```

But the primary automation path should be the explicit `workflow_dispatch` call from `auto-tag.yml`.

#### Responsibilities
- determine version from tag/input
- build UI
- build Go binaries
- package IPK artifacts
- generate SHA256 checksums
- create GitHub Release

#### Important release behavior
- prerelease should be `true` for tags containing `alpha`, `beta`, or `rc`
- release generation should use fixed dependency versions rather than upstream `latest`

#### Optional idempotency behavior
Before creating a release, check whether the release for the tag already exists.

Possible policies:
- strict: fail or exit if release exists
- flexible: update/overwrite prerelease assets for alpha tags

Recommended default for now:
- if release exists, exit cleanly

---

## Recommended Default Policy Set

If no further policy decisions are made, use the following defaults:

### Auto-merge gate
- CI success
- base branch = `main`
- not draft
- label `automerge`

### Versioning
- merged PRs produce `alpha` tags only
- stable releases remain manual

### Merge style
- squash merge

### Release trigger
- `workflow_dispatch` from `auto-tag.yml`

### Idempotency
- if merged `main` HEAD already has a `v*` tag, do not generate another one

### Concurrency
- serialize the merge/tag/release orchestration job

### Dependency policy
- pin `MIHOMO_VERSION`
- avoid `latest`

---

## Proposed Implementation Order

### Phase 1: safety and correctness
1. add `automerge` label gate to `auto-tag.yml`
2. add `concurrency` to `auto-tag.yml`
3. add `HEAD already tagged` idempotency check

### Phase 2: versioning and reproducibility
4. simplify auto-tag logic to alpha-only incrementing
5. pin Mihomo version in `release.yml`

### Phase 3: CI quality improvements
6. add `go test ./...`
7. optionally add lint / vet
8. optionally require review approval

---

## Open Decisions

These decisions should be confirmed before implementation:

1. Should every successful auto-merged PR produce an alpha release?
2. Should review approval be required in addition to the `automerge` label?
3. Should existing alpha releases be replaceable on rerun, or should reruns always no-op?
4. Where should the pinned Mihomo version live: workflow env, version file, or another repo-managed config file?

---

## Summary

The repository already contains the correct workflow split in principle. The roadmap does **not** require a complete redesign.

The recommended next step is to harden the current implementation so that it becomes:

- explicit
- serial
- idempotent
- reproducible
- safe to rerun

Once those improvements are applied, the repository will have a robust pipeline for:

```text
PR -> CI -> auto merge -> auto tag -> auto release
```
