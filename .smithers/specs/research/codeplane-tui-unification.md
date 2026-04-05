Research: cross-codebase analysis of Crush, Smithers, and Plue/Codeplane to determine the right TUI architecture for supporting both Smithers workflow orchestration and Codeplane forge operations in a single terminal experience.

## Codebase Inventory

### Smithers (../smithers/)
- **What**: Deterministic, durable AI workflow orchestration framework (TypeScript/Bun)
- **Surface**: JSX-based workflow definitions, SQLite persistence, multi-agent support (Claude, Codex, Gemini, etc.), hot reload, approval gates, hijacking, SSE event streaming
- **Server API**: Run-centric REST + SSE (`/v1/runs`, `/v1/runs/:id`, `/v1/runs/:id/events`, `/v1/runs/:id/frames`, approval endpoints)
- **DB model**: `_smithers_runs`, `_smithers_nodes`, `_smithers_attempts`, `_smithers_approvals`, `_smithers_events`, `_smithers_cron`
- **CLI**: `smithers run`, `smithers list`, `smithers chat`, `smithers approve/deny`, `smithers hijack`, `smithers watch`
- **TUI v2**: Exists in `src/cli/tui-v2/` with workspace-feed-run broker abstractions
- **Key insight**: Smithers is the engine. It doesn't know about repos, issues, or landings. It orchestrates tasks.

### Plue/Codeplane (../plue/)
- **What**: jj-native software forge — full platform with repos, issues, landings, CI/CD, workspaces, AI agents
- **Editions**: Community (OSS, Bun/PGLite single-binary) + Cloud (Go/PostgreSQL, Firecracker VMs)
- **Existing CLI**: TypeScript/Bun compiled binary, 30+ command families (auth, repo, issue, land, change, workflow, workspace, agent, search, wiki, org, label, secret, etc.)
- **WIP web UI**: SolidJS + Vite, minimal — Login.tsx, EventSource, RepoContext. Not feature-complete.
- **Backend API**: Go (Chi router), 160 route files (~43k LOC), clean REST endpoints
- **Smithers integration**: `.smithers/workflows/` directory with implement, plan, research, review workflows. Smithers is Plue's AI execution engine.
- **Key insight**: Plue is the product. Users think "my repo, my issue, my workflow" — Smithers is the engine running underneath.

### Crush (this repo, fork)
- **What**: Terminal AI coding assistant (Go/Bubbletea) by Charm
- **TUI stack**: Bubbletea v2 + Lipgloss v2 + Glamour v2, production-grade
- **Architecture**: View router (stack-based push/pop), component system (toast, split-pane, runtable), state machine (onboarding → initialize → landing → chat/smithersView)
- **Smithers client**: `internal/smithers/` — multi-transport (HTTP → SQLite → exec), typed Go client with runs, tickets, prompts, systems, time-travel, workflows, events, workspace context
- **Current Smithers views**: agents, runs, tickets, approvals, live-chat, timeline, prompts (various stages of completion)
- **Agent system**: Multi-model LLM chat with tools, MCP, LSP integration, session persistence
- **Key insight**: Crush has the best TUI infrastructure of the three. Rebuilding this in TypeScript would be months of work.

## The Relationship

```
Smithers (engine) ← used by → Plue/Codeplane (platform)
     ↑                              ↑
     └──── Crush TUI (terminal interface for both) ────┘
```

Smithers is to Plue what GitHub Actions is to GitHub. You wouldn't build a separate GitHub Actions TUI — it's a tab within the GitHub experience.

## Decision: Single TUI, Codeplane-branded

### Recommendation
One Go/Bubbletea TUI with three modules:
1. **Chat** (existing Crush functionality) — LLM coding assistant
2. **Smithers** (in progress) — workflow monitoring, runs, approvals, events
3. **Codeplane** (new) — repos, issues, landings, workspaces

### Why one TUI, not two
| Factor | One TUI | Two TUIs |
|--------|---------|----------|
| Context switching | None — tab between views | Open different terminals, lose context |
| Infrastructure reuse | One view router, one component system, one style system | Duplicate Bubbletea infra or build TS TUI from scratch |
| Cross-module features | "View the run triggered by this issue" | Copy-paste IDs between terminals |
| AI chat integration | Chat has context from current view (run, issue, landing) | Chat is isolated, no forge context |
| Maintenance burden | One binary, one test suite | Two binaries, two release pipelines |

### Why not a Smithers TUI with Codeplane plugin
Gets the hierarchy backwards. Smithers is the engine, not the product. Users think in terms of "my repos" and "my issues" — Smithers runs are implementation details of how work gets done. The forge is the product; the orchestrator is infrastructure.

### Why not build TUI inside Plue's TypeScript codebase
- Bun has no equivalent to Bubbletea/Lipgloss for terminal rendering
- Crush already has: view router, component system, toast notifications, split panes, key bindings, dialog system, markdown rendering, syntax highlighting
- Building comparable TUI in TS would be months of work vs. adding an API client in Go

### Why Codeplane-branded
- Crush is a Charm product name for a general AI assistant
- We're building a forge TUI — brand it as such
- The AI chat capability is a feature of the Codeplane TUI, not the other way around
- Rebrand: binary name, config dirs, config file names

## Architecture

