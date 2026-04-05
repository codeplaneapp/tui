# Platform: Config Namespace Migration Research

First-pass research based on current code (not speculation).

## Existing Crush Surface

### What Has Already Been Migrated

The core config namespace constants in `internal/config/config.go` are **already updated**:

- `appName = "smithers-tui"` ([config.go](/Users/williamcory/crush/internal/config/config.go#L25))
- `defaultDataDirectory = ".smithers-tui"` ([config.go](/Users/williamcory/crush/internal/config/config.go#L26))
- `defaultInitializeAs = "AGENTS.md"` ([config.go](/Users/williamcory/crush/internal/config/config.go#L27))
- `defaultContextPaths` includes `smithers-tui.md`, `SMITHERS-TUI.md`, `Smithers-tui.md` and `.local.md` variants, and no `crush.md` entries ([config.go](/Users/williamcory/crush/internal/config/config.go#L30))

Scope comments are already updated: `ScopeGlobal` targets `~/.local/share/smithers-tui/smithers-tui.json` and `ScopeWorkspace` targets `.smithers-tui/smithers-tui.json` ([scope.go](/Users/williamcory/crush/internal/config/scope.go#L7))

`ConfigStore` comment strings reference `smithers-tui` paths ([store.go](/Users/williamcory/crush/internal/config/store.go#L28))

All environment variables are already migrated to `SMITHERS_TUI_*` in `load.go`:
- `SMITHERS_TUI_GLOBAL_CONFIG` ([load.go](/Users/williamcory/crush/internal/config/load.go#L750))
- `SMITHERS_TUI_GLOBAL_DATA` ([load.go](/Users/williamcory/crush/internal/config/load.go#L759))
- `SMITHERS_TUI_SKILLS_DIR` ([load.go](/Users/williamcory/crush/internal/config/load.go#L799))
- `SMITHERS_TUI_DISABLE_PROVIDER_AUTO_UPDATE` ([load.go](/Users/williamcory/crush/internal/config/load.go#L424))
- `SMITHERS_TUI_DISABLE_DEFAULT_PROVIDERS` ([load.go](/Users/williamcory/crush/internal/config/load.go#L428))
- `SMITHERS_TUI_DISABLE_METRICS` in root.go ([root.go](/Users/williamcory/crush/internal/cmd/root.go#L256))
- `SMITHERS_TUI_DISABLE_ANTHROPIC_CACHE` in agent.go ([agent.go](/Users/williamcory/crush/internal/agent/agent.go#L708))
- `SMITHERS_TUI_CORE_UTILS` in shell/coreutils.go ([coreutils.go](/Users/williamcory/crush/internal/shell/coreutils.go#L14))
- `SMITHERS_TUI_UI_DEBUG` in ui.go ([ui.go](/Users/williamcory/crush/internal/ui/model/ui.go#L2186))

Skills directories are already updated: `~/.config/smithers-tui/skills` and `~/.config/agents/skills` (global), `.smithers-tui/skills` (project) ([load.go](/Users/williamcory/crush/internal/config/load.go#L795))

Custom commands directories are already updated: `~/.config/smithers-tui/commands` and `~/.smithers-tui/commands` ([commands.go](/Users/williamcory/crush/internal/commands/commands.go#L94))

Config file lookup already searches `smithers-tui.json` and `.smithers-tui.json` via `appName + ".json"` pattern ([load.go](/Users/williamcory/crush/internal/config/load.go#L669))

Data path defaults: `GlobalConfig()` returns `~/.config/smithers-tui/smithers-tui.json`; `GlobalConfigData()` returns `~/.local/share/smithers-tui/smithers-tui.json` on Linux/macOS, `%LOCALAPPDATA%/smithers-tui/smithers-tui.json` on Windows; respects `XDG_DATA_HOME` ([load.go](/Users/williamcory/crush/internal/config/load.go#L748))

Load function writes workspace config to `.smithers-tui/smithers-tui.json` and log to `.smithers-tui/logs/smithers-tui.log` ([load.go](/Users/williamcory/crush/internal/config/load.go#L48))

`PushPopSmithersTUIEnv` already prefix-strips `SMITHERS_TUI_` ([load.go](/Users/williamcory/crush/internal/config/load.go#L125))

Config tests already assert `.smithers-tui` as the data directory and `AGENTS.md` as the initialize-as default ([load_test.go](/Users/williamcory/crush/internal/config/load_test.go#L56))

`SmithersConfig` struct is defined with `DBPath`, `APIURL`, `APIToken`, `WorkflowDir` fields ([config.go](/Users/williamcory/crush/internal/config/config.go#L373)); defaults point to `.smithers/smithers.db` and `.smithers/workflows` (server data, intentionally separate from `.smithers-tui/` TUI data) ([load.go](/Users/williamcory/crush/internal/config/load.go#L401))

`Config` struct top-level description says "holds the configuration for smithers-tui" ([config.go](/Users/williamcory/crush/internal/config/config.go#L381))

`Attribution.GeneratedWith` description references "Smithers TUI" ([config.go](/Users/williamcory/crush/internal/config/config.go#L232))

`Options.DataDirectory` jsonschema default already references `.smithers-tui` ([config.go](/Users/williamcory/crush/internal/config/config.go#L251))

### What Has NOT Yet Been Migrated

The Cobra root command still declares `Use: "crush"` and all examples use `crush`:
- `internal/cmd/root.go:59`: `Use: "crush"` ([root.go](/Users/williamcory/crush/internal/cmd/root.go#L59))
- Examples in `root.go` lines 64–86 still use `crush` ([root.go](/Users/williamcory/crush/internal/cmd/root.go#L64))
- Note: line 79 mixes old/new: `crush --data-dir /path/to/custom/.smithers-tui` — binary name not yet updated

Multiple subcommand files still embed `crush` in example strings:
- `internal/cmd/models.go`: `crush models`, `crush models gpt5`, "please run 'crush'" error message ([models.go](/Users/williamcory/crush/internal/cmd/models.go#L22))
- `internal/cmd/run.go`: six `crush run ...` examples and one "please run 'crush'" error ([run.go](/Users/williamcory/crush/internal/cmd/run.go#L24))
- `internal/cmd/dirs.go`: `crush dirs`, `crush dirs config`, `crush dirs data` ([dirs.go](/Users/williamcory/crush/internal/cmd/dirs.go#L21))
- `internal/cmd/update_providers.go`: `crush update-providers ...` examples ([update_providers.go](/Users/williamcory/crush/internal/cmd/update_providers.go#L21))
- `internal/cmd/logs.go`: `Short: "View crush logs"` ([logs.go](/Users/williamcory/crush/internal/cmd/logs.go#L25))
- `internal/cmd/schema.go`: `Long: "Generate JSON schema for the crush configuration file"` ([schema.go](/Users/williamcory/crush/internal/cmd/schema.go#L15))
- `internal/cmd/projects.go`: `crush projects`, `crush projects --json` ([projects.go](/Users/williamcory/crush/internal/cmd/projects.go#L20))
- `internal/cmd/login.go`: `crush login`, `crush login copilot` ([login.go](/Users/williamcory/crush/internal/cmd/login.go#L30))

UI files still reference `crush` as a string literal:
- `internal/ui/model/ui.go:2238`: `v.WindowTitle = "crush " + ...` ([ui.go](/Users/williamcory/crush/internal/ui/model/ui.go#L2238))
- `internal/ui/model/ui.go:2775`: string literal `"crush"` used in context ([ui.go](/Users/williamcory/crush/internal/ui/model/ui.go#L2775))
- `internal/ui/common/diff.go:13`: `chroma.MustNewStyle("crush", ...)` — Chroma syntax theme named "crush" ([diff.go](/Users/williamcory/crush/internal/ui/common/diff.go#L13))
- `internal/ui/common/highlight.go:34`: same `chroma.MustNewStyle("crush", ...)` ([highlight.go](/Users/williamcory/crush/internal/ui/common/highlight.go#L34))
- `internal/ui/notification/icon_other.go:9`: `//go:embed crush-icon-solo.png` — embedded icon file still named with `crush` prefix ([icon_other.go](/Users/williamcory/crush/internal/ui/notification/icon_other.go#L9))

No `CRUSH_*` → `SMITHERS_TUI_*` fallback / transition shim exists. The engineering spec calls for a `envWithFallback` helper that checks `SMITHERS_TUI_*` first, then falls back to `CRUSH_*`. This is not yet implemented — there is no fallback at all. Users who had previously set `CRUSH_GLOBAL_CONFIG` etc. will silently lose their config.

No `TestConfig_lookupConfigs`, `TestConfig_envVarFallback`, `TestConfig_GlobalSkillsDirs`, `TestConfig_ProjectSkillsDir`, or `TestConfig_SmithersDefaultModel` tests exist yet.

No Smithers-specific default model logic: when `c.Smithers != nil` and `Models` map is empty, there is no code that defaults `SelectedModelTypeLarge` to `claude-opus-4-6`. This is called for by the engineering spec but not implemented.

## Config File Format — Current State

The `crush.json` in the repo root is a **development-tool config** (LSP settings for gopls), not an application config. Its `$schema` references `https://charm.land/crush.json`. This file is unrelated to the namespace migration — it is the developer's editor config and can be renamed to `smithers-tui.json` with the `$schema` pointing to the Smithers TUI JSON schema once the schema command is updated.

The application config format itself (the `Config` struct) is already fully namespaced to Smithers TUI. The JSON key structure (`"smithers"`, `"models"`, `"providers"`, etc.) does not contain any `crush` keys.

## XDG and Home Directory Conventions

- **Config dir** (`~/.config/smithers-tui/`): respects `XDG_CONFIG_HOME` via `home.Config()` ([load.go](/Users/williamcory/crush/internal/config/load.go#L753))
- **Data dir** (`~/.local/share/smithers-tui/`): respects `XDG_DATA_HOME` explicitly checked ([load.go](/Users/williamcory/crush/internal/config/load.go#L762))
- **Windows**: `%LOCALAPPDATA%/smithers-tui/` for data; `%USERPROFILE%/.config/smithers-tui/` for config ([load.go](/Users/williamcory/crush/internal/config/load.go#L769))
- **Workspace**: `.smithers-tui/smithers-tui.json` — relative to `cwd`, searched upward by `fsext.LookupClosest` ([load.go](/Users/williamcory/crush/internal/config/load.go#L380))
- Project-level config names searched: `smithers-tui.json` and `.smithers-tui.json`, found via `fsext.Lookup` walking upward from cwd ([load.go](/Users/williamcory/crush/internal/config/load.go#L669))

The XDG implementation is correct and complete. No gaps here.

## Backwards Compatibility Considerations

**Env var gap**: No `CRUSH_*` fallback is implemented. The engineering spec explicitly called for a transition-period `envWithFallback` helper with a `slog.Warn` when the legacy var is used. Without this, any scripts or CI environments that set `CRUSH_GLOBAL_CONFIG`, `CRUSH_GLOBAL_DATA`, or `CRUSH_DISABLE_METRICS` will silently receive defaults, not the intended override.

**Config file gap**: The old config file name `crush.json` is not searched. `fsext.Lookup` only looks for `smithers-tui.json` and `.smithers-tui.json`. For a hard fork in pre-release this is acceptable (no production user base), but the spec calls this out as a known risk.

**Data directory gap**: `.crush/` directories are not migrated or read. Sessions, logs, and workspace configs from any prior Crush development usage will be orphaned. Acceptable for pre-release.

**Chroma theme name**: The Chroma syntax highlighting theme is registered as `"crush"` in two places. This is an internal theme registry key, not a user-facing name, but it should still be updated to avoid confusion when debugging.

**Window title**: The window title string `"crush "` appears in the Bubble Tea UI model. This is user-visible (in terminal title bar / tmux tab) and will still say "crush" until updated.

**Notification icon**: The embedded `crush-icon-solo.png` file is referenced by `//go:embed`. The file itself exists at that path; renaming it requires both renaming the file and updating the directive.

## All Config References in Codebase

### Fully migrated (no action needed)
- `internal/config/config.go` — constants, structs, field descriptions
- `internal/config/scope.go` — Scope constants and comments
- `internal/config/store.go` — ConfigStore comments and path fields
- `internal/config/load.go` — `GlobalConfig()`, `GlobalConfigData()`, `GlobalSkillsDirs()`, `ProjectSkillsDir()`, `setDefaults()`, env var reads
- `internal/config/load_test.go` — assertions for `.smithers-tui` and `AGENTS.md`
- `internal/commands/commands.go` — `buildCommandSources()` paths
- `internal/agent/agent.go` — `SMITHERS_TUI_DISABLE_ANTHROPIC_CACHE`
- `internal/shell/coreutils.go` — `SMITHERS_TUI_CORE_UTILS`
- `internal/ui/model/ui.go` — `SMITHERS_TUI_UI_DEBUG` (partially — window title and one other string still say `crush`)
- `internal/config/provider.go` — error message references `SMITHERS_TUI_DISABLE_PROVIDER_AUTO_UPDATE`

### Partially migrated (action needed)
- `internal/cmd/root.go` — env var check is done, but `Use: "crush"` and all examples still say `crush`
- `internal/ui/model/ui.go` — `SMITHERS_TUI_UI_DEBUG` done, but window title and one string literal still say `crush`

### Not yet migrated (action needed)
- `internal/cmd/models.go` — example strings and error message
- `internal/cmd/run.go` — example strings and error message
- `internal/cmd/dirs.go` — example strings
- `internal/cmd/update_providers.go` — example strings
- `internal/cmd/logs.go` — Short description
- `internal/cmd/schema.go` — Long description
- `internal/cmd/projects.go` — example strings
- `internal/cmd/login.go` — example strings
- `internal/ui/common/diff.go` — Chroma theme name
- `internal/ui/common/highlight.go` — Chroma theme name
- `internal/ui/notification/icon_other.go` — embedded icon filename (requires file rename)
- `internal/config/load.go` — no `CRUSH_*` fallback shim
- `internal/config/load.go` — no Smithers-specific default model when `c.Smithers != nil`

## Gaps Summary

1. **Cobra `Use: "crush"`** — most visible gap; the binary will still self-describe as `crush` in help output
2. **Subcommand help strings** — ~20 `crush` string literals across 8 cmd files
3. **Window title** — `"crush "` string in `ui.go` visible in terminal title bar
4. **Chroma theme** — internal registry key `"crush"` in diff and highlight renderers
5. **Notification icon** — embedded file name `crush-icon-solo.png`
6. **No `CRUSH_*` env var fallback** — transition safety gap for users migrating from Crush
7. **No Smithers default model** — missing `claude-opus-4-6` default when `c.Smithers != nil`
8. **Missing config tests** — lookup, env fallback, skills dirs, Smithers model default not yet tested
9. **Dev config `crush.json`** — repo-root LSP config still uses old `$schema` URL and filename

## Recommended Direction

The heavy lifting is done. The remaining work is cosmetic + safety:

1. Do Cobra + subcommand string sweep in `internal/cmd/` — global replace `crush` → `smithers-tui` in example and error strings, change `Use: "crush"` to `Use: "smithers-tui"`.
2. Fix window title in `ui.go` and update Chroma theme name in `diff.go` and `highlight.go`.
3. Rename `crush-icon-solo.png` and update the `//go:embed` directive.
4. Add `envWithFallback` helper in `load.go`; wire it into all five `SMITHERS_TUI_*` reads with a `slog.Warn` for legacy usage.
5. Add Smithers default model logic in `setDefaults` or `configureSelectedModels`.
6. Add the six missing config tests.
7. Optionally rename the repo-root `crush.json` dev config to `smithers-tui.json` and update its `$schema`.

## Files To Touch

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
- [internal/ui/notification/crush-icon-solo.png](/Users/williamcory/crush/internal/ui/notification/crush-icon-solo.png) (file rename)
- [crush.json](/Users/williamcory/crush/crush.json) (optional dev-config rename)

```json
{
  "document": "First research pass completed."
}
```
