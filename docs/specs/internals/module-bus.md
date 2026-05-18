# Technical Document: Module `internal/bus`

Lifecycle: `Current`.

## Purpose

Describe the **in-process event bus**: typed run lifecycle events, apply outcomes, and helper parsers for subscriber payloads.

## Scope

- `internal/bus/bus.go` — `EventBus`, `Publish`, `Subscribe`, handler invocation
- `internal/bus/payload.go` — `ParseObjectAppliedPayload`, `ParseObjectFailedPayload`, related types
- Tests: `internal/bus/bus_test.go`, `internal/bus/payload_test.go`

## System context

`engine` publishes high-level run and diff events. `apply` publishes fine-grained apply success/failure batches. `audit` and `report` subscribe.

## Interfaces and boundaries

- Inputs: `context.Context`, event name (`types.Event*` constants), `payload any`
- Outputs: synchronous dispatch to subscribers; no buffering
- Contract: subscribers must not assume goroutine isolation—`Publish` runs handlers on the caller stack.

## Assumptions and constraints

- Assumption: payload types match the `Parse*` helpers for apply events.
- Constraint: `Publish` is a no-op when no handlers registered for an event (see `HasHandlers` in implementation).

## Nominal flow

1. Producer calls `Publish` with typed pointer payload.
2. Bus iterates subscribers, invokes `invokeBusHandler` with `recover` per handler.

## Off-nominal behavior and failure containment

- Panic in handler: recovered, optional logging path (see `invokeBusHandler`).
- Slow handler: blocks producer (`Publish` is synchronous).

## Verification and validation

- `make check`

## Operations and recovery

- New events require: constant in `internal/types`, producer callsites, subscriber registration, and parser helpers if batched payloads are used.

## Open issues and non-goals

- Non-goals: no cross-process messaging.

## References

- `internal/types/events.go` — `Event` constants and run/apply payload structs
- `docs/specs/internals/module-engine.md`, `docs/specs/internals/module-apply.md`
