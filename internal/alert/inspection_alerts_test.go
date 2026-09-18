package alert

import (
	"strings"
	"testing"
)

// ============ 巡检类告警生成函数（磁盘健康/DDNS/NAS/ACME）============

// TestCheckDiskHealthLevels 空明细无动作 → warning（扇区异常）→
// error 升级（盘 FAILED，title 不同可与 warning 叠加）
func TestCheckDiskHealthLevels(t *testing.T) {
	db := testDB(t)
	g := NewGenerator(db, nil, nil)

	// 空明细（全健康/unknown 由调用方过滤）无动作
	g.CheckDiskHealth("a1", "web-1", false, nil)
	alerts, _ := db.ListAlerts(10)
	if len(alerts) != 0 {
		t.Fatalf("empty issues created %d alerts, want 0", len(alerts))
	}

	// warning：扇区异常
	g.CheckDiskHealth("a1", "web-1", false, []string{"sda: 3 个扇区重映射"})
	alerts, _ = db.ListAlerts(10)
	if len(alerts) != 1 || alerts[0].Type != "warning" || !strings.Contains(alerts[0].Title, "磁盘健康提醒") {
		t.Fatalf("warning alert = %+v", alerts)
	}

	// 同主机同级别未读 → 去重
	g.CheckDiskHealth("a1", "web-1", false, []string{"sda: 3 个扇区重映射"})
	alerts, _ = db.ListAlerts(10)
	if len(alerts) != 1 {
		t.Fatalf("after dedup alerts = %d, want 1", len(alerts))
	}

	// 升级为 error（title 不同 → 叠加为新事件）
	g.CheckDiskHealth("a1", "web-1", true, []string{"sdb: 盘 FAILED"})
	alerts, _ = db.ListAlerts(10)
	if len(alerts) != 2 {
		t.Fatalf("after escalate alerts = %d, want 2", len(alerts))
	}
	byType := map[string]bool{}
	for _, a := range alerts {
		byType[a.Type] = true
	}
	if !byType["warning"] || !byType["error"] {
		t.Fatalf("levels = %v", byType)
	}
}

// TestCheckDiskHealthTruncate 超 10 块截断 + 总数提示
func TestCheckDiskHealthTruncate(t *testing.T) {
	db := testDB(t)
	g := NewGenerator(db, nil, nil)

	issues := make([]string, 13)
	for i := range issues {
		issues[i] = "sd" + string(rune('a'+i)) + ": 扇区异常"
	}
	g.CheckDiskHealth("a1", "nas-1", false, issues)
	alerts, _ := db.ListAlerts(10)
	if len(alerts) != 1 {
		t.Fatalf("alerts = %d, want 1", len(alerts))
	}
	lines := strings.Split(alerts[0].Message, "\n")
	// 首行说明 + 10 行明细 + 截断提示行
	if len(lines) != 12 || !strings.Contains(lines[11], "13") {
		t.Fatalf("message lines = %d, tail = %q", len(lines), lines[len(lines)-1])
	}
}

// TestCheckDDNSCreatesAndDedups 创建 → 未读去重（按记录名隔离）
func TestCheckDDNSCreatesAndDedups(t *testing.T) {
	db := testDB(t)
	g := NewGenerator(db, nil, nil)

	g.CheckDDNS("home.example.com", "鉴权失败")
	alerts, _ := db.ListAlerts(10)
	if len(alerts) != 1 || alerts[0].Type != "warning" {
		t.Fatalf("alerts = %+v", alerts)
	}
	a := alerts[0]
	if a.ResourceType == nil || *a.ResourceType != "ddns" || *a.ResourceID != "home.example.com" {
		t.Fatalf("resource = %+v", a)
	}
	if !strings.Contains(a.Title, "home.example.com") || !strings.Contains(a.Message, "鉴权失败") {
		t.Fatalf("title/message = %q / %q", a.Title, a.Message)
	}

	// 同记录未读去重；另一记录独立告警
	g.CheckDDNS("home.example.com", "超时")
	alerts, _ = db.ListAlerts(10)
	if len(alerts) != 1 {
		t.Fatalf("after dedup alerts = %d, want 1", len(alerts))
	}
	g.CheckDDNS("blog.example.com", "超时")
	alerts, _ = db.ListAlerts(10)
	if len(alerts) != 2 {
		t.Fatalf("other record alerts = %d, want 2", len(alerts))
	}
}

