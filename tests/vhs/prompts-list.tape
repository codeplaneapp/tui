# Prompts list view happy-path smoke recording.
Output tests/vhs/output/prompts-list.gif
Set Shell zsh
Set FontSize 14
Set Width 1200
Set Height 800

# Ensure fixture prompts exist in the local .smithers/prompts/ directory.
Type "mkdir -p .smithers/prompts"
Enter
Sleep 500ms
Type "printf '# Code Review\\n\\nReview {props.lang} code for {props.focus}.\\n' > .smithers/prompts/code-review.mdx"
Enter
Sleep 500ms
Type "printf '# Deploy\\n\\nDeploy {props.service} to {props.env}.\\n' > .smithers/prompts/deploy.mdx"
Enter
Sleep 500ms

# Launch the TUI.
Type "CRUSH_GLOBAL_CONFIG=tests/vhs/fixtures CRUSH_GLOBAL_DATA=/tmp/crush-vhs go run ."
Enter
Sleep 3s

# Open the command palette and navigate to Prompt Templates.
Type "/"
Sleep 500ms
Type "Prompt"
Sleep 300ms
Enter
Sleep 2s

# Navigate the list.
Type "j"
Sleep 500ms
Type "k"
Sleep 500ms

Screenshot tests/vhs/output/prompts-list.png

# Return to chat.
Escape
Sleep 1s

Ctrl+c
Sleep 1s
