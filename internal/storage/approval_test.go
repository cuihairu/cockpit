package storage

import (
	"testing"
	"time"
)

// approvalFixture 建一条 pending 审批（可选到期时刻）。
func approvalFixture(t *testing.T, db *DB, id string, expires *time.Time) {
	t.Helper()
	if err := db.CreateApproval(&Approval{
		ID: id, Kind: "job", Payload: `{"type":"agent.exec","target":"a1"}`,
		Summary: "uptime", Target: "a1", Actor: "cui",
		Status: ApprovalStatusPending, ExpiresAt: expires, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("CreateApproval %s: %v", id, err)
	}
}

// TestApprovalCRUD 建查列表全链 + status 过滤。
func TestApprovalCRUD(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	now := time.Now()
	approvalFixture(t, db, "ap-1", &now)
	future := now.Add(time.Hour)
	approvalFixture(t, db, "ap-2", &future)

	got, err := db.GetApproval("ap-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != "job" || got.Target != "a1" || got.Actor != "cui" || got.Status != ApprovalStatusPending {
		t.Fatalf("got = %+v", got)
	}
	// 不存在的 id：ErrRecordNotFound（handler 侧 404 语义的库面来源）
	if _, err := db.GetApproval("ap-none"); err == nil {
		t.Fatal("GetApproval missing id: want error")
	}

	list, err := db.ListApprovals(ApprovalStatusPending, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("pending list = %d, want 2", len(list))
	}
	list, err = db.ListApprovals(ApprovalStatusApproved, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("approved list = %d, want 0", len(list))
	}
}

// TestDecideApprovalConditions 条件终态写：pending 未到期命中一次；重复决策、
// 已过期不命中；无到期时刻恒可决。SetApprovalJobID 回写对账。
func TestDecideApprovalConditions(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	now := time.Now()
	future := now.Add(time.Hour)
	past := now.Add(-time.Minute)
	approvalFixture(t, db, "ap-ok", &future)
	approvalFixture(t, db, "ap-stale", &past)
	approvalFixture(t, db, "ap-forever", nil)

	// 未到期 pending：命中一次
	if n, err := db.DecideApproval("ap-ok", ApprovalStatusApproved, "admin", now); err != nil || n != 1 {
		t.Fatalf("decide hit = %d, %v; want 1", n, err)
	}
	// 重复决策（终态行）：0 行
	if n, err := db.DecideApproval("ap-ok", ApprovalStatusRejected, "admin", now); err != nil || n != 0 {
		t.Fatalf("re-decide hit = %d, %v; want 0", n, err)
	}
	// 已过期：0 行（批准条件闭合，不依赖先查后改）
	if n, err := db.DecideApproval("ap-stale", ApprovalStatusApproved, "admin", now); err != nil || n != 0 {
		t.Fatalf("expired decide hit = %d, %v; want 0", n, err)
	}
	// 无到期时刻：恒可决
	if n, err := db.DecideApproval("ap-forever", ApprovalStatusRejected, "admin", now); err != nil || n != 1 {
		t.Fatalf("no-expiry decide hit = %d, %v; want 1", n, err)
	}

	got, _ := db.GetApproval("ap-ok")
	if got.Status != ApprovalStatusApproved || got.DecidedBy != "admin" || got.DecidedAt == nil {
		t.Fatalf("decided = %+v", got)
	}
	if err := db.SetApprovalJobID("ap-ok", "job-9"); err != nil {
		t.Fatal(err)
	}
	got, _ = db.GetApproval("ap-ok")
	if got.JobID != "job-9" {
		t.Fatalf("jobID = %q", got.JobID)
	}
}

// TestExpireApprovals 批量过期：只动到期的 pending，已决行与未到期行不动。
func TestExpireApprovals(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	now := time.Now()
	past := now.Add(-time.Minute)
	future := now.Add(time.Hour)
	approvalFixture(t, db, "ap-old", &past)
	approvalFixture(t, db, "ap-new", &future)
	approvalFixture(t, db, "ap-rej", &past)
	if _, err := db.DecideApproval("ap-rej", ApprovalStatusRejected, "admin", now.Add(-2*time.Minute)); err != nil {
		t.Fatal(err)
	}

	n, err := db.ExpireApprovals(now)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expired = %d, want 1", n)
	}
	got, _ := db.GetApproval("ap-old")
	if got.Status != ApprovalStatusExpired {
		t.Fatalf("ap-old = %s", got.Status)
	}
	if got, _ := db.GetApproval("ap-new"); got.Status != ApprovalStatusPending {
		t.Fatalf("ap-new = %s", got.Status)
	}
	if got, _ := db.GetApproval("ap-rej"); got.Status != ApprovalStatusRejected {
		t.Fatalf("ap-rej = %s", got.Status)
	}
}

// TestApprovalClosedDB 关库失败面：ListApprovals 错误上抛（server 台账
// 500/降级判定的库面来源）。
func TestApprovalClosedDB(t *testing.T) {
	db := testDB(t)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ListApprovals("", 50); err == nil {
		t.Fatal("ListApprovals on closed db: want error")
	}
}
