# Smithers TUI — Design Document

**Status**: Draft
**Date**: 2026-04-02

---

## 1. Design Principles

1. **Chat-first**: The default experience is a conversation. Every feature
   is reachable via natural language or a slash command.
2. **Progressive disclosure**: Start simple (chat), reveal complexity
   (dashboards, time-travel) only when needed.
3. **Keyboard-driven**: Every action reachable via keyboard. Mouse optional.
4. **Information density**: Terminal users expect density. Show more, not less.
5. **Crush DNA**: Preserve Crush's proven UX patterns — command palette,
   session management, tool rendering, MCP status.

---

## 2. View Architecture

```
┌─────────────────────────────────────────────────┐
│                  SMITHERS TUI                    │
├─────────────────────────────────────────────────┤
│                                                  │
│  ┌──────────┐   ┌──────────────────────────┐    │
│  │          │   │                           │    │
│  │  Views   │──▶│   Active View Content     │    │
│  │  Stack   │   │                           │    │
│  │          │   │                           │    │
│  └──────────┘   └──────────────────────────┘    │
│                                                  │
│  Workspace Views (mirrors GUI sidebar):          │
│  ├── Run Dashboard                               │
│  │   ├── Node Inspector                          │
│  │   │   └── Task Tabs (Input/Output/Config/Chat)│
│  │   ├── Live Chat Viewer                        │
│  │   │   └── Hijack Mode                         │
│  │   └── Timeline / Time-Travel                  │
│  ├── Workflow List & Executor                     │
│  ├── Agent Browser & Chat                        │
│  ├── Prompt Editor & Preview                     │
│  └── Ticket Manager                              │
│                                                  │
│  Systems Views:                                  │
│  ├── Console / Chat (default)                    │
│  ├── SQL Browser                                 │
│  └── Triggers / Cron Manager                     │
│                                                  │
│  Overlay Views:                                  │
│  ├── Approval Queue                              │
│  ├── Scores / ROI                                │
│  └── Memory Browser                              │
│                                                  │
│  Navigation: / commands, Ctrl+P palette, Esc back│
└─────────────────────────────────────────────────┘
```

---

## 3. Screen Layouts

### 3.1 Chat View (Default)

This is what users see on launch. Identical to Crush's chat but with
Smithers branding and a Smithers-specialized agent.

```
┌─────────────────────────────────────────────────────────────────────┐
│░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░│
│░░  ███████╗███╗   ███╗██╗████████╗██╗  ██╗███████╗██████╗ ███████╗░│
│░░  ██╔════╝████╗ ████║██║╚══██╔══╝██║  ██║██╔════╝██╔══██╗██╔════╝░│
│░░  ███████╗██╔████╔██║██║   ██║   ███████║█████╗  ██████╔╝███████╗░│
│░░  ╚════██║██║╚██╔╝██║██║   ██║   ██╔══██║██╔══╝  ██╔══██╗╚════██║░│
│░░  ███████║██║ ╚═╝ ██║██║   ██║   ██║  ██║███████╗██║  ██║███████║░│
│░░  ╚══════╝╚═╝     ╚═╝╚═╝   ╚═╝   ╚═╝  ╚═╝╚══════╝╚═╝  ╚═╝╚══════╝░│
│░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░│
│ ~/my-project                                                        │
│                                                                     │
│ ◇ Claude Opus 4.6 via Anthropic                                    │
│   Smithers Agent Mode                                               │
│                                                                     │
│ MCPs                          Runs                                  │
│ ● smithers  connected         3 active · 1 pending approval        │
│                                                                     │
│─────────────────────────────────────────────────────────────────────│
│                                                                     │
│ ◆ You:                                                              │
│   What's the status of all active runs?                             │
│                                                                     │
│ ◇ Smithers Agent:                                                   │
│   Here are your 3 active runs:                                      │
│                                                                     │
│   ┌─────────────────────────────────────────────────────────┐       │
│   │ ID       │ Workflow        │ Status    │ Step    │ Time │       │
│   ├──────────┼────────────────┼───────────┼─────────┼──────┤       │
│   │ abc123   │ code-review    │ running   │ 3/5     │ 2m   │       │
│   │ def456   │ deploy-staging │ approval  │ 4/6     │ 8m   │       │
│   │ ghi789   │ test-suite     │ running   │ 1/3     │ 30s  │       │
│   └─────────────────────────────────────────────────────────┘       │
│                                                                     │
│   Run def456 has a pending approval gate at the "deploy"            │
│   step. Would you like to approve it?                               │
│                                                                     │
│─────────────────────────────────────────────────────────────────────│
│                                                                     │
│ > Ready...                                                          │
│ :::                                                                 │
│                                                                     │
│ / or ctrl+p  commands    shift+enter  newline        ctrl+g  less   │
│ ctrl+r       runs        @            mention file   ctrl+c  quit   │
│ ctrl+s       sessions    ctrl+o       open editor                   │
│─────────────────────────────────────────────────────────────────────│
```

