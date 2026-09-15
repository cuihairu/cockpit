package alert

import (
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/storage"
)

func TestCreateAlertDedupesUnread(t *testing.T) {
	db := testDB(t)
	g := NewGenerator(db, nil, nil)

	// 同一资源同一问题多轮检查：只产生一条告警（D15 真去重）
	for i := 0; i < 3; i++ {
		g.createAlertIfNotExists("error", "服务宕机", "msg", "svc-1", "service")
	}
	alerts, err := db.ListAlerts(100)
	if err != nil {
		t.Fatalf("ListAlerts: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("got %d alerts, want 1 (dedup)", len(alerts))
	}

	// 用户已读 = 已知晓，同一问题再现 → 新告警
	if err := db.MarkAllAlertsAsRead(); err != nil {
		t.Fatalf("MarkAllAlertsAsRead: %v", err)
	}
	g.createAlertIfNotExists("error", "服务宕机", "msg again", "svc-1", "service")
	alerts, _ = db.ListAlerts(100)
	if len(alerts) != 2 {
		t.Fatalf("after read, got %d alerts, want 2", len(alerts))
	}
}

func TestCreateAlertDedupScopesByResourceAndTitle(t *testing.T) {
	db := testDB(t)
	g := NewGenerator(db, nil, nil)

	g.createAlertIfNotExists("error", "服务宕机", "msg", "svc-1", "service")

	// 不同资源同名问题 → 各自一条
	g.createAlertIfNotExists("error", "服务宕机", "msg", "svc-2", "service")
	// 同资源不同问题 → 新增一条
	g.createAlertIfNotExists("warning", "磁盘空间不足", "msg", "svc-1", "agent")

	alerts, _ := db.ListAlerts(100)
	if len(alerts) != 3 {
		t.Fatalf("got %d alerts, want 3", len(alerts))
	}
}

func TestCheckDownServicesDedupesAcrossRuns(t *testing.T) {
	db := testDB(t)
	g := NewGenerator(db, nil, nil)

	if err := db.UpsertService(&storage.Service{ID: "svc-down", Name: "api", Type: "http", Status: "down"}); err != nil {
		t.Fatalf("UpsertService: %v", err)
	}

	// 两轮 CheckAllChecks：第二轮不再重复建告警
	g.CheckDownServices()
	g.CheckDownServices()

	alerts, _ := db.ListAlerts(100)
	if len(alerts) != 1 {
		t.Fatalf("got %d alerts after 2 runs, want 1", len(alerts))
	}
}

func TestLoadThresholdsFromSetting(t *testing.T) {
	db := testDB(t)
	g := NewGenerator(db, nil, nil)

	// 默认值
	if g.diskThreshold != DefaultDiskThreshold || g.memThreshold != DefaultMemoryThreshold ||
		g.certWarnDays != DefaultCertWarnDays || g.certInfoDays != DefaultCertInfoDays {
		t.Fatalf("defaults not initialized: %+v", g)
	}

	settings := map[string]string{
		DiskThresholdSettingKey:   "90",
		MemoryThresholdSettingKey: "95",
		CertWarnDaysSettingKey:    "3",
		CertInfoDaysSettingKey:    "14",
	}
	for k, v := range settings {
		if err := db.SetSetting(k, v); err != nil {
			t.Fatalf("SetSetting(%s): %v", k, err)
		}
	}
	g.loadThresholds()
	if g.diskThreshold != 90 || g.memThreshold != 95 || g.certWarnDays != 3 || g.certInfoDays != 14 {
		t.Fatalf("thresholds = %d/%d/%d/%d, want 90/95/3/14",
			g.diskThreshold, g.memThreshold, g.certWarnDays, g.certInfoDays)
	}

	// 越界值忽略保持当前；未配置的键也保持
	db.SetSetting(DiskThresholdSettingKey, "10")
	db.DeleteSetting(MemoryThresholdSettingKey)
	g.loadThresholds()
	if g.diskThreshold != 90 || g.memThreshold != 95 {
		t.Fatalf("invalid/missing settings should keep values, got disk=%d mem=%d", g.diskThreshold, g.memThreshold)
	}
}

// 证书阈值生效：20 天证书默认（warn=7/info=30）触发 info 档；
// info 调成 14 后不再触发；再调成 25 又触发
func TestCertThresholdFromSetting(t *testing.T) {
	db := testDB(t)
	g := NewGenerator(db, nil, nil)

	cert := &storage.Certificate{DomainName: "example.com", Status: "valid",
		ExpiresAt: time.Now().Add(20 * 24 * time.Hour)}
	if err := db.UpsertCertificate(cert); err != nil {
		t.Fatalf("UpsertCertificate: %v", err)
	}

	// 默认 warn=7 < 20 ≤ info=30 → info 档告警
	g.CheckExpiringCertificates()
	alerts, _ := db.ListAlerts(100)
	if len(alerts) != 1 || alerts[0].Type != "warning" {
		t.Fatalf("default should alert on info tier, got %d alerts", len(alerts))
	}

	// info 调成 14：20 > 14 → 不再触发（warn 未变，title 不同不去重）
	db.MarkAllAlertsAsRead()
	db.SetSetting(CertInfoDaysSettingKey, "14")
	g.loadThresholds()
	g.CheckExpiringCertificates()
	alerts, _ = db.ListAlerts(100)
	if len(alerts) != 1 {
		t.Fatalf("info=14 should not alert on 20-day cert, got %d", len(alerts))
	}

	// info 调成 25：20 ≤ 25 → 新 title 新告警
	db.MarkAllAlertsAsRead()
	db.SetSetting(CertInfoDaysSettingKey, "25")
	g.loadThresholds()
	g.CheckExpiringCertificates()
	alerts, _ = db.ListAlerts(100)
	if len(alerts) != 2 {
		t.Fatalf("info=25 should alert on 20-day cert, got %d", len(alerts))
	}
}
