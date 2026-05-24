package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{"default config", Config{}},
		{"with token", Config{Token: "test-token"}},
		{"with base URL", Config{BaseURL: "https://api.github.example.com"}},
		{"with timeout", Config{Timeout: 10 * time.Second}},
		{"full config", Config{
			Token:   "test-token",
			BaseURL: "https://api.github.example.com",
			Timeout: 60 * time.Second,
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewClient(tt.cfg)
			if c == nil {
				t.Error("NewClient() should not return nil")
			}

			expectedBaseURL := tt.cfg.BaseURL
			if expectedBaseURL == "" {
				expectedBaseURL = "https://api.github.com"
			}
			if c.baseURL != expectedBaseURL {
				t.Errorf("baseURL = %v, want %v", c.baseURL, expectedBaseURL)
			}

			expectedTimeout := tt.cfg.Timeout
			if expectedTimeout == 0 {
				expectedTimeout = 30 * time.Second
			}
			if c.timeout != expectedTimeout {
				t.Errorf("timeout = %v, want %v", c.timeout, expectedTimeout)
			}

			if c.token != tt.cfg.Token {
				t.Errorf("token = %v, want %v", c.token, tt.cfg.Token)
			}

			if c.client == nil {
				t.Error("HTTP client should not be nil")
			}
		})
	}
}

func TestNewClientDefaults(t *testing.T) {
	c := NewClient(Config{})

	if c.baseURL != "https://api.github.com" {
		t.Errorf("default baseURL = %v, want https://api.github.com", c.baseURL)
	}

	if c.timeout != 30*time.Second {
		t.Errorf("default timeout = %v, want 30s", c.timeout)
	}

	if c.token != "" {
		t.Error("default token should be empty")
	}
}

func TestClientFields(t *testing.T) {
	cfg := Config{
		Token:   "test-token-123",
		BaseURL: "https://api.example.com",
		Timeout: 45 * time.Second,
	}

	c := NewClient(cfg)

	if c.token != "test-token-123" {
		t.Errorf("token = %v, want test-token-123", c.token)
	}

	if c.baseURL != "https://api.example.com" {
		t.Errorf("baseURL = %v, want https://api.example.com", c.baseURL)
	}

	if c.timeout != 45*time.Second {
		t.Errorf("timeout = %v, want 45s", c.timeout)
	}
}

func TestConfigDefaults(t *testing.T) {
	cfg := Config{}

	if cfg.Token != "" {
		t.Error("Token should be empty by default")
	}

	if cfg.BaseURL != "" {
		t.Error("BaseURL should be empty by default")
	}

	if cfg.Timeout != 0 {
		t.Error("Timeout should be 0 by default")
	}
}

func TestClientHTTPClient(t *testing.T) {
	c := NewClient(Config{Timeout: 10 * time.Second})

	if c.client == nil {
		t.Error("client should not be nil")
	}

	if c.client.Timeout != 10*time.Second {
		t.Errorf("HTTP client timeout = %v, want 10s", c.client.Timeout)
	}
}

func TestMultipleClients(t *testing.T) {
	cfg := Config{Token: "test"}

	for i := 0; i < 5; i++ {
		c := NewClient(cfg)
		if c == nil {
			t.Errorf("NewClient() iteration %d returned nil", i)
		}
		if c.token != "test" {
			t.Errorf("token = %v, want test", c.token)
		}
	}
}

func TestClientWithEmptyToken(t *testing.T) {
	c := NewClient(Config{Token: ""})

	if c.token != "" {
		t.Error("token should be empty")
	}

	// Client should still be valid
	if c.client == nil {
		t.Error("HTTP client should not be nil even without token")
	}
}

func TestClientTimeoutVariations(t *testing.T) {
	tests := []struct {
		name     string
		timeout  time.Duration
		expected time.Duration
	}{
		{"1 second", 1 * time.Second, 1 * time.Second},
		{"30 seconds", 30 * time.Second, 30 * time.Second},
		{"1 minute", 1 * time.Minute, 1 * time.Minute},
		{"zero (defaults to 30s)", 0, 30 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewClient(Config{Timeout: tt.timeout})
			if c.timeout != tt.expected {
				t.Errorf("timeout = %v, want %v", c.timeout, tt.expected)
			}
		})
	}
}

