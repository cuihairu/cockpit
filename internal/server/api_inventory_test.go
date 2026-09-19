package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/storage"
	inventorysync "github.com/cuihairu/cockpit/internal/sync"
)

// M6（drift-design.md D28/D30）端点测试：接线 inventorySync 后出报告，
// 未接线时 503 指名引导。
func TestInventoryConsistencyEndpoint(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	invPath := filepath.Join(t.TempDir(), "inventory.yaml")
	content := "version: v1\nregions:\n  r1:\n    zones:\n      z1:\n        agents:\n          agent1:\n            hostname: test-host\n            ip: 10.0.0.2\n          ghost:\n            hostname: gone\n"
	if err := os.WriteFile(invPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	mgr, err := inventorysync.NewManager(invPath, s.db)
	if err != nil {
		t.Fatal(err)
	}
	s.inventorySync = mgr
	t.Cleanup(mgr.Stop)

	// agent1 实报与声明一致（hostname 大小写不敏感）；stray 未在 YAML 声明
	for _, a := range []struct{ id, host, ip string }{
		{"agent1", "Test-Host", "10.0.0.2"},
		{"stray", "stray", "10.0.0.9"},
	} {
		if err := s.db.UpsertAgent(&storage.Agent{ID: a.id, Hostname: a.host, IP: a.ip}); err != nil {
			t.Fatal(err)
		}
	}

	_, req := doAuthenticatedRequest(s, "GET", "/api/inventory/consistency", nil)
	rec := callWithAuth(s, s.handleInventoryConsistency, req)
	if rec.Code != 200 {
		t.Fatalf("code = %d, body = %s", rec.Code, rec.Body.String())
	}

	var report struct {
		Summary struct {
			Total        int `json:"total"`
			OK           int `json:"ok"`
			Unregistered int `json:"unregistered"`
			Undeclared   int `json:"undeclared"`
		} `json:"summary"`
		Agents []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Summary.Total != 3 || report.Summary.OK != 1 ||
		report.Summary.Unregistered != 1 || report.Summary.Undeclared != 1 {
		t.Fatalf("summary = %+v, want total=3 ok=1 unregistered=1 undeclared=1", report.Summary)
	}
	statuses := map[string]string{}
	for _, a := range report.Agents {
		statuses[a.ID] = a.Status
	}
	if statuses["agent1"] != "ok" || statuses["ghost"] != "unregistered" || statuses["stray"] != "undeclared" {
		t.Fatalf("statuses = %v", statuses)
	}
}

func TestInventoryConsistencyEndpointNotConfigured(t *testing.T) {
	s := newTestServerWithDB(t) // inventorySync 保持 nil
	setupAdmin(s)
	_, req := doAuthenticatedRequest(s, "GET", "/api/inventory/consistency", nil)
	rec := callWithAuth(s, s.handleInventoryConsistency, req)
	if rec.Code != 503 {
		t.Fatalf("code = %d, want 503", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "inventory.path") {
		t.Fatalf("body should mention inventory.path: %s", rec.Body.String())
	}
}