**Key differences from Crush**:
- Header: SMITHERS branding instead of CRUSH
- Status bar: Shows MCP connection to Smithers + active run count
- Agent: Smithers-specialized system prompt
- Help bar: Adds `ctrl+r runs` shortcut
- Tool results: Smithers-specific renderers (run tables, approval cards, etc.)

---

### 3.2 Run Dashboard View

Accessed via `/runs` or `Ctrl+R`.

```
┌─────────────────────────────────────────────────────────────────────┐
│ SMITHERS › Runs                                         [Esc] Back  │
│─────────────────────────────────────────────────────────────────────│
│ Filter: [All ▾]  Status: [All ▾]  Search: [____________]           │
│─────────────────────────────────────────────────────────────────────│
│                                                                     │
│ ● ACTIVE (3)                                                        │
│                                                                     │
│ ▸ abc123  code-review       ████████░░  3/5 nodes   2m 14s         │
│   └─ claude-code agent on "review auth module"                      │
│                                                                     │
│ ▸ def456  deploy-staging    ██████░░░░  4/6 nodes   8m 02s   ⚠ 1  │
│   └─ ⏸ APPROVAL PENDING: "Deploy to staging?"          [a]pprove   │
│                                                                     │
│ ▸ ghi789  test-suite        ██░░░░░░░░  1/3 nodes   30s            │
│   └─ codex agent on "run integration tests"                         │
│                                                                     │
│─────────────────────────────────────────────────────────────────────│
│ ● COMPLETED TODAY (12)                                              │
│                                                                     │
│   jkl012  code-review       ██████████  5/5 nodes   4m 30s   ✓    │
│   mno345  lint-fix          ██████████  2/2 nodes   1m 12s   ✓    │
│   pqr678  deploy-prod       ██████████  6/6 nodes   12m 05s  ✓    │
│   ...                                                               │
│                                                                     │
│─────────────────────────────────────────────────────────────────────│
│ ● FAILED (1)                                                        │
│                                                                     │
│   stu901  dependency-update ████░░░░░░  2/5 nodes   3m 20s   ✗    │
│   └─ Error: Schema validation failed at "update-lockfile"           │
│                                                                     │
│─────────────────────────────────────────────────────────────────────│
│                                                                     │
│ [Enter] Inspect  [c] Chat  [h] Hijack  [a] Approve  [x] Cancel    │
│ [/] Back to chat                          Total: 16 runs today     │
│─────────────────────────────────────────────────────────────────────│
```

**Interactions**:
- `↑`/`↓` navigate runs
- `Enter` opens run inspector (detailed node view)
- `c` opens live chat viewer for selected run
- `h` hijacks the agent on selected run
- `a` approves pending gate (if applicable)
- `d` denies pending gate
- `x` cancels selected run
- `Esc` returns to chat

---

### 3.3 Live Chat Viewer

Accessed via `/chat <run_id>` or pressing `c` on a run.

```
┌─────────────────────────────────────────────────────────────────────┐
│ SMITHERS › Chat › abc123 (code-review)              [Esc] Back     │
│ Agent: claude-code │ Node: review-auth │ Attempt: 1 │ ⏱ 2m 14s    │
│─────────────────────────────────────────────────────────────────────│
│                                                                     │
│ [00:00] System Prompt                                               │
│ ┊ You are reviewing the auth module for security issues...          │
│                                                                     │
│ [00:02] Agent                                                       │
│ ┊ I'll start by reading the auth middleware files.                   │
│ ┊                                                                   │
│ ┊ ┌─ read ──────────────────────────────────────────┐               │
│ ┊ │ src/auth/middleware.ts                           │               │
│ ┊ │ 142 lines read                                  │               │
│ ┊ └─────────────────────────────────────────────────┘               │
│ ┊                                                                   │
│ ┊ ┌─ read ──────────────────────────────────────────┐               │
│ ┊ │ src/auth/session.ts                             │               │
│ ┊ │ 89 lines read                                   │               │
│ ┊ └─────────────────────────────────────────────────┘               │
│ ┊                                                                   │
│ ┊ I found several issues:                                           │
│ ┊ 1. Session tokens stored in plain text (line 45)                  │
│ ┊ 2. No CSRF protection on /api/auth endpoints                     │
│ ┊ 3. JWT expiry set to 30 days (should be ~1 hour)                 │
│ ┊                                                                   │
│ ┊ ┌─ edit ──────────────────────────────────────────┐               │
│ ┊ │ src/auth/middleware.ts:45                        │               │
│ ┊ │ - const token = rawToken;                       │               │
│ ┊ │ + const token = hashToken(rawToken);            │               │
│ ┊ └─────────────────────────────────────────────────┘               │
│ ┊                                                                   │
│ █ (streaming...)                                                    │
│                                                                     │
│─────────────────────────────────────────────────────────────────────│
│ [h] Hijack this session  [f] Follow (auto-scroll)  [Esc] Back     │
│─────────────────────────────────────────────────────────────────────│
```