// TestCheckNasPoolLevels degraded=warning 与 failed=error 两种 title；
// 同状态未读去重
func TestCheckNasPoolLevels(t *testing.T) {
	db := testDB(t)
	g := NewGenerator(db, nil, nil)

	g.CheckNasPool("a1", "nas-1", "pool0", "degraded", "mirror 缺 1 块成员盘")
	alerts, _ := db.ListAlerts(10)
	if len(alerts) != 1 || alerts[0].Type != "warning" || !strings.Contains(alerts[0].Title, "存储池异常") {
		t.Fatalf("degraded alert = %+v", alerts)
	}
	if alerts[0].ResourceType == nil || *alerts[0].ResourceType != "nas_pool" || *alerts[0].ResourceID != "pool0" {
		t.Fatalf("resource = %+v", alerts[0])
	}

	// 同池未读去重
	g.CheckNasPool("a1", "nas-1", "pool0", "degraded", "scrub 进行中")
	alerts, _ = db.ListAlerts(10)
	if len(alerts) != 1 {
		t.Fatalf("after dedup alerts = %d, want 1", len(alerts))
	}

	// failed 升级：title 不同 → 叠加
	g.CheckNasPool("a1", "nas-1", "pool0", "failed", "成员盘离线")
	alerts, _ = db.ListAlerts(10)
	if len(alerts) != 2 {
		t.Fatalf("after failed alerts = %d, want 2", len(alerts))
	}
	last := alerts[0]
	if last.Type != "error" || !strings.Contains(last.Title, "存储池故障") || !strings.Contains(last.Message, "立即检查") {
		t.Fatalf("failed alert = %+v", last)
	}
}

// TestCheckNasUsageCreatesAndDedups 容量告警：title 含挂载路径与百分比
func TestCheckNasUsageCreatesAndDedups(t *testing.T) {
	db := testDB(t)
	g := NewGenerator(db, nil, nil)

	g.CheckNasUsage("a1", "nas-1", "/volume1/media", 91)
	alerts, _ := db.ListAlerts(10)
	if len(alerts) != 1 || alerts[0].Type != "warning" {
		t.Fatalf("alerts = %+v", alerts)
	}
	a0 := alerts[0]
	if a0.ResourceType == nil || *a0.ResourceType != "nas_mount" || *a0.ResourceID != "/volume1/media" {
		t.Fatalf("resource = %+v", a0)
	}
	if !strings.Contains(a0.Title, "/volume1/media") || strings.Contains(a0.Title, "91%") {
		t.Fatalf("title = %q", a0.Title)
	}
	if !strings.Contains(a0.Message, "91%") {
		t.Fatalf("message = %q", a0.Message)
	}

	// 同挂载点未读去重
	g.CheckNasUsage("a1", "nas-1", "/volume1/media", 92)
	alerts, _ = db.ListAlerts(10)
	if len(alerts) != 1 {
		t.Fatalf("after dedup alerts = %d, want 1", len(alerts))
	}
}

// TestCheckACMECreatesAndDedups 证书签发失败告警：按主域名去重
func TestCheckACMECreatesAndDedups(t *testing.T) {
	db := testDB(t)
	g := NewGenerator(db, nil, nil)

	g.CheckACME("example.com", "DNS-01 验证超时")
	alerts, _ := db.ListAlerts(10)
	if len(alerts) != 1 || alerts[0].Type != "warning" {
		t.Fatalf("alerts = %+v", alerts)
	}
	a0 := alerts[0]
	if a0.ResourceType == nil || *a0.ResourceType != "acme" || *a0.ResourceID != "example.com" {
		t.Fatalf("resource = %+v", a0)
	}
	if !strings.Contains(a0.Title, "example.com") || !strings.Contains(a0.Message, "DNS-01") {
		t.Fatalf("title/message = %q / %q", a0.Title, a0.Message)
	}

	// 未读去重 + 已读后重新失败再告警
	g.CheckACME("example.com", "限流")
	alerts, _ = db.ListAlerts(10)
	if len(alerts) != 1 {
		t.Fatalf("after dedup alerts = %d, want 1", len(alerts))
	}
	if err := db.MarkAlertAsRead(alerts[0].ID); err != nil {
		t.Fatal(err)
	}
	g.CheckACME("example.com", "限流")
	alerts, _ = db.ListAlerts(10)
	if len(alerts) != 2 {
		t.Fatalf("after read+fail alerts = %d, want 2", len(alerts))
	}
}
