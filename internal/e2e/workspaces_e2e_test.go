package e2e_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func writeFakeJJHub(t *testing.T) (string, map[string]string) {
	t.Helper()

	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	scriptPath := filepath.Join(dir, "jjhub")

	const state = `{
  "repo": {
    "id": 1,
    "name": "repo",
    "full_name": "acme/repo",
    "owner": "acme",
    "description": "Fake repo",
    "default_bookmark": "main",
    "is_public": true,
    "is_archived": false,
    "num_issues": 0,
    "num_stars": 0,
    "created_at": "2025-04-01T12:00:00Z",
    "updated_at": "2025-04-01T12:00:00Z"
  },
  "workspaces": [
    {
      "id": "ws-1",
      "repository_id": 1,
      "user_id": 1,
      "name": "alpha",
      "status": "running",
      "is_fork": false,
      "parent_workspace_id": null,
      "freestyle_vm_id": "vm-1",
      "persistence": "sticky",
      "ssh_host": "alpha.example.com",
      "snapshot_id": null,
      "idle_timeout_seconds": 1800,
      "suspended_at": null,
      "created_at": "2025-04-01T12:00:00Z",
      "updated_at": "2025-04-01T12:00:00Z"
    }
  ],
  "snapshots": [
    {
      "id": "snap-1",
      "repository_id": 1,
      "user_id": 1,
      "name": "baseline",
      "workspace_id": "ws-1",
      "snapshot_id": "snap-1",
      "created_at": "2025-04-01T12:00:00Z",
      "updated_at": "2025-04-01T12:00:00Z"
    }
  ]
}`

	const script = `#!/usr/bin/env python3
import json
import os
import sys
from datetime import datetime, timezone

state_path = os.environ["CRUSH_FAKE_JJHUB_STATE"]

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

def clean_args(argv):
    result = []
    i = 0
    while i < len(argv):
        arg = argv[i]
        if arg == "--no-color":
            i += 1
            continue
        if arg in ("-R", "--repo"):
            i += 2
            continue
        if arg.startswith("--json") or arg.startswith("--toon"):
            i += 1
            continue
        result.append(arg)
        i += 1
    return result

def find_workspace(state, workspace_id):
    for workspace in state["workspaces"]:
        if workspace["id"] == workspace_id:
            return workspace
    raise SystemExit(f"workspace {workspace_id} not found")

def find_snapshot(state, snapshot_id):
    for snapshot in state["snapshots"]:
        if snapshot["id"] == snapshot_id:
            return snapshot
    raise SystemExit(f"snapshot {snapshot_id} not found")

def next_id(items, prefix):
    max_num = 0
    for item in items:
        raw = item["id"]
        if raw.startswith(prefix + "-"):
            try:
                max_num = max(max_num, int(raw.split("-", 1)[1]))
            except ValueError:
                pass
    return f"{prefix}-{max_num + 1}"

args = clean_args(sys.argv[1:])
state = load_state()

if args[:2] == ["repo", "view"]:
    print_json(state["repo"])
    raise SystemExit(0)

if args[:2] == ["land", "list"]:
    print_json([])
    raise SystemExit(0)

if args[:2] == ["issue", "list"]:
    print_json([])
    raise SystemExit(0)

if args[:2] == ["workspace", "list"]:
    print_json(state["workspaces"])
    raise SystemExit(0)

if args[:2] == ["workspace", "view"] and len(args) >= 3:
    print_json(find_workspace(state, args[2]))
    raise SystemExit(0)

if args[:2] == ["workspace", "create"]:
    name = ""
    snapshot_id = ""
    i = 2
    while i < len(args):
        if args[i] == "-n" and i + 1 < len(args):
            name = args[i + 1]
            i += 2
            continue
        if args[i] == "--snapshot" and i + 1 < len(args):
            snapshot_id = args[i + 1]
            i += 2
            continue
        i += 1
    workspace_id = next_id(state["workspaces"], "ws")
    workspace = {
        "id": workspace_id,
        "repository_id": 1,
        "user_id": 1,
        "name": name,
        "status": "running",
        "is_fork": False,
        "parent_workspace_id": None,
        "freestyle_vm_id": f"vm-{workspace_id}",
        "persistence": "sticky",
        "ssh_host": f"{workspace_id}.example.com",
        "snapshot_id": snapshot_id or None,
        "idle_timeout_seconds": 1800,
        "suspended_at": None,
        "created_at": now(),
        "updated_at": now(),
    }
    state["workspaces"].insert(0, workspace)
    save_state(state)
    print_json(workspace)
    raise SystemExit(0)

if args[:2] == ["workspace", "delete"] and len(args) >= 3:
    workspace_id = args[2]
    state["workspaces"] = [workspace for workspace in state["workspaces"] if workspace["id"] != workspace_id]
    save_state(state)
    raise SystemExit(0)

if args[:2] == ["workspace", "suspend"] and len(args) >= 3:
    workspace = find_workspace(state, args[2])
    workspace["status"] = "suspended"
    workspace["ssh_host"] = None
    workspace["suspended_at"] = now()
    workspace["updated_at"] = now()
    save_state(state)
    print_json(workspace)
    raise SystemExit(0)

if args[:2] == ["workspace", "resume"] and len(args) >= 3:
    workspace = find_workspace(state, args[2])
    workspace["status"] = "running"
    workspace["ssh_host"] = f"{workspace['id']}.example.com"
    workspace["suspended_at"] = None
    workspace["updated_at"] = now()
    save_state(state)
    print_json(workspace)
    raise SystemExit(0)

if args[:2] == ["workspace", "fork"] and len(args) >= 3:
    source = find_workspace(state, args[2])
    name = ""
    i = 3
    while i < len(args):
        if args[i] == "-n" and i + 1 < len(args):
            name = args[i + 1]
            i += 2
            continue
        i += 1
    workspace_id = next_id(state["workspaces"], "ws")
    fork = dict(source)
    fork["id"] = workspace_id
    fork["name"] = name
    fork["is_fork"] = True
    fork["parent_workspace_id"] = source["id"]
    fork["freestyle_vm_id"] = f"vm-{workspace_id}"
    fork["ssh_host"] = f"{workspace_id}.example.com"
    fork["created_at"] = now()
    fork["updated_at"] = now()
    state["workspaces"].insert(0, fork)
    save_state(state)
    print_json(fork)
    raise SystemExit(0)

if args[:3] == ["workspace", "snapshot", "list"]:
    print_json(state["snapshots"])
    raise SystemExit(0)

if args[:3] == ["workspace", "snapshot", "view"] and len(args) >= 4:
    print_json(find_snapshot(state, args[3]))
    raise SystemExit(0)

if args[:3] == ["workspace", "snapshot", "create"] and len(args) >= 4:
    workspace_id = args[3]
    name = ""
    i = 4
    while i < len(args):
        if args[i] == "-n" and i + 1 < len(args):
            name = args[i + 1]
            i += 2
            continue
        i += 1
    snapshot_id = next_id(state["snapshots"], "snap")
    snapshot = {
        "id": snapshot_id,
        "repository_id": 1,
        "user_id": 1,
        "name": name,
        "workspace_id": workspace_id,
        "snapshot_id": snapshot_id,
        "created_at": now(),
        "updated_at": now(),
    }
    state["snapshots"].insert(0, snapshot)
    save_state(state)
    print_json(snapshot)
    raise SystemExit(0)

if args[:3] == ["workspace", "snapshot", "delete"] and len(args) >= 4:
    snapshot_id = args[3]
    state["snapshots"] = [snapshot for snapshot in state["snapshots"] if snapshot["id"] != snapshot_id]
    save_state(state)
    raise SystemExit(0)

if len(args) >= 3 and args[0] == "workspace" and args[1] == "ssh":
    workspace_id = args[-1]
    sys.stdout.write(f"Connected to {workspace_id}\n")
    raise SystemExit(0)

sys.stderr.write("unsupported fake jjhub invocation: " + " ".join(args) + "\n")
raise SystemExit(1)
`

	require.NoError(t, os.WriteFile(statePath, []byte(state), 0o644))
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))

	return dir, map[string]string{
		"CRUSH_FAKE_JJHUB_STATE": statePath,
	}
}

