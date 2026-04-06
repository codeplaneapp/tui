// Package jjhub shells out to the jjhub CLI and parses JSON output.
// This is the POC adapter — will be replaced by direct Go API calls later.
package jjhub

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ---- Data types (mirrors jjhub --json output) ----

type User struct {
	ID    int    `json:"id"`
	Login string `json:"login"`
}

type Repo struct {
	ID              int       `json:"id"`
	Name            string    `json:"name"`
	FullName        string    `json:"full_name"`
	Owner           string    `json:"owner"`
	Description     string    `json:"description"`
	DefaultBookmark string    `json:"default_bookmark"`
	IsPublic        bool      `json:"is_public"`
	IsArchived      bool      `json:"is_archived"`
	NumIssues       int       `json:"num_issues"`
	NumStars        int       `json:"num_stars"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Landing struct {
	Number         int      `json:"number"`
	Title          string   `json:"title"`
	Body           string   `json:"body"`
	State          string   `json:"state"` // open, closed, merged, draft
	TargetBookmark string   `json:"target_bookmark"`
	ChangeIDs      []string `json:"change_ids"`
	StackSize      int      `json:"stack_size"`
	ConflictStatus string   `json:"conflict_status"`
	Author         User     `json:"author"`
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at"`
}

// LandingDetail is the rich response from `jjhub land view`.
type LandingDetail struct {
	Landing   Landing         `json:"landing"`
	Changes   []LandingChange `json:"changes"`
	Conflicts LandingConflict `json:"conflicts"`
	Reviews   []Review        `json:"reviews"`
}

type LandingChange struct {
	ID                int    `json:"id"`
	ChangeID          string `json:"change_id"`
	LandingRequestID  int    `json:"landing_request_id"`
	PositionInStack   int    `json:"position_in_stack"`
	CreatedAt         string `json:"created_at"`
}

type LandingConflict struct {
	ConflictStatus     string            `json:"conflict_status"`
	HasConflicts       bool              `json:"has_conflicts"`
	ConflictsByChange  map[string]string `json:"conflicts_by_change"`
}

type Review struct {
	ID               int    `json:"id"`
	LandingRequestID int    `json:"landing_request_id"`
	ReviewerID       int    `json:"reviewer_id"`
	State            string `json:"state"` // approve, request_changes, comment
	Type             string `json:"type"`
	Body             string `json:"body"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

type Issue struct {
	ID           int     `json:"id"`
	Number       int     `json:"number"`
	Title        string  `json:"title"`
	Body         string  `json:"body"`
	State        string  `json:"state"` // open, closed
	Author       User    `json:"author"`
	Assignees    []User  `json:"assignees"`
	CommentCount int     `json:"comment_count"`
	MilestoneID  *int    `json:"milestone_id"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
	Labels       []Label `json:"labels"`
}

type Label struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

type Notification struct {
	ID        int    `json:"id"`
	Title     string `json:"title"`
	Type      string `json:"type"`
	RepoName  string `json:"repo_name"`
	Unread    bool   `json:"unread"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type Workspace struct {
	ID                 string  `json:"id"`
	RepositoryID       int     `json:"repository_id"`
	UserID             int     `json:"user_id"`
	Name               string  `json:"name"`
	Status             string  `json:"status"` // pending, running, stopped, failed
	IsFork             bool    `json:"is_fork"`
	ParentWorkspaceID  *string `json:"parent_workspace_id"`
	FreestyleVMID      string  `json:"freestyle_vm_id"`
	Persistence        string  `json:"persistence"`
	SSHHost            *string `json:"ssh_host"`
	SnapshotID         *string `json:"snapshot_id"`
	IdleTimeoutSeconds int     `json:"idle_timeout_seconds"`
	SuspendedAt        *string `json:"suspended_at"`
	CreatedAt          string  `json:"created_at"`
	UpdatedAt          string  `json:"updated_at"`
}

type Workflow struct {
	ID           int    `json:"id"`
	RepositoryID int    `json:"repository_id"`
	Name         string `json:"name"`
	Path         string `json:"path"`
	IsActive     bool   `json:"is_active"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

type Change struct {
	ChangeID      string   `json:"change_id"`
	CommitID      string   `json:"commit_id"`
	Description   string   `json:"description"`
	Author        Author   `json:"author"`
	Timestamp     string   `json:"timestamp"`
	IsEmpty       bool     `json:"is_empty"`
	IsWorkingCopy bool     `json:"is_working_copy"`
	Bookmarks     []string `json:"bookmarks"`
}

type Author struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// ---- Client ----

type Client struct {
	repo string // owner/repo, empty = auto-detect from cwd
}

func NewClient(repo string) *Client {
	return &Client{repo: repo}
}

func (c *Client) run(args ...string) ([]byte, error) {
	allArgs := append(args, "--json", "--no-color")
	cmd := exec.Command("jjhub", allArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Extract just the error message, not the full stderr dump.
		msg := strings.TrimSpace(string(out))
		if idx := strings.Index(msg, "Error:"); idx >= 0 {
			msg = strings.TrimSpace(msg[idx+6:])
		}
		return nil, fmt.Errorf("%s", msg)
	}
	return out, nil
}

func (c *Client) runRaw(args ...string) (string, error) {
	allArgs := append(args, "--no-color")
	cmd := exec.Command("jjhub", allArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if idx := strings.Index(msg, "Error:"); idx >= 0 {
			msg = strings.TrimSpace(msg[idx+6:])
		}
		return "", fmt.Errorf("%s", msg)
	}
	return string(out), nil
}

func (c *Client) repoArgs() []string {
	if c.repo != "" {
		return []string{"-R", c.repo}
	}
	return nil
}

// ---- List methods ----

func (c *Client) ListLandings(state string, limit int) ([]Landing, error) {
	args := []string{"land", "list", "-s", state, "-L", fmt.Sprint(limit)}
	args = append(args, c.repoArgs()...)
	out, err := c.run(args...)
	if err != nil {
		return nil, err
	}
	var landings []Landing
	if err := json.Unmarshal(out, &landings); err != nil {
		return nil, fmt.Errorf("parse landings: %w", err)
	}
	return landings, nil
}

func (c *Client) ListIssues(state string, limit int) ([]Issue, error) {
	args := []string{"issue", "list", "-s", state, "-L", fmt.Sprint(limit)}
	args = append(args, c.repoArgs()...)
	out, err := c.run(args...)
	if err != nil {
		return nil, err
	}
	var issues []Issue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("parse issues: %w", err)
	}
	return issues, nil
}

