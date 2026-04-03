# Connect Crush to Smithers Cloud API

**Repo:** crush (Go TUI)
**Feature:** Smithers Integration
**Priority:** P0

## Description

Update Crush's Smithers client to connect to the Smithers Cloud hosted API instead of only local SQLite/HTTP. Auth via the same JWT token stored by `smithers auth login`.

## Acceptance Criteria

- [ ] Crush reads Smithers Cloud API URL from `~/.config/smithers/auth.json`
- [ ] Uses JWT token for authentication
- [ ] Falls back to local SQLite if no cloud config exists (backwards compat)
- [ ] `internal/smithers/client.go` updated with cloud API endpoints
- [ ] Stack view, Runs view, Approvals view all work against the cloud API

## E2E Test

```
1. smithers auth login (in CLI)
2. Launch crush → connects to Smithers Cloud API
3. Stack view shows stacks from connected repos
4. Runs view shows workflow runs
```

## Reference

- Current Smithers client: `internal/smithers/client.go`
- Current types: `internal/smithers/types.go`
