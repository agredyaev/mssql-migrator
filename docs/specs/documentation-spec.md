# Documentation Spec

Lifecycle: `Current`.

## Purpose

The repository must keep enough checked-in context that a maintainer can understand a material change without relying on tribal knowledge, chat logs, or pipeline archaeology.

Every substantive repository artifact should answer three questions:

- What is it?
- Why does it exist or why was it chosen?
- How does it work, get validated, and get operated?

## Applies To

- architecture plans and design notes
- ADRs
- component `README.md` files
- runbooks
- image and delivery documentation
- templates and specifications that define repository documentation shape

## Language Requirements

- Canonical repository documentation must be written in English.
- Titles, headings, prose, placeholders, and inline guidance in checked-in templates must be written in English.
- Use short, direct sentences. Prefer factual statements over marketing or conversational language.
- Use exact repository paths, command lines, environment variable names, and artifact names verbatim.
- Keep code, YAML, JSON, shell commands, and Kubernetes object names in their original technical form.
- If discussion happens in another language outside the repository, the checked-in durable artifact still must be normalized into English.

## NASA-Style Rule

- Every durable repository document must follow the repository's NASA-style
  documentation standard defined in `docs/specs/nasa-document-spec.md`.
- The exact heading names may vary by document type, but the document must make
  its context, interfaces, assumptions, constraints, off-nominal behavior,
  verification evidence, and operational boundaries explicit.
- New general-purpose documents must start from
  `docs/templates/document-template.md` unless a more specific template exists.
- Existing legacy documents must be migrated toward the NASA-style structure
  when they are materially updated.

## Canonical Source Rule

- Temporary planning artifacts must not be cited as canonical source documents
  in durable repository documentation.
- Durable documents must cite stable implementation references such as checked-in
  specifications, ADRs, READMEs, runbooks, contracts, and scripts.
- Temporary plans may be used during active execution, but they must not become
  the long-term source of truth for repository behavior.

## Required Context

Every durable doc should capture, either directly or through explicit references:

- scope and canonical path
- lifecycle status when relevant: `Current`, `Target`, `Planned`, `Historical`, or `Generated`
- exact repository paths for the code, config, or contracts it describes
- why the artifact exists, including the problem, constraint, or decision behind it
- the interfaces, inputs, outputs, and ownership boundaries that matter
- the assumptions and constraints that limit the design or operation
- how the artifact behaves at build time, runtime, deploy time, or recovery time
- off-nominal behavior, failure containment, or rollback implications when they matter
- external prerequisites, secrets, or environment inputs when they matter
- validation commands, guardrails, or contracts that prove the artifact is still correct
- known boundaries and non-goals so future edits do not silently cross layers

## Authoring Rules

- Prefer exact repository paths over vague names.
- Prefer explicit commands over prose like "run the usual checks".
- Prefer describing ownership and boundaries over repeating implementation detail line by line.
- Prefer explicit failure containment and rollback description over implying success-only behavior.
- Do not use vague or low-information adjectives such as `improved`,
  `comprehensive`, `robust`, or similar filler unless the sentence also states
  the exact measurable change.
- If a word does not add technical meaning, omit it.
- Keep one canonical document per concern, then link to that document from related READMEs and ADRs.
- If a design changed materially, update the checked-in docs in the same change, not later.
- Make placeholders instructional, not symbolic only. A reader should understand what belongs in the block from the placeholder text itself.

## Placeholder Rule

Templates must not use empty headings with no guidance.

Each template block should contain placeholder text that tells the author:

- what information belongs in the block
- which repository paths or runtime artifacts to name explicitly
- whether the block should describe `what`, `why`, `how`, or `validation`
- whether the block should include options, consequences, prerequisites, or non-goals

## Minimum Standard For New Artifacts

When adding a new component, image, deploy path, or architecture decision, ensure the repository contains:

1. a canonical description of the artifact
2. the reason it exists or the reason it was chosen
3. the build/runtime/deploy flow
4. the validation or contract checks
5. the operational or ownership boundary

## Templates

- Generic durable document template: `docs/templates/document-template.md`
- Generic NASA-style document rule: `docs/specs/nasa-document-spec.md`

## Internal Go packages

- Canonical specifications for `internal/*` packages: `docs/specs/internals/README.md` and the `module-*.md` files in the same directory.

## Rust migrator (`migrator-core`)

- Canonical specifications for `rust/crates/core/src/`: `docs/specs/rust/README.md` and the `module-*.md` files in the same directory.
- Production operator CLI: `rust/crates/cli/` (see `docs/specs/rust/module-cli.md`).

## Change Synchronization Rule

- Material changes to implementation paths must include at least one durable
  documentation update in the same change.
- Documentation synchronization is enforced by
  `ops/quality/scripts/check_doc_sync.py`.