func TestClientBaseURLVariations(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		expected string
	}{
		{"GitHub API", "", "https://api.github.com"},
		{"Custom URL", "https://api.example.com", "https://api.example.com"},
		{"Enterprise", "https://github.company.com/api/v3", "https://github.company.com/api/v3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewClient(Config{BaseURL: tt.baseURL})
			if c.baseURL != tt.expected {
				t.Errorf("baseURL = %v, want %v", c.baseURL, tt.expected)
			}
		})
	}
}

func TestConcurrentClientCreation(t *testing.T) {
	done := make(chan *Client, 10)

	cfg := Config{Token: "test", Timeout: 10 * time.Second}

	for i := 0; i < 10; i++ {
		go func() {
			c := NewClient(cfg)
			done <- c
		}()
	}

	for i := 0; i < 10; i++ {
		c := <-done
		if c == nil {
			t.Error("NewClient() should not return nil")
		}
		if c.token != "test" {
			t.Errorf("token = %v, want test", c.token)
		}
	}
}

func TestClientImmutableConfig(t *testing.T) {
	cfg := Config{Token: "original"}
	c1 := NewClient(cfg)

	// Modify original config
	cfg.Token = "modified"

	// Create another client
	c2 := NewClient(cfg)

	// First client should not be affected
	if c1.token != "original" {
		t.Errorf("c1.token = %v, want original", c1.token)
	}

	// Second client should have new value
	if c2.token != "modified" {
		t.Errorf("c2.token = %v, want modified", c2.token)
	}
}

// ============ HTTP API Tests ============