func (c *Client) ListRepos(limit int) ([]Repo, error) {
	args := []string{"repo", "list", "-L", fmt.Sprint(limit)}
	out, err := c.run(args...)
	if err != nil {
		return nil, err
	}
	var repos []Repo
	if err := json.Unmarshal(out, &repos); err != nil {
		return nil, fmt.Errorf("parse repos: %w", err)
	}
	return repos, nil
}

func (c *Client) ListNotifications(limit int) ([]Notification, error) {
	args := []string{"notification", "list", "-L", fmt.Sprint(limit)}
	out, err := c.run(args...)
	if err != nil {
		return nil, err
	}
	var notifications []Notification
	if err := json.Unmarshal(out, &notifications); err != nil {
		return nil, fmt.Errorf("parse notifications: %w", err)
	}
	return notifications, nil
}

func (c *Client) ListWorkspaces(limit int) ([]Workspace, error) {
	args := []string{"workspace", "list", "-L", fmt.Sprint(limit)}
	args = append(args, c.repoArgs()...)
	out, err := c.run(args...)
	if err != nil {
		return nil, err
	}
	var ws []Workspace
	if err := json.Unmarshal(out, &ws); err != nil {
		return nil, fmt.Errorf("parse workspaces: %w", err)
	}
	return ws, nil
}

func (c *Client) ListWorkflows(limit int) ([]Workflow, error) {
	args := []string{"workflow", "list", "-L", fmt.Sprint(limit)}
	args = append(args, c.repoArgs()...)
	out, err := c.run(args...)
	if err != nil {
		return nil, err
	}
	var wf []Workflow
	if err := json.Unmarshal(out, &wf); err != nil {
		return nil, fmt.Errorf("parse workflows: %w", err)
	}
	return wf, nil
}

func (c *Client) ListChanges(limit int) ([]Change, error) {
	args := []string{"change", "list", "--limit", fmt.Sprint(limit)}
	out, err := c.run(args...)
	if err != nil {
		return nil, err
	}
	var changes []Change
	if err := json.Unmarshal(out, &changes); err != nil {
		return nil, fmt.Errorf("parse changes: %w", err)
	}
	return changes, nil
}

// ---- Detail methods ----

func (c *Client) ViewLanding(number int) (*LandingDetail, error) {
	args := []string{"land", "view", fmt.Sprint(number)}
	args = append(args, c.repoArgs()...)
	out, err := c.run(args...)
	if err != nil {
		return nil, err
	}
	var d LandingDetail
	if err := json.Unmarshal(out, &d); err != nil {
		return nil, fmt.Errorf("parse landing detail: %w", err)
	}
	return &d, nil
}

func (c *Client) ViewIssue(number int) (*Issue, error) {
	args := []string{"issue", "view", fmt.Sprint(number)}
	args = append(args, c.repoArgs()...)
	out, err := c.run(args...)
	if err != nil {
		return nil, err
	}
	var i Issue
	if err := json.Unmarshal(out, &i); err != nil {
		return nil, fmt.Errorf("parse issue: %w", err)
	}
	return &i, nil
}

func (c *Client) GetCurrentRepo() (*Repo, error) {
	out, err := c.run("repo", "view")
	if err != nil {
		return nil, err
	}
	var r Repo
	if err := json.Unmarshal(out, &r); err != nil {
		return nil, fmt.Errorf("parse repo: %w", err)
	}
	return &r, nil
}

// ---- Diff methods ----

func (c *Client) LandingDiff(number int) (string, error) {
	args := []string{"land", "diff", fmt.Sprint(number)}
	args = append(args, c.repoArgs()...)
	return c.runRaw(args...)
}

func (c *Client) ChangeDiff(changeID string) (string, error) {
	args := []string{"change", "diff", changeID}
	return c.runRaw(args...)
}

func (c *Client) WorkingCopyDiff() (string, error) {
	cmd := exec.Command("jj", "diff", "--no-color")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("jj diff: %w", err)
	}
	return string(out), nil
}
