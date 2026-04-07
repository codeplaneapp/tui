import os
import re

files = [
    "agents.go", "approvals.go", "changes.go", "chat.go", "dashboard.go",
    "issues.go", "jjhub_workflows.go", "landings.go", "livechat.go", "memory.go",
    "prompts.go", "runinspect.go", "runs.go", "scores.go", "search.go", "sql.go",
    "ticketdetail.go", "tickets.go", "timeline.go", "triggers.go", "workflows.go", "workspaces.go",
    "helpers.go", "livechat_context_pane.go"
]

color_map = {
    '"1"': "Red",
    '"2"': "Green",
    '"3"': "Yellow",
    '"4"': "Blue",
    '"6"': "Primary",
    '"8"': "Gray",
    '"9"': "RedDark",
    '"10"': "GreenLight",
    '"12"': "BlueLight",
    '"42"': "Green",
    '"99"': "Purple",
    '"111"': "BlueLight",
    '"203"': "Red",
    '"214"': "Yellow",
    '"240"': "Border",
    '"245"': "FgMuted",
    '"252"': "FgBase",
}

def replace_color(match):
    color = match.group(1)
    field = color_map.get(color, "FgBase")
    return f"com.Styles.{field}"

def process_file(filepath):
    if not os.path.exists(filepath):
        print(f"Not found: {filepath}")
        return

    with open(filepath, "r") as f:
        content = f.read()

    original = content

    # Add import common if not exists
    if '"github.com/charmbracelet/crush/internal/ui/common"' not in content:
        content = re.sub(r'import \(\n', 'import (\n\t"github.com/charmbracelet/crush/internal/ui/common"\n', content, count=1)

    # Add com *common.Common to view structs
    def struct_repl(m):
        return f'type {m.group(1)} struct {{\n\tcom *common.Common'
    content = re.sub(r'type (\w+View) struct \{', struct_repl, content)

    # Update constructor signatures
    def constructor_repl(m):
        full_match = m.group(0)
        if 'com *common.Common' in full_match:
            return full_match
        name = m.group(1)
        suffix = m.group(2)
        args = m.group(3)
        if args.strip() == "":
            new_args = "com *common.Common"
        else:
            new_args = f"com *common.Common, {args}"
        return f'func New{name}{suffix}({new_args})'
    
    content = re.sub(r'func New(\w+)(View|ViewWith\w+)\((.*?)\)', constructor_repl, content)

    # Update struct initialization in constructors
    def return_repl(m):
        return m.group(0) + '\n\t\tcom: com,'
    content = re.sub(r'return &(\w+View)\{', return_repl, content)

    # Replace lipgloss.Color(...)
    # Note: Depending on where this occurs, `com` needs to be accessible. 
    # For standalone functions, we'll need to pass `com` or change the function signature manually.
    content = re.sub(r'lipgloss\.Color\((.*?)\)', replace_color, content)
    
    # We also need to map `v.com.Styles` instead of `com.Styles` if we are inside a method.
    # However, sometimes we are not in a method, so we use `v.com` or `com`.
    # Let's just use `com.Styles` and then replace `com.Styles` with `v.com.Styles` for receiver methods.
    
    if content != original:
        with open(filepath, "w") as f:
            f.write(content)

for f in files:
    process_file(f"internal/ui/views/{f}")