func TestListWorkflowRuns(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo/actions/runs" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}

		resp := map[string]interface{}{
			"total_count": 2,
			"workflow_runs": []WorkflowRun{
				{ID: 123456, Name: "CI", Status: "completed", Conclusion: "success"},
				{ID: 123457, Name: "CI", Status: "in_progress"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{Token: "test-token", BaseURL: server.URL})

	runs, err := client.ListWorkflowRuns(context.Background(), "owner", "repo", ListOptions{})
	if err != nil {
		t.Fatalf("ListWorkflowRuns() error = %v", err)
	}

	if len(runs) != 2 {
		t.Errorf("len(runs) = %d, want 2", len(runs))
	}
}

func TestListWorkflowRunsWithPagination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.RawQuery
		if query != "per_page=10&page=2" {
			t.Errorf("unexpected query: %s", query)
		}

		resp := map[string]interface{}{
			"total_count":   1,
			"workflow_runs": []WorkflowRun{{ID: 1}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{Token: "test-token", BaseURL: server.URL})

	runs, err := client.ListWorkflowRuns(context.Background(), "owner", "repo", ListOptions{Page: 2, PerPage: 10})
	if err != nil {
		t.Fatalf("ListWorkflowRuns() error = %v", err)
	}

	if len(runs) != 1 {
		t.Errorf("len(runs) = %d, want 1", len(runs))
	}
}

func TestGetWorkflowRun(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo/actions/runs/123456" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		resp := WorkflowRun{ID: 123456, Name: "CI", Status: "completed", Conclusion: "success"}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{Token: "test-token", BaseURL: server.URL})

	run, err := client.GetWorkflowRun(context.Background(), "owner", "repo", 123456)
	if err != nil {
		t.Fatalf("GetWorkflowRun() error = %v", err)
	}

	if run.ID != 123456 {
		t.Errorf("ID = %d, want 123456", run.ID)
	}
}

func TestRerunWorkflowRun(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/repos/owner/repo/actions/runs/123456/rerun" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := NewClient(Config{Token: "test-token", BaseURL: server.URL})

	err := client.RerunWorkflowRun(context.Background(), "owner", "repo", 123456)
	if err != nil {
		t.Errorf("RerunWorkflowRun() error = %v", err)
	}
}

func TestCancelWorkflowRun(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/repos/owner/repo/actions/runs/123456/cancel" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := NewClient(Config{Token: "test-token", BaseURL: server.URL})

	err := client.CancelWorkflowRun(context.Background(), "owner", "repo", 123456)
	if err != nil {
		t.Errorf("CancelWorkflowRun() error = %v", err)
	}
}

func TestListJobsForWorkflowRun(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo/actions/runs/123456/jobs" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		resp := map[string]interface{}{
			"total_count": 2,
			"jobs": []Job{
				{ID: 1, Name: "build", Status: "completed"},
				{ID: 2, Name: "test", Status: "completed"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{Token: "test-token", BaseURL: server.URL})

	jobs, err := client.ListJobsForWorkflowRun(context.Background(), "owner", "repo", 123456, ListOptions{})
	if err != nil {
		t.Fatalf("ListJobsForWorkflowRun() error = %v", err)
	}

	if len(jobs) != 2 {
		t.Errorf("len(jobs) = %d, want 2", len(jobs))
	}
}

func TestGetRepository(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		resp := Repository{ID: 12345, Name: "repo", FullName: "owner/repo", Private: false}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{Token: "test-token", BaseURL: server.URL})

	repo, err := client.GetRepository(context.Background(), "owner", "repo")
	if err != nil {
		t.Fatalf("GetRepository() error = %v", err)
	}

	if repo.Name != "repo" {
		t.Errorf("Name = %s, want repo", repo.Name)
	}
}

func TestGetWorkflowRunStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"total_count": 4,
			"workflow_runs": []WorkflowRun{
				{ID: 1, Status: "completed", Conclusion: "success"},
				{ID: 2, Status: "completed", Conclusion: "failure"},
				{ID: 3, Status: "pending"},
				{ID: 4, Status: "in_progress"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{Token: "test-token", BaseURL: server.URL})

	status, err := client.GetWorkflowRunStatus(context.Background(), "owner", "repo")
	if err != nil {
		t.Fatalf("GetWorkflowRunStatus() error = %v", err)
	}

	if status.Total != 4 {
		t.Errorf("Total = %d, want 4", status.Total)
	}
	if status.Success != 1 {
		t.Errorf("Success = %d, want 1", status.Success)
	}
	if status.Failure != 1 {
		t.Errorf("Failure = %d, want 1", status.Failure)
	}
	if status.Pending != 1 {
		t.Errorf("Pending = %d, want 1", status.Pending)
	}
	if status.Running != 1 {
		t.Errorf("Running = %d, want 1", status.Running)
	}
}

func TestTriggerWorkflow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/repos/owner/repo/actions/workflows/ci.yml/dispatches" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		var req TriggerWorkflowOptions
		json.NewDecoder(r.Body).Decode(&req)

		if req.Ref != "main" {
			t.Errorf("Ref = %s, want main", req.Ref)
		}

		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(Config{Token: "test-token", BaseURL: server.URL})

	err := client.TriggerWorkflow(context.Background(), "owner", "repo", "ci.yml", TriggerWorkflowOptions{
		Ref: "main",
		Inputs: map[string]interface{}{"test": "value"},
	})
	if err != nil {
		t.Errorf("TriggerWorkflow() error = %v", err)
	}
}

func TestListArtifacts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo/actions/artifacts" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		resp := map[string]interface{}{
			"total_count": 1,
			"artifacts": []Artifact{{ID: 123, Name: "build", SizeInBytes: 1024}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{Token: "test-token", BaseURL: server.URL})

	artifacts, err := client.ListArtifacts(context.Background(), "owner", "repo", ListOptions{})
	if err != nil {
		t.Fatalf("ListArtifacts() error = %v", err)
	}

	if len(artifacts) != 1 {
		t.Errorf("len(artifacts) = %d, want 1", len(artifacts))
	}
}

