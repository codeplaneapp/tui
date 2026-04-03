# VHS Scenarios

Run the Smithers domain-system-prompt tape with:

```bash
vhs tests/vhs/smithers-domain-system-prompt.tape
```

This recording is a happy-path smoke flow for booting the TUI with Smithers
config and sending one chat prompt.

Run the Smithers helpbar-shortcuts tape with:

```bash
vhs tests/vhs/helpbar-shortcuts.tape
```

This recording is a happy-path smoke flow for the new `ctrl+r` and `ctrl+a`
Smithers shortcut behavior in the TUI help bar.
