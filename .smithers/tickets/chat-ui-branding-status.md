# Chat UI Branding & Status Bar Enhancements

## Metadata
- ID: chat-ui-branding-status
- Group: Chat And Console (chat-and-console)
- Type: engineering
- Feature: PLATFORM_SMITHERS_REBRAND
- Dependencies: none

## Summary

Update the default Crush UI to reflect the Smithers brand, including logo updates and structural changes to the status/header bar to support Smithers connection and run metrics.

## Acceptance Criteria

- The application header displays the Smithers ASCII art instead of Crush.
- The header/status components are prepared to receive and display dynamic Smithers client state.

## Source Context

- internal/ui/logo/logo.go
- internal/ui/model/header.go
- internal/ui/model/status.go
- internal/ui/styles/styles.go

---

## Objective

Replace the Crush brand identity across the TUI header surface with Smithers branding, and extend the header/status structures to carry Smithers-specific runtime state (active run count, pending approval count, MCP connection status). After this ticket the header renders "SMITHERS" instead of "CRUSH", uses the Smithers colour palette, and exposes typed slots that downstream tickets (`chat-active-run-summary`, `chat-pending-approval-summary`, `chat-mcp-connection-status`) can populate with live data.

This corresponds to the `PLATFORM_SMITHERS_REBRAND` feature in `docs/smithers-tui/features.ts` and the "§8 Branding" table in `docs/smithers-tui/01-PRD.md`. The upstream Smithers TUI-v2 reference is `../smithers/src/cli/tui-v2/client/components/TopBar.tsx`, which renders a single-row header with bold "Smithers" text, repo, workspace, profile, mode, run count, and approval count fields.

## Scope

### In scope

1. **Logo replacement** — New `SMITHERS` letterform set in `internal/ui/logo/logo.go`, replacing the existing `C-R-U-S-H` letterforms. Retain the same rendering pipeline (wide + compact + small modes), but spell out `S-M-I-T-H-E-R-S`.
2. **Compact logo text** — Change the compact logo string in `internal/ui/model/header.go` from `"Charm™ CRUSH"` to `"SMITHERS"` with the new gradient colours.
3. **Colour scheme** — Update `internal/ui/styles/styles.go` `DefaultStyles()` to swap the Crush colours (`charmtone.Charple` / `charmtone.Dolly`) for a Smithers-appropriate palette. Reference: the Smithers TUI-v2 design spec uses bright cyan (`#e2e8f0`) for branding, blue accent (`#63b3ed`) for active/focus, and gray tones (`#718096`, `#a0aec0`, `#cbd5e0`) for labels. The exact values may differ in Lip Gloss land (ANSI adaptive), but the intent is cyan-leaning primary + neutral secondary instead of purple/yellow.
4. **Header status slots** — Add a `SmithersStatus` struct to `internal/ui/model/header.go` carrying optional fields: `ActiveRuns int`, `PendingApprovals int`, `MCPConnected bool`, `MCPServerName string`. Wire this into `header.drawHeader()` so it renders these values to the right of the existing working-dir + context-percentage metadata when populated.
5. **Status bar update** — Extend the `Status` model in `internal/ui/model/status.go` to accept an optional `SmithersStatus` pointer, so notification-level information (e.g., "3 active · 1 pending approval") can be rendered alongside the existing help keybindings when in Smithers mode.

### Out of scope

- Actually populating the status slots with live data (covered by `chat-active-run-summary`, `chat-mcp-connection-status`, `chat-pending-approval-summary`).
- View router, view stack, new keybindings (covered by `platform-view-router`, `platform-keyboard-nav`).
- Smithers system prompt changes (covered by `chat-domain-system-prompt`).
- Config namespace rename from `.crush/` to `.smithers-tui/` (covered by `platform-config-namespace`).

---

## Implementation Plan

### Slice 1 — Smithers letterforms in `internal/ui/logo/logo.go`

**Goal**: Replace the five `letterC`, `letterR`, `letterU`, `letterSStylized`, `letterH` functions with eight Smithers letterforms: `letterS`, `letterM`, `letterI`, `letterT`, `letterH`, `letterE`, `letterR`, `letterS2`.

