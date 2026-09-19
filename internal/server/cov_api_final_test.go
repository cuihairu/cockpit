package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	inventorysync "github.com/cuihairu/cockpit/internal/sync"
)

// TestCovCronDeleteWithUser DELETE 带目标用户：params/details 均携带 user（M4 D23/D25）
func TestCovCronDeleteWithUser(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		if params["user"] != "postgres" {
			return nil, "user not forwarded on delete"
		}
		return map[string]interface{}{"removed": "t1"}, ""
	})

	rec := httptest.NewRecorder()
	s.handleAgentCronAPI(rec, cronReq(http.MethodDelete, "a1", "jobs/t1?user=postgres", ""), "a1/cron/jobs/t1")
	if rec.Code != http.StatusOK {
		t.Fatalf("delete code = %d body: %s", rec.Code, rec.Body.String())
	}
}

// TestCovInventoryConsistencyDBError 一致性检查遇 db 错误 → 500
func TestCovInventoryConsistencyDBError(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	invPath := filepath.Join(t.TempDir(), "inventory.yaml")
	if err := os.WriteFile(invPath, []byte("version: v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mgr, err := inventorysync.NewManager(invPath, s.db)
	if err != nil {
		t.Fatal(err)
	}
	s.inventorySync = mgr
	t.Cleanup(mgr.Stop)
	s.db.Close() // Consistency 内 ListAgents 报错

	_, req := doAuthenticatedRequest(s, "GET", "/api/inventory/consistency", nil)
	rec := callWithAuth(s, s.handleInventoryConsistency, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code = %d body = %s", rec.Code, rec.Body.String())
	}
}

// TestCovServeAPIOverlayCloudRoute 走完整路由分派到 overlay cloud（路由 case 行）；
// 未配置云凭据时返回 configured:false 的状态响应
func TestCovServeAPIOverlayCloudRoute(t *testing.T) {
	s := newTestServerWithDB(t)
	setupAdmin(s)

	_, req := doAuthenticatedRequest(s, "GET", "/api/overlay/cloud", nil)
	rec := callWithAuth(s, s.serveAPI, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"configured":false`) {
		t.Fatalf("code = %d body = %s", rec.Code, rec.Body.String())
	}
}