func TestWorkspacesLifecycleE2E(t *testing.T) {
	skipUnlessTmuxE2E(t)

	fakeDir, env := writeFakeJJHub(t)
	tui := launchTUIWithOptions(t, tuiLaunchOptions{
		env:          env,
		pathPrefixes: []string{fakeDir},
	})
	t.Cleanup(tui.Terminate)

	require.NoError(t, tui.WaitForAnyText([]string{"Overview", "Start Chat"}, 20*time.Second),
		"dashboard must render; buffer:\n%s", tui.Snapshot())

	openCommandsPalette(t, tui)
	time.Sleep(300 * time.Millisecond)
	tui.SendText("workspaces")
	time.Sleep(300 * time.Millisecond)
	tui.SendKeys("\n")
	require.NoError(t, tui.WaitForText("JJHUB › Workspaces", 10*time.Second),
		"workspaces view must open; buffer:\n%s", tui.Snapshot())
	require.NoError(t, tui.WaitForText("alpha", 10*time.Second),
		"seed workspace must appear; buffer:\n%s", tui.Snapshot())

	tui.SendKeys("c")
	require.NoError(t, tui.WaitForText("Create Workspace", 5*time.Second))
	tui.SendKeys("dev-sandbox\n")
	require.NoError(t, tui.WaitForText("Created dev-sandbox", 10*time.Second),
		"create action must succeed; buffer:\n%s", tui.Snapshot())
	require.NoError(t, tui.WaitForText("dev-sandbox", 10*time.Second))

	tui.SendKeys("s")
	require.NoError(t, tui.WaitForText("Suspended dev-sandbox", 10*time.Second),
		"suspend action must succeed; buffer:\n%s", tui.Snapshot())

	tui.SendKeys("s")
	require.NoError(t, tui.WaitForText("Resumed dev-sandbox", 10*time.Second),
		"resume action must succeed; buffer:\n%s", tui.Snapshot())

	tui.SendKeys("\n")
	require.NoError(t, tui.WaitForText("Disconnected from ws-2", 10*time.Second),
		"ssh handoff must return to TUI; buffer:\n%s", tui.Snapshot())

	tui.SendKeys("f")
	require.NoError(t, tui.WaitForText("Fork Workspace", 5*time.Second))
	tui.SendKeys("forked-copy\n")
	require.NoError(t, tui.WaitForText("Forked forked-copy", 10*time.Second),
		"fork action must succeed; buffer:\n%s", tui.Snapshot())

	tui.SendKeys("n")
	require.NoError(t, tui.WaitForText("Create Snapshot", 5*time.Second))
	tui.SendKeys("baseline-two\n")
	require.NoError(t, tui.WaitForText("Created snapshot baseline-two", 10*time.Second),
		"snapshot creation must succeed; buffer:\n%s", tui.Snapshot())

	tui.SendKeys("\t")
	require.NoError(t, tui.WaitForText("baseline-two", 10*time.Second),
		"snapshots tab must show created snapshot; buffer:\n%s", tui.Snapshot())

	tui.SendKeys("c")
	require.NoError(t, tui.WaitForText("Create Workspace From Snapshot", 5*time.Second))
	tui.SendKeys("restored-workspace\n")
	require.NoError(t, tui.WaitForText("Created workspace from restored-workspace", 10*time.Second),
		"create from snapshot must succeed; buffer:\n%s", tui.Snapshot())

	tui.SendKeys("d")
	require.NoError(t, tui.WaitForText("Delete Snapshot", 5*time.Second))
	tui.SendKeys("\n")
	require.NoError(t, tui.WaitForText("Deleted snapshot baseline-two", 10*time.Second),
		"snapshot delete must succeed; buffer:\n%s", tui.Snapshot())

	tui.SendKeys("\t")
	require.NoError(t, tui.WaitForText("restored-workspace", 10*time.Second),
		"restored workspace must appear back in workspace mode; buffer:\n%s", tui.Snapshot())
}
