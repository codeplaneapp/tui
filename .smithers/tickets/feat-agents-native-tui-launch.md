# Agent Direct Chat (Native TUI Launch)

## Metadata
- ID: feat-agents-native-tui-launch
- Group: Agents (agents)
- Type: feature
- Feature: AGENTS_NATIVE_TUI_LAUNCH
- Dependencies: feat-agents-browser, feat-agents-cli-detection

## Summary

Implement the TUI handoff mechanism allowing users to press Enter on an agent to launch its native CLI/TUI and suspend Smithers TUI.

## Acceptance Criteria

- Pressing Enter on an available agent displays a brief 'Launching...' handoff message.
- The TUI suspends and executes the agent's binary using tea.ExecProcess.
- The agent is given full control of the terminal I/O.
- When the agent process exits, the Smithers TUI resumes automatically.

## Source Context

- internal/ui/views/agents.go
- docs/smithers-tui/03-ENGINEERING.md

## Implementation Notes

- Review section 2.4 'TUI Handoff Pattern' in the engineering doc. Use tea.ExecProcess(cmd, callback) rather than trying to proxy PTY output.
