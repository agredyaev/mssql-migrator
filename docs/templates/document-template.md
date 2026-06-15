# <Title>

<!-- Write this document in English. Use exact repository paths, commands, artifact names, and runtime component names. Follow the NASA-style analytical structure required by docs/specs/nasa-document-spec.md. -->

Lifecycle: `<Planned | Current | Target | Historical | Generated>`.

## Purpose

<State what this document describes, why it exists, and which engineering or operational question it answers.>

## Scope

- <List the exact repository paths, runtime components, pipeline stages, artifacts, or contracts that are in scope.>

Reference examples:

- Path example: `ops/delivery/azure-devops/pipelines/deploy-job.yml`
- Contract example: `ops/quality/contracts/sqlmesh-runtime-contract.json`
- Command example: `uv run python ops/quality/scripts/check_doc_context.py`

## System Context

<Describe the current operating model or repository context so a reader can understand this document without chat history or CI archaeology.>

## Interfaces And Boundaries

- Inputs: <List the exact inputs, upstream producers, or required artifacts.>
- Outputs: <List the exact outputs, downstream consumers, or produced artifacts.>
- Ownership boundaries: <State which paths, teams, or pipeline stages own each boundary.>

## Assumptions And Constraints

- Assumptions: <List the assumptions that this document depends on.>
- Constraints: <List the contracts, invariants, technology constraints, or non-negotiable operating limits.>

## Nominal Flow

1. <Describe the normal sequence, lifecycle, or execution path.>
2. <State the expected evidence or state transition at each major step.>

## Off-Nominal Behavior And Failure Containment

- Failure mode: <Describe a concrete failure condition.>
  Containment: <State how the system fails closed, stops, rolls back, or escalates.>
- Failure mode: <Describe another concrete failure condition.>
  Containment: <State how the system fails closed, stops, rolls back, or escalates.>

## Verification And Validation

- Contracts and checks: <List the exact validation scripts, tests, or policy checks.>
- Evidence artifacts: <List the exact proof artifacts, logs, manifests, or rendered outputs.>
- Exit criteria: <State the measurable condition that proves the documented behavior is correct.>

## Operations And Recovery

- Routine operation: <State the normal operational use or maintenance flow.>
- Recovery or rollback: <State the exact runbook, command path, or recovery boundary.>

## Open Issues And Non-Goals

- Open issues: <List unresolved items explicitly.>
- Non-goals: <List what this document does not define or change.>

## References

- Canonical source paths: <List the exact repository paths.>
- Related contracts and scripts: <List the exact validation and contract paths.>
- Related runbooks or ADRs: <List the exact repository references.>
