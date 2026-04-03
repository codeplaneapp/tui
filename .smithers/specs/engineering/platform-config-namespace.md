# Engineering Spec: Platform Config Namespace

**Ticket**: `platform-config-namespace`
**Feature**: `PLATFORM_SMITHERS_CONFIG_NAMESPACE`
**Depends on**: `platform-smithers-rebrand`
**Date**: 2026-04-03

---

## Objective

Migrate the Crush configuration namespace (directories, file names, environment variables, skills/commands paths) to a `smithers-tui` namespace so that the Smithers TUI stores its state separately from a co-installed Crush binary. After this change, a user can run both `crush` and `smithers-tui` in the same project without config collisions.

This is the data-path complement to the rebrand ticket (`platform-smithers-rebrand`), which handles binary name, UI chrome, and user-facing strings. This ticket handles everything that touches the filesystem or environment.

---

## Scope

### In scope

1. **Constants rename** — `appName`, `defaultDataDirectory`, and `defaultInitializeAs` in `internal/config/config.go`.
2. **Config file name** — Project-level lookup changes from `crush.json` / `.crush.json` to `smithers-tui.json` / `.smithers-tui.json`.
3. **Data directory** — Default data dir changes from `.crush/` to `.smithers-tui/`.
4. **Global config paths** — `~/.config/crush/` → `~/.config/smithers-tui/`, `~/.local/share/crush/` → `~/.local/share/smithers-tui/`.
5. **Environment variables** — `CRUSH_*` → `SMITHERS_TUI_*` (with `CRUSH_*` fallback in a transition period).
6. **Skills directories** — Global and project skills paths that reference `crush`.
7. **Custom commands directories** — `~/.config/crush/commands`, `~/.crush/commands` → `smithers-tui` equivalents.
8. **Context paths** — `crush.md`, `Crush.md`, `CRUSH.md`, and their `.local.md` variants are replaced with `smithers-tui.md` / `SMITHERS-TUI.md` equivalents. `AGENTS.md` is kept as the default `initializeAs` value.
9. **Cobra root command** — `Use: "crush"` → `Use: "smithers-tui"`, examples, and `--data-dir` default text.
10. **Workspace config path** — `.crush/crush.json` → `.smithers-tui/smithers-tui.json`.
11. **Log path** — `.crush/logs/crush.log` → `.smithers-tui/logs/smithers-tui.log`.
12. **Scope comments** — `scope.go` doc strings referencing `.crush`.
13. **Smithers sub-config defaults** — `SmithersConfig` defaults (`dbPath`, `workflowDir`) remain `.smithers/` (this is the Smithers *server* data, not TUI data).
14. **Default model for Smithers agent** — When `SmithersConfig` is present, default to `claude-opus-4-6` via the `anthropic` provider instead of inheriting generic Crush defaults.

### Out of scope

- Binary name change (handled by `platform-smithers-rebrand`).
- Go module path rename (handled by `platform-smithers-rebrand`).
- UI logo/colors (handled by `platform-smithers-rebrand`).
- New Smithers-specific config keys (handled by downstream tickets like `platform-http-api-client`).

---

## Implementation Plan

### Slice 1: Core constants and data directory

**Goal**: All filesystem paths derived from `appName` and `defaultDataDirectory` resolve to the new namespace.

**Files**:

| File | Change |
|------|--------|
| `internal/config/config.go:24-28` | `appName = "smithers-tui"`, `defaultDataDirectory = ".smithers-tui"` |
| `internal/config/config.go:30-47` | Replace context path entries: `crush.md` → `smithers-tui.md`, `Crush.md` → `Smithers-tui.md`, `CRUSH.md` → `SMITHERS-TUI.md`, and their `.local.md` variants. Keep `AGENTS.md`, `CLAUDE.md`, and other non-Crush entries. |
| `internal/config/config.go:251` | Update `DataDirectory` jsonschema default/example from `.crush` to `.smithers-tui`. |
| `internal/config/scope.go:7-10` | Update doc comments: `~/.local/share/smithers-tui/smithers-tui.json`, `.smithers-tui/smithers-tui.json`. |
| `internal/config/store.go:28-29` | Update comments to `~/.local/share/smithers-tui/smithers-tui.json` and `.smithers-tui/smithers-tui.json`. |

**Data flow affected**: `Load()` → `setDefaults()` → `fsext.LookupClosest(workingDir, ".smithers-tui")` → workspace path `.smithers-tui/smithers-tui.json`. All downstream consumers of `cfg.Options.DataDirectory` automatically pick up the new path (logs, init flag, session DB, etc.).

