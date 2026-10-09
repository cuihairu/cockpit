package storage

import (
	"testing"
	"time"
)

// TestUpsertProbeTargetsIdempotent 同 agent+target 重复 upsert 只有一行，
// 后到者覆盖状态面。
func TestUpsertProbeTargetsIdempotent(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	up := func(name, state string, since time.Time) {
		t.Helper()
		err := db.UpsertProbeTargets("probe-a", []ProbeTargetSnapshot{
			{Target: name, State: state, Since: since, LastChecked: since, LastError: "e-" + state},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	t0 := time.Now()
	up("blog", "faulty", t0)
	up("blog", "healthy", t0.Add(time.Minute))
	up("pg", "unknown", time.Time{})

	rows, err := db.ListProbeTargets()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 (idempotent upsert)", len(rows))
	}
	byName := map[string]*ProbeTargetSnapshot{}
	for _, r := range rows {
		byName[r.Target] = r
	}
	got := byName["blog"]
	if got.State != "healthy" || got.AgentID != "probe-a" || !got.Since.Equal(t0.Add(time.Minute)) {
		t.Fatalf("blog row = %+v", got)
	}
	if byName["pg"].State != "unknown" || !byName["pg"].Since.IsZero() {
		t.Fatalf("pg row = %+v, want zero since", byName["pg"])
	}
}

// TestUpsertProbeWindowsClose 同 (agent,target,started_at) 开窗→关窗上报
// 把 EndedAt 从 nil 刷成时刻，不产生第二行。
func TestUpsertProbeWindowsClose(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	started := time.Now().Add(-time.Hour)
	if err := db.UpsertProbeWindows("probe-a", []ProbeWindow{
		{Target: "blog", StartedAt: started, LastError: "conn refused"},
	}); err != nil {
		t.Fatal(err)
	}
	rows, err := db.ListProbeWindows("probe-a", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].EndedAt != nil {
		t.Fatalf("open window rows = %+v, want 1 open", rows)
	}

	ended := started.Add(20 * time.Minute)
	if err := db.UpsertProbeWindows("probe-a", []ProbeWindow{
		{Target: "blog", StartedAt: started, EndedAt: &ended, LastError: "conn refused"},
	}); err != nil {
		t.Fatal(err)
	}
	rows, err = db.ListProbeWindows("probe-a", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].EndedAt == nil || !rows[0].EndedAt.Equal(ended) {
		t.Fatalf("closed window rows = %+v, want 1 closed at %v", rows, ended)
	}
}

// TestProbeUpsertEmptyAndDefaultLimit 空批量 upsert 直通、limit<=0 回退
// 缺省 50（死参数分支走真路径）。
func TestProbeUpsertEmptyAndDefaultLimit(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	if err := db.UpsertProbeTargets("probe-a", nil); err != nil {
		t.Fatalf("empty targets upsert: %v", err)
	}
	if err := db.UpsertProbeWindows("probe-a", nil); err != nil {
		t.Fatalf("empty windows upsert: %v", err)
	}

	started := time.Now().Add(-time.Hour)
	if err := db.UpsertProbeWindows("probe-a", []ProbeWindow{
		{Target: "blog", StartedAt: started},
	}); err != nil {
		t.Fatal(err)
	}
	rows, err := db.ListProbeWindows("probe-a", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
}

// TestListProbeWindowsFiltersAndLimit agent/target 过滤与 limit 上限。
func TestListProbeWindowsFiltersAndLimit(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	base := time.Now().Add(-24 * time.Hour)
	ws := []ProbeWindow{
		{Target: "blog", StartedAt: base},
		{Target: "blog", StartedAt: base.Add(time.Hour)},
		{Target: "pg", StartedAt: base.Add(2 * time.Hour)},
	}
	if err := db.UpsertProbeWindows("probe-a", ws); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertProbeWindows("probe-b", []ProbeWindow{
		{Target: "blog", StartedAt: base.Add(3 * time.Hour)},
	}); err != nil {
		t.Fatal(err)
	}

	rows, err := db.ListProbeWindows("probe-a", "blog", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].StartedAt.Before(rows[1].StartedAt) {
		t.Fatalf("filtered rows = %+v, want 2 desc by started_at", rows)
	}

	rows, err = db.ListProbeWindows("probe-a", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Target != "pg" {
		t.Fatalf("limited rows = %+v, want newest 1 (pg)", rows)
	}
}

// TestDeleteProbeWindowsOlderThan 保留期外窗口清理只删旧行。
func TestDeleteProbeWindowsOlderThan(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	old := time.Now().Add(-31 * 24 * time.Hour)
	fresh := time.Now().Add(-time.Hour)
	if err := db.UpsertProbeWindows("probe-a", []ProbeWindow{
		{Target: "blog", StartedAt: old},
		{Target: "pg", StartedAt: fresh},
	}); err != nil {
		t.Fatal(err)
	}

	n, err := db.DeleteProbeWindowsOlderThan(time.Now().Add(-30 * 24 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("deleted = %d, want 1", n)
	}
	rows, err := db.ListProbeWindows("probe-a", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Target != "pg" {
		t.Fatalf("remaining rows = %+v, want pg only", rows)
	}
}
