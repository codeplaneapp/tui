## Goal

Complete the Smithers TUI config namespace migration. The core filesystem paths (`.smithers-tui/`), environment variables (`SMITHERS_TUI_*`), skills directories, and commands paths are already migrated. The remaining work is: (1) Cobra and subcommand help-text sweep, (2) UI string literals and embedded asset rename, (3) `CRUSH_*` → `SMITHERS_TUI_*` transition fallback shim, (4) Smithers-specific default model when `SmithersConfig` is present, and (5) missing config unit tests.

## Steps

### 1. Cobra root command and subcommand text sweep

Replace every `crush` string literal in `internal/cmd/` that is user-visible (command `Use` fields, `Short`/`Long` descriptions, `Example` strings, and inline error messages). These are purely textual and carry no logic risk.

- `internal/cmd/root.go`: `Use: "crush"` → `Use: "smithers-tui"`. Update all example lines: replace `crush` with `smithers-tui`, replace `.crush` with `.smithers-tui`. Update `Short` and `Long` to reference Smithers TUI.
- `internal/cmd/models.go`: replace `crush models` in examples and "please run 'crush'" in error message.
- `internal/cmd/run.go`: replace six `crush run ...` examples and one "please run 'crush'" error.
- `internal/cmd/dirs.go`: replace `crush dirs` examples.
- `internal/cmd/update_providers.go`: replace `crush update-providers` examples.
- `internal/cmd/logs.go`: `Short: "View crush logs"` → `Short: "View smithers-tui logs"`.
- `internal/cmd/schema.go`: `Long: "Generate JSON schema for the crush configuration file"` → references smithers-tui.
- `internal/cmd/projects.go`: replace `crush projects` examples.
- `internal/cmd/login.go`: replace `crush login` examples.

### 2. UI string literal updates

- `internal/ui/model/ui.go:2238`: `v.WindowTitle = "crush " + ...` → `v.WindowTitle = "smithers-tui " + ...`
- `internal/ui/model/ui.go:2775`: replace `"crush"` string literal — check context to confirm appropriate replacement (likely window title or metric identifier).
- `internal/ui/common/diff.go:13`: `chroma.MustNewStyle("crush", ...)` → `chroma.MustNewStyle("smithers-tui", ...)` — this is an internal Chroma theme registry key; no user-visible impact but should match the binary name for consistency.
- `internal/ui/common/highlight.go:34`: same Chroma theme rename.

### 3. Notification icon rename

- Rename `internal/ui/notification/crush-icon-solo.png` → `internal/ui/notification/smithers-tui-icon.png`.
- Update `//go:embed crush-icon-solo.png` directive in `internal/ui/notification/icon_other.go` to `//go:embed smithers-tui-icon.png`.
- Update the corresponding variable that holds the embedded bytes.

This is a binary file; rename with `git mv` to preserve history.

### 4. Add `envWithFallback` transition shim

Introduce a private helper in `internal/config/load.go`:

```go
// envWithFallback returns the value of the primary env var, falling back to
// the legacy CRUSH_* name if unset. A warning is logged when the legacy name
// is used so operators can migrate.
func envWithFallback(primary, legacy string) string {
    if v := os.Getenv(primary); v != "" {
        return v
    }
    if v := os.Getenv(legacy); v != "" {
        slog.Warn("Using legacy environment variable; please migrate to the new name",
            "legacy", legacy, "replacement", primary)
        return v
    }
    return ""
}
```

Wire into the five existing env var reads that currently check `SMITHERS_TUI_*` only:

