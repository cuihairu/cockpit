package probe

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/notification"
	"github.com/cuihairu/cockpit/internal/storage"
)

func TestFailThresholdDefaultsAndClamps(t *testing.T) {
	r, _ := newTestRunner(t)
	if r.FailThreshold() != DefaultFailThreshold {
		t.Errorf("default threshold = %d, want %d", r.FailThreshold(), DefaultFailThreshold)
	}

	r.SetFailThreshold(0) // 低于下限 → 夹紧到 1
	if r.FailThreshold() != MinFailThreshold {
		t.Errorf("threshold = %d, want %d", r.FailThreshold(), MinFailThreshold)
	}
	r.SetFailThreshold(99) // 高于上限 → 夹紧到 10
	if r.FailThreshold() != MaxFailThreshold {
		t.Errorf("threshold = %d, want %d", r.FailThreshold(), MaxFailThreshold)
	}
	r.SetFailThreshold(3)
	if r.FailThreshold() != 3 {
		t.Errorf("threshold = %d, want 3", r.FailThreshold())
	}
}

func TestLoadFailThresholdFromSetting(t *testing.T) {
	r, db := newTestRunner(t)

	// 未配置 → 默认值
	r.LoadFailThreshold()
	if r.FailThreshold() != DefaultFailThreshold {
		t.Fatalf("threshold = %d, want default %d", r.FailThreshold(), DefaultFailThreshold)
	}

	if err := db.SetSetting(FailThresholdSettingKey, "4"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	r.LoadFailThreshold()
	if r.FailThreshold() != 4 {
		t.Errorf("threshold = %d, want 4 from setting", r.FailThreshold())
	}

	// 非法值（越界/非数字）忽略，保持当前值
	db.SetSetting(FailThresholdSettingKey, "999")
	r.LoadFailThreshold()
	if r.FailThreshold() != 4 {
		t.Errorf("out-of-range setting should be ignored, got %d", r.FailThreshold())
	}
	db.SetSetting(FailThresholdSettingKey, "abc")
	r.LoadFailThreshold()
	if r.FailThreshold() != 4 {
		t.Errorf("non-numeric setting should be ignored, got %d", r.FailThreshold())
	}
}

// 自定义阈值生效：threshold=3 时前两次失败不通知，第三次才通知
func TestTrackServiceEdgeCustomThreshold(t *testing.T) {
	r, _, received := newEdgeTestRunner(t, map[string]*config.EventConfig{
		"down": {Type: notification.ServiceDown, Enabled: true},
		"up":   {Type: notification.ServiceUp, Enabled: true},
	})
	r.SetFailThreshold(3)

	down := ProbeResult{ResourceType: "service", ResourceID: "svc-1", Name: "web", Status: "down", Error: "conn refused", Message: "conn refused"}
	r.trackServiceEdge(down)
	r.trackServiceEdge(down)
	select {
	case b := <-received:
		t.Fatalf("threshold=3 but notified after 2 failures: %s", b)
	case <-time.After(300 * time.Millisecond):
	}

	r.trackServiceEdge(down)
	expectEvent(t, received, notification.ServiceDown)
}

// 阈值调小后 >= 语义仍能触发：以 5 开始失败两次后调成 2，第三次失败即通知
func TestTrackServiceEdgeThresholdLoweredMidway(t *testing.T) {
	r, _, received := newEdgeTestRunner(t, map[string]*config.EventConfig{
		"down": {Type: notification.ServiceDown, Enabled: true},
	})
	r.SetFailThreshold(5)

	down := ProbeResult{ResourceType: "service", ResourceID: "svc-1", Name: "web", Status: "down", Error: "x", Message: "x"}
	r.trackServiceEdge(down)
	r.trackServiceEdge(down)

	r.SetFailThreshold(2)
	r.trackServiceEdge(down) // failCount=3 >= 2
	expectEvent(t, received, notification.ServiceDown)
}

func TestRunAllChecksRecordsHistory(t *testing.T) {
	r, db := newTestRunner(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	svc := &storage.Service{ID: "svc-hist", Name: "api", Type: "http", URL: srv.URL, Status: "unknown"}
	if err := db.UpsertService(svc); err != nil {
		t.Fatalf("UpsertService: %v", err)
	}

	res := r.RunAllChecks()
	if res.Services != 1 {
		t.Fatalf("services probed = %d, want 1", res.Services)
	}

	rows, err := db.ListProbeResults("service", svc.ID, 10)
	if err != nil {
		t.Fatalf("ListProbeResults: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("history rows = %d, want 1", len(rows))
	}
	if rows[0].Status != "up" || rows[0].Name != svc.Name || rows[0].ResourceID != svc.ID {
		t.Errorf("row = %+v", rows[0])
	}
	if rows[0].ID == "" {
		t.Error("history row ID should be generated")
	}
}

func TestMaybePruneHistory(t *testing.T) {
	r, db := newTestRunner(t)

	old := time.Now().Add(-40 * 24 * time.Hour)
	fresh := time.Now()
	if err := db.CreateProbeResults([]*storage.ProbeResult{
		{ResourceType: "service", ResourceID: "x", Status: "up", CheckedAt: old},
		{ResourceType: "service", ResourceID: "x", Status: "up", CheckedAt: fresh},
	}); err != nil {
		t.Fatalf("seed history: %v", err)
	}

	// lastPrune 零值 → 首次调用即清理
	r.maybePruneHistory()
	rest, _ := db.ListProbeResults("service", "x", 10)
	if len(rest) != 1 {
		t.Fatalf("prune should remove rows older than retention, got %d rows", len(rest))
	}

	// 24h 内不重复清理：再插一条过期行，lastPrune 刚刚 → 不清
	if err := db.CreateProbeResults([]*storage.ProbeResult{
		{ResourceType: "service", ResourceID: "x", Status: "up", CheckedAt: old},
	}); err != nil {
		t.Fatalf("seed again: %v", err)
	}
	r.lastPrune = time.Now()
	r.maybePruneHistory()
	rest, _ = db.ListProbeResults("service", "x", 10)
	if len(rest) != 2 {
		t.Errorf("prune within interval should be skipped, got %d rows", len(rest))
	}

	// 回拨 lastPrune → 下次调用清理
	r.lastPrune = time.Now().Add(-25 * time.Hour)
	r.maybePruneHistory()
	rest, _ = db.ListProbeResults("service", "x", 10)
	if len(rest) != 1 {
		t.Errorf("prune should run again after interval, got %d rows", len(rest))
	}
}
