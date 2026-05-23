package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// Client GitHub client
type Client struct {
	token     string
	baseURL   string
	client    *http.Client
	timeout   time.Duration
}

// Config client configuration
type Config struct {
	Token     string
	BaseURL   string
	Timeout   time.Duration
}

// NewClient creates GitHub client
func NewClient(cfg Config) *Client {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.github.com"
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &Client{
		token:   cfg.Token,
		baseURL: baseURL,
		client: &http.Client{
			Timeout: timeout,
		},
		timeout: timeout,
	}
}

// doRequest performs HTTP request
func (c *Client) doRequest(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	url := c.baseURL + path

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Set headers
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if c.token != "" {
		req.Header.Set("Authorization", "token "+c.token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("GitHub API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// WorkflowRun workflow run information
type WorkflowRun struct {
	ID             int64     `json:"id"`
	Name           string    `json:"name"`
	NodeID         string    `json:"node_id"`
	HeadBranch     string    `json:"head_branch"`
	HeadSHA        string    `json:"head_sha"`
	Status         string    `json:"status"`
	Conclusion     string    `json:"conclusion"`
	WorkflowID     int64     `json:"workflow_id"`
	URL            string    `json:"html_url"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	StartedAt      time.Time `json:"run_started_at"`
	CompletedAt    time.Time `json:"-"` // 从 UpdatedAt 推导
	RunNumber      int       `json:"run_number"`
	RunAttempt     int       `json:"run_attempt"`
	Event          string    `json:"event"`
	TriggeredBy    string    `json:"triggered_by"`
	JobsURL        string    `json:"jobs_url"`
	LogsURL        string    `json:"logs_url"`
	CheckSuiteURL  string    `json:"check_suite_url"`
	ArtifactsURL   string    `json:"artifacts_url"`
	CancelURL      string    `json:"cancel_url"`
	RerunURL       string    `json:"rerun_url"`
}

// ListWorkflowRuns lists workflow runs for a repository
func (c *Client) ListWorkflowRuns(ctx context.Context, owner, repo string, opts ListOptions) ([]WorkflowRun, error) {
	path := fmt.Sprintf("/repos/%s/%s/actions/runs", owner, repo)
	if opts.PerPage > 0 {
		path += "?per_page=" + strconv.Itoa(opts.PerPage)
	}
	if opts.Page > 0 {
		if opts.PerPage > 0 {
			path += "&"
		} else {
			path += "?"
		}
		path += "page=" + strconv.Itoa(opts.Page)
	}

	body, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("list workflow runs: %w", err)
	}

	var resp struct {
		TotalCount   int           `json:"total_count"`
		WorkflowRuns []WorkflowRun `json:"workflow_runs"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return resp.WorkflowRuns, nil
}

// ListOptions list options
type ListOptions struct {
	Page    int
	PerPage int
}

// GetWorkflowRun gets a workflow run
func (c *Client) GetWorkflowRun(ctx context.Context, owner, repo string, runID int64) (*WorkflowRun, error) {
	path := fmt.Sprintf("/repos/%s/%s/actions/runs/%d", owner, repo, runID)

	body, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("get workflow run: %w", err)
	}

	var run WorkflowRun
	if err := json.Unmarshal(body, &run); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &run, nil
}

// RerunWorkflowRun reruns a workflow run
func (c *Client) RerunWorkflowRun(ctx context.Context, owner, repo string, runID int64) error {
	path := fmt.Sprintf("/repos/%s/%s/actions/runs/%d/rerun", owner, repo, runID)

	_, err := c.doRequest(ctx, "POST", path, nil)
	if err != nil {
		return fmt.Errorf("rerun workflow: %w", err)
	}

	return nil
}

// CancelWorkflowRun cancels a workflow run
func (c *Client) CancelWorkflowRun(ctx context.Context, owner, repo string, runID int64) error {
	path := fmt.Sprintf("/repos/%s/%s/actions/runs/%d/cancel", owner, repo, runID)

	_, err := c.doRequest(ctx, "POST", path, nil)
	if err != nil {
		return fmt.Errorf("cancel workflow: %w", err)
	}

	return nil
}

// Job workflow job information
type Job struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	RunnerName  string    `json:"runner_name"`
	RunnerGroup string    `json:"runner_group"`
	Status      string    `json:"status"`
	Conclusion  string    `json:"conclusion"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
	Steps       []Step    `json:"steps"`
}

// Step job step
type Step struct {
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	Conclusion  string     `json:"conclusion"`
	Number      int        `json:"number"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt time.Time  `json:"completed_at"`
}

// ListJobsForWorkflowRun lists jobs for a workflow run
func (c *Client) ListJobsForWorkflowRun(ctx context.Context, owner, repo string, runID int64, opts ListOptions) ([]Job, error) {
	path := fmt.Sprintf("/repos/%s/%s/actions/runs/%d/jobs", owner, repo, runID)
	if opts.PerPage > 0 {
		path += "?per_page=" + strconv.Itoa(opts.PerPage)
	}
	if opts.Page > 0 {
		if opts.PerPage > 0 {
			path += "&"
		} else {
			path += "?"
		}
		path += "page=" + strconv.Itoa(opts.Page)
	}

	body, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}

	var resp struct {
		TotalCount int   `json:"total_count"`
		Jobs       []Job `json:"jobs"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return resp.Jobs, nil
}

// Repository repository information
type Repository struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	FullName    string `json:"full_name"`
	Private     bool   `json:"private"`
	Description string `json:"description"`
	Fork        bool   `json:"fork"`
	URL         string `json:"html_url"`
}

// GetRepository gets repository information
func (c *Client) GetRepository(ctx context.Context, owner, repo string) (*Repository, error) {
	path := fmt.Sprintf("/repos/%s/%s", owner, repo)

	body, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("get repository: %w", err)
	}

	var r Repository
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &r, nil
}

// WorkflowRunStatus workflow run status
type WorkflowRunStatus struct {
	Total   int `json:"total"`
	Success int `json:"success"`
	Failure int `json:"failure"`
	Pending int `json:"pending"`
	Running int `json:"running"`
}

// GetWorkflowRunStatus gets status summary of recent workflow runs
func (c *Client) GetWorkflowRunStatus(ctx context.Context, owner, repo string) (*WorkflowRunStatus, error) {
	runs, err := c.ListWorkflowRuns(ctx, owner, repo, ListOptions{PerPage: 50})
	if err != nil {
		return nil, err
	}

	status := &WorkflowRunStatus{}
	status.Total = len(runs)

	for _, run := range runs {
		switch run.Status {
		case "completed":
			if run.Conclusion == "success" {
				status.Success++
			} else {
				status.Failure++
			}
		case "pending", "queued":
			status.Pending++
		case "in_progress":
			status.Running++
		}
	}

	return status, nil
}

// TriggerWorkflowOptions options for triggering workflow
type TriggerWorkflowOptions struct {
	Ref       string                 `json:"ref,omitempty"`
	Inputs    map[string]interface{} `json:"inputs,omitempty"`
}

// TriggerWorkflow triggers a workflow
func (c *Client) TriggerWorkflow(ctx context.Context, owner, repo, workflowID string, opts TriggerWorkflowOptions) error {
	path := fmt.Sprintf("/repos/%s/%s/actions/workflows/%s/dispatches", owner, repo, workflowID)

	body, err := json.Marshal(opts)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	_, err = c.doRequest(ctx, "POST", path, body)
	if err != nil {
		return fmt.Errorf("trigger workflow: %w", err)
	}

	return nil
}

// Artifact artifact information
type Artifact struct {
	ID                 int64     `json:"id"`
	Name               string    `json:"name"`
	SizeInBytes        int64     `json:"size_in_bytes"`
	URL                string    `json:"archive_download_url"`
	Expired            bool      `json:"expired"`
	CreatedAt          time.Time `json:"created_at"`
	ExpiresAt          time.Time `json:"expires_at"`
	WorkflowRun        struct {
		ID    int64  `json:"id"`
		URL   string `json:"html_url"`
		Number int   `json:"run_number"`
	} `json:"workflow_run"`
}

// ListArtifacts lists artifacts for a repository
func (c *Client) ListArtifacts(ctx context.Context, owner, repo string, opts ListOptions) ([]Artifact, error) {
	path := fmt.Sprintf("/repos/%s/%s/actions/artifacts", owner, repo)
	if opts.PerPage > 0 {
		path += "?per_page=" + strconv.Itoa(opts.PerPage)
	}
	if opts.Page > 0 {
		if opts.PerPage > 0 {
			path += "&"
		} else {
			path += "?"
		}
		path += "page=" + strconv.Itoa(opts.Page)
	}

	body, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("list artifacts: %w", err)
	}

	var resp struct {
		TotalCount int        `json:"total_count"`
		Artifacts  []Artifact `json:"artifacts"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return resp.Artifacts, nil
}

// ListWorkflowArtifacts lists artifacts for a workflow run
func (c *Client) ListWorkflowArtifacts(ctx context.Context, owner, repo string, runID int64) ([]Artifact, error) {
	path := fmt.Sprintf("/repos/%s/%s/actions/runs/%d/artifacts", owner, repo, runID)

	body, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("list workflow artifacts: %w", err)
	}

	var resp struct {
		TotalCount int        `json:"total_count"`
		Artifacts  []Artifact `json:"artifacts"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return resp.Artifacts, nil
}

// DownloadArtifact downloads an artifact
func (c *Client) DownloadArtifact(ctx context.Context, owner, repo, name string, runID int64) ([]byte, error) {
	path := fmt.Sprintf("/repos/%s/%s/actions/artifacts/%s/zip", owner, repo, name)

	body, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("download artifact: %w", err)
	}

	return body, nil
}

// DeleteArtifact deletes an artifact
func (c *Client) DeleteArtifact(ctx context.Context, owner, repo string, artifactID int64) error {
	path := fmt.Sprintf("/repos/%s/%s/actions/artifacts/%d", owner, repo, artifactID)

	_, err := c.doRequest(ctx, "DELETE", path, nil)
	if err != nil {
		return fmt.Errorf("delete artifact: %w", err)
	}

	return nil
}

// GetRateLimit gets rate limit status
func (c *Client) GetRateLimit(ctx context.Context) (*RateLimit, error) {
	path := "/rate_limit"

	body, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("get rate limit: %w", err)
	}

	var resp struct {
		Resources struct {
			Core   RateLimit `json:"core"`
			GraphQL RateLimit `json:"graphql"`
			Search RateLimit `json:"search"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &resp.Resources.Core, nil
}

// RateLimit rate limit information
type RateLimit struct {
	Limit     int `json:"limit"`
	Remaining int `json:"remaining"`
	Reset     int `json:"reset"`
	Used      int `json:"used"`
}

// IsCompleted returns true if workflow run is completed
func (w *WorkflowRun) IsCompleted() bool {
	return w.Status == "completed"
}

// IsSuccess returns true if workflow run succeeded
func (w *WorkflowRun) IsSuccess() bool {
	return w.Status == "completed" && w.Conclusion == "success"
}

// IsFailed returns true if workflow run failed
func (w *WorkflowRun) IsFailed() bool {
	return w.Status == "completed" && w.Conclusion != "success"
}

// IsRunning returns true if workflow run is running
func (w *WorkflowRun) IsRunning() bool {
	return w.Status == "in_progress"
}

// IsPending returns true if workflow run is pending
func (w *WorkflowRun) IsPending() bool {
	return w.Status == "pending" || w.Status == "queued"
}

// GetDuration returns workflow run duration
func (w *WorkflowRun) GetDuration() time.Duration {
	// 对于已完成的运行，UpdatedAt 作为完成时间
	endTime := w.UpdatedAt
	if w.IsCompleted() {
		endTime = w.UpdatedAt
	} else if !w.UpdatedAt.IsZero() {
		return time.Since(w.StartedAt)
	}
	if endTime.IsZero() {
		return time.Since(w.StartedAt)
	}
	return endTime.Sub(w.StartedAt)
}
