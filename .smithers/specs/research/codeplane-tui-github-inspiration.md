Research: survey of high-quality GitHub/Git TUI clients for architectural inspiration when building the Codeplane TUI.

## Top Picks

### 1. gh-dash (dlvhdr) — Best Direct Inspiration

- **Repo**: github.com/dlvhdr/gh-dash
- **Stars**: ~11,300
- **Stack**: Go, Bubbletea v2 + Lipgloss v2 + Glamour + BubbleZone (mouse tracking)
- **What it does**: GitHub dashboard TUI — PRs, issues, notifications in configurable sections

**Architecture patterns we should steal:**

**Tab + Section + Table + Sidebar** layout:
- Root model contains: `tabs`, `sidebar`, `footer`, plus arrays of `sections` (PRs, Issues, Notifications)
- Each section is a filterable table with configurable columns
- Sidebar is a toggleable preview pane (default 45% width) showing PR/issue details with markdown rendering
- Footer has status bar, task spinner, confirmation prompts

**Component tree** (`components/`):
- `table`, `tabs`, `sidebar`, `footer`, `search`, `autocomplete`, `prompt`
- Domain rows: `prrow`, `issuerow`, `notificationrow`
- Domain sections: `prssection`, `issuessection`, `notificationssection`
- Domain views: `prview`, `issueview`, `notificationview`
- Supporting: `listviewport`, `branch`, `branchsidebar`, `carousel`, `tasks`

**Vim-style keybindings**:
- `j`/`k` up/down, `h`/`l` prev/next section
- `w` toggle preview sidebar
- `/` search, `?` help
- Focus routing: root Update() checks search/input focus before handling global keys

**YAML-driven configuration**:
- Sections defined with `title`, `filters` (GitHub search syntax), `layout` (column widths), `limit`
- Go template functions in filters (e.g., `nowModify "-2w"`)
- Per-column config: `updatedAt`, `state`, `repo`, `title`, `author`, `reviewStatus`, `ci`, `lines`, `labels` — each with `width`/`grow`/`hidden`

**Auto-refresh** and paginated data loading for background updates.

**Why this is our best model**: Same stack (Bubbletea v2 + Lipgloss v2), same problem domain (API-backed dashboard with multiple entity types), proven at scale (11k stars, actively maintained). The tab + section + sidebar pattern maps directly to our needs: Runs tab, Issues tab, Repos tab, each with filterable tables and a detail sidebar.

---

### 2. lazygit (jesseduffield) — Best Navigation Architecture

- **Repo**: github.com/jesseduffield/lazygit
- **Stars**: ~75,600
- **Stack**: Go, gocui (custom fork, tcell-based) — NOT Bubbletea
- **What it does**: Full-featured git client TUI

**Architecture patterns worth studying:**

**Window/View model**:
- UI organized into logical "windows" (Files, Branches, Commits, Main)
- Each window contains multiple "views" that tab-switch
- `WindowViewNameMap` tracks active view per window

**Context stack navigation** (most relevant to us):
- Stack-based `ContextMgr` for modal navigation
- Four context types:
  - `SIDE_CONTEXT` — files, branches, commits (replaces other side contexts)
  - `MAIN_CONTEXT` — diffs, logs (preserves side context)
  - `TEMPORARY_POPUP` — menus, search
  - `PERSISTENT_POPUP` — confirmations
- Escape pops the stack

**Controller/Helper pattern**:
- Controllers define keybindings per context (`FilesController`, `BranchesController`)
- Helpers contain shared logic (`RefreshHelper`, `MergeRebaseHelper`)
- All receive a shared `HelperCommon` struct — clean dependency injection

**Refresh flow**:
- After operations, `RefreshHelper` reloads affected models
- Three refresh modes: SYNC (blocks), ASYNC (background), BLOCK_UI (background + blocks input)
- This is important for us: after approving a Smithers node, refresh the run view async

**State per repo**: `RepoStateMap` stores separate state per worktree — relevant if we support multi-workspace views.

**Undo/redo**: Tracks every action. Nice-to-have for us.

**Why study this**: lazygit's context stack is more sophisticated than gh-dash's tabs. Our TUI needs both: tabs for top-level navigation + context stack for drill-down (issue → linked run → node detail → back).

---

### 3. gitui (gitui-org) — Performance Benchmark

- **Repo**: github.com/gitui-org/gitui
- **Stars**: ~21,700
- **Stack**: Rust, ratatui
- **What it does**: Fast git TUI

**Key takeaway**: Async API prevents UI freezing. On the Linux kernel repo (900k+ commits):
- gitui: 24s startup, 0.17GB memory
- lazygit: 57s startup, 2.6GB memory

**Lesson for us**: All Smithers/Codeplane API calls must be async (tea.Cmd). Never block the UI waiting for HTTP responses. Use loading spinners and progressive rendering.

---

### 4. Soft Serve (Charmbracelet) — Same Ecosystem

- **Repo**: github.com/charmbracelet/soft-serve
- **Stars**: ~6,800
- **Stack**: Go, Bubbletea
- **What it does**: Git server with SSH-accessible TUI for browsing repos

**Relevant patterns**: File tree navigation, syntax highlighting, repo browsing — all things we need for the Repos view. Built by the Bubbletea authors so it's idiomatic.

---

### 5. Superfile — Multi-Panel Layout