func TestListWorkflowArtifacts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo/actions/runs/123456/artifacts" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		resp := map[string]interface{}{
			"total_count": 1,
			"artifacts":   []Artifact{{ID: 1, Name: "logs"}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{Token: "test-token", BaseURL: server.URL})

	artifacts, err := client.ListWorkflowArtifacts(context.Background(), "owner", "repo", 123456)
	if err != nil {
		t.Fatalf("ListWorkflowArtifacts() error = %v", err)
	}

	if len(artifacts) != 1 {
		t.Errorf("len(artifacts) = %d, want 1", len(artifacts))
	}
}

func TestDeleteArtifact(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if r.URL.Path != "/repos/owner/repo/actions/artifacts/123" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(Config{Token: "test-token", BaseURL: server.URL})

	err := client.DeleteArtifact(context.Background(), "owner", "repo", 123)
	if err != nil {
		t.Errorf("DeleteArtifact() error = %v", err)
	}
}

func TestGetRateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rate_limit" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		resp := map[string]interface{}{
			"resources": map[string]interface{}{
				"core": RateLimit{Limit: 5000, Remaining: 4999, Reset: 1234567890, Used: 1},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{Token: "test-token", BaseURL: server.URL})

	rateLimit, err := client.GetRateLimit(context.Background())
	if err != nil {
		t.Fatalf("GetRateLimit() error = %v", err)
	}

	if rateLimit.Limit != 5000 {
		t.Errorf("Limit = %d, want 5000", rateLimit.Limit)
	}
}

// ============ WorkflowRun Method Tests ============

func TestWorkflowRunIsCompleted(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		expected bool
	}{
		{"completed", "completed", true},
		{"in progress", "in_progress", false},
		{"pending", "pending", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := &WorkflowRun{Status: tt.status}
			if run.IsCompleted() != tt.expected {
				t.Errorf("IsCompleted() = %v, want %v", run.IsCompleted(), tt.expected)
			}
		})
	}
}

func TestWorkflowRunIsSuccess(t *testing.T) {
	tests := []struct {
		name      string
		status    string
		conclusion string
		expected  bool
	}{
		{"success", "completed", "success", true},
		{"failed", "completed", "failure", false},
		{"running", "in_progress", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := &WorkflowRun{Status: tt.status, Conclusion: tt.conclusion}
			if run.IsSuccess() != tt.expected {
				t.Errorf("IsSuccess() = %v, want %v", run.IsSuccess(), tt.expected)
			}
		})
	}
}

func TestWorkflowRunIsFailed(t *testing.T) {
	tests := []struct {
		name      string
		status    string
		conclusion string
		expected  bool
	}{
		{"failed", "completed", "failure", true},
		{"success", "completed", "success", false},
		{"running", "in_progress", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := &WorkflowRun{Status: tt.status, Conclusion: tt.conclusion}
			if run.IsFailed() != tt.expected {
				t.Errorf("IsFailed() = %v, want %v", run.IsFailed(), tt.expected)
			}
		})
	}
}

func TestWorkflowRunIsRunning(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		expected bool
	}{
		{"running", "in_progress", true},
		{"completed", "completed", false},
		{"pending", "pending", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := &WorkflowRun{Status: tt.status}
			if run.IsRunning() != tt.expected {
				t.Errorf("IsRunning() = %v, want %v", run.IsRunning(), tt.expected)
			}
		})
	}
}

func TestWorkflowRunIsPending(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		expected bool
	}{
		{"pending", "pending", true},
		{"queued", "queued", true},
		{"running", "in_progress", false},
		{"completed", "completed", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := &WorkflowRun{Status: tt.status}
			if run.IsPending() != tt.expected {
				t.Errorf("IsPending() = %v, want %v", run.IsPending(), tt.expected)
			}
		})
	}
}