**Verification**: `TestConfig_setDefaults` must assert `filepath.Join("/tmp", ".smithers-tui")`. Existing `TestConfig_setDefaultsWithSmithers` unchanged (it tests `.smithers/` server paths).

### Slice 2: Environment variables

**Goal**: Replace `CRUSH_*` env vars with `SMITHERS_TUI_*`, with fallback to `CRUSH_*` for transition.

**Files**:

| File | Change |
|------|--------|
| `internal/config/load.go:748-753` | `GlobalConfig()`: check `SMITHERS_TUI_GLOBAL_CONFIG` first, then `CRUSH_GLOBAL_CONFIG` as fallback. |
| `internal/config/load.go:758-777` | `GlobalConfigData()`: check `SMITHERS_TUI_GLOBAL_DATA` first, then `CRUSH_GLOBAL_DATA`. |
| `internal/config/load.go:424-430` | `setDefaults()`: check `SMITHERS_TUI_DISABLE_PROVIDER_AUTO_UPDATE` and `SMITHERS_TUI_DISABLE_DEFAULT_PROVIDERS` first, fall back to `CRUSH_*`. |
| `internal/config/load.go:799` | `GlobalSkillsDirs()`: check `SMITHERS_TUI_SKILLS_DIR` first, then `CRUSH_SKILLS_DIR`. |
| `internal/cmd/root.go:256` | Check `SMITHERS_TUI_DISABLE_METRICS` first, then `CRUSH_DISABLE_METRICS`. |

**Pattern**: Introduce a helper to reduce boilerplate:

```go
// envWithFallback returns the value of the primary env var,
// falling back to the legacy name if unset.
func envWithFallback(primary, legacy string) string {
    if v := os.Getenv(primary); v != "" {
        return v
    }
    return os.Getenv(legacy)
}
```

**Full env var mapping**:

| New (`SMITHERS_TUI_*`) | Legacy (`CRUSH_*`) |
|-------------------------|--------------------|
| `SMITHERS_TUI_GLOBAL_CONFIG` | `CRUSH_GLOBAL_CONFIG` |
| `SMITHERS_TUI_GLOBAL_DATA` | `CRUSH_GLOBAL_DATA` |
| `SMITHERS_TUI_SKILLS_DIR` | `CRUSH_SKILLS_DIR` |
| `SMITHERS_TUI_DISABLE_METRICS` | `CRUSH_DISABLE_METRICS` |
| `SMITHERS_TUI_DISABLE_PROVIDER_AUTO_UPDATE` | `CRUSH_DISABLE_PROVIDER_AUTO_UPDATE` |
| `SMITHERS_TUI_DISABLE_DEFAULT_PROVIDERS` | `CRUSH_DISABLE_DEFAULT_PROVIDERS` |
| `SMITHERS_TUI_DISABLE_ANTHROPIC_CACHE` | `CRUSH_DISABLE_ANTHROPIC_CACHE` |

### Slice 3: Skills and commands directories

**Goal**: Global and project skills/commands resolve to `smithers-tui` paths.

**Files**:

| File | Change |
|------|--------|
| `internal/config/load.go:795-834` | `GlobalSkillsDirs()`: `~/.config/smithers-tui/skills` (keep `~/.config/agents/skills` as shared). `ProjectSkillsDir()`: add `.smithers-tui/skills` alongside `.agents/skills`, `.claude/skills`, `.cursor/skills`. Remove `.crush/skills`. |
| `internal/commands/commands.go:93-107` | `buildCommandSources()`: `~/.config/smithers-tui/commands` replaces `~/.config/crush/commands`. `~/.smithers-tui/commands` replaces `~/.crush/commands`. Third source unchanged (uses `cfg.Options.DataDirectory` which already resolves to `.smithers-tui/`). |

### Slice 4: Root command and CLI help text

**Goal**: All user-visible CLI text references `smithers-tui`.

**Files**:

| File | Change |
|------|--------|
| `internal/cmd/root.go:58-86` | `Use: "smithers-tui"`, update `Short`, `Long`, all `Example` strings. Replace `crush` → `smithers-tui` and `.crush` → `.smithers-tui` in examples. |

### Slice 5: Smithers-specific default model

**Goal**: When `SmithersConfig` is present in the config, default the large model to `claude-opus-4-6` via the `anthropic` provider.

**Files**:

| File | Change |
|------|--------|
| `internal/config/load.go` (within `setDefaults` or model selection) | If `c.Smithers != nil` and `c.Models[SelectedModelTypeLarge]` is zero-valued, set it to `SelectedModel{Model: "claude-opus-4-6", Provider: "anthropic", Think: true}`. |

