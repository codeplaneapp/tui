# E2E and VHS Testing for MCP Tool Renderers

## Metadata
- ID: eng-mcp-integration-tests
- Group: Mcp Integration (mcp-integration)
- Type: engineering
- Feature: n/a
- Dependencies: feat-mcp-tool-discovery, feat-mcp-runs-tools, feat-mcp-control-tools

## Summary

Create terminal E2E tests and VHS-style recordings for Smithers MCP tool integrations in the chat interface.

## Acceptance Criteria

- Terminal E2E path modeled on the upstream @microsoft/tui-test harness verifies tool discovery and execution via the TUI.
- At least one VHS-style happy-path recording test verifies the rendering of a Smithers MCP tool result in the chat.

## Source Context

- tests/tui.e2e.test.ts
- tests/tui-helpers.ts

## Implementation Notes

- Model testing harness on `../smithers/tests/tui.e2e.test.ts` and `../smithers/tests/tui-helpers.ts`.
- Add a `.tape` file for the VHS recording.
