package alert

import (
	"strings"
	"testing"
)

// TestCheckDriftScanCreatesAndDedups 汇总告警创建 → 未读存在期间去重 →
// 已读后再次漂移重新告警（与拨测 M2 去重语义一致）
func TestCheckDriftScanCreatesAndDedups(t *testing.T) {
	db := testDB(t)
	g := NewGenerator(db, nil, nil)

	g.CheckDriftScan("a1", "web-1", []string{"nginx/app（drifted）", "stack/web/compose.yml（missing）"})

	alerts, err := db.ListAlerts(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(alerts) != 1 {
		t.Fatalf("alerts = %d, want 1", len(alerts))
	}
	a := alerts[0]
	if a.Type != "warning" || a.ResourceType == nil || *a.ResourceType != "agent" || *a.ResourceID != "a1" {
		t.Fatalf("alert = %+v", a)
	}
	if !strings.Contains(a.Title, "web-1") || !strings.Contains(a.Message, "nginx/app") {
		t.Fatalf("title/message = %q / %q", a.Title, a.Message)
	}

	// 未读存在 → 去重不新建
	g.CheckDriftScan("a1", "web-1", []string{"nginx/app（drifted）"})
	alerts, _ = db.ListAlerts(10)
	if len(alerts) != 1 {
		t.Fatalf("after dedup alerts = %d, want 1", len(alerts))
	}

	// 已读（已知晓）→ 再次漂移重新告警
	if err := db.MarkAlertAsRead(alerts[0].ID); err != nil {
		t.Fatal(err)
	}
	g.CheckDriftScan("a1", "web-1", []string{"cron/cockpit（drifted）"})
	alerts, _ = db.ListAlerts(10)
	if len(alerts) != 2 {
		t.Fatalf("after read+redrift alerts = %d, want 2", len(alerts))
	}
}

func TestCheckDriftScanNoopAndTruncate(t *testing.T) {
	db := testDB(t)
	g := NewGenerator(db, nil, nil)

	// 空列表（全部一致）无动作
	g.CheckDriftScan("a1", "web-1", nil)
	alerts, _ := db.ListAlerts(10)
	if len(alerts) != 0 {
		t.Fatalf("empty scan created %d alerts, want 0", len(alerts))
	}

	// 超 10 项截断 + 总数提示
	many := make([]string, 13)
	for i := range many {
		many[i] = "nginx/site-" + string(rune('a'+i))
	}
	g.CheckDriftScan("a1", "web-1", many)
	alerts, _ = db.ListAlerts(10)
	if len(alerts) != 1 {
		t.Fatalf("alerts = %d, want 1", len(alerts))
	}
	lines := strings.Split(alerts[0].Message, "\n")
	// 首行说明 + 10 行明细 + 截断提示行
	if len(lines) != 12 || !strings.Contains(lines[11], "13") {
		t.Fatalf("message lines = %d, tail = %q", len(lines), lines[len(lines)-1])
	}
}
