# Indexed session conversation windows

## Goal

Keep initial and paginated conversation loading proportional to a lightweight JSONL structure index plus the requested rendered window. Old large entries outside that window must not be passed to `JSON.parse`.

Preserve GRIPi transcript, status, attachment, and tool rendering behavior while following Pi’s persisted latest branch by default.

## Completed TDD rounds

- [x] Characterize exact rendered-window behavior and prove huge off-window entries are not parsed.
- [x] Add a bounded structural JSONL scanner and immutable per-file index with stable snapshot validation.
- [x] Add branch-local selective projection with exact rendered-message cursors and tool/subagent dependencies.
- [x] Route initial and older conversation windows through selective projection with conservative fallback for unknown layouts.
- [x] Replace one-shot full-history server responses with bounded client-side paging.
- [x] Reuse and append-extend indexes through a byte- and entry-bounded in-memory cache.
- [x] Benchmark copied large sessions, run the full suite, and complete independent review.

Persistent indexes remain deferred because repeated in-memory loads are already fast and persistence would add format, cleanup, permissions, and privacy complexity.

## Validation

- Full suite: 905 runs, 6,117 assertions, no failures.
- Differential checks: 250 native Pi sessions and 2,360 paginated windows without projection or cursor drift.
- Large-entry checks: 296 native entries without cardinality, identity, or lower-bound failures.
- A copied 15.6 MiB session reduced cold full-page peak RSS from about 49 MiB to 36 MiB. Stable indexed conversation loads take about 0.01 seconds.