**Files**:
- `internal/ui/logo/logo.go` — rewrite letterform functions and update the `Render()` call to use the new set.
- `internal/ui/logo/rand.go` — no structural change; the random-stretch mechanism still applies.

**Details**:
- Each letterform follows the existing pattern: a `func(bool) string` that returns 3-row ASCII art using `▄`, `▀`, `█`, and box-drawing characters.
- The `Render()` function at line 37 currently builds the letterform slice at lines 46–52 as `letterC, letterR, letterU, letterSStylized, letterH`. Replace with the 8 Smithers letterforms.
- The `crush` variable at line 58 (`crush := renderWord(spacing, stretchIndex, letterforms...)`) and its width calculation at line 59 (`crushWidth := lipgloss.Width(crush)`) remain unchanged in structure — only the letterform inputs change.
- Retain the `stretchLetterformPart` helper and the random-stretch feature.
- The `SmallRender()` function at line 120 changes `"Crush"` to `"Smithers"` and drops the `"Charm™"` prefix.
- The `const charm = " Charm™"` at line 38 should be removed or replaced with `" Smithers"`.

**Smithers ASCII art reference** (from `docs/smithers-tui/02-DESIGN.md` §3.1):
```
███████╗███╗   ███╗██╗████████╗██╗  ██╗███████╗██████╗ ███████╗
██╔════╝████╗ ████║██║╚══██╔══╝██║  ██║██╔════╝██╔══██╗██╔════╝
███████╗██╔████╔██║██║   ██║   ███████║█████╗  ██████╔╝███████╗
╚════██║██║╚██╔╝██║██║   ██║   ██╔══██║██╔══╝  ██╔══██╗╚════██║
███████║██║ ╚═╝ ██║██║   ██║   ██║  ██║███████╗██║  ██║███████║
╚══════╝╚═╝     ╚═╝╚═╝   ╚═╝   ╚═╝  ╚═╝╚══════╝╚═╝  ╚═╝╚══════╝
```

The implementation should use the half-block letterform style (`▄▀█`) already in use by Crush's logo engine, not the full-block style from the design doc mockup. The design doc mockup is for illustration; the implementation must fit the Crush rendering pipeline.

**Upstream mismatch**: Smithers TUI-v2 uses a single-row bold text `"Smithers"` in its `TopBar.tsx` (line 22: `<text style={{ bold: true, color: "#e2e8f0" }}>Smithers</text>`) — no ASCII art. The Crush fork retains the large ASCII art header because it's a core Crush UX pattern (visible in wide mode, collapsed in compact mode). This is an intentional divergence.

**Verification**: `go build ./...` succeeds; visual inspection in a terminal ≥ 80 cols shows "SMITHERS" letterforms; compact mode shows the small "SMITHERS" text.

---

### Slice 2 — Colour palette swap in `internal/ui/styles/styles.go`

**Goal**: Replace the Crush brand colours (purple `charmtone.Charple` primary, yellow `charmtone.Dolly` secondary) with a Smithers-appropriate palette (cyan/blue primary, neutral secondary).

**Files**:
- `internal/ui/styles/styles.go` — `DefaultStyles()` starting at line 500.

**Details**:
- Change `primary` (line 503) from `charmtone.Charple` to a bright cyan value. If `charmtone` doesn't have a suitable cyan, use `lipgloss.Color("#63b3ed")` (matching Smithers TUI-v2 accent from `TopBar.tsx` line 30).
- Change `secondary` (line 504) from `charmtone.Dolly` to a light gray, e.g., `lipgloss.Color("#e2e8f0")` (matching Smithers TUI-v2 branding text from `TopBar.tsx` line 22).
- Also update `borderFocus` (line 523) from `charmtone.Charple` to the new primary, so focus rings use the new accent colour.
- Propagate through the existing `s.Primary = primary` / `s.Secondary = secondary` assignments — the rest of the style tree already derives from these.
- Update logo colour assignments (lines 1212–1216 per agent report):
  - `LogoFieldColor` → primary (cyan)
  - `LogoTitleColorA` → secondary (light gray)
  - `LogoTitleColorB` → primary (cyan)
  - `LogoCharmColor` → remove or repurpose as a subdued label (this was for "Charm™", which no longer renders)
  - `LogoVersionColor` → muted gray

