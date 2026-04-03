# Chat Domain System Prompt — Research Summary

## Ticket
- **ID:** chat-domain-system-prompt
- **Group:** Chat And Console
- **Type:** feature
- **Feature flag:** CHAT_SMITHERS_DOMAIN_SYSTEM_PROMPT
- **Dependencies:** none

## Acceptance Criteria
1. The agent is initialized with the Smithers system prompt instead of the default coding prompt.
2. The prompt includes instructions on formatting runs, mentioning pending approvals, and using Smithers MCP tools.

## Source Context
- `internal/agent/templates/smithers.md.tpl` — new template to create
- `internal/agent/agent.go` — wire up template selection

## Codebase Exploration
- Explored `internal/` directory structure: found subdirectories for agent, app, cli, config, db, history, home, log, lsp, message, oauth, permission, projects, pubsub, session, shell, skills, ui, update, version, and stringext.
- No existing `internal/agent/templates/` directory or `smithers.md.tpl` file was found — these need to be created.
- No existing agent.go file was found under `internal/agent/` — the agent package may need scaffolding.

## Implementation Plan
1. Create `internal/agent/templates/` directory.
2. Create `smithers.md.tpl` Go template with Smithers-specific system prompt content covering: workflow management, run formatting, pending approval mentions, and MCP tool usage instructions.
3. Create or update `internal/agent/agent.go` to load and select the Smithers template when the Smithers domain is active.
4. Ensure the template is embedded or accessible at runtime (e.g., via `embed.FS`).

## Status
Research phase complete. No code changes made yet — ready for implementation.