```
┌──────────────────────────────────────────────────────┐
│                 Codeplane TUI (Go/Bubbletea)         │
│                                                      │
│  ┌───────────┐  ┌────────────┐  ┌────────────────┐  │
│  │   Chat    │  │  Smithers  │  │   Codeplane    │  │
│  │  Module   │  │   Module   │  │    Module      │  │
│  │           │  │            │  │                │  │
│  │ • LLM    │  │ • Runs     │  │ • Repos        │  │
│  │ • Tools  │  │ • Approvals│  │ • Issues       │  │
│  │ • MCP    │  │ • Events   │  │ • Landings     │  │
│  │ • LSP    │  │ • Hijack   │  │ • Workspaces   │  │
│  │          │  │ • Timeline │  │ • Workflows    │  │
│  │          │  │ • Agents   │  │ • CI/Checks    │  │
│  └───────────┘  └────────────┘  └────────────────┘  │
│                                                      │
│  ┌──────────────────────────────────────────────────┐│
│  │           View Router (existing stack-based)     ││
│  │      Tab bar / Split pane / Command palette      ││
│  └──────────────────────────────────────────────────┘│
│                                                      │
│  ┌────────────────┐  ┌───────────────────────────┐  │
│  │ Smithers Client│  │ Codeplane Client (new)    │  │
│  │ (existing)     │  │ same multi-transport      │  │
│  │ HTTP → SQLite  │  │ pattern as Smithers       │  │
│  │ → exec         │  │ HTTP calls to Plue API    │  │
│  └────────────────┘  └───────────────────────────┘  │
└──────────────────────────────────────────────────────┘
```

### Navigation Model
```
Tab bar:  [ Chat ]  [ Runs ]  [ Repos ]  [ Issues ]  [ Workspaces ]
              │         │         │          │            │
           existing  Smithers  ←── Codeplane module ───→
```

Chat is always available. When viewing a run or issue, the AI chat can be opened with context injected (run details, issue body, landing diff).

### Codeplane Client (`internal/codeplane/`)
Same pattern as `internal/smithers/`:
- Typed Go HTTP client calling Plue's REST API routes
- Bearer token auth (reuse Plue's existing auth flow)
- Methods: `ListRepos`, `GetRepo`, `ListIssues`, `GetIssue`, `ListLandings`, `GetLanding`, `ListWorkspaces`, `CreateWorkspace`, etc.
- Maps directly to Plue's `internal/routes/` endpoints

### Codeplane Views (`internal/ui/views/`)
New view files:
- `repos.go` — Repository list + detail (maps to `/repos` API)
- `issues.go` — Issue list + detail (maps to `/repos/:id/issues`)
- `landings.go` — Landing review + diff view (maps to `/repos/:id/landings`)
- `workspaces.go` — Workspace list + management (maps to `/workspaces`)
- `checks.go` — CI/workflow check results

### Existing TS CLI Coexistence
The Plue TypeScript CLI (`jjhub`) stays as-is for:
- Non-interactive scripting and CI
- JSON/TOON output piping
- Commands that don't need a TUI (auth, secrets, webhooks)

The Go TUI is for humans sitting at their terminal. The TS CLI is for automation.

## Phased Migration

### Phase 1 (current): Smithers Module
Finish what's in progress:
- Runs dashboard, approvals queue, live chat viewer, time-travel timeline
- Event streaming (SSE), workspace context injection
- Toast notifications for approvals

### Phase 2: Codeplane Client + Read-Only Views
- Add `internal/codeplane/client.go` (HTTP client for Plue API)
- Repos list view, issue list view (read-only)
- Auth flow (reuse Plue's token system)

### Phase 3: Interactive Codeplane Features
- Create/update issues from TUI
- Review landings with inline diff
- Workspace create/suspend/resume
- Search across repos

### Phase 4: Cross-Module Integration
- "View the workflow run triggered by this issue"
- "See the landing created by this workflow"
- Context-aware AI chat: ask about a run/issue/landing with full context injected
- Notifications: toast when a landing needs review, when an approval is requested

Each phase is independently useful. Phase 1 = Smithers TUI. Phase 2 = Codeplane TUI. Phase 4 = the integrated experience.

## Risks & Mitigations
- **Plue API stability**: Plue's routes are mature (160 files, ~43k LOC). Low risk of breaking changes.
- **Auth complexity**: Plue has OAuth, GitHub App, WorkOS SSO. Start with simple bearer token (like Smithers client). Add OAuth later.
- **Scope creep**: 30+ Plue CLI command families. Only build TUI views for the 5 highest-value surfaces (repos, issues, landings, workspaces, checks). Everything else stays CLI-only.
- **Branding/fork divergence**: Rebrand early (Phase 2) to avoid identity confusion. Clean separation from upstream Crush.

## Open Questions
- Should the binary be `codeplane`, `cp`, or something else?
- Do we need offline mode (SQLite fallback) for Codeplane, or is HTTP-only acceptable?
- How does auth flow work in the TUI? Interactive OAuth redirect, or paste-a-token?
- Should Smithers module work standalone (no Codeplane backend) or require Codeplane?

## Reference TUI Inspiration
See companion research: `codeplane-tui-github-inspiration.md` (analysis of existing GitHub/forge TUIs for architectural patterns).