**Upstream reference** (from `TopBar.tsx` lines 22–41):
| Token | Hex | Usage |
|-------|-----|-------|
| Brand bright | `#e2e8f0` | Header text ("Smithers") |
| Accent blue | `#63b3ed` | Focus, active (profile, mode) |
| Label muted | `#718096` | Field labels ("repo:", "workspace:") |
| Value light | `#cbd5e0` | Field values (repo name, workspace title) |
| Hint gray | `#a0aec0` | Run/approval counts, keyboard hints |

**Verification**: `go build ./...` succeeds; visual inspection shows cyan/blue tonality across header, spinners, and tool borders. No purple (`charmtone.Charple`) or yellow (`charmtone.Dolly`) remains in brand-facing surfaces.

---

### Slice 3 — Compact header text in `internal/ui/model/header.go`

**Goal**: Update the compact header from `"Charm™ CRUSH"` to `"SMITHERS"`.

**Files**:
- `internal/ui/model/header.go` — `newHeader()` at lines 38–46.

**Details**:
- Lines 43–44 currently build the compact logo as:
  ```go
  h.compactLogo = t.Header.Charm.Render("Charm™") + " " +
      styles.ApplyBoldForegroundGrad(t, "CRUSH", t.Secondary, t.Primary) + " "
  ```
- Replace with:
  ```go
  h.compactLogo = styles.ApplyBoldForegroundGrad(t, "SMITHERS", t.Secondary, t.Primary) + " "
  ```
- Remove the `t.Header.Charm` style usage from this call site. The style field can remain in the `Styles` struct for now (dead code cleanup is out of scope) but the "Charm™" label no longer renders anywhere.
- The `availDetailWidth` calculation at line 77 will automatically adjust because `lipgloss.Width(b.String())` reflects the new shorter compact logo (no "Charm™ " prefix — saves ~7 visual columns, reclaimed as detail space).

**Verification**: Launch TUI in a terminal < 120 cols (triggering compact mode per `compactModeWidthBreakpoint = 120` at `internal/ui/model/ui.go:64`); confirm header reads `SMITHERS ╱╱╱ ~/project • 5% • ctrl+d open`.

---

### Slice 4 — SmithersStatus struct and header integration

**Goal**: Add a typed struct for Smithers runtime status and wire it into the header draw path so downstream tickets can populate it.

**Files**:
- `internal/ui/model/header.go` — new `SmithersStatus` type, updated `drawHeader` and `renderHeaderDetails`.
- `internal/ui/model/ui.go` — thread `SmithersStatus` from UI model to header.

**Details**:

```go
// SmithersStatus holds Smithers runtime metrics displayed in the header.
// Fields are populated by downstream tickets; this ticket only defines the
// struct and the rendering logic.
type SmithersStatus struct {
    ActiveRuns       int
    PendingApprovals int
    MCPConnected     bool
    MCPServerName    string  // e.g., "smithers"
}
```

- Add a `smithersStatus *SmithersStatus` field to the `header` struct (after line 34).
- Add `SetSmithersStatus(s *SmithersStatus)` setter method.
- In `renderHeaderDetails()` (line 108), after the existing `cwd + metadata` rendering at line 149, append Smithers-specific fields if `smithersStatus != nil`:
  - `"● smithers connected"` or `"○ smithers disconnected"` (MCP indicator, matching `docs/smithers-tui/02-DESIGN.md` §3.1 line "MCPs ● smithers connected").
  - `"N active"` run count (only if > 0).
  - `"⚠ N pending approval"` (only if > 0, using the `warning` colour from styles).
- These are rendered as dot-separated segments (using the existing `dot := t.Header.Separator.Render(" • ")` pattern at line 141) appended to the metadata string.
- The `renderHeaderDetails` function signature gains a new parameter: `smithersStatus *SmithersStatus`. The single call site at line 78 passes `h.smithersStatus`.

**Data flow**: `UI.smithersStatus` → `header.SetSmithersStatus()` → `header.drawHeader()` → `renderHeaderDetails()`. The `UI` model will expose `SetSmithersStatus()` which downstream tickets (`chat-active-run-summary`, etc.) call when they receive Smithers events.

