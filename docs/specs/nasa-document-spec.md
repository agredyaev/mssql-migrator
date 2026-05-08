# NASA-Style Document Spec

Lifecycle: `Current`.

## Purpose

This repository uses a NASA-style technical-document structure for durable
engineering documentation.

The goal is disciplined technical communication: state mission context,
interfaces, assumptions, constraints, off-nominal behavior, verification
evidence, and operational boundaries explicitly so a future maintainer does not
need chat history or CI archaeology.

## Source Basis

- NASA Systems Engineering Handbook: systems context, interfaces, assumptions,
  constraints, verification, and technical management discipline.
- NASA technical-document practice: explicit lifecycle status, exact technical
  references, and fail-closed treatment of off-nominal behavior.

## Applies To

- every durable checked-in Markdown document in the repository
- root `README.md`
- docs under `docs/`
- component and operations documentation under `ops/`, `platform/`, and
  `services/`
- templates and specifications under `docs/templates/` and `docs/specs/`

## Language Requirements

- All durable repository documentation must be written in English.
- Use direct engineering language.
- Name exact repository paths, artifact names, commands, contracts, pipeline
  stages, and runtime components where they matter.
- Prefer precise statements of fact, constraint, and evidence over narrative or
  marketing phrasing.
- Do not use vague or low-information adjectives such as `improved`,
  `comprehensive`, `robust`, `efficient`, or similar filler unless the exact
  measurable meaning is stated in the same sentence.
- If a word does not add technical meaning, omit it.

## Required Analytical Elements

Every durable repository document must explicitly cover these analytical
elements, either as top-level sections or inside the document type's native
required sections:

- lifecycle status and scope boundary
- purpose and mission context
- interfaces, inputs, outputs, and ownership boundaries
- assumptions and constraints
- nominal flow, operating sequence, or implementation sequence
- off-nominal behavior, failure containment, or rollback implications
- verification or validation evidence
- references to canonical source paths, contracts, and runbooks

## Section Tailoring Rule

- The exact section names may vary by document type.
- ADRs, WBS documents, runbooks, READMEs, and architecture specs do not need
  identical headings.
- They do need to preserve the same analytical content.
- When a document type already has required headings, NASA-style content must be
  embedded inside those headings rather than dropped.

## CI/CD Documentation Rule

Any document that describes build, validation, deploy, promotion, rollback, or
proof-publication behavior must also make these points explicit:

- stage names or execution phases
- artifact producers and consumers
- entry criteria and exit criteria
- failure gates and fail-closed behavior
- verification commands and evidence artifacts
- recovery or rollback path

## Migration Rule

- This standard applies repository-wide.
- New durable documents must start from `docs/templates/document-template.md` or
  a document-type-specific template that preserves the same analytical elements.
- Existing legacy documents must be migrated to the NASA-style structure when
  they are materially updated.
- Material implementation changes must update durable documentation in the same
  change.

## Verification Expectations

- Documentation must pass `ops/quality/scripts/check_doc_structure.py`.
- Documentation must pass `ops/quality/scripts/check_doc_context.py`.
- Documentation must pass `ops/quality/scripts/check_doc_path_references.py`.
- Documentation must pass `ops/quality/scripts/check_doc_language.py`.
- Documentation synchronization must pass `ops/quality/scripts/check_doc_sync.py`.

## Template

Use `docs/templates/document-template.md` as the generic starting point for new
durable repository documentation.