**Features**:
- Real-time streaming of agent output
- Tool call rendering (reusing Crush's tool renderers)
- Timestamp markers relative to run start
- Attempt navigation (if retries occurred)
- Smooth transition to hijack mode

---

### 3.4 Hijack Mode

Triggered by `/hijack <run_id>` or pressing `h` in chat viewer.

```
┌─────────────────────────────────────────────────────────────────────┐
│ SMITHERS › HIJACK › abc123 (code-review)            [Esc] Release  │
│ ⚡ YOU ARE DRIVING │ Agent: claude-code │ Node: review-auth         │
│─────────────────────────────────────────────────────────────────────│
│                                                                     │
│ [context from agent's prior conversation shown above]               │
│                                                                     │
│ ─ ─ ─ ─ ─ ─ ─ SESSION HIJACKED ─ ─ ─ ─ ─ ─ ─                     │
│                                                                     │
│ ◆ You:                                                              │
│   Actually, don't change the token hashing yet. First check if      │
│   there's an existing hash utility in src/utils/crypto.ts           │
│                                                                     │
│ ◇ Agent:                                                            │
│   Good call. Let me check that file first.                          │
│   ┌─ read ──────────────────────────────────────────┐               │
│   │ src/utils/crypto.ts                             │               │
│   │ 67 lines read                                   │               │
│   └─────────────────────────────────────────────────┘               │
│   Yes! There's already a `hashToken()` function at line 23          │
│   that uses SHA-256. I'll use that instead of introducing           │
│   a new dependency.                                                 │
│                                                                     │
│─────────────────────────────────────────────────────────────────────│
│                                                                     │
│ > _                                                                 │
│ :::                                                                 │
│                                                                     │
│ /resume  hand back to Smithers   /cancel  abort the run            │
│─────────────────────────────────────────────────────────────────────│
```

**Key UX details**:
- Clear visual indicator: "YOU ARE DRIVING" banner with highlight color
- Prior agent context visible above the hijack marker
- Separator line marks where human took over
- `/resume` returns control to Smithers (agent continues from where human left off)
- `/cancel` aborts the entire run
- `Esc` releases hijack (same as `/resume`)

---

### 3.5 Approval Queue View

Accessed via `/approvals` or notification badge click.

```
┌─────────────────────────────────────────────────────────────────────┐
│ SMITHERS › Approvals                                    [Esc] Back │
│─────────────────────────────────────────────────────────────────────│
│                                                                     │
│ ⚠ 2 PENDING APPROVALS                                              │
│                                                                     │
│ ┌───────────────────────────────────────────────────────────────┐   │
│ │ 1. Deploy to staging                              8m ago     │   │
│ │    Run: def456 (deploy-staging)                              │   │
│ │    Node: deploy · Step 4 of 6                                │   │
│ │                                                              │   │
│ │    Context:                                                  │   │
│ │    The deploy workflow has completed build, test, and lint   │   │
│ │    steps. All passed. Ready to deploy commit a1b2c3d to     │   │
│ │    staging environment.                                      │   │
│ │                                                              │   │
│ │    Changes: 3 files modified, 47 insertions, 12 deletions   │   │
│ │                                                              │   │
│ │    [a] Approve    [d] Deny    [i] Inspect run               │   │
│ ├───────────────────────────────────────────────────────────────┤   │
│ │ 2. Delete user data (GDPR request)                2m ago     │   │
│ │    Run: vwx234 (gdpr-cleanup)                                │   │
│ │    Node: delete-records · Step 3 of 4                        │   │
│ │                                                              │   │
│ │    Context:                                                  │   │
│ │    Ready to delete 142 records for user ID 98765.            │   │
│ │    This action is irreversible.                              │   │
│ │                                                              │   │
│ │    [a] Approve    [d] Deny    [i] Inspect run               │   │
│ └───────────────────────────────────────────────────────────────┘   │
│                                                                     │
│ RECENT DECISIONS                                                    │
│   ✓ Approved "Run migrations" (abc789) — 1h ago                    │
│   ✗ Denied "Force push to main" (xyz456) — 3h ago                  │
│                                                                     │
│─────────────────────────────────────────────────────────────────────│
```

---

### 3.6 Time-Travel / Timeline View

Accessed via `/timeline <run_id>`.

```
┌─────────────────────────────────────────────────────────────────────┐
│ SMITHERS › Timeline › abc123 (code-review)              [Esc] Back │
│─────────────────────────────────────────────────────────────────────│
│                                                                     │
│ Snapshots (7)                                                       │
│                                                                     │
│  ①──②──③──④──⑤──⑥──⑦                                              │
│  │  │  │  │  │  │  └─ [now] review-auth attempt 1 complete         │
│  │  │  │  │  │  └──── lint-check complete ✓                        │
│  │  │  │  │  └─────── test-runner complete ✓                       │
│  │  │  │  └────────── build complete ✓                             │
│  │  │  └───────────── fetch-deps complete ✓                        │
│  │  └──────────────── parse-config complete ✓                      │
│  └─────────────────── workflow started                              │
│                                                                     │
│─────────────────────────────────────────────────────────────────────│
│ Snapshot ⑤ → ⑥ Diff:                                               │
│                                                                     │
│  Nodes changed:                                                     │
│    lint-check: pending → finished                                   │
│  Outputs added:                                                     │
│    lint-check.result: "3 warnings, 0 errors"                       │
│  Duration: 12.4s                                                    │
│  VCS: jj change k7m2n9p                                            │
│                                                                     │
│─────────────────────────────────────────────────────────────────────│
│                                                                     │
│ [←/→] Navigate snapshots  [d] Diff selected  [f] Fork from here   │
│ [r] Replay from here      [Enter] Inspect snapshot                 │
│─────────────────────────────────────────────────────────────────────│
```

---

### 3.7 Agent Browser

Accessed via `/agents`. Mirrors `AgentsList.tsx`.

```
┌─────────────────────────────────────────────────────────────────────┐
│ SMITHERS › Agents                                       [Esc] Back │
│─────────────────────────────────────────────────────────────────────│
│                                                                     │
│ Detected CLI Agents                                                 │
│                                                                     │
│ ▸ claude-code                                                       │
│   Binary: /usr/local/bin/claude-code                                │
│   Status: ● likely-subscription                                     │
│   Auth: ✓  API Key: ✓  Roles: coding, review                      │
│                                                                     │
│   codex                                                             │
│   Binary: /usr/local/bin/codex                                      │
│   Status: ● api-key                                                 │
│   Auth: ✗  API Key: ✓  Roles: coding                              │
│                                                                     │
│   gemini                                                            │
│   Binary: /usr/local/bin/gemini                                     │
│   Status: ● api-key                                                 │
│   Auth: ✗  API Key: ✓  Roles: coding, research                    │
│                                                                     │
│   kimi                                                              │
│   Binary: —                                                         │
│   Status: ○ unavailable                                             │
│                                                                     │
│   amp                                                               │
│   Binary: /usr/local/bin/amp                                        │
│   Status: ● binary-only                                             │
│   Auth: ✗  API Key: ✗  Roles: coding                              │
│                                                                     │
│─────────────────────────────────────────────────────────────────────│
│ [Enter] Chat with agent  [r] Refresh                  [Esc] Back   │
│─────────────────────────────────────────────────────────────────────│
```

**Agent Chat** (pressing Enter on an agent):

```
┌─────────────────────────────────────────────────────────────────────┐
│ SMITHERS › Agents › claude-code                         [Esc] Back │
│─────────────────────────────────────────────────────────────────────│
│                                                                     │
│ ◆ You:                                                              │
│   What files changed in the last commit?                            │
│                                                                     │
│ ◇ claude-code:                                                      │
│   The last commit modified 3 files:                                 │
│   - src/auth/middleware.ts (47 insertions, 12 deletions)            │
│   - src/auth/session.ts (8 insertions, 3 deletions)                │
│   - tests/auth.test.ts (22 insertions)                              │
│                                                                     │
│─────────────────────────────────────────────────────────────────────│
│                                                                     │
│ > _                                                                 │
│ :::                                                                 │
│─────────────────────────────────────────────────────────────────────│
```

---

### 3.8 Ticket Manager

Accessed via `/tickets`. Mirrors `TicketsList.tsx`.

```
┌─────────────────────────────────────────────────────────────────────┐
│ SMITHERS › Tickets                                      [Esc] Back │
│─────────────────────────────────────────────────────────────────────│
│                                                                     │
│ Tickets              │ PROJ-123: Auth module security review        │
│ ──────────           │ ────────────────────────────────────         │
│                      │                                              │
│ ▸ PROJ-123           │ ## Description                               │
│   PROJ-124           │                                              │
│   PROJ-125           │ Review the auth module for security          │
│   PROJ-126           │ vulnerabilities. Focus on:                   │
│                      │                                              │
│                      │ - Session token storage                      │
│                      │ - CSRF protection                            │
│                      │ - JWT expiry settings                        │
│                      │                                              │
│                      │ ## Acceptance Criteria                       │
│                      │                                              │
│                      │ - [ ] All tokens hashed at rest              │
│                      │ - [ ] CSRF middleware on all API routes      │
│                      │ - [ ] JWT expiry < 1 hour                   │
│                      │                                              │
│ [n] New  [e] Edit    │                                              │
│─────────────────────────────────────────────────────────────────────│
│ [↑/↓] Select  [Enter] View  [e] Edit  [n] New ticket  [Esc] Back  │
│─────────────────────────────────────────────────────────────────────│
```

**Edit mode** (pressing `e`):

```
┌─────────────────────────────────────────────────────────────────────┐
│ SMITHERS › Tickets › PROJ-123 (editing)                 [Esc] Back │
│─────────────────────────────────────────────────────────────────────│
│                                                                     │
│ ┌─────────────────────────────────────────────────────────────┐     │
│ │ ## Description                                              │     │
│ │                                                             │     │
│ │ Review the auth module for security                         │     │
│ │ vulnerabilities. Focus on:                                  │     │
│ │                                                             │     │
│ │ - Session token storage                                     │     │
│ │ - CSRF protection                                           │     │
│ │ - JWT expiry settings█                                      │     │
│ │                                                             │     │
│ └─────────────────────────────────────────────────────────────┘     │
│                                                                     │
│─────────────────────────────────────────────────────────────────────│
│ [Ctrl+S] Save  [Ctrl+O] Open in $EDITOR  [Esc] Cancel             │
│─────────────────────────────────────────────────────────────────────│
```

---

### 3.9 Prompt Editor & Preview

Accessed via `/prompts`. Mirrors `PromptsList.tsx`.

```
┌─────────────────────────────────────────────────────────────────────┐
│ SMITHERS › Prompts                                      [Esc] Back │
│─────────────────────────────────────────────────────────────────────│
│                                                                     │
│ Prompts       │ Source                  │ Props & Preview            │
│ ──────        │ ──────                  │ ──────────────             │
│               │                         │                            │
│ ▸ code-review │ You are a senior code   │ Props:                     │
│   deploy-plan │ reviewer. Review the    │ ┌──────────────────────┐  │
│   test-gen    │ following {props.lang}  │ │ lang: [typescript   ]│  │
│   summarize   │ code for:              │ │ focus: [security    ]│  │
│               │                         │ └──────────────────────┘  │
│               │ - Security issues       │                            │
│               │ - Performance problems  │ Preview:                   │
│               │ - Code style            │ ─────────                  │
│               │                         │ You are a senior code      │
│               │ Focus area:             │ reviewer. Review the       │
│               │ {props.focus}           │ following typescript code   │
│               │                         │ for:                       │
│               │                         │                            │
│               │                         │ - Security issues          │
│               │                         │ - Performance problems     │
│               │                         │ - Code style               │
│               │                         │                            │
│               │                         │ Focus area: security       │
│               │                         │                            │
│─────────────────────────────────────────────────────────────────────│
│ [e] Edit source  [Tab] Switch pane  [Enter] Render  [Ctrl+S] Save  │
│─────────────────────────────────────────────────────────────────────│
```

---

### 3.10 SQL Browser

Accessed via `/sql`. Mirrors `SqlBrowser.tsx`.

```
┌─────────────────────────────────────────────────────────────────────┐
│ SMITHERS › SQL                                          [Esc] Back │
│─────────────────────────────────────────────────────────────────────│
│                                                                     │
│ Tables               │ Query:                                       │
│ ──────               │ ┌───────────────────────────────────────┐    │
│ _smithers_runs       │ │ SELECT id, status, started            │    │
│ _smithers_nodes      │ │ FROM _smithers_runs                   │    │
│ _smithers_events     │ │ WHERE status = 'failed'               │    │
│ _smithers_chat_attem │ │ ORDER BY started DESC                 │    │
│ _smithers_memory     │ │ LIMIT 10;█                            │    │
│                      │ └───────────────────────────────────────┘    │
│                      │ [Ctrl+Enter] Execute                        │
│                      │                                              │
│                      │ Results (3 rows):                            │
│                      │ ┌───────────┬──────────┬───────────────┐    │
│                      │ │ id        │ status   │ started       │    │
│                      │ ├───────────┼──────────┼───────────────┤    │
│                      │ │ stu901    │ failed   │ 2h ago        │    │
│                      │ │ uvw234    │ failed   │ yesterday     │    │
│                      │ │ xyz567    │ failed   │ 2 days ago    │    │
│                      │ └───────────┴──────────┴───────────────┘    │
│                      │                                              │
│─────────────────────────────────────────────────────────────────────│
│ [Tab] Switch pane  [Ctrl+Enter] Execute query  [Esc] Back         │
│─────────────────────────────────────────────────────────────────────│
```

---

### 3.11 Workflow Executor

When selecting a workflow in the Workflow List and pressing `r` to run:

```
┌─────────────────────────────────────────────────────────────────────┐
│ SMITHERS › Workflows › code-review (configure)          [Esc] Back │
│─────────────────────────────────────────────────────────────────────│
│                                                                     │
│ Workflow: code-review                                               │
│ Entry: .smithers/workflows/code-review.tsx                          │
│                                                                     │
│ Inputs:                                                             │
│ ┌───────────────────────────────────────────────────────────────┐   │
│ │ ticketId (string):     [PROJ-123                            ]│   │
│ │ targetBranch (string): [main                                ]│   │
│ │ maxRetries (number):   [3                                   ]│   │
│ │ dryRun (boolean):      [✓]                                   │   │
│ └───────────────────────────────────────────────────────────────┘   │
│                                                                     │
│                                          [Enter] Execute Workflow   │
│                                                                     │
│─────────────────────────────────────────────────────────────────────│
│ [Tab] Next field  [Enter] Execute  [Esc] Cancel                    │
│─────────────────────────────────────────────────────────────────────│
```

Dynamic form generation supporting string, number, boolean, object, and
array input types — matching the GUI's `WorkflowsList.tsx` form builder.

---

### 3.12 Node Inspector + Task Tabs

Reached by pressing `Enter` on a run then selecting a node. Mirrors
`NodeInspector.tsx` + `TaskTabs.tsx` from the GUI.

```
┌─────────────────────────────────────────────────────────────────────┐
│ SMITHERS › Inspect › abc123 › review-auth               [Esc] Back │
│─────────────────────────────────────────────────────────────────────│
│                                                                     │
│ Nodes                │  [Input] [Output] [Config] [Chat Logs]      │
│ ─────                │  ───────────────────────────────────────     │
│                      │                                              │
│ ✓ fetch-deps         │  Output:                                     │
│ ✓ build              │  {                                           │
│ ✓ test               │    "issues": [                               │
│ ✓ lint               │      {                                       │
│ ▸ review-auth ◀      │        "severity": "high",                   │
│ ○ deploy             │        "file": "src/auth/middleware.ts",     │
│ ○ verify             │        "line": 45,                           │
│                      │        "message": "Plain text token storage" │
│                      │      },                                      │
│                      │      {                                       │
│                      │        "severity": "medium",                 │
│                      │        "file": "src/auth/session.ts",       │
│                      │        "line": 12,                           │
│                      │        "message": "No CSRF protection"      │
│                      │      }                                       │
│                      │    ],                                        │
│                      │    "summary": "2 issues found"               │
│                      │  }                                           │
│                      │                                              │
│─────────────────────────────────────────────────────────────────────│
│ [Tab] Switch tab  [↑/↓] Select node  [c] Chat  [Esc] Back         │
│─────────────────────────────────────────────────────────────────────│
```

**Tabs**:
- **Input**: Raw JSON input passed to this node
- **Output**: Raw JSON output (with special renderers for kanban, grill-me)
- **Config**: Node configuration + attempt count
- **Chat Logs**: Agent chat frames for this node's attempts

---

### 3.13 Command Palette

Extended from Crush's command palette with Smithers-specific commands.

```
┌─────────────────────────────────────────────────────────────────────┐
│                                                                     │
│  ┌─ Commands ─────────────────────────────────────────────────┐    │
│  │                                                             │    │
│  │  > Type to filter                                          │    │
│  │                                                             │    │
│  │  ▌ Workspace ────────────────────────────────────────────  │    │
│  │  Runs Dashboard                                  ctrl+r    │    │
│  │  Workflows                                       ctrl+w    │    │
│  │  Agents                                                    │    │
│  │  Prompts                                                   │    │
│  │  Tickets                                                   │    │
│  │                                                             │    │
│  │  ▌ Systems ─────────────────────────────────────────────   │    │
│  │  SQL Browser                                               │    │
│  │  Triggers                                                  │    │
│  │                                                             │    │
│  │  ▌ Actions ─────────────────────────────────────────────   │    │
│  │  Approvals                                       ctrl+a    │    │
│  │  View Run Chat...                                          │    │
│  │  Hijack Run...                                             │    │
│  │  Timeline...                                               │    │
│  │  Scores                                                    │    │
│  │  Memory Browser                                            │    │
│  │                                                             │    │
│  │  ▌ Session ──────────────────────────────────────────────  │    │
│  │  New Session                                     ctrl+n    │    │
│  │  Sessions                                        ctrl+s    │    │
│  │  Switch Model                                    ctrl+l    │    │
│  │                                                             │    │
│  │  ▌ General ──────────────────────────────────────────────  │    │
│  │  Open External Editor                            ctrl+o    │    │
│  │  Toggle Help                                     ctrl+g    │    │
│  │  Quit                                            ctrl+c    │    │
│  │                                                             │    │
│  └─────────────────────────────────────────────────────────────┘    │
│                                                                     │
│  tab switch  ↑/↓ choose  enter confirm  esc cancel                 │
│─────────────────────────────────────────────────────────────────────│
```

---

### 3.14 Run Inspector (Summary)

When pressing `Enter` on a run in the dashboard — shows DAG overview
before drilling into individual nodes via the Node Inspector (§3.12).

```
┌─────────────────────────────────────────────────────────────────────┐
│ SMITHERS › Inspect › def456 (deploy-staging)            [Esc] Back │
│─────────────────────────────────────────────────────────────────────│
│                                                                     │
│ Status: ⏸ Paused (approval pending)                                │
│ Started: 8m 02s ago                                                 │
│ Workflow: deploy-staging.tsx                                        │
│ Agent: claude-code (claude-opus-4-6)                               │
│                                                                     │
│ DAG:                                                                │
│   ✓ fetch-deps ──┐                                                 │
│   ✓ build ───────┤                                                 │
│   ✓ test ────────┼──▸ ⏸ deploy ──▸ ○ verify ──▸ ○ notify          │
│   ✓ lint ────────┘       ⚠ needs                                   │
│                          approval                                   │
│                                                                     │
│ Node Details (deploy):                                              │
│   Agent: claude-code                                                │
│   Attempts: 0 (waiting for approval)                                │
│   Inputs:                                                           │
│     build.artifact: "dist/app-v2.3.1.tar.gz"                      │
│     test.result: "47 passed, 0 failed"                             │
│                                                                     │
│─────────────────────────────────────────────────────────────────────│
│                                                                     │
│ [Enter] Node Inspector  [a] Approve  [d] Deny  [c] Chat           │
│ [t] Timeline  [x] Cancel                                           │
│─────────────────────────────────────────────────────────────────────│
```

---

### 3.15 Notification System

Approval gates and run failures trigger inline notifications.

```
┌─────────────────────────────────────────────────────────────────────┐
│ ... (normal chat view) ...                                          │
│                                                                     │
│                    ┌──────────────────────────────────┐              │
│                    │ ⚠ Approval needed                │              │
│                    │ "Deploy to staging" (def456)     │              │
│                    │                                  │              │
│                    │ [a] Approve  [d] Deny  [v] View │              │
│                    └──────────────────────────────────┘              │
│                                                                     │
│ > _                                                                 │
│─────────────────────────────────────────────────────────────────────│
```

Notifications appear as toast-style overlays. They don't interrupt
typing but are visible. They auto-dismiss after 30s or on keypress.

---

### 3.16 Scores / ROI View

Accessed via `/scores`.

```
┌─────────────────────────────────────────────────────────────────────┐
│ SMITHERS › Scores                                       [Esc] Back │
│─────────────────────────────────────────────────────────────────────│
│                                                                     │
│ Today's Summary                                                     │
│ ─────────────────────────────────────────                           │
│ Runs: 16 total  │  12 ✓  │  3 running  │  1 ✗                     │
│ Tokens: 847K input · 312K output · $4.23 est. cost                 │
│ Avg duration: 4m 12s                                                │
│ Cache hit rate: 73%                                                 │
│                                                                     │
│ Top Workflows by Efficiency                                         │
│ ─────────────────────────────────────────                           │
│ Workflow            │ Runs │ Avg Time │ Avg Cost │ Success │ Score  │
│ ────────────────────┼──────┼──────────┼──────────┼─────────┼─────  │
│ code-review         │    8 │   3m 20s │    $0.42 │    100% │  9.2  │
│ deploy-staging      │    3 │   8m 10s │    $1.12 │     67% │  7.1  │
│ test-suite          │    3 │   2m 45s │    $0.31 │    100% │  9.5  │
│ dependency-update   │    2 │   5m 30s │    $0.67 │     50% │  5.3  │
│                                                                     │
│ Recent Evaluations                                                  │
│ ─────────────────────────────────────────                           │
│ abc123  code-review  │ relevancy: 0.94 │ faithfulness: 0.88        │
│ jkl012  code-review  │ relevancy: 0.91 │ faithfulness: 0.95        │
│                                                                     │
│─────────────────────────────────────────────────────────────────────│
│ [Enter] Drill into workflow  [r] Refresh  [Esc] Back               │
│─────────────────────────────────────────────────────────────────────│
```

---

## 4. Color Scheme

| Element | Color | Usage |
|---------|-------|-------|
| Brand/Header | Bright cyan | SMITHERS logo, active indicators |
| Running | Green | Active runs, streaming indicators |
| Approval pending | Yellow/amber | Warning badges, approval gates |
| Failed/Error | Red | Failed runs, errors |
| Completed | Dim green | Finished runs |
| Hijack mode | Bright magenta | "YOU ARE DRIVING" banner |
| Chat (user) | White/bright | User messages |
| Chat (agent) | Cyan | Agent responses |
| Tool calls | Dim/gray border | Tool call boxes |
| Timestamps | Dim gray | Relative timestamps |

---

## 5. Keybinding Map

| Key | Context | Action |
|-----|---------|--------|
| `/` or `Ctrl+P` | Any | Open command palette |
| `Esc` | Any view | Go back / return to chat |
| `Ctrl+R` | Any | Open Run Dashboard |
| `Ctrl+A` | Any | Open Approval Queue |
| `Enter` | Run list | Inspect selected run |
| `c` | Run list | View run's live chat |
| `h` | Run list / Chat viewer | Hijack run |
| `a` | Run list / Approval view | Approve gate |
| `d` | Approval view | Deny gate |
| `x` | Run list | Cancel run |
| `f` | Chat viewer | Toggle follow mode |
| `←`/`→` | Timeline | Navigate snapshots |
| `Ctrl+N` | Chat | New session |
| `Ctrl+S` | Chat | Switch session |
| `Ctrl+L` | Chat | Switch model |
| `Ctrl+O` | Chat | Open external editor |
| `Ctrl+G` | Any | Toggle help |
| `Ctrl+C` | Any | Quit |

---

## 6. State Transitions

```
                         ┌──────────┐
                         │  Console │ (default / home)
                         │  (Chat)  │
                         └────┬─────┘
                              │
     ┌────────┬───────┬───────┼───────┬────────┬────────┬────────┐
     │        │       │       │       │        │        │        │
     ▼        ▼       ▼       ▼       ▼        ▼        ▼        ▼
┌────────┐┌──────┐┌──────┐┌──────┐┌──────┐┌──────┐┌──────┐┌──────┐
│  Runs  ││Wflow ││Agents││Prompt││Ticket││  SQL ││Trigrs││Scores│
│  Dash  ││ List ││Browse││Editor││ Mgr  ││Browse││/Cron ││ /ROI │
└───┬────┘└──┬───┘└──┬───┘└──────┘└──────┘└──────┘└──────┘└──────┘
    │        │       │
    │        │       └──▸ Agent Chat
    │        │
    │        └──▸ Workflow Executor (input form)
    │
    ├──▸ Run Inspector (DAG overview)
    │       │
    │       └──▸ Node Inspector + Task Tabs
    │
    ├──▸ Live Chat Viewer
    │       │
    │       └──▸ Hijack Mode
    │
    ├──▸ Timeline / Time-Travel
    │
    └──▸ Approval Queue
```

Every view has `Esc` back to parent. Console/Chat is always the root.

---

## 7. Responsive Layout

The TUI adapts to terminal width:

**Wide (120+ cols)**: Full layout as shown above.

**Medium (80-119 cols)**: Tables truncate long fields, progress bars shorten.

**Narrow (< 80 cols)**: Single-column layout, tables become card-style lists:

```
┌─────────────────────────────────────┐
│ SMITHERS › Runs          [Esc] Back │
│─────────────────────────────────────│
│                                     │
│ ● abc123                            │
│   code-review · running             │
│   ████████░░ 3/5 · 2m 14s          │
│                                     │
│ ● def456                            │
│   deploy-staging · approval ⚠       │
│   ██████░░░░ 4/6 · 8m 02s          │
│                                     │
│ ● ghi789                            │
│   test-suite · running              │
│   ██░░░░░░░░ 1/3 · 30s             │
│                                     │
│─────────────────────────────────────│
```

---

## 8. Animations & Transitions

- **Spinner**: Active runs show an animated spinner (inherited from Crush).
- **Streaming cursor**: Live chat shows a blinking block cursor at stream end.
- **View transitions**: Slide-in/out between views (if Bubble Tea supports).
- **Notifications**: Fade in from right, dismiss after timeout.
- **Progress bars**: Smooth fill animation as nodes complete.
