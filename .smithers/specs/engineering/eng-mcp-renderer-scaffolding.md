# Research: eng-mcp-renderer-scaffolding

## Ticket Summary

The ticket asks us to create base scaffolding for Smithers-specific MCP tool renderers in the TUI chat interface. Key acceptance criteria:
1. A standard pattern for parsing Smithers tool result JSON
2. Common UI styles (tables, cards, success/error indicators) for Smithers tools in `internal/ui/styles`
3. A registry entry or switch case mapping `smithers_*` tool calls to their respective renderers

## Current Architecture

### Tool Message Item Pattern (`internal/ui/chat/tools.go`)

The existing tool rendering system uses a `ToolMessageItem` struct with a well-established pattern:

```go
type ToolMessageItem struct {
    toolCall messages.ToolCall
    result   messages.ToolResult
    canceled bool
    style    *styles.Styles
}
```

Key methods on `ToolMessageItem`:
- `Title() string` - Returns display title for the tool
- `FormattedInput() string` - Formats the tool's input parameters for display
- `FormattedOutput() string` - Formats the tool's output/result for display
- `Icon() string` - Returns an icon character for the tool type
- `Color() lipgloss.Color` - Returns the color for the tool type

### Tool Registration Pattern

New tool items are created via a factory function `NewToolMessageItem()` which uses a switch statement on `toolCall.Name`:

```go
func NewToolMessageItem(sty *styles.Styles, toolCall messages.ToolCall, result messages.ToolResult, canceled bool) ToolMessageItem {
    var item ToolMessageItem
    switch toolCall.Name {
    case tools.ReadToolName:
        item = NewReadToolMessageItem(sty, toolCall, result, canceled)
    case tools.WriteToolName:
        item = NewWriteToolMessageItem(sty, toolCall, result, canceled)
    // ... more cases
    default:
        item = NewDefaultToolMessageItem(sty, toolCall, result, canceled)
    }
    return item
}
```

Each specific tool renderer (e.g., `NewReadToolMessageItem`, `NewBashToolMessageItem`) creates a `ToolMessageItem` with custom implementations for formatting input/output.

### Default Tool Renderer

The `NewDefaultToolMessageItem` handles unknown tools by:
1. Parsing input as `map[string]any` and displaying key-value pairs
2. Attempting to parse output as JSON, falling back to plain text
3. Using a generic wrench icon and gray color

This is what Smithers tools currently fall through to.

### Existing Styles (`internal/ui/styles/styles.go`)

The styles package uses lipgloss for terminal styling. The `Styles` struct contains pre-configured styles for various UI elements. Colors are defined in a theme system with light/dark variants.

### Smithers Tools Context

From the MCP tool discovery ticket and Smithers documentation, the Smithers MCP tools follow the naming pattern `smithers_*` and include:
- `smithers_list_workflows` / `smithers_get_workflow`
- `smithers_start_run` / `smithers_get_run` / `smithers_list_runs` / `smithers_cancel_run`
- `smithers_list_approvals` / `smithers_approve` / `smithers_deny`
- `smithers_get_run_events`
- Various others for memory, prompts, scores, SQL, tickets, time-travel, triggers, etc.

These tools return structured JSON responses from the Smithers HTTP API.

## Implementation Plan

### 1. Smithers Tool Result Parser (`internal/ui/chat/smithers_tools.go`)

Create a standard pattern for parsing Smithers tool results:

```go
// SmithersToolResult represents the common envelope for Smithers API responses
type SmithersToolResult struct {
    Data    json.RawMessage `json:"data"`
    Error   string          `json:"error,omitempty"`
    Status  string          `json:"status,omitempty"`
}

func parseSmithersResult(result messages.ToolResult) (*SmithersToolResult, error) { ... }
```

### 2. Common Smithers UI Styles (`internal/ui/styles/smithers.go`)

Add Smithers-specific styles to the styles package:
- Table styles for list results (workflows, runs, approvals)
- Card styles for detail views
- Status indicator styles (running/completed/failed/pending)
- Success/error banner styles

### 3. Registry Integration

Add cases to the `NewToolMessageItem` switch for Smithers tools. Two approaches:

**Option A: Prefix matching** (recommended) - Check if `toolCall.Name` starts with `smithers_` and route to a Smithers-specific dispatcher:

```go
default:
    if strings.HasPrefix(toolCall.Name, "smithers_") {
        item = NewSmithersToolMessageItem(sty, toolCall, result, canceled)
    } else {
        item = NewDefaultToolMessageItem(sty, toolCall, result, canceled)
    }
```

**Option B: Individual cases** - Add each `smithers_*` tool as its own case. This is more explicit but more verbose.

Option A is better because it:
- Automatically handles new Smithers tools without code changes
- Keeps the switch statement manageable
- Allows the Smithers dispatcher to handle sub-routing internally

### 4. Base Smithers Tool Renderer

Create `NewSmithersToolMessageItem` that:
1. Uses the Smithers result parser to extract data
2. Detects the specific tool type from the name
3. Formats output using appropriate Smithers styles (tables for lists, cards for details)
4. Shows error states with Smithers-branded error styling
5. Falls back to formatted JSON for unrecognized Smithers tools

### Files to Create/Modify

1. **Create `internal/ui/chat/smithers_tools.go`** - Smithers tool result parser, base renderer, and sub-routing
2. **Create `internal/ui/styles/smithers.go`** - Smithers-specific UI styles
3. **Modify `internal/ui/chat/tools.go`** - Add `smithers_` prefix check in the switch default case

### Dependencies

- **feat-mcp-tool-discovery** - Need to know the exact tool names and response shapes. However, the scaffolding can be built with the prefix-matching approach and generic JSON rendering, then specific renderers added later.
- **internal/ui/styles/styles.go** - Existing styles infrastructure to extend
- **internal/ui/chat/tools.go** - Existing tool rendering to integrate with

### Risk Assessment

- **Low risk**: The prefix-matching approach in the default case is non-breaking
- **Medium risk**: Smithers API response format may vary across tools - the parser needs to handle both envelope and raw responses
- **Low risk**: Style additions are additive and don't affect existing styles