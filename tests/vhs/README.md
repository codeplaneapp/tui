# VHS Scenarios

Build the binary once before recording any tape:

```bash
go build .
```

The tapes run `./crush` directly to avoid `go run` compile latency in the
recordings.

Run the Smithers domain-system-prompt tape with:

```bash
vhs tests/vhs/smithers-domain-system-prompt.tape
```

This recording is a happy-path smoke flow for booting the TUI with Smithers
config and landing in the Smithers overview.

Run the Smithers chat-default-console tape with:

```bash
vhs tests/vhs/chat-default-console.tape
```

This recording is a happy-path smoke flow for the default Smithers overview,
opening pushed views, and returning to the root with `esc`.

Run the Smithers helpbar-shortcuts tape with:

```bash
vhs tests/vhs/helpbar-shortcuts.tape
```

This recording is a happy-path smoke flow for the new `ctrl+r` and `ctrl+a`
Smithers shortcut behavior in the TUI help bar, plus the `ctrl+g` help
overlay.

Run the Smithers branding-status tape with:

```bash
vhs tests/vhs/branding-status.tape
```

This recording is a happy-path smoke flow for Smithers header branding and
status-bar rendering.

Run the MCP tool discovery tape with:

```bash
vhs tests/vhs/mcp-tool-discovery.tape
```

This recording is a happy-path smoke flow for the Start Chat path into the
built-in Smithers chat. It captures the MCP discovery state shown in the chat
chrome.

Run the command-palette work-items tape with:

```bash
vhs tests/vhs/command-palette-work-items.tape
```

This recording is a happy-path smoke flow for the command palette filter and
navigation into the Work Items split-pane view.
