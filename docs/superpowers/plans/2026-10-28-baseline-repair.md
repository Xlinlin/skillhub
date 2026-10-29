# Standalone Baseline Repair Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans to implement this plan task-by-task. Preserve existing working changes; no new Agent features.

**Goal:** Restore frontend build/lint, Go test/vet/build and the current Skill login/publish/download flow.

**Architecture:** The active entrypoint is the independent cinience module. Remove unchanged remnants of the retired saker integration after verifying callers and taking an external source snapshot. Preserve the standalone implementation and adapt tests to supported interfaces.

**Tech Stack:** Go, Gin, GORM, SQLite/PostgreSQL, Git, Bleve, React, TypeScript, Vite, ESLint.

**Spec:** Current standalone README/API and baseline prerequisites in ../specs/2026-10-28-agent-hosting-design.md. User explicitly authorized baseline repair; Agent hosting remains out of scope.

## Global Constraints

- Preserve the uncommitted standalone state; do not reset to HEAD.
- Snapshot externally before removing individually verified obsolete files; no whole-directory deletion.
- No mock packages, compiler exclusions, disabled lint rules or skipped tests to hide failures.
- Do not restore retired plugin/team/OAuth APIs to satisfy stale imports.
- No unrelated dependency upgrades; report existing audit results separately.
- Keep SQLite default and PostgreSQL compatibility; do not touch real data.

## Review Focus

- Cleanup must not remove live standalone call sites: verify imports and run existing tests.
- Search filtering and numeric sorting must work together: real Bleve regression.
- Effect fixes must preserve auth refresh and sort/query transitions.
- Published files must match downloads: isolated HTTP smoke.
- Preserve existing changes: external snapshot and explicit removal manifest.

## Task 1: Restore standalone source boundaries

- [x] Snapshot the source and retain the previously reproduced red Go output.
- [x] Remove unchanged, unreferenced legacy integration files; record paths externally. Adapt active search tests.
- [x] Resolve remaining compilation failures without reinstating retired APIs; add regressions for active behavior fixes.
- [x] Run package tests, go test -mod=readonly ./... and go vet -mod=readonly ./....

## Task 2: Restore frontend

- [x] Remove unreferenced legacy forwarders and obsolete utility tests after checking callers.
- [x] Fix reproduced effect lint failures using appropriate state lifetimes; preserve rules and type checks.
- [x] Run npm run build and npm run lint before embedding assets.

## Task 3: Complete validation

- [x] Build and run with temporary SQLite/Git/Bleve paths on loopback.
- [x] Verify health, login/whoami, publish, search, versions and archive contents; stop helper.
- [x] Final build/lint/test/vet, source-snapshot diff and fresh whole-change review.
- [x] Update status docs; leave all changes uncommitted for review.

## Execution ledger

- Ruling: retain the actual checkout on codex/fix-standalone-baseline; a fresh HEAD checkout omits the user's rewrite.
- Ruling: user authorized this repair; proceed without another design gate and do not commit unrelated work.
- Initial red: Go setup fails on legacy imports/types; frontend has 10 TypeScript and 3 lint errors.

- Complete: removed 68 unchanged retired integration files; seven surviving code/test files changed and one archive regression added. Source snapshot and exact manifest retained externally.
- Complete: actual frontend build/lint, Go uncached full tests/vet/build, isolated HTTP smoke and browser query/sort/detail checks all passed. Test helper and browser session stopped.
- Ruling: fix three active regressions found during verification: filtered sorting, search total field, and host Git autocrlf altering ZIP bytes. The search and archive tests were observed failing before the fix and passing after it.
- Review: fresh reviewer found no important/critical issue introduced by the repair. PostgreSQL/Docker runtime and dependency upgrades were set aside and are reported as limitations.
