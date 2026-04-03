# Crush Stack View

**Repo:** crush (Go TUI)
**Feature:** Stacked PRs
**Priority:** P1

## Description

Add a Stack view to Crush that shows the current stack with GitHub PR status, review state, and CI checks. Live-updating equivalent of `smithers stack status`.

## Acceptance Criteria

- [ ] New view accessible via keyboard shortcut (e.g., `K` for stacK)
- [ ] Shows all changes in the current stack with:
  - Change ID
  - PR number + title
  - Review status (approved/pending/changes_requested)
  - CI status (passing/failing/pending)
- [ ] Auto-refreshes every 3 seconds
- [ ] j/k navigation between stack entries
- [ ] Enter opens PR in browser
- [ ] Works with Smithers Cloud API

## E2E Test

```
1. Submit a stack via CLI
2. Open crush → navigate to Stack view
3. All 3 PRs visible with correct status
4. Approve a PR on GitHub → status updates within 3 seconds
```