This ensures the Smithers agent gets a capable model by default without requiring users to manually configure it.

### Slice 6: Update existing tests

**Goal**: All existing config tests pass against the new namespace.

**Files**:

| File | Change |
|------|--------|
| `internal/config/load_test.go:56` | Assert `.smithers-tui` instead of `.crush`. |
| `internal/config/load_test.go` | Add test for env var fallback (`CRUSH_*` → `SMITHERS_TUI_*`). |
| `internal/config/load_test.go` | Add test for config file lookup: `smithers-tui.json` and `.smithers-tui.json` are found. |
| `internal/config/load_test.go` | Add test: `SMITHERS_TUI_GLOBAL_CONFIG` overrides `CRUSH_GLOBAL_CONFIG`. |

---

## Validation

### Unit tests (`go test ./internal/config/...`)

1. **`TestConfig_setDefaults`** — asserts `cfg.Options.DataDirectory == filepath.Join(workDir, ".smithers-tui")`.
2. **`TestConfig_setDefaultsWithSmithers`** — unchanged, still asserts `.smithers/smithers.db` (server paths).
3. **`TestConfig_lookupConfigs`** — new test: creates `smithers-tui.json` in a temp dir, runs `lookupConfigs`, asserts it is found. Also verifies `.crush.json` is NOT found.
4. **`TestConfig_envVarFallback`** — new test: sets `CRUSH_GLOBAL_CONFIG=/old`, asserts `GlobalConfig()` uses it. Then sets `SMITHERS_TUI_GLOBAL_CONFIG=/new`, asserts it takes precedence.
5. **`TestConfig_GlobalSkillsDirs`** — new test: asserts paths contain `smithers-tui` and `agents`, not `crush`.
6. **`TestConfig_ProjectSkillsDir`** — new test: asserts `.smithers-tui/skills` is in the list, `.crush/skills` is not.
7. **`TestConfig_SmithersDefaultModel`** — new test: config with `Smithers: &SmithersConfig{}` and empty `Models` map. After `setDefaults`, assert `Models[SelectedModelTypeLarge].Model == "claude-opus-4-6"`.

Run: `go test -v -run "Test" ./internal/config/...`

### Unit tests (`go test ./internal/commands/...`)

8. **`TestBuildCommandSources`** — new test: asserts command source paths contain `smithers-tui`, not `crush`.

Run: `go test -v ./internal/commands/...`

### Integration smoke test (manual)

9. **Fresh directory**: Run `smithers-tui` in a new project dir. Verify `.smithers-tui/` is created, not `.crush/`. Verify `.smithers-tui/smithers-tui.json` is the workspace config. Verify `.smithers-tui/logs/smithers-tui.log` is written.
10. **Env var override**: `SMITHERS_TUI_GLOBAL_CONFIG=/tmp/test smithers-tui --debug` — verify debug log shows the overridden path.
11. **Legacy fallback**: `CRUSH_GLOBAL_CONFIG=/tmp/legacy smithers-tui --debug` — verify it still picks up the legacy path when `SMITHERS_TUI_*` is unset.

### Terminal E2E test (modeled on upstream `@microsoft/tui-test` harness)

The upstream Smithers TUI E2E tests (`smithers_tmp/tests/tui.e2e.test.ts` + `smithers_tmp/tests/tui-helpers.ts`) use a process-spawning approach: launch the TUI binary, poll stdout for expected text, send keystrokes, and assert buffer contents.

We replicate this pattern in Go using a helper that wraps `exec.Command` to spawn the `smithers-tui` binary:

**File**: `tests/e2e/config_namespace_test.go`

```go
func TestE2E_ConfigNamespace(t *testing.T) {
    // 1. Build the binary
    // 2. Create a temp project dir
    // 3. Spawn `smithers-tui` with TERM=xterm-256color
    // 4. Wait for startup text ("SMITHERS" header)
    // 5. Send Ctrl+C to exit
    // 6. Assert .smithers-tui/ directory was created
    // 7. Assert .smithers-tui/smithers-tui.json exists
    // 8. Assert .crush/ was NOT created
    // 9. Assert .smithers-tui/logs/smithers-tui.log exists
}
```