**Upstream reference**: Smithers TUI-v2 `TopBar.tsx` (lines 12–17) computes `activeRunCount` by filtering `runSummaries` for `"running"` or `"waiting-approval"` status, and `approvalCount` by summing `approvals` per workspace. We adopt the same logic shape but in Go, with the actual data population deferred to downstream tickets.

**Verification**: With `SmithersStatus` set to `nil`, behaviour is identical to current Crush header (no Smithers segments appear). With a non-nil status, the additional segments render after the working-dir. Unit test can call `renderHeaderDetails()` with a mock `SmithersStatus{ActiveRuns: 3, PendingApprovals: 1, MCPConnected: true, MCPServerName: "smithers"}` and assert the output contains `"● smithers"`, `"3 active"`, and `"⚠ 1 pending"`.

---

### Slice 5 — Status bar Smithers integration in `internal/ui/model/status.go`

**Goal**: Extend `Status` to optionally render a Smithers status summary alongside the help keybindings.

**Files**:
- `internal/ui/model/status.go` — updated `Status` struct and `Draw()`.

**Details**:
- Add a `smithersStatus *SmithersStatus` field to the `Status` struct (after line 25).
- Add `SetSmithersStatus(s *SmithersStatus)` setter method.
- In `Draw()` (line 71), when `smithersStatus != nil` and `s.msg.IsEmpty()`, render a small right-aligned summary like `"3 runs · 1 approval"` after the help view (drawn at line 73). This mirrors the Smithers TUI-v2 bottom status bar in `TuiAppV2.tsx` which renders `[statusLine] [commandHint] [feedCount]`.
- Use the existing muted foreground styling (`t.Muted`) so it doesn't clash with help text.
- The summary only renders when the message slot is empty (i.e., no error/warning/info is being shown), since the message overlay takes priority per the existing `Draw()` logic at lines 77–112.

**Verification**: Visual inspection with status set vs. nil; no layout regression in help bar. The help keybindings remain unchanged and the Smithers summary appears to the right only when present.

---

## Validation

### Unit tests

1. **Logo rendering** — Add `internal/ui/logo/logo_test.go` (this file does not currently exist):
   - `TestRender_Wide` — call `logo.Render()` at width=120 with `compact=false`, assert output contains at least 3 rows and substrings from the SMITHERS letterforms (e.g., `"█"` blocks are present). Assert the string does NOT contain `"Charm"`.
   - `TestRender_Compact` — call `logo.Render()` with `compact=true`, assert output height matches expected compact height.
   - `TestSmallRender` — call `logo.SmallRender()`, assert output contains `"Smithers"` and does NOT contain `"Crush"` or `"Charm"`.

2. **Header details with SmithersStatus** — Add `internal/ui/model/header_test.go` (this file does not currently exist):
   - Call `renderHeaderDetails()` with `SmithersStatus{ActiveRuns: 2, PendingApprovals: 1, MCPConnected: true, MCPServerName: "smithers"}`, assert ANSI-stripped output contains `"● smithers"`, `"2 active"`, `"⚠ 1 pending"`.
   - Call with `nil` SmithersStatus, assert output matches existing Crush format (contains working dir and percentage, no Smithers segments).
   - Use a test helper matching the pattern in `internal/ui/model/layout_test.go` — construct a minimal `*common.Common` via `common.DefaultCommon(nil)` with default styles.

3. **Status bar rendering** — Add `internal/ui/model/status_test.go` (this file does not currently exist):
   - Confirm `Draw()` with `nil` SmithersStatus renders only help text.
   - Confirm `Draw()` with populated SmithersStatus includes run/approval summary text when no info message is active.

4. **Colour sanity** — Add a test in `internal/ui/styles/styles_test.go`:
   - Call `DefaultStyles()`, assert `s.Primary` is not `charmtone.Charple` and `s.Secondary` is not `charmtone.Dolly`.

### Terminal E2E tests (modeled on upstream @microsoft/tui-test harness)

The upstream Smithers E2E test harness in `../smithers/tests/tui-helpers.ts` provides the pattern: spawn the TUI binary with piped stdio, buffer stdout, strip ANSI escapes, and poll for expected text via `waitForText()`. The test suite in `../smithers/tests/tui.e2e.test.ts` exercises this by verifying layout text like `"Smithers Runs"` and keyboard-driven drill-downs.

