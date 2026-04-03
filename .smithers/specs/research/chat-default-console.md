## Existing Crush Surface
- The ticket/spec and planning docs require chat as default root and `Esc` back-to-chat: [chat-default-console ticket](/Users/williamcory/crush/.smithers/tickets/chat-default-console.md#L16), [engineering spec](/Users/williamcory/crush/.smithers/specs/engineering/chat-default-console.md#L7), [PRD nav model](/Users/williamcory/crush/docs/smithers-tui/01-PRD.md#L310), [Design keymap/state](/Users/williamcory/crush/docs/smithers-tui/02-DESIGN.md#L865), [Engineering router target](/Users/williamcory/crush/docs/smithers-tui/03-ENGINEERING.md#L381), [feature inventory](/Users/williamcory/crush/docs/smithers-tui/features.ts#L32).
- Crush UI state still starts in landing for configured users (`desiredState := uiLanding`): [internal/ui/model/ui.go](/Users/williamcory/crush/internal/ui/model/ui.go#L348).
- Initial session loading is gated to landing only (`case m.state != uiLanding`): [internal/ui/model/ui.go](/Users/williamcory/crush/internal/ui/model/ui.go#L391).
- Chat is entered only after session load or first send (`m.setState(uiChat, ...)`): [internal/ui/model/ui.go](/Users/williamcory/crush/internal/ui/model/ui.go#L506), [internal/ui/model/ui.go](/Users/williamcory/crush/internal/ui/model/ui.go#L3008).
- Router is stack-only with no built-in chat root (`NewRouter()` creates empty stack, `Pop()` can empty it): [internal/ui/views/router.go](/Users/williamcory/crush/internal/ui/views/router.go#L22).
- Only Smithers view currently wired is Agents; opening it pushes router and enters `uiSmithersView`: [internal/ui/model/ui.go](/Users/williamcory/crush/internal/ui/model/ui.go#L1436), [internal/ui/views/agents.go](/Users/williamcory/crush/internal/ui/views/agents.go#L25).
- Pop behavior can return to landing (not always chat) when no session: [internal/ui/model/ui.go](/Users/williamcory/crush/internal/ui/model/ui.go#L1443).
- `Esc` is bound primarily as cancel/clear key, and in `uiSmithersView` key handling is delegated to current view; there is no global "Esc -> chat root" branch: [internal/ui/model/keys.go](/Users/williamcory/crush/internal/ui/model/keys.go#L163), [internal/ui/model/ui.go](/Users/williamcory/crush/internal/ui/model/ui.go#L1720), [internal/ui/model/ui.go](/Users/williamcory/crush/internal/ui/model/ui.go#L1735).
- Branding is still Crush-centric in header/logo/notifications: [internal/ui/model/header.go](/Users/williamcory/crush/internal/ui/model/header.go#L43), [internal/ui/logo/logo.go](/Users/williamcory/crush/internal/ui/logo/logo.go#L1), [internal/ui/model/ui.go](/Users/williamcory/crush/internal/ui/model/ui.go#L648), [internal/ui/notification/native.go](/Users/williamcory/crush/internal/ui/notification/native.go#L20).
- Smithers config/agent scaffolding exists (`Config.Smithers`, Smithers agent selection, Smithers prompt): [internal/config/config.go](/Users/williamcory/crush/internal/config/config.go#L373), [internal/config/config.go](/Users/williamcory/crush/internal/config/config.go#L523), [internal/agent/coordinator.go](/Users/williamcory/crush/internal/agent/coordinator.go#L124), [internal/agent/templates/smithers.md.tpl](/Users/williamcory/crush/internal/agent/templates/smithers.md.tpl#L1).
- Smithers client transport exists, but `ListAgents` is still stubbed placeholder data: [internal/smithers/client.go](/Users/williamcory/crush/internal/smithers/client.go#L106).

## Upstream Smithers Reference
- `smithers tui` (legacy app) currently defaults to runs view, not chat (`useState(..."runs")`), and `Esc` returns toward runs: [src/cli/tui/app.tsx](/Users/williamcory/smithers/src/cli/tui/app.tsx#L29), [src/cli/tui/app.tsx](/Users/williamcory/smithers/src/cli/tui/app.tsx#L75).
- TUI v2 prototype is chat/control-plane oriented with composer-focused default state and `Esc` returning focus to composer: [src/cli/tui-v2/client/state/store.ts](/Users/williamcory/smithers/src/cli/tui-v2/client/state/store.ts#L57), [src/cli/tui-v2/client/app/TuiAppV2.tsx](/Users/williamcory/smithers/src/cli/tui-v2/client/app/TuiAppV2.tsx#L222), [src/cli/tui-v2/broker/Broker.ts](/Users/williamcory/smithers/src/cli/tui-v2/broker/Broker.ts#L826).
- Upstream v2 data model is explicit (`AppState`, workspaces/feed/overlay/focus), unlike Crush’s current enum-plus-submodel pattern: [src/cli/tui-v2/shared/types.ts](/Users/williamcory/smithers/src/cli/tui-v2/shared/types.ts#L135).
- Upstream server API provides runs list/detail/start/resume/cancel, approvals, and SSE events: [src/server/index.ts](/Users/williamcory/smithers/src/server/index.ts#L559), [src/server/index.ts](/Users/williamcory/smithers/src/server/index.ts#L865), [src/server/index.ts](/Users/williamcory/smithers/src/server/index.ts#L1019), [src/server/index.ts](/Users/williamcory/smithers/src/server/index.ts#L1075).
- Upstream terminal E2E harness pattern is process-launch + terminal buffer polling + key sends: [tests/tui-helpers.ts](/Users/williamcory/smithers/tests/tui-helpers.ts#L10), with an E2E run->detail->Esc flow: [tests/tui.e2e.test.ts](/Users/williamcory/smithers/tests/tui.e2e.test.ts#L18).
- Handoff doc confirms v2 direction is chat-first control plane and broker/state architecture: [docs/guides/smithers-tui-v2-agent-handoff.md](/Users/williamcory/smithers/docs/guides/smithers-tui-v2-agent-handoff.md#L15).
- Requested GUI references are not usable in this checkout: [../smithers/gui](/Users/williamcory/smithers/gui) has no `src`, and [../smithers/gui-ref](/Users/williamcory/smithers/gui-ref) is missing.

## Gaps
- Data-model gap: Crush router has no guaranteed chat root and chat is not modeled as a router `View`; navigation is split across `uiState` + optional router stack.
- Transport gap: Crush Smithers client has multi-transport plumbing, but key surface for this flow (agents list powering a non-chat route) is stubbed; live run/event transport is not wired into default-console navigation.
- Rendering gap: Smithers branding requirement is unmet in core shell (header/logo/notifications still use Crush naming).
- UX gap: launch path is landing-first; `Esc` does not universally restore chat root from every non-chat surface.
- Test gap: no dedicated Crush E2E for default-console + Esc-return behavior; no VHS tape covering this ticket path yet. Existing only covers domain prompt smoke: [tests/vhs/smithers-domain-system-prompt.tape](/Users/williamcory/crush/tests/vhs/smithers-domain-system-prompt.tape#L1), [internal/e2e/chat_domain_system_prompt_test.go](/Users/williamcory/crush/internal/e2e/chat_domain_system_prompt_test.go#L18).

## Recommended Direction
- Make chat the router base view (per engineering doc intent) so stack cannot pop below chat; adapt existing chat model into a `views.View` wrapper or equivalent base-route abstraction.
- Change startup routing to enter chat console after onboarding/init in Smithers mode, and remove landing-only gating from initial session restore.
- Implement global `Esc` back behavior in `UI.Update()` for non-chat states/views, with precedence rules so dialogs and active cancel flows still behave correctly.
- Align Smithers shell branding in header/logo/notification surfaces (dependency: `chat-ui-branding-status`).
- Keep this ticket scoped to navigation/branding correctness; treat deeper run/feed transport parity as follow-on tickets.
- Testing plan for this ticket:
  - Terminal E2E path in Crush modeled after upstream helper semantics (spawn TUI, poll buffer text, send keys, assert transitions).
  - At least one VHS happy-path tape for "launch -> chat console default -> open secondary view -> Esc returns to chat".

## Files To Touch
- [internal/ui/model/ui.go](/Users/williamcory/crush/internal/ui/model/ui.go): startup state, initial-session gating, Esc/back handling, router/chat state transitions.
- [internal/ui/views/router.go](/Users/williamcory/crush/internal/ui/views/router.go): enforce chat-root stack semantics (`IsChat`, non-empty base).
- [internal/ui/views/agents.go](/Users/williamcory/crush/internal/ui/views/agents.go): ensure PopView/Esc cooperates with global back-to-chat behavior.
- [internal/ui/dialog/commands.go](/Users/williamcory/crush/internal/ui/dialog/commands.go): ensure command palette exposes/returns to console-home semantics as needed.
- [internal/ui/model/keys.go](/Users/williamcory/crush/internal/ui/model/keys.go): keybinding clarity for back vs cancel conflicts.
- [internal/ui/model/header.go](/Users/williamcory/crush/internal/ui/model/header.go), [internal/ui/logo/logo.go](/Users/williamcory/crush/internal/ui/logo/logo.go), [internal/ui/notification/native.go](/Users/williamcory/crush/internal/ui/notification/native.go): Smithers branding surfaces.
- [internal/e2e/tui_helpers_test.go](/Users/williamcory/crush/internal/e2e/tui_helpers_test.go) and a new e2e test file (for default-console navigation assertions).
- New VHS tape under `tests/vhs/` plus [tests/vhs/README.md](/Users/williamcory/crush/tests/vhs/README.md) update for reproducible happy-path recording.

```json
{
  "document": "First-pass research completed for chat-default-console with code-backed analysis across Crush and upstream Smithers, including gaps and a concrete direction/testing plan."
}
```