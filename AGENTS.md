# AGENT.md

Lifecycle: `Current`.

This repository uses checked-in docs as the source of truth.

## Core Rules

- Write for readers who do not know this stack.
- Use short, direct English.
- Name exact repository paths, commands, artifact names, and runtime components.
- Do not rely on chat history or CI archaeology.
- If behavior is non-obvious, document it in the same change.
- If implementation changes materially, update the durable docs in the same change.
- Keep one canonical document per concern and link to it.

Work in a token- and compute-efficient mode.

Main rules:

1. Keep answers short.
   - Do not add introductions.
   - Do not repeat the question.
   - Do not explain obvious things.
   - If the answer fits into 3–5 bullets, use 3–5 bullets.

2. Do not flood responses with tokens.
   - Do not write long reasoning unless required.
   - Do not add alternatives unless asked.
   - Do not provide deep analysis for simple tasks.
   - Do not create long “just in case” lists.

3. Use only the required context.
   - Do not read all project files every time.
   - First identify which files are actually needed.
   - Open only relevant files.
   - If context is missing, ask for the specific file or detail.

4. Do not scan the whole project without a reason.
   - Do not run global searches for local tasks.
   - Do not inspect unrelated folders.
   - Do not reread files already used in the current context.

5. Before reading files, define a short plan.
   - Which files are needed.
   - Why they are needed.
   - What exactly must be checked.
   - If the task can be solved without reading files, solve it without reading files.

6. When working with code:
   - Change only the minimum required part.
   - Do not rewrite a full file without a reason.
   - Do not refactor together with the requested change.
   - Do not fix unrelated issues.
   - Show only meaningful changes.

7. For analysis tasks:
   - Start with the short conclusion.
   - Then briefly explain the reason.
   - Add details only when required.

8. If something is unclear:
   - Do not guess.
   - Ask one precise question.
   - Do not provide a long list of options.

9. Response style:
   - Short sentences.
   - Clear structure.
   - No marketing or academic tone.
   - No unnecessary warnings or disclaimers.

10. Priorities:
   - Accuracy.
   - Low token usage.
   - Minimal file reading.
   - Minimal changes.
   - Fast useful result.

11. Memory usage:
   - Create memory only for key facts that will be useful in future conversations.
   - Do not save temporary details, one-off tasks, drafts, or minor preferences.
   - Do not save assumptions.
   - Do not save information unless the user explicitly states it or it is clearly important.
   - Keep memory entries short, factual, and specific.
   - If unsure whether something is worth remembering, do not save it.

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