For this ticket, create a Go-side E2E test file `tests/tui_branding_e2e_test.go` that adapts the upstream pattern to Go:

1. **TUITestInstance helper** — Create `tests/tui_test_helpers.go` implementing the same interface as `../smithers/tests/tui-helpers.ts`:
   ```go
   type TUITestInstance struct { ... }
   func LaunchTUI(args []string) (*TUITestInstance, error)
   func (t *TUITestInstance) WaitForText(text string, timeout time.Duration) error
   func (t *TUITestInstance) WaitForNoText(text string, timeout time.Duration) error
   func (t *TUITestInstance) SendKeys(text string)
   func (t *TUITestInstance) Snapshot() string
   func (t *TUITestInstance) Terminate()
   ```
   - **Spawn**: `exec.Command` with the built Crush binary, piped stdin/stdout/stderr, `TERM=xterm-256color` env.
   - **Buffer**: Goroutine reads stdout into a thread-safe buffer (matching `BunSpawnBackend.readStream` at `tui-helpers.ts:28–38`).
   - **ANSI stripping**: Same regex as upstream: `\x1B\[[0-9;]*[a-zA-Z]` (matching `tui-helpers.ts:43`).
   - **Text matching**: `WaitForText` polls at 100ms intervals up to timeout (matching `tui-helpers.ts:55–62`). Includes the compact whitespace fallback from `tui-helpers.ts:51`.
   - **Poll interval**: 100ms (matching `POLL_INTERVAL_MS` at `tui-helpers.ts:8`).
   - **Default timeout**: 10 seconds (matching `DEFAULT_WAIT_TIMEOUT_MS` at `tui-helpers.ts:7`).

2. **Test: branding renders correctly** (`tests/tui_branding_e2e_test.go`):
   - Build the binary: `go build -o <tmpdir>/smithers-tui .`
   - `LaunchTUI([]string{})` — starts with default chat view.
   - `WaitForText("SMITHERS", 10s)` — the compact or full logo renders.
   - `WaitForNoText("CRUSH", 3s)` — no Crush branding visible.
   - `WaitForNoText("Charm™", 3s)` — no Charm branding visible.
   - `SendKeys("\x03")` (Ctrl+C) to cleanly exit.
   - `Terminate()` to clean up.

3. **Test: SmithersStatus rendering** (if a `--smithers-status-mock` test-only flag is added):
   - Launch with mock flag.
   - `WaitForText("● smithers", 10s)` — MCP indicator visible.
   - `WaitForText("active", 10s)` — run count visible.

### VHS happy-path recording test

Create a VHS tape file `tests/vhs/branding.tape`:

```vhs
Output tests/vhs/branding.gif
Set Shell "bash"
Set FontSize 14
Set Width 120
Set Height 35
Set TypingSpeed 50ms

Type "go run . 2>/dev/null || ./smithers-tui"
Enter
Sleep 3s

# Verify Smithers header is visible
Screenshot tests/vhs/branding_header.png

# Type a message to confirm chat works
Type "hello"
Enter
Sleep 2s

Screenshot tests/vhs/branding_chat.png

# Exit
Ctrl+C
Sleep 1s
```

Run with `vhs tests/vhs/branding.tape` and visually verify the screenshots show Smithers branding. Add a CI step that runs VHS and stores the GIF + screenshots as pipeline artifacts.

A `Makefile` target should wrap this:
```makefile
.PHONY: test-vhs-branding
test-vhs-branding:
	@command -v vhs >/dev/null 2>&1 || { echo "vhs not installed"; exit 1; }
	vhs tests/vhs/branding.tape
```

### Manual verification

1. `go build -o smithers-tui . && ./smithers-tui` — wide terminal (≥ 120 cols): Smithers ASCII art renders with cyan/blue gradient. No purple. No "Charm™" or "CRUSH" visible.
2. Resize terminal to < 120 cols: compact header shows `SMITHERS ╱╱╱ ~/project • 5% • ctrl+d open`.
3. Resize terminal to < 80 cols: small render shows "Smithers" inline text.
4. `grep -ri "crush" internal/ui/logo/ internal/ui/model/header.go` returns zero hits for display text (Go import paths with `crush` in the module name are expected and separate from branding).

