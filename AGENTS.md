# AGENT.md

This repository uses checked-in docs as the source of truth.

## Core Rules

- Write for readers who do not know this stack.
- Use short, direct English.
- Name exact repository paths, commands, artifact names, and runtime components.
- Do not rely on chat history or CI archaeology.
- If behavior is non-obvious, document it in the same change.
- If implementation changes materially, update the durable docs in the same change.
- Keep one canonical document per concern and link to it.

## Required Context

Any durable doc should state:

- what it is
- why it exists
- how it works
- how it is validated
- what it depends on
- what it does not cover

## Documentation Sources

- `docs/specs/documentation-spec.md`
- `docs/specs/nasa-document-spec.md`

## Templates

- `docs/templates/document-template.md`

## When Writing Docs

- Start from the matching template.
- Keep the NASA-style structure.
- Include context, interfaces, assumptions, constraints, off-nominal behavior, verification, and references.
- Use exact paths and commands, not vague descriptions.
