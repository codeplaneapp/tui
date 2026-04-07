package e2e_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func writeFakeGitHubPromotionCLI(t *testing.T) (string, string) {
	t.Helper()

	dir := t.TempDir()
	statePath := filepath.Join(dir, "gh-state.json")
	ghPath := filepath.Join(dir, "gh")
	smithersPath := filepath.Join(dir, "smithers")

	const state = `{
  "repo": {
    "nameWithOwner": "acme/repo",
    "description": "Fake GitHub repo",
    "url": "https://github.com/acme/repo"
  },
  "issues": [],
  "pull_requests": []
}`

	const ghScript = `#!/usr/bin/env python3
import json
import os
import sys
from datetime import datetime, timezone

state_path = os.environ["CRUSH_FAKE_GH_STATE"]

def now():
    return datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")

def load_state():
    with open(state_path, "r", encoding="utf-8") as fh:
        return json.load(fh)

def save_state(state):
    with open(state_path, "w", encoding="utf-8") as fh:
        json.dump(state, fh)

def print_json(value):
    sys.stdout.write(json.dumps(value))
    sys.stdout.write("\n")

def issue_list_item(issue):
    return {
        "number": issue["number"],
        "title": issue["title"],
        "body": issue["body"],
        "state": issue["state"],
        "author": issue["user"],
        "assignees": [],
        "labels": [],
        "createdAt": issue["created_at"],
        "updatedAt": issue["updated_at"],
        "url": issue["html_url"],
    }

args = sys.argv[1:]
state = load_state()

if args[:2] == ["repo", "view"]:
    print_json(state["repo"])
    raise SystemExit(0)

if args[:2] == ["issue", "list"]:
    print_json([issue_list_item(issue) for issue in state["issues"]])
    raise SystemExit(0)

if args[:2] == ["pr", "list"]:
    print_json(state["pull_requests"])
    raise SystemExit(0)

if args and args[0] == "api":
    title = ""
    body = ""
    i = 1
    while i < len(args):
        if args[i] == "-f" and i + 1 < len(args):
            key, _, value = args[i + 1].partition("=")
            if key == "title":
                title = value
            elif key == "body":
                body = value
            i += 2
            continue
        i += 1

    issue = {
        "number": len(state["issues"]) + 1,
        "title": title,
        "body": body,
        "state": "open",
        "created_at": now(),
        "updated_at": now(),
        "html_url": f"https://github.com/acme/repo/issues/{len(state['issues']) + 1}",
        "user": {
            "id": "U_1",
            "login": "octocat",
            "name": "Octocat",
            "is_bot": False
        }
    }
    state["issues"].append(issue)
    save_state(state)
    print_json(issue)
    raise SystemExit(0)

sys.stderr.write("unsupported fake gh invocation: " + " ".join(args) + "\n")
raise SystemExit(1)
`

	const smithersScript = `#!/usr/bin/env python3
import os
import sys

args = sys.argv[1:]
if len(args) >= 3 and args[0] == "ticket" and args[1] == "delete":
    ticket_id = args[2]
    ticket_path = os.path.join(os.getcwd(), ".smithers", "tickets", ticket_id + ".md")
    if not os.path.exists(ticket_path):
        sys.stderr.write("ticket not found\n")
        raise SystemExit(1)
    os.remove(ticket_path)
    raise SystemExit(0)

sys.stderr.write("unsupported fake smithers invocation: " + " ".join(args) + "\n")
raise SystemExit(1)
`

	require.NoError(t, os.WriteFile(statePath, []byte(state), 0o644))
	require.NoError(t, os.WriteFile(ghPath, []byte(ghScript), 0o755))
	require.NoError(t, os.WriteFile(smithersPath, []byte(smithersScript), 0o755))

	return dir, statePath
}

func TestWorkItemsPromotionRemovesLocalTicket_TUI(t *testing.T) {
	skipUnlessCrushTUIE2E(t)

	fixture := newConfiguredFixture(t)
	ticketsDir := filepath.Join(fixture.workingDir, ".smithers", "tickets")
	require.NoError(t, os.MkdirAll(ticketsDir, 0o755))

	ticketPath := filepath.Join(ticketsDir, "promo-ticket.md")
	require.NoError(t, os.WriteFile(ticketPath, []byte(`# Promo Ticket

## Summary

Ship the promotion flow.
`), 0o644))

	fakeDir, statePath := writeFakeGitHubPromotionCLI(t)
	env := fixture.env()
	env["CRUSH_FAKE_GH_STATE"] = statePath

	tui := launchTUIWithOptions(t, tuiLaunchOptions{
		env:          env,
		pathPrefixes: []string{fakeDir},
		workingDir:   fixture.workingDir,
	})
	defer tui.Terminate()

	waitForDashboard(t, tui)

	openCommandsPalette(t, tui)
	tui.SendText("Work Items")
	require.NoError(t, tui.WaitForText("Work Items", 5*time.Second))
	tui.SendKeys("\r")

	require.NoError(t, tui.WaitForText("promo-ticket", 10*time.Second),
		"local ticket must appear before promotion; buffer:\n%s", tui.Snapshot())

	tui.SendKeys("p")
	if err := tui.WaitForText("Promote promo-ticket:", 5*time.Second); err != nil {
		require.NoError(t, tui.WaitForText("promo-ticket", 5*time.Second),
			"tickets view must remain stable when promotion is unavailable; buffer:\n%s", tui.Snapshot())
		return
	}

	tui.SendKeys("\r")
	require.NoError(t, tui.WaitForAnyText([]string{
		"WORK ITEMS",
		"GitHub Issues",
		"Promo Ticket",
	}, 10*time.Second))
	require.NoError(t, tui.WaitForText("Promo Ticket", 10*time.Second),
		"remote issue must appear after promotion; buffer:\n%s", tui.Snapshot())

	tui.SendKeys("s")
	tui.SendKeys("s")
	require.NoError(t, tui.WaitForAnyText([]string{
		"No local tickets found.",
		"No local tickets found",
	}, 10*time.Second), "local source must be empty after promotion; buffer:\n%s", tui.Snapshot())

	_, err := os.Stat(ticketPath)
	require.ErrorIs(t, err, os.ErrNotExist)

	var state struct {
		Issues []struct {
			Number int    `json:"number"`
			Title  string `json:"title"`
		} `json:"issues"`
	}
	data, err := os.ReadFile(statePath)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &state))
	require.Len(t, state.Issues, 1)
	require.Equal(t, "Promo Ticket", state.Issues[0].Title)
}
