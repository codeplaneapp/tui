// Package jjhub shells out to the jjhub CLI and parses JSON output.
package jjhub

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

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
	State          string   `json:"state"`
	TargetBookmark string   `json:"target_bookmark"`
	ChangeIDs      []string `json:"change_ids"`
	StackSize      int      `json:"stack_size"`
	ConflictStatus string   `json:"conflict_status"`
	Author         User     `json:"author"`
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at"`
}

type LandingDetail struct {
	Landing   Landing         `json:"landing"`
	Changes   []LandingChange `json:"changes"`
	Conflicts LandingConflict `json:"conflicts"`
	Reviews   []Review        `json:"reviews"`
}

type LandingChange struct {
	ID               int    `json:"id"`
	ChangeID         string `json:"change_id"`
	LandingRequestID int    `json:"landing_request_id"`
	PositionInStack  int    `json:"position_in_stack"`
	CreatedAt        string `json:"created_at"`
}

type LandingConflict struct {
	ConflictStatus    string            `json:"conflict_status"`
	HasConflicts      bool              `json:"has_conflicts"`
	ConflictsByChange map[string]string `json:"conflicts_by_change"`
}

type Review struct {
	ID               int    `json:"id"`
	LandingRequestID int    `json:"landing_request_id"`
	ReviewerID       int    `json:"reviewer_id"`
	State            string `json:"state"`
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
	State        string  `json:"state"`
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
	Status             string  `json:"status"`
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

type WorkspaceSnapshot struct {
	ID           string  `json:"id"`
	RepositoryID int     `json:"repository_id"`
	UserID       int     `json:"user_id"`
	Name         string  `json:"name"`
	WorkspaceID  *string `json:"workspace_id"`
	SnapshotID   string  `json:"snapshot_id"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
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

type Client struct {
	repo string
}

var errEmptyCLIResponse = errors.New("jjhub: empty response from CLI")

func NewClient(repo string) *Client {
	return &Client{repo: repo}
}

func isEmptyCLIOutput(data []byte) bool {
	return len(bytes.TrimSpace(data)) == 0
}

func parseCLIError(out []byte, err error) error {
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(string(out))
	if idx := strings.Index(msg, "Error:"); idx >= 0 {
		msg = strings.TrimSpace(msg[idx+6:])
	}
	if msg == "" {
		return err
	}
	return fmt.Errorf("%s", msg)
}

func (c *Client) run(args ...string) ([]byte, error) {
	return c.runContext(context.Background(), args...)
}

func (c *Client) runContext(ctx context.Context, args ...string) ([]byte, error) {
	allArgs := append(args, "--json", "--no-color")
	cmd := exec.CommandContext(ctx, "jjhub", allArgs...)
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err := parseCLIError(out, err); err != nil {
		return nil, err
	}
	if isEmptyCLIOutput(out) {
		return nil, errEmptyCLIResponse
	}
	return out, nil
}

func (c *Client) runRaw(args ...string) (string, error) {
	return c.runRawContext(context.Background(), args...)
}

func (c *Client) runRawContext(ctx context.Context, args ...string) (string, error) {
	allArgs := append(args, "--no-color")
	cmd := exec.CommandContext(ctx, "jjhub", allArgs...)
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err := parseCLIError(out, err); err != nil {
		return "", err
	}
	return string(out), nil
}

func decodeJSON[T any](data []byte, label string) (*T, error) {
	if isEmptyCLIOutput(data) {
		return nil, errEmptyCLIResponse
	}
	var result T
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parse %s: %w", label, err)
	}
	return &result, nil
}

func (c *Client) repoArgs() []string {
	if c.repo != "" {
		return []string{"-R", c.repo}
	}
	return nil
}

func (c *Client) ListLandings(ctx context.Context, state string, limit int) ([]Landing, error) {
	args := []string{"land", "list", "-s", state, "-L", fmt.Sprint(limit)}
	args = append(args, c.repoArgs()...)
	out, err := c.runContext(ctx, args...)
	if err != nil {
		return nil, err
	}
	var landings []Landing
	if err := json.Unmarshal(out, &landings); err != nil {
		return nil, fmt.Errorf("parse landings: %w", err)
	}
	return landings, nil
}

func (c *Client) ListIssues(ctx context.Context, state string, limit int) ([]Issue, error) {
	args := []string{"issue", "list", "-s", state, "-L", fmt.Sprint(limit)}
	args = append(args, c.repoArgs()...)
	out, err := c.runContext(ctx, args...)
	if err != nil {
		return nil, err
	}
	var issues []Issue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("parse issues: %w", err)
	}
	return issues, nil
}

func (c *Client) CreateIssue(ctx context.Context, title, body string) (*Issue, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, fmt.Errorf("title must not be empty")
	}

	args := []string{"issue", "create", "-t", title}
	if strings.TrimSpace(body) != "" {
		args = append(args, "-b", body)
	}
	args = append(args, c.repoArgs()...)

	out, err := c.runContext(ctx, args...)
	if err != nil {
		return nil, err
	}

	var issue Issue
	if err := json.Unmarshal(out, &issue); err != nil {
		return nil, fmt.Errorf("parse created issue: %w", err)
	}
	return &issue, nil
}

func (c *Client) ListRepos(ctx context.Context, limit int) ([]Repo, error) {
	args := []string{"repo", "list", "-L", fmt.Sprint(limit)}
	out, err := c.runContext(ctx, args...)
	if err != nil {
		return nil, err
	}
	var repos []Repo
	if err := json.Unmarshal(out, &repos); err != nil {
		return nil, fmt.Errorf("parse repos: %w", err)
	}
	return repos, nil
}

func (c *Client) ListNotifications(ctx context.Context, limit int) ([]Notification, error) {
	args := []string{"notification", "list", "-L", fmt.Sprint(limit)}
	out, err := c.runContext(ctx, args...)
	if err != nil {
		return nil, err
	}
	var notifications []Notification
	if err := json.Unmarshal(out, &notifications); err != nil {
		return nil, fmt.Errorf("parse notifications: %w", err)
	}
	return notifications, nil
}

func (c *Client) ListWorkspaces(ctx context.Context, limit int) ([]Workspace, error) {
	args := []string{"workspace", "list", "-L", fmt.Sprint(limit)}
	args = append(args, c.repoArgs()...)
	out, err := c.runContext(ctx, args...)
	if err != nil {
		return nil, err
	}
	var workspaces []Workspace
	if err := json.Unmarshal(out, &workspaces); err != nil {
		return nil, fmt.Errorf("parse workspaces: %w", err)
	}
	return workspaces, nil
}

func (c *Client) ViewWorkspace(ctx context.Context, workspaceID string) (*Workspace, error) {
	args := []string{"workspace", "view", workspaceID}
	args = append(args, c.repoArgs()...)
	out, err := c.runContext(ctx, args...)
	if err != nil {
		return nil, err
	}
	return decodeJSON[Workspace](out, "workspace")
}

func (c *Client) CreateWorkspace(ctx context.Context, name, snapshotID string) (*Workspace, error) {
	args := []string{"workspace", "create"}
	if strings.TrimSpace(name) != "" {
		args = append(args, "-n", name)
	}
	if strings.TrimSpace(snapshotID) != "" {
		args = append(args, "--snapshot", snapshotID)
	}
	args = append(args, c.repoArgs()...)
	out, err := c.runContext(ctx, args...)
	if err != nil {
		return nil, err
	}
	return decodeJSON[Workspace](out, "workspace")
}

func (c *Client) DeleteWorkspace(ctx context.Context, workspaceID string) error {
	args := []string{"workspace", "delete", workspaceID}
	args = append(args, c.repoArgs()...)
	_, err := c.runRawContext(ctx, args...)
	return err
}

func (c *Client) SuspendWorkspace(ctx context.Context, workspaceID string) (*Workspace, error) {
	args := []string{"workspace", "suspend", workspaceID}
	args = append(args, c.repoArgs()...)
	out, err := c.runContext(ctx, args...)
	if err != nil {
		return nil, err
	}
	return decodeJSON[Workspace](out, "workspace")
}

func (c *Client) ResumeWorkspace(ctx context.Context, workspaceID string) (*Workspace, error) {
	args := []string{"workspace", "resume", workspaceID}
	args = append(args, c.repoArgs()...)
	out, err := c.runContext(ctx, args...)
	if err != nil {
		return nil, err
	}
	return decodeJSON[Workspace](out, "workspace")
}

func (c *Client) ForkWorkspace(ctx context.Context, workspaceID, name string) (*Workspace, error) {
	args := []string{"workspace", "fork", workspaceID}
	if strings.TrimSpace(name) != "" {
		args = append(args, "-n", name)
	}
	args = append(args, c.repoArgs()...)
	out, err := c.runContext(ctx, args...)
	if err != nil {
		return nil, err
	}
	return decodeJSON[Workspace](out, "workspace")
}

func (c *Client) ListWorkspaceSnapshots(ctx context.Context, limit int) ([]WorkspaceSnapshot, error) {
	args := []string{"workspace", "snapshot", "list", "-L", fmt.Sprint(limit)}
	args = append(args, c.repoArgs()...)
	out, err := c.runContext(ctx, args...)
	if err != nil {
		return nil, err
	}
	var snapshots []WorkspaceSnapshot
	if err := json.Unmarshal(out, &snapshots); err != nil {
		return nil, fmt.Errorf("parse workspace snapshots: %w", err)
	}
	return snapshots, nil
}

func (c *Client) ViewWorkspaceSnapshot(ctx context.Context, snapshotID string) (*WorkspaceSnapshot, error) {
	args := []string{"workspace", "snapshot", "view", snapshotID}
	args = append(args, c.repoArgs()...)
	out, err := c.runContext(ctx, args...)
	if err != nil {
		return nil, err
	}
	return decodeJSON[WorkspaceSnapshot](out, "workspace snapshot")
}

func (c *Client) CreateWorkspaceSnapshot(ctx context.Context, workspaceID, name string) (*WorkspaceSnapshot, error) {
	args := []string{"workspace", "snapshot", "create", workspaceID}
	if strings.TrimSpace(name) != "" {
		args = append(args, "-n", name)
	}
	args = append(args, c.repoArgs()...)
	out, err := c.runContext(ctx, args...)
	if err != nil {
		return nil, err
	}
	return decodeJSON[WorkspaceSnapshot](out, "workspace snapshot")
}

func (c *Client) DeleteWorkspaceSnapshot(ctx context.Context, snapshotID string) error {
	args := []string{"workspace", "snapshot", "delete", snapshotID}
	args = append(args, c.repoArgs()...)
	_, err := c.runRawContext(ctx, args...)
	return err
}

func (c *Client) ListWorkflows(ctx context.Context, limit int) ([]Workflow, error) {
	args := []string{"workflow", "list", "-L", fmt.Sprint(limit)}
	args = append(args, c.repoArgs()...)
	out, err := c.runContext(ctx, args...)
	if err != nil {
		return nil, err
	}
	var workflows []Workflow
	if err := json.Unmarshal(out, &workflows); err != nil {
		return nil, fmt.Errorf("parse workflows: %w", err)
	}
	return workflows, nil
}

func (c *Client) ListChanges(ctx context.Context, limit int) ([]Change, error) {
	args := []string{"change", "list", "--limit", fmt.Sprint(limit)}
	args = append(args, c.repoArgs()...)
	out, err := c.runContext(ctx, args...)
	if err != nil {
		return nil, err
	}
	var changes []Change
	if err := json.Unmarshal(out, &changes); err != nil {
		return nil, fmt.Errorf("parse changes: %w", err)
	}
	return changes, nil
}

func (c *Client) ViewLanding(ctx context.Context, number int) (*LandingDetail, error) {
	args := []string{"land", "view", fmt.Sprint(number)}
	args = append(args, c.repoArgs()...)
	out, err := c.runContext(ctx, args...)
	if err != nil {
		return nil, err
	}
	var detail LandingDetail
	if err := json.Unmarshal(out, &detail); err != nil {
		return nil, fmt.Errorf("parse landing detail: %w", err)
	}
	return &detail, nil
}

func (c *Client) ViewIssue(ctx context.Context, number int) (*Issue, error) {
	args := []string{"issue", "view", fmt.Sprint(number)}
	args = append(args, c.repoArgs()...)
	out, err := c.runContext(ctx, args...)
	if err != nil {
		return nil, err
	}
	var issue Issue
	if err := json.Unmarshal(out, &issue); err != nil {
		return nil, fmt.Errorf("parse issue: %w", err)
	}
	return &issue, nil
}

func (c *Client) CloseIssue(ctx context.Context, number int, comment string) (*Issue, error) {
	args := []string{"issue", "close", fmt.Sprint(number)}
	if strings.TrimSpace(comment) != "" {
		args = append(args, "-c", comment)
	}
	args = append(args, c.repoArgs()...)
	out, err := c.runContext(ctx, args...)
	if err != nil {
		return nil, err
	}
	var issue Issue
	if err := json.Unmarshal(out, &issue); err != nil {
		return c.ViewIssue(ctx, number)
	}
	return &issue, nil
}

func (c *Client) ViewChange(ctx context.Context, changeID string) (*Change, error) {
	args := []string{"change", "show", changeID}
	args = append(args, c.repoArgs()...)
	out, err := c.runContext(ctx, args...)
	if err != nil {
		return nil, err
	}
	var change Change
	if err := json.Unmarshal(out, &change); err != nil {
		return nil, fmt.Errorf("parse change: %w", err)
	}
	return &change, nil
}

func (c *Client) GetCurrentRepo(ctx context.Context) (*Repo, error) {
	out, err := c.runContext(ctx, "repo", "view")
	if err != nil {
		return nil, err
	}
	var repo Repo
	if err := json.Unmarshal(out, &repo); err != nil {
		return nil, fmt.Errorf("parse repo: %w", err)
	}
	return &repo, nil
}

func (c *Client) LandingDiff(ctx context.Context, number int) (string, error) {
	args := []string{"land", "diff", fmt.Sprint(number)}
	args = append(args, c.repoArgs()...)
	return c.runRawContext(ctx, args...)
}

func (c *Client) ChangeDiff(ctx context.Context, changeID string) (string, error) {
	args := []string{"change", "diff"}
	if strings.TrimSpace(changeID) != "" {
		args = append(args, changeID)
	}
	args = append(args, c.repoArgs()...)
	return c.runRawContext(ctx, args...)
}

func (c *Client) WorkingCopyDiff(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "jj", "diff", "--no-color")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("jj diff: %w", err)
	}
	return string(out), nil
}

func (c *Client) Status(ctx context.Context) (string, error) {
	args := []string{"status"}
	args = append(args, c.repoArgs()...)
	return c.runRawContext(ctx, args...)
}