The test helper mirrors the upstream `tui-helpers.ts` pattern:
- `launchTUI(dir string) *TUIInstance` — spawns binary with piped stdin/stdout
- `(*TUIInstance).WaitForText(text string, timeout time.Duration) error` — polls stripped ANSI output
- `(*TUIInstance).SendKeys(keys string)` — writes to stdin
- `(*TUIInstance).Snapshot() string` — returns current buffer
- `(*TUIInstance).Terminate()` — sends SIGTERM

**File**: `tests/e2e/helpers_test.go` — reusable TUI test harness in Go.

### VHS happy-path recording test

**File**: `tests/vhs/config_namespace.tape`

```
# VHS tape: Verify Smithers TUI config namespace
Output config_namespace.gif
Set Shell bash
Set Width 120
Set Height 30
Set FontSize 14

Type "cd $(mktemp -d)" Enter
Sleep 500ms

Type "smithers-tui" Enter
Sleep 2s

# Verify header says SMITHERS, not CRUSH
Screenshot config_namespace_header.png

Type "/quit" Enter
Sleep 500ms

# Verify data directory
Type "ls -la .smithers-tui/" Enter
Sleep 500ms
Screenshot config_namespace_dir.png

Type "cat .smithers-tui/smithers-tui.json 2>/dev/null || echo 'no config yet'" Enter
Sleep 500ms
Screenshot config_namespace_config.png

Type "test -d .crush && echo 'FAIL: .crush exists' || echo 'PASS: no .crush'" Enter
Sleep 500ms
Screenshot config_namespace_no_crush.png
```

Run: `vhs tests/vhs/config_namespace.tape`

This produces a GIF recording and screenshots that can be visually inspected in CI or manually.

---

## Risks

### 1. Existing `.crush/` user data is orphaned

**Impact**: Medium. Users who have been running the Crush fork during development will have sessions, configs, and logs in `.crush/`. After this change, those are invisible.

**Mitigation**: Do NOT add migration logic in v1. This is a pre-release fork — there is no installed user base to migrate. If needed later, a `smithers-tui migrate` subcommand can copy `.crush/` → `.smithers-tui/`.

### 2. Hardcoded `.crush` references outside `internal/config/` and `internal/commands/`

**Impact**: Low-medium. Grep reveals the constants are well-centralized, but there may be test fixtures, documentation, or generated schema output that reference `.crush`.

**Mitigation**: Run `grep -r '\.crush' --include='*.go' internal/` after the change and fix any stragglers. Also grep for `"crush"` in non-test Go files to catch string literals.

### 3. Upstream Crush cherry-pick conflicts

**Impact**: Low. Changing `appName` and `defaultDataDirectory` in `config.go` means any upstream Crush commit that touches these constants will conflict on merge.

**Mitigation**: These constants are stable (rarely changed upstream). The fork strategy (per `03-ENGINEERING.md` §1.1) already accepts divergence in config paths as an expected fork cost.

### 4. Environment variable fallback creates ambiguity

**Impact**: Low. If both `SMITHERS_TUI_GLOBAL_CONFIG` and `CRUSH_GLOBAL_CONFIG` are set, the new one wins. This could confuse users who set both.

**Mitigation**: Log a warning at `slog.Warn` level when the legacy variable is used: `"Using legacy CRUSH_GLOBAL_CONFIG; set SMITHERS_TUI_GLOBAL_CONFIG instead"`. Remove fallback in a future release.

### 5. Mismatch: Smithers server data vs TUI data

**Impact**: Low but worth calling out. The Smithers server stores its data in `.smithers/` (DB, workflows, tickets). The TUI stores its data in `.smithers-tui/` (sessions, config, logs). These are intentionally separate — the TUI is a thin frontend that reads `.smithers/` but writes its own state to `.smithers-tui/`.

**Mitigation**: Document this clearly. The `SmithersConfig.DBPath` and `SmithersConfig.WorkflowDir` defaults already point to `.smithers/`, not `.smithers-tui/`. This is correct and must not be changed.

### 6. Context paths: `crush.md` vs `smithers-tui.md` discoverability

**Impact**: Low. Replacing `crush.md` with `smithers-tui.md` in `defaultContextPaths` means existing `crush.md` files won't be auto-discovered.

**Mitigation**: Acceptable for a hard fork. The primary context file is `AGENTS.md` (kept as default). Users can explicitly add `crush.md` to `context_paths` in config if needed.

### 7. Crush upstream divergence in `internal/config/load.go`

**Impact**: Medium. The `load.go` file is the most complex config file and is the most likely to receive upstream changes. Our env var changes touch multiple functions in this file.

**Mitigation**: Keep changes minimal and well-commented. The `envWithFallback` helper centralizes the pattern, making future merge conflicts easier to resolve.
