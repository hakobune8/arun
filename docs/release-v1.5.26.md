# ARUN v1.5.26 Release Notes

Date: 2026-07-05

## Highlights

- Tightens generated artifact hygiene for product planning documents.
- Keeps `docs/product-brief.md` as the canonical generated product brief when
  case/separator variants such as `docs/PRODUCT_BRIEF.md` are also present.

## Fixes

- Removes duplicate docs-level product brief files that can preserve an
  alternate product name or mechanic after a completed run.
- Adds regression coverage for the v1.5.25 follow-up run pattern where
  implementation, README, and `docs/product-brief.md` agree, but
  `docs/PRODUCT_BRIEF.md` describes a different concept.

## Validation

- `go test ./internal/orchestrator`
- `go test ./...`
- `git diff --check`

## Upgrade Notes

- No data migration is required.
- Existing generated branches are not rewritten. Re-run the workflow on
  v1.5.26 to apply the stricter generated artifact cleanup.
