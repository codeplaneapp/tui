# External Editor Handoff for Prompts

## Metadata
- ID: feat-prompts-external-editor-handoff
- Group: Content And Prompts (content-and-prompts)
- Type: feature
- Feature: PROMPTS_EXTERNAL_EDITOR_HANDOFF
- Dependencies: feat-prompts-source-edit

## Summary

Implement `Ctrl+O` handoff to suspend the TUI and launch `$EDITOR` on the prompt file.

## Acceptance Criteria

- Pressing `Ctrl+O` launches `$EDITOR`.
- TUI suspends correctly and resumes when the editor closes.
- Prompt data is reloaded on resume.
- Terminal E2E test verifying external editor handoff.

## Source Context

- docs/smithers-tui/03-ENGINEERING.md

## Implementation Notes

- Use `tea.ExecProcess` configured with the path to the physical prompt file.
- Watch for `tea.ExecFinishedMsg` to trigger a refetch of the prompt.
