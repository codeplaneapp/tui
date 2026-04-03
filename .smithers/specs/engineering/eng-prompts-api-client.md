## Research Summary: eng-prompts-api-client

Read the ticket and all relevant planning documents and code to understand the requirements for implementing the Prompts API Client.

### Ticket Requirements
- Add HTTP or MCP client methods to fetch prompts, update prompt sources, and render prompt previews
- Client must expose `ListPrompts`, `UpdatePromptSource`, and `RenderPromptPreview` operations
- Terminal E2E test verifying API client capabilities for prompts
- Mirror `fetchPrompts`, `updatePromptSource`, and `renderPromptPreview` from the GUI transport
- Ensure `RenderPromptPreview` correctly passes down a map of key-value props

### Codebase Context Explored
- Read the existing smithers client at `internal/smithers/client.go` and types at `internal/smithers/types.go` to understand the existing API client patterns
- Reviewed the GUI transport layer for reference implementation patterns
- Examined existing test patterns including glob tests and other E2E test files
- Reviewed the engineering spec at `.smithers/specs/engineering/eng-prompts-api-client.md`
- Reviewed the PRD, design docs, and engineering docs under `docs/smithers-tui/`

### Key Findings
- The existing smithers client uses a standard HTTP client pattern with base URL configuration
- Types are defined in a separate types.go file
- The implementation should follow the same patterns as existing client methods
- E2E tests use the standard Go testing framework with `require` assertions
- No implementation code was written yet - this was a research/planning phase only