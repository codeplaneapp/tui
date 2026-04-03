# Smithers Specs Artifacts

The workflow [specs.tsx](/Users/williamcory/crush/.smithers/workflows/specs.tsx)
writes its planning artifacts here.

Primary inputs:
- [01-PRD.md](/Users/williamcory/crush/docs/smithers-tui/01-PRD.md)
- [02-DESIGN.md](/Users/williamcory/crush/docs/smithers-tui/02-DESIGN.md)
- [03-ENGINEERING.md](/Users/williamcory/crush/docs/smithers-tui/03-ENGINEERING.md)
- [features.ts](/Users/williamcory/crush/docs/smithers-tui/features.ts)

Primary outputs:
- `feature-groups.json`: the canonical feature-group manifest used by the workflow.
- `tickets.json`: the combined structured ticket manifest.
- `ticket-groups/*.json`: per-group ticket DAGs.
- `engineering/*.md`: per-ticket engineering specifications.
- `research/*.md`: per-ticket research notes grounded in Crush and upstream Smithers.
- `plans/*.md`: per-ticket implementation plans.
- `reviews/*.md`: failed review feedback captured during iterative passes.
- `implementation/*.md`: per-ticket implementation summaries.

Human-readable tickets live in
[.smithers/tickets](/Users/williamcory/crush/.smithers/tickets).