---

## Risks

### 1. Charmtone colour availability

**Risk**: The `charmtone` package may not expose a suitable cyan/blue tone. The Crush codebase uses `charmtone.Charple` (purple), `charmtone.Dolly` (yellow), etc., which are Charm-branded colour names. The available palette is defined by the charmbracelet/x/exp/charmtone package and may not include a direct cyan equivalent.

**Mitigation**: Fall back to raw `lipgloss.Color("#63b3ed")` / `lipgloss.Color("#e2e8f0")` hex values if `charmtone` doesn't have a matching named colour. These will work on truecolor terminals and degrade gracefully on 256-color terminals via Lip Gloss's ANSI adaptation. Check `charmtone.Malibu` (currently used for `info` at line 528) as a potential primary — it's already a blue tone in use.

### 2. Letterform complexity

**Risk**: "SMITHERS" is 8 characters vs. "CRUSH" at 5. The wide-mode logo will be ~60% wider, which may not fit comfortably in terminals < 100 cols.

**Mitigation**: Use narrower letterform widths (3-column base vs. Crush's 4-column) and reduce the left/right diagonal field widths. The `compactModeWidthBreakpoint` at 120 cols (defined in `internal/ui/model/ui.go:64`) already triggers compact mode for smaller terminals, so the wide logo only appears in generous viewports. If still too wide, consider an abbreviated 3-letter logo (`S·T·I`) for the wide mode and save "SMITHERS" for the compact text.

### 3. Upstream Crush cherry-pick friction

**Risk**: Replacing the logo and header creates a permanent conflict zone for any upstream Crush changes to `internal/ui/logo/logo.go`, `internal/ui/model/header.go`, and `internal/ui/styles/styles.go`.

**Mitigation**: These files are branding-specific and unlikely to receive functional fixes from upstream. If upstream does change the logo rendering pipeline (e.g., new Lip Gloss API), the conflict will be in the rendering mechanics, not the letterform content — which is straightforward to resolve. The engineering doc (`docs/smithers-tui/03-ENGINEERING.md` §1.1) explicitly acknowledges this as acceptable for a hard fork.

### 4. SmithersStatus nil safety

**Risk**: Passing a nil `SmithersStatus` through the header path could cause nil-pointer panics if future developers forget the nil check.

**Mitigation**: Use a pointer receiver (`*SmithersStatus`) and guard all access sites with `if s.smithersStatus != nil`. Consider making `SmithersStatus` a value type with a `Connected bool` sentinel (zero-valued struct means "not configured") to eliminate nil checks entirely. The unit tests added in Validation §2 verify both nil and non-nil paths.

### 5. Crush–Smithers rendering model mismatch

**Risk**: The Smithers TUI-v2 reference (`TopBar.tsx`) uses a single-row header with inline text fields (repo, workspace, profile, mode, run count, approval count — lines 21–42). Crush uses a multi-row ASCII art header with a separate compact mode. Downstream tickets that wire in live run/approval data may expect the Smithers single-row layout.

**Mitigation**: This ticket establishes the pattern: Smithers status data renders in the compact header detail line (the row after the ASCII art in wide mode, or the single row in compact mode). Downstream tickets populate data into the same `SmithersStatus` slots. If the single-row Smithers layout is later desired, it can be added as an additional compact threshold without restructuring. The `renderHeaderDetails()` function is the single integration point, making future layout changes isolated.

### 6. E2E test fragility

**Risk**: Terminal E2E tests that spawn the full TUI binary and poll stdout are inherently timing-sensitive. The upstream Smithers tests (`tui.e2e.test.ts`) use generous timeouts (15s) and `await new Promise(r => setTimeout(r, 200))` delays between key presses (lines 36, 51, 57), suggesting this is a known concern.

**Mitigation**: Use the same polling strategy as upstream (100ms poll interval, 10s default timeout). Keep the branding E2E test minimal — only verify text presence/absence, no keyboard navigation. The more complex interaction testing is deferred to downstream tickets that have richer UI surfaces to validate. On CI, run the E2E test with an extended timeout multiplier (e.g., `SMITHERS_E2E_TIMEOUT_MULT=3`) to account for slow runners.
