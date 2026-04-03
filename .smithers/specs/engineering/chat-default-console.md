# Research: chat-default-console

## Ticket Summary
Establish the chat interface as the default Smithers TUI view, ensuring it acts as the base of the navigation stack.

## Acceptance Criteria
- Launching the application opens the chat interface.
- Pressing `Esc` from any view returns the user to the chat console.
- The chat interface displays correctly under the new Smithers branding.

## Key Files
- `internal/ui/model/ui.go` — Main UI model with `uiState` enum (`uiOnboarding`, `uiInitialize`, `uiLanding`, `uiChat`, `uiSmithersView`). The `UI` struct holds session, common, and view state.
- `internal/ui/views/router.go` — New router scaffolding for view navigation.
- `internal/ui/views/agents.go` — Example view registered with the router.
- `internal/ui/dialog/actions.go` / `internal/ui/dialog/commands.go` — Dialog/action layer that dispatches navigation commands.
- `.smithers/tickets/chat-default-console.md` — Ticket definition with metadata, acceptance criteria, and implementation notes.

## Current Architecture
The UI uses a `uiState` enum to track which view is active. States flow: `uiOnboarding` → `uiInitialize` → `uiLanding` → `uiChat`. There is also a `uiSmithersView` state for Smithers-specific views. The `UI` struct in `ui.go` is the top-level Bubble Tea model. A new `views.Router` has been scaffolded in `internal/ui/views/router.go` to support named view switching.

## Implementation Approach
1. **Set `uiChat` as the default post-initialization state** — After onboarding/initialization completes, transition directly to `uiChat` instead of `uiLanding`.
2. **Wire Esc key to return to chat** — In the router or top-level `Update` method, handle `Esc` key to reset `uiState` back to `uiChat` from any `uiSmithersView`.
3. **Integrate with views.Router** — Register the chat view as the base/default route in the router so it serves as the navigation stack root.
4. **Branding integration** — Ensure the chat view renders with Smithers branding (depends on `chat-ui-branding-status` ticket).

## Dependencies
- `chat-ui-branding-status` — Must be completed first for branding to display correctly in the chat console.

## Risks / Open Questions
- The `uiLanding` state may still be needed for certain flows (e.g., first-time onboarding). Need to determine if landing is fully replaced or conditionally skipped.
- Esc-to-chat behavior must not conflict with Esc handling in dialogs or nested views.