- **Repo**: github.com/yorukot/superfile
- **Stars**: ~17,000
- **Stack**: Go, Bubbletea
- **What it does**: File manager with multi-panel layout

**Relevant pattern**: Opens multiple panels simultaneously, customizable themes and hotkeys. Relevant for our split-pane Smithers views (run list + run detail side by side).

---

### 6. Pug (leg100) — Bubbletea Architecture Guide

- **Repo**: github.com/leg100/pug
- **Stars**: ~670
- **Stack**: Go, Bubbletea
- **What it does**: Terraform TUI

**Most relevant contribution**: Author wrote the definitive guide on structuring large Bubbletea apps:
- **Model tree**: Root model is message router + screen compositor; children handle their own Init/Update/View
- **Stack-based navigation**: Push models when drilling down, pop when returning
- **Message routing**: Root handles global keys, routes msgs to current child, broadcasts resize to all
- **Performance**: Offload expensive work into `tea.Cmd`; never block Update/View
- **Dynamic model creation**: Create children on-demand, cache frequently accessed ones

This guide (leg100.github.io/en/posts/building-bubbletea-programs/) is required reading.

---

### 7. K9s — Real-Time Dashboard UX

- **Repo**: github.com/derailed/k9s
- **Stars**: ~33,300
- **Stack**: Go, tcell/tview (not Bubbletea)
- **What it does**: Kubernetes cluster TUI

**Relevant patterns**:
- Resource-oriented navigation (similar to our entity types: runs, issues, repos)
- Real-time cluster monitoring (similar to our SSE event streams)
- Vim-style keybindings + plugin system
- The UX of "select a resource type → see a live table → drill into details" is exactly our pattern

---

## Pattern Matrix

| Pattern | gh-dash | lazygit | gitui | K9s | Soft Serve |
|---------|---------|---------|-------|-----|------------|
| Panel/split layout | Sidebar + table | Multi-panel (5+) | Multi-panel | Resource list + detail | File tree + content |
| Navigation | Vim keys + tabs | Context stack | Tab + vim | Resource type nav | Tree nav |
| Detail view | Sidebar drawer | Main panel | Popup/panel | Detail panel | Content pane |
| Real-time updates | Auto-refresh timer | After-action refresh | Async git API | Live watch | Static |
| Config format | YAML | YAML | TOML | YAML | SSH config |
| Keybinding model | Vim, rebindable | Context-specific | Vim-compatible | Vim + plugins | Basic |
| Framework | Bubbletea v2 | gocui (tcell) | ratatui | tview (tcell) | Bubbletea |

## Recommended Patterns for Codeplane TUI

### 1. Layout: gh-dash's Tab + Table + Sidebar
```
┌─ Tabs ──────────────────────────────────────────────┐
│ [ Runs ] [ Repos ] [ Issues ] [ Landings ] [ Chat ] │
├─────────────────────────────┬───────────────────────┤
│ Table (filterable)          │ Sidebar (detail)      │
│                             │                       │
│ ▸ Run #42  ✓ completed      │ ## Run #42            │
│   Run #41  ⏳ running       │ Workflow: implement   │
│   Run #40  ✗ failed         │ Started: 2m ago       │
│                             │ Nodes: 3/5 done       │
│                             │                       │
│                             │ ### Current Node      │
│                             │ research (running)    │
├─────────────────────────────┴───────────────────────┤
│ Footer: [j/k] navigate  [w] toggle sidebar  [?] help│
└─────────────────────────────────────────────────────┘
```

### 2. Navigation: lazygit's Context Stack (for drill-down)
- Tabs switch between top-level entity types (like gh-dash)
- Within a tab, drilling into an item pushes onto a context stack (like lazygit)
- Escape pops back. This gives us: Issues tab → Issue #42 → Linked Run → Node Detail → Escape back

### 3. Data Loading: gitui's Async Pattern
- All API calls via `tea.Cmd` — never block the UI
- Loading spinners in tables while data fetches
- Progressive rendering: show cached data immediately, update when fresh data arrives
- Three refresh modes (from lazygit): SYNC, ASYNC, BLOCK_UI

### 4. Configuration: gh-dash's YAML Sections
- Let users define custom sections with filters
- "My open issues", "Team PRs needing review", "Failed runs this week"
- Column layout is configurable (which columns, widths, visibility)

### 5. Keybindings: Vim-style, Context-Aware
- Global: `1`-`5` switch tabs, `?` help, `/` search, `q` quit
- Table: `j`/`k` navigate, `Enter` select, `w` toggle sidebar
- Detail: `Escape` back, specific actions per entity type
- Rebindable via config

### 6. Real-Time: K9s-style Live Updates
- SSE event stream keeps run tables live (status changes, new events)
- Auto-refresh timer as fallback (like gh-dash)
- Visual indicators: spinner for in-progress, color-coded status badges

## Key Takeaway

**gh-dash is our primary architectural reference.** Same stack, same problem shape (API-backed multi-entity dashboard), proven patterns. We should study its source code closely, particularly:
- `ui/` directory for the root model and component wiring
- `ui/components/` for the table, sidebar, and tabs implementations
- `config/` for YAML section definitions
- How it handles GitHub API pagination and rate limiting (maps to our Smithers/Codeplane API calls)

Then layer in lazygit's context stack for drill-down navigation and K9s's real-time update patterns for SSE integration.
