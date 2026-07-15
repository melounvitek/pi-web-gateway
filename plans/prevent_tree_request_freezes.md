# Prevent tree request freezes

## Goal

Keep `/tree` bounded and responsive for large Pi sessions, and prevent a stalled Pi RPC command from exhausting Puma threads.

## TDD rounds

- [x] Replace raw RPC `get_tree` with one compact native extension snapshot containing projected entries, current leaf, and effective settings.
- [x] Restore lightweight current-leaf lookup for ordinary session rendering.
- [x] Add a bounded timeout for tree bridge requests and safely discard late responses.
- [x] Add regression coverage for large tree data and stalled requests.
- [x] Run the full suite and independent review.