| Function / location | Old check | New check |
|---------------------|-----------|-----------|
| `GlobalConfig()` (load.go:750) | `os.Getenv("SMITHERS_TUI_GLOBAL_CONFIG")` | `envWithFallback("SMITHERS_TUI_GLOBAL_CONFIG", "CRUSH_GLOBAL_CONFIG")` |
| `GlobalConfigData()` (load.go:759) | `os.Getenv("SMITHERS_TUI_GLOBAL_DATA")` | `envWithFallback("SMITHERS_TUI_GLOBAL_DATA", "CRUSH_GLOBAL_DATA")` |
| `GlobalSkillsDirs()` (load.go:799) | `os.Getenv("SMITHERS_TUI_SKILLS_DIR")` | `envWithFallback("SMITHERS_TUI_SKILLS_DIR", "CRUSH_SKILLS_DIR")` |
| `setDefaults()` (load.go:424) | `os.LookupEnv("SMITHERS_TUI_DISABLE_PROVIDER_AUTO_UPDATE")` | use `envWithFallback` + `strconv.ParseBool` |
| `setDefaults()` (load.go:428) | `os.LookupEnv("SMITHERS_TUI_DISABLE_DEFAULT_PROVIDERS")` | use `envWithFallback` + `strconv.ParseBool` |
| `root.go:256` | `os.Getenv("SMITHERS_TUI_DISABLE_METRICS")` | `envWithFallback("SMITHERS_TUI_DISABLE_METRICS", "CRUSH_DISABLE_METRICS")` |

Note: `SMITHERS_TUI_DISABLE_ANTHROPIC_CACHE` (agent.go), `SMITHERS_TUI_CORE_UTILS` (shell/coreutils.go), and `SMITHERS_TUI_UI_DEBUG` (ui.go) were never `CRUSH_*` vars — they are new Smithers additions and do not need fallback.

Mark the fallback for removal in a future release with a `// TODO(smithers-tui): remove CRUSH_* fallback after v1.0` comment.

### 5. Smithers-specific default model

In `internal/config/load.go`, within `setDefaults()`, after the `c.Smithers != nil` block that sets `DBPath` and `WorkflowDir`, add:

```go
if c.Smithers != nil {
    if _, ok := c.Models[SelectedModelTypeLarge]; !ok {
        if c.Models == nil {
            c.Models = make(map[SelectedModelType]SelectedModel)
        }
        c.Models[SelectedModelTypeLarge] = SelectedModel{
            Model:    "claude-opus-4-6",
            Provider: "anthropic",
            Think:    true,
        }
    }
}
```

This fires only when `smithers` config is present and the user has not explicitly chosen a large model. The `configureSelectedModels` function downstream will validate and potentially fall back to the provider default if `claude-opus-4-6` is not available.

### 6. Missing config unit tests

Add to `internal/config/load_test.go`:

**`TestConfig_lookupConfigs`** — create a temp dir, write `smithers-tui.json` in it, run `lookupConfigs(tempDir)`, assert the file is in the returned slice. Also assert `crush.json` is NOT discovered.

**`TestConfig_envVarFallback`** — set `CRUSH_GLOBAL_CONFIG=/tmp/legacy`, call `GlobalConfig()`, assert it contains `/tmp/legacy`. Then also set `SMITHERS_TUI_GLOBAL_CONFIG=/tmp/primary`, assert `GlobalConfig()` returns the primary. Unset both. Repeat for `CRUSH_GLOBAL_DATA` / `SMITHERS_TUI_GLOBAL_DATA`.

**`TestConfig_GlobalSkillsDirs`** — call `GlobalSkillsDirs()` with no env override, assert returned paths contain `smithers-tui` and `agents`, do NOT contain `crush`.

**`TestConfig_ProjectSkillsDir`** — call `ProjectSkillsDir("/tmp/proj")`, assert `.smithers-tui/skills` is in the list, `.crush/skills` is not.

**`TestConfig_SmithersDefaultModel`** — create `Config{Smithers: &SmithersConfig{}}`, call `setDefaults("/tmp", "")`, assert `cfg.Models[SelectedModelTypeLarge].Model == "claude-opus-4-6"` and `cfg.Models[SelectedModelTypeLarge].Provider == "anthropic"`.

**`TestConfig_SmithersDefaultModelNotOverridden`** — create `Config{Smithers: &SmithersConfig{}, Models: map[SelectedModelType]SelectedModel{SelectedModelTypeLarge: {Model: "gpt-4o", Provider: "openai"}}}`, call `setDefaults("/tmp", "")`, assert the large model is still `gpt-4o` (user preference not clobbered).

Add to `internal/commands/commands_test.go` (new file if not present):

**`TestBuildCommandSources`** — assert returned sources contain `smithers-tui` in all paths, do NOT contain `crush`.

### 7. Optional: rename dev config

