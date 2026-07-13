# External Pi CLI session handoff

## Goal

Keep the gateway safe and understandable when Pi CLI appends to a session already loaded by a gateway RPC process, without changing Pi CLI, Pi core, or Pi session formats.

## Behavior

- Preserve intentional gateway tree navigation when the RPC process knows the persisted append cursor.
- Enter external-follow mode when the file gains valid entries unknown to the gateway RPC process, or changes while no gateway RPC writer exists.
- Follow persisted Pi CLI entries through the existing event polling and server-rendered conversation fragment.
- Disable gateway session mutations in external-follow mode while preserving the draft.
- Require explicit optimistic takeover after the user finishes using the session in Pi CLI.
- Fail closed for file replacement/truncation, unsupported synchronization RPC, and reconciliation failures.

## Implementation rounds

1. Add file snapshots, RPC `get_entries` position checks, synchronized per-session activity state, and detection tests.
2. Integrate state into session rendering, event polling, mutation guards, stale-client retirement, and request-level tests.
3. Add explicit takeover with startup/file race checks and tests.
4. Add external-follow UI, live fragment refresh, draft/scroll preservation, and focused frontend tests.
5. Run the full suite and an independent simplification/correctness review; address findings and rerun validation.

## Constraints

- Gateway-only metadata remains in memory and never enters Pi JSONL files.
- Takeover is cooperative, not an enforceable lease: the gateway cannot know whether Pi CLI remains open or prevent Pi CLI from writing later.
- No automatic abort of a busy gateway task after external activity; Stop remains available and takeover waits for idle.