func TestWorkflowRunGetDuration(t *testing.T) {
	now := time.Now()
	past := now.Add(-1 * time.Hour)

	tests := []struct {
		name        string
		startedAt   time.Time
		updatedAt   time.Time
		status      string
		minDuration time.Duration
		maxDuration time.Duration
	}{
		{
			name:        "completed run",
			startedAt:   past,
			updatedAt:   now,
			status:      "completed",
			minDuration: 59 * time.Minute,
			maxDuration: 61 * time.Minute,
		},
		{
			name:        "running run",
			startedAt:   past,
			updatedAt:   past.Add(30 * time.Minute),
			status:      "in_progress",
			minDuration: 59 * time.Minute,
			maxDuration: 61 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := &WorkflowRun{StartedAt: tt.startedAt, UpdatedAt: tt.updatedAt, Status: tt.status}
			duration := run.GetDuration()
			if duration < tt.minDuration || duration > tt.maxDuration {
				t.Errorf("GetDuration() = %v, want between %v and %v", duration, tt.minDuration, tt.maxDuration)
			}
		})
	}
}

// ============ Error Handling Tests ============

func TestHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message": "bad credentials"}`))
	}))
	defer server.Close()

	client := NewClient(Config{Token: "wrong-token", BaseURL: server.URL[7:]})

	_, err := client.ListWorkflowRuns(context.Background(), "owner", "repo", ListOptions{})
	if err == nil {
		t.Error("ListWorkflowRuns() with wrong token should return error")
	}
}

func TestInvalidJSONResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{invalid json}`))
	}))
	defer server.Close()

	client := NewClient(Config{Token: "test-token", BaseURL: server.URL})

	_, err := client.ListWorkflowRuns(context.Background(), "owner", "repo", ListOptions{})
	if err == nil {
		t.Error("ListWorkflowRuns() with invalid JSON should return error")
	}
}

func TestEmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"total_count":   0,
			"workflow_runs": []WorkflowRun{},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{Token: "test-token", BaseURL: server.URL})

	runs, err := client.ListWorkflowRuns(context.Background(), "owner", "repo", ListOptions{})
	if err != nil {
		t.Fatalf("ListWorkflowRuns() error = %v", err)
	}

	if len(runs) != 0 {
		t.Errorf("len(runs) = %d, want 0", len(runs))
	}
}

// ============ Struct Tests ============

func TestStepStruct(t *testing.T) {
	step := Step{Name: "build", Status: "completed", Conclusion: "success", Number: 1}
	if step.Name != "build" {
		t.Errorf("Name = %s, want build", step.Name)
	}
}

func TestJobStruct(t *testing.T) {
	job := Job{ID: 1, Name: "build", Status: "completed"}
	if job.ID != 1 {
		t.Errorf("ID = %d, want 1", job.ID)
	}
}

func TestRepositoryStruct(t *testing.T) {
	repo := Repository{ID: 123, Name: "test", FullName: "owner/test"}
	if repo.ID != 123 {
		t.Errorf("ID = %d, want 123", repo.ID)
	}
}

func TestArtifactStruct(t *testing.T) {
	artifact := Artifact{ID: 1, Name: "build", SizeInBytes: 1024}
	if artifact.ID != 1 {
		t.Errorf("ID = %d, want 1", artifact.ID)
	}
}

func TestRateLimitStruct(t *testing.T) {
	rateLimit := RateLimit{Limit: 5000, Remaining: 4999}
	if rateLimit.Limit != 5000 {
		t.Errorf("Limit = %d, want 5000", rateLimit.Limit)
	}
}

func TestListOptionsStruct(t *testing.T) {
	opts := ListOptions{Page: 2, PerPage: 50}
	if opts.Page != 2 {
		t.Errorf("Page = %d, want 2", opts.Page)
	}
}

func TestWorkflowRunStatusStruct(t *testing.T) {
	status := WorkflowRunStatus{Total: 10, Success: 8, Failure: 2}
	if status.Total != 10 {
		t.Errorf("Total = %d, want 10", status.Total)
	}
}

func TestTriggerWorkflowOptionsStruct(t *testing.T) {
	opts := TriggerWorkflowOptions{Ref: "main"}
	if opts.Ref != "main" {
		t.Errorf("Ref = %s, want main", opts.Ref)
	}
}
