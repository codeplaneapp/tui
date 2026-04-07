// Package github shells out to the gh CLI and parses JSON output.
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type User struct {
	ID    string `json:"id"`
	Login string `json:"login"`
	Name  string `json:"name"`
	IsBot bool   `json:"is_bot"`
}

type Label struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color"`
}

type Repo struct {
	NameWithOwner string `json:"nameWithOwner"`
	Description   string `json:"description"`
	URL           string `json:"url"`
}

type Issue struct {
	Number    int     `json:"number"`
	Title     string  `json:"title"`
	Body      string  `json:"body"`
	State     string  `json:"state"`
	Author    User    `json:"author"`
	Assignees []User  `json:"assignees"`
	Labels    []Label `json:"labels"`
	CreatedAt string  `json:"createdAt"`
	UpdatedAt string  `json:"updatedAt"`
	URL       string  `json:"url"`
}

type PullRequest struct {
	Number           int     `json:"number"`
	Title            string  `json:"title"`
	Body             string  `json:"body"`
	State            string  `json:"state"`
	IsDraft          bool    `json:"isDraft"`
	Author           User    `json:"author"`
	Assignees        []User  `json:"assignees"`
	Labels           []Label `json:"labels"`
	CreatedAt        string  `json:"createdAt"`
	UpdatedAt        string  `json:"updatedAt"`
	URL              string  `json:"url"`
	HeadRefName      string  `json:"headRefName"`
	BaseRefName      string  `json:"baseRefName"`
	ChangedFiles     int     `json:"changedFiles"`
	MergeStateStatus string  `json:"mergeStateStatus"`
	ReviewDecision   string  `json:"reviewDecision"`
}

type Client struct {
	repo string
}

type createIssueResponse struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	State     string `json:"state"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	URL       string `json:"html_url"`
	User      User   `json:"user"`
}

func NewClient(repo string) *Client {
	return &Client{repo: repo}
}

func (c *Client) run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("%s", msg)
	}
	return out, nil
}

func (c *Client) repoArgs() []string {
	if c.repo == "" {
		return nil
	}
	return []string{"--repo", c.repo}
}

func (c *Client) resolveRepo(ctx context.Context) (string, error) {
	if c.repo != "" {
		return c.repo, nil
	}

	repo, err := c.GetCurrentRepo(ctx)
	if err != nil {
		return "", err
	}
	if repo == nil || repo.NameWithOwner == "" {
		return "", fmt.Errorf("resolve repo: no repository available")
	}
	return repo.NameWithOwner, nil
}

func (c *Client) ListIssues(ctx context.Context, state string, limit int) ([]Issue, error) {
	if state == "" {
		state = "open"
	}
	if limit <= 0 {
		limit = 30
	}

	args := []string{
		"issue", "list",
		"--state", state,
		"--limit", fmt.Sprint(limit),
		"--json", "number,title,body,state,author,assignees,labels,createdAt,updatedAt,url",
	}
	args = append(args, c.repoArgs()...)

	out, err := c.run(ctx, args...)
	if err != nil {
		return nil, err
	}

	var issues []Issue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("parse issues: %w", err)
	}
	return issues, nil
}

func (c *Client) ListPullRequests(ctx context.Context, state string, limit int) ([]PullRequest, error) {
	if state == "" {
		state = "open"
	}
	if limit <= 0 {
		limit = 30
	}

	args := []string{
		"pr", "list",
		"--state", state,
		"--limit", fmt.Sprint(limit),
		"--json", "number,title,body,state,isDraft,author,assignees,labels,createdAt,updatedAt,url,headRefName,baseRefName,changedFiles,mergeStateStatus,reviewDecision",
	}
	args = append(args, c.repoArgs()...)

	out, err := c.run(ctx, args...)
	if err != nil {
		return nil, err
	}

	var prs []PullRequest
	if err := json.Unmarshal(out, &prs); err != nil {
		return nil, fmt.Errorf("parse pull requests: %w", err)
	}
	return prs, nil
}

func (c *Client) CreateIssue(ctx context.Context, title, body string) (*Issue, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, fmt.Errorf("title must not be empty")
	}

	repo, err := c.resolveRepo(ctx)
	if err != nil {
		return nil, err
	}

	args := []string{
		"api",
		"--method", "POST",
		fmt.Sprintf("repos/%s/issues", repo),
		"-f", "title=" + title,
	}
	if strings.TrimSpace(body) != "" {
		args = append(args, "-f", "body="+body)
	}

	out, err := c.run(ctx, args...)
	if err != nil {
		return nil, err
	}

	var created createIssueResponse
	if err := json.Unmarshal(out, &created); err != nil {
		return nil, fmt.Errorf("parse created issue: %w", err)
	}

	return &Issue{
		Number:    created.Number,
		Title:     created.Title,
		Body:      created.Body,
		State:     created.State,
		Author:    created.User,
		CreatedAt: created.CreatedAt,
		UpdatedAt: created.UpdatedAt,
		URL:       created.URL,
	}, nil
}

func (c *Client) GetCurrentRepo(ctx context.Context) (*Repo, error) {
	args := []string{"repo", "view"}
	if c.repo != "" {
		args = append(args, c.repo)
	}
	args = append(args, "--json", "nameWithOwner,description,url")

	out, err := c.run(ctx, args...)
	if err != nil {
		return nil, err
	}

	var repo Repo
	if err := json.Unmarshal(out, &repo); err != nil {
		return nil, fmt.Errorf("parse repo: %w", err)
	}
	return &repo, nil
}