Rename `crush.json` in the repo root to `smithers-tui.json`. Update `$schema` from `https://charm.land/crush.json` to point to the Smithers TUI schema endpoint (or remove the `$schema` key until the Smithers schema is published). This file is a gopls editor config, not an application config, but the naming inconsistency is confusing.

## File Plan

- [internal/cmd/root.go](/Users/williamcory/crush/internal/cmd/root.go)
- [internal/cmd/models.go](/Users/williamcory/crush/internal/cmd/models.go)
- [internal/cmd/run.go](/Users/williamcory/crush/internal/cmd/run.go)
- [internal/cmd/dirs.go](/Users/williamcory/crush/internal/cmd/dirs.go)
- [internal/cmd/update_providers.go](/Users/williamcory/crush/internal/cmd/update_providers.go)
- [internal/cmd/logs.go](/Users/williamcory/crush/internal/cmd/logs.go)
- [internal/cmd/schema.go](/Users/williamcory/crush/internal/cmd/schema.go)
- [internal/cmd/projects.go](/Users/williamcory/crush/internal/cmd/projects.go)
- [internal/cmd/login.go](/Users/williamcory/crush/internal/cmd/login.go)
- [internal/config/load.go](/Users/williamcory/crush/internal/config/load.go)
- [internal/config/load_test.go](/Users/williamcory/crush/internal/config/load_test.go)
- [internal/ui/model/ui.go](/Users/williamcory/crush/internal/ui/model/ui.go)
- [internal/ui/common/diff.go](/Users/williamcory/crush/internal/ui/common/diff.go)
- [internal/ui/common/highlight.go](/Users/williamcory/crush/internal/ui/common/highlight.go)
- [internal/ui/notification/icon_other.go](/Users/williamcory/crush/internal/ui/notification/icon_other.go)
- [internal/ui/notification/crush-icon-solo.png](/Users/williamcory/crush/internal/ui/notification/crush-icon-solo.png) → rename to `smithers-tui-icon.png`
- [crush.json](/Users/williamcory/crush/crush.json) (optional rename to `smithers-tui.json`)
- [internal/commands/commands_test.go](/Users/williamcory/crush/internal/commands/commands_test.go) (new or extend)

## Validation

1. `go build -o /tmp/smithers-tui-test ./...` — confirms no compilation errors after string changes and embed directive update.
2. `go test ./internal/config/... -v -run 'TestConfig_'` — runs all config tests including new ones.
3. `go test ./internal/commands/... -v -run 'TestBuildCommandSources'` — confirms commands path test.
4. `go test ./internal/config/... -v -run 'TestConfig_envVarFallback'` — specifically validates the CRUSH_* transition shim.
5. Manual smoke: `CRUSH_GLOBAL_CONFIG=/tmp/legacy ./smithers-tui --debug 2>&1 | grep -i legacy` — should emit a `slog.Warn` log line referencing the legacy var.
6. Manual smoke: run `./smithers-tui --help` and confirm the output says `smithers-tui`, not `crush`, in usage line and examples.
7. `./smithers-tui --help` — verify `--data-dir` description says `.smithers-tui` not `.crush`.
8. Run in fresh temp dir: `mkdir /tmp/stui-test && ./smithers-tui --cwd /tmp/stui-test` (then immediately exit); verify `/tmp/stui-test/.smithers-tui/` was created and no `/tmp/stui-test/.crush/` directory was created.
9. `go test ./...` — full suite must pass.

## Open Questions

1. The `envWithFallback` helper centralizes five call sites in `load.go` but `SMITHERS_TUI_DISABLE_METRICS` is in `internal/cmd/root.go`. Should the helper live in a shared location (e.g., `internal/config/env.go`) accessible to both packages, or should `root.go` inline its own fallback?
2. For the notification icon rename: should the new file be `smithers-tui-icon.png` (matching binary name) or `smithers-icon.png` (matching product brand)? The icon is used for desktop notifications and ideally carries the Smithers wordmark, not the TUI-specific binary name.
3. The Chroma theme name `"crush"` is an internal registry key. If any external user has referenced this theme name in a custom config, renaming it would break them. However, since Chroma styles are only registered at init time by internal code, there should be no external exposure. Confirm before renaming.
4. Should the repo-root `crush.json` rename to `smithers-tui.json` be a separate commit from the code changes, to keep the PR diff clean?
