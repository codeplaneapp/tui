---
name: codeplane-config
description: Configure Codeplane settings including providers, LSPs, MCPs, skills, permissions, observability, and behavior options. Use when the user needs help with codeplane.json configuration, setting up providers, configuring LSPs, adding MCP servers, or changing Codeplane behavior.
---

# Codeplane Configuration

Codeplane uses JSON configuration files with the following priority
(highest to lowest):

1. `.codeplane.json` (project-local, hidden)
2. `codeplane.json` (project-local)
3. `$XDG_CONFIG_HOME/codeplane/codeplane.json` or
   `$HOME/.config/codeplane/codeplane.json` (global)

Legacy `smithers-tui.json`, `crush.json`, `SMITHERS_TUI_*`, and `CRUSH_*`
sources are still read as fallbacks, but Codeplane now writes to Codeplane
paths and names only.

## Basic Structure

```json
{
  "$schema": "https://charm.land/codeplane.json",
  "options": {}
}
```

The `$schema` property enables IDE autocomplete but is optional.

## Common Configurations

### Project-Local Skills

Add a relative path to keep project-specific skills alongside your code:

```json
{
  "options": {
    "skills_paths": ["./skills"]
  }
}
```

> [!IMPORTANT]
> Keep in mind that the following paths are loaded by default, so they do not
> need to be added to `skills_paths`:
>
> * `.agents/skills`
> * `.codeplane/skills`
> * `.claude/skills`
> * `.cursor/skills`

### LSP Configuration

```json
{
  "lsp": {
    "go": {
      "command": "gopls",
      "env": {
        "GOTOOLCHAIN": "go1.24.5"
      }
    },
    "typescript": {
      "command": "typescript-language-server",
      "args": ["--stdio"]
    }
  }
}
```

### MCP Servers

```json
{
  "mcp": {
    "filesystem": {
      "type": "stdio",
      "command": "node",
      "args": ["/path/to/mcp-server.js"]
    },
    "github": {
      "type": "http",
      "url": "https://api.githubcopilot.com/mcp/",
      "headers": {
        "Authorization": "Bearer $GH_PAT"
      }
    }
  }
}
```

### Custom Provider

```json
{
  "providers": {
    "deepseek": {
      "type": "openai-compat",
      "base_url": "https://api.deepseek.com/v1",
      "api_key": "$DEEPSEEK_API_KEY",
      "models": [
        {
          "id": "deepseek-chat",
          "name": "Deepseek V3",
          "context_window": 64000
        }
      ]
    }
  }
}
```

### Tool Permissions

```json
{
  "permissions": {
    "allowed_tools": ["view", "ls", "grep", "edit"]
  }
}
```

### Disable Built-in Tools

```json
{
  "options": {
    "disabled_tools": ["bash", "sourcegraph"]
  }
}
```

### Disable Skills

```json
{
  "options": {
    "disabled_skills": ["codeplane-config"]
  }
}
```

`disabled_skills` disables skills by name, including both builtin skills and
skills discovered from disk paths. Legacy `crush-config` still maps to
`codeplane-config`.

### Debug Options

```json
{
  "options": {
    "debug": true,
    "debug_lsp": true
  }
}
```

### Attribution Settings

```json
{
  "options": {
    "attribution": {
      "trailer_style": "assisted-by",
      "generated_with": true
    }
  }
}
```

## Environment Variables

- `CODEPLANE_GLOBAL_CONFIG` - Override global config location.
- `CODEPLANE_GLOBAL_DATA` - Override data directory location.
- `CODEPLANE_SKILLS_DIR` - Override default skills directory.

Legacy `SMITHERS_TUI_*` and `CRUSH_*` environment variables are still read as
fallbacks during migration.

## Provider Types

- `openai` - For OpenAI or OpenAI-compatible APIs that route through OpenAI.
- `openai-compat` - For non-OpenAI providers with OpenAI-compatible APIs.
- `anthropic` - For Anthropic-compatible APIs.
