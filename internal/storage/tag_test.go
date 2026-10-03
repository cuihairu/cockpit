package storage

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// tag_test.go 服务器标签存储层（tag.go，bc1098c）：
// 归一化/重复映射（ErrTagNameTaken）、标签 CRUD、计数聚合、
// 覆盖式关联设置（去重/未知 tag 拒绝/幂等）、agent 删除连带清理。

func TestNormalizeTagName(t *testing.T) {
	if got := NormalizeTagName("  prod  "); got != "prod" {
		t.Errorf("trim = %q, want prod", got)
	}
	long := strings.Repeat("x", 70)
	if got := NormalizeTagName(long); len(got) != 64 {
		t.Errorf("len = %d, want 64", len(got))
	}
	// 截断边界切出尾部空格时仍归一：前 64 字节以空格结尾 → 再 trim
	padded := strings.Repeat("x", 62) + "   zz"
	if got := NormalizeTagName(padded); got != strings.Repeat("x", 62) {
		t.Errorf("truncate-trim = %q, want 62x", got)
	}
}

func TestCreateTag(t *testing.T) {
	d := testDB(t)
	defer d.Close()

	// 正常创建：名称归一、颜色去空白
	tag, err := d.CreateTag("  prod  ", " blue ")
	if err != nil {
		t.Fatalf("CreateTag() error = %v", err)
	}
	if tag.Name != "prod" || tag.Color != "blue" || tag.ID == "" {
		t.Errorf("tag = %+v, want name=prod color=blue id set", tag)
	}

	// 重复 → ErrTagNameTaken（先查重路径）
	if _, err := d.CreateTag("prod", "red"); !errors.Is(err, ErrTagNameTaken) {
		t.Errorf("duplicate = %v, want ErrTagNameTaken", err)
	}
	// 归一化后空名 → required
	if _, err := d.CreateTag("   ", ""); err == nil || err.Error() != "tag name is required" {
		t.Errorf("blank name = %v, want required error", err)
	}
	// 查询失败（DB 已关）→ 非 NotFound 的查重错误透传
	if err := d.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := d.CreateTag("later", ""); err == nil || errors.Is(err, ErrTagNameTaken) {
		t.Errorf("closed db = %v, want generic error", err)
	}
}

// TestCreateTagRandFail newTagID 的 randRead 注入失败（cov_inject 同款手法）
func TestCreateTagRandFail(t *testing.T) {
	d := testDB(t)
	defer d.Close()

	orig := randRead
	t.Cleanup(func() { randRead = orig })
	randRead = func([]byte) (int, error) { return 0, errors.New("boom rand") }

	if _, err := d.CreateTag("prod", ""); err == nil || err.Error() != "boom rand" {
		t.Errorf("randRead fail = %v, want propagated", err)
	}
}

// TestCreateTagUniqueRace 落库撞唯一约束（并发查重空档）的兜底：
// 注入 randRead 返回已存在标签的 ID，同名检查通过但主键冲突，
// 必须映射回 ErrTagNameTaken（供 handler 转 409）
func TestCreateTagUniqueRace(t *testing.T) {
	d := testDB(t)
	defer d.Close()

	first, err := d.CreateTag("prod", "")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	idBytes := make([]byte, 16)
	if _, err := decodeHexInto(first.ID, idBytes); err != nil {
		t.Fatalf("decode id: %v", err)
	}
	orig := randRead
	t.Cleanup(func() { randRead = orig })
	randRead = func(b []byte) (int, error) {
		copy(b, idBytes)
		return len(b), nil
	}

	if _, err := d.CreateTag("staging", ""); !errors.Is(err, ErrTagNameTaken) {
		t.Errorf("pk conflict = %v, want ErrTagNameTaken", err)
	}
}

// TestCreateTagTriggerFail 非唯一约束的落库错误原样透传
// （SQLite TRIGGER RAISE(ABORT) 注入无 "UNIQUE" 文本的失败）
func TestCreateTagTriggerFail(t *testing.T) {
	d := testDB(t)
	defer d.Close()

	if err := d.db.Exec(`CREATE TRIGGER tag_boom BEFORE INSERT ON agent_tags
		BEGIN SELECT RAISE(ABORT, 'tag insert boom'); END`).Error; err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	if _, err := d.CreateTag("prod", ""); err == nil ||
		errors.Is(err, ErrTagNameTaken) || !strings.Contains(err.Error(), "tag insert boom") {
		t.Errorf("trigger fail = %v, want raw boom error", err)
	}
}

func TestUpdateTag(t *testing.T) {
	d := testDB(t)
	defer d.Close()

	tag, err := d.CreateTag("prod", "blue")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	other, _ := d.CreateTag("db", "")

	// 重命名 + 改色
	if err := d.UpdateTag(tag.ID, "production", "green"); err != nil {
		t.Fatalf("UpdateTag() error = %v", err)
	}
	got, _ := d.GetTag(tag.ID)
	if got.Name != "production" || got.Color != "green" {
		t.Errorf("after update = %+v", got)
	}
	// 空名不改名，只改色
	if err := d.UpdateTag(tag.ID, "", "red"); err != nil {
		t.Fatalf("color-only: %v", err)
	}
	got, _ = d.GetTag(tag.ID)
	if got.Name != "production" || got.Color != "red" {
		t.Errorf("color-only = %+v", got)
	}
	// 改名撞别的标签 → ErrTagNameTaken
	if err := d.UpdateTag(tag.ID, "db", ""); !errors.Is(err, ErrTagNameTaken) {
		t.Errorf("clash = %v, want ErrTagNameTaken", err)
	}
	// 不存在 → ErrTagNotFound
	if err := d.UpdateTag("nope", "x", ""); !errors.Is(err, ErrTagNotFound) {
		t.Errorf("missing = %v, want ErrTagNotFound", err)
	}
	_ = other
	// DB 关闭 → 查询失败透传
	if err := d.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := d.UpdateTag(tag.ID, "prod2", ""); err == nil || errors.Is(err, ErrTagNotFound) {
		t.Errorf("closed db = %v, want generic error", err)
	}
}

func TestGetTag(t *testing.T) {
	d := testDB(t)
	defer d.Close()

	tag, _ := d.CreateTag("prod", "blue")
	got, err := d.GetTag(tag.ID)
	if err != nil || got.Name != "prod" {
		t.Fatalf("GetTag() = %+v, %v", got, err)
	}
	if _, err := d.GetTag("nope"); !errors.Is(err, ErrTagNotFound) {
		t.Errorf("missing = %v, want ErrTagNotFound", err)
	}
	// 查询错误（非 NotFound）原样返回
	if err := d.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := d.GetTag(tag.ID); err == nil || errors.Is(err, ErrTagNotFound) {
		t.Errorf("closed db = %v, want generic error", err)
	}
}

func TestDeleteTag(t *testing.T) {
	d := testDB(t)
	defer d.Close()

	if err := d.UpsertAgent(&Agent{ID: "a1", Hostname: "h1"}); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	tag, _ := d.CreateTag("prod", "")
	if err := d.SetAgentTags("a1", []string{tag.ID}); err != nil {
		t.Fatalf("assign: %v", err)
	}

	if err := d.DeleteTag(tag.ID); err != nil {
		t.Fatalf("DeleteTag() error = %v", err)
	}
	// 标签与关联行都消失，agent 本身保留
	if _, err := d.GetTag(tag.ID); !errors.Is(err, ErrTagNotFound) {
		t.Errorf("tag still present: %v", err)
	}
	if n := len(d.mustTagIDs(t, tag.ID)); n != 0 {
		t.Errorf("assignments left: %d", n)
	}
	if _, err := d.GetAgent("a1"); err != nil {
		t.Errorf("agent should survive: %v", err)
	}
	// 删除不存在 → ErrTagNotFound
	if err := d.DeleteTag("nope"); !errors.Is(err, ErrTagNotFound) {
		t.Errorf("missing = %v, want ErrTagNotFound", err)
	}
	// 关闭后删除 → 通用错误
	if err := d.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := d.DeleteTag(tag.ID); err == nil || errors.Is(err, ErrTagNotFound) {
		t.Errorf("closed db = %v, want generic error", err)
	}
}

// mustTagIDs 取标签下 agent ID（测试内断言关联行用）
func (d *DB) mustTagIDs(t *testing.T, tagID string) []string {
	t.Helper()
	ids, err := d.AgentIDsByTag(tagID)
	if err != nil {
		t.Fatalf("AgentIDsByTag: %v", err)
	}
	return ids
}

func TestListTagsAndCounts(t *testing.T) {
	d := testDB(t)
	defer d.Close()

	if err := d.UpsertAgent(&Agent{ID: "a1", Hostname: "h1"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := d.UpsertAgent(&Agent{ID: "a2", Hostname: "h2"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	prod, _ := d.CreateTag("prod", "")
	stg, _ := d.CreateTag("staging", "")
	if err := d.SetAgentTags("a1", []string{prod.ID, stg.ID}); err != nil {
		t.Fatalf("assign a1: %v", err)
	}
	if err := d.SetAgentTags("a2", []string{prod.ID}); err != nil {
		t.Fatalf("assign a2: %v", err)
	}

	// 列表按创建时间升序
	tags, err := d.ListTags()
	if err != nil || len(tags) != 2 {
		t.Fatalf("ListTags() = %v, %v", tags, err)
	}
	if tags[0].Name != "prod" || tags[1].Name != "staging" {
		t.Errorf("order = %s,%s", tags[0].Name, tags[1].Name)
	}

	// TagCounts：prod 挂 2 台、staging 挂 1 台
	counts, err := d.TagCounts()
	if err != nil {
		t.Fatalf("TagCounts: %v", err)
	}
	if counts[prod.ID] != 2 || counts[stg.ID] != 1 {
		t.Errorf("counts = %v", counts)
	}

	// AgentTagCounts：按 agent 统计 + 空入参短路
	ac, err := d.AgentTagCounts([]string{"a1", "a2", "ghost"})
	if err != nil || ac["a1"] != 2 || ac["a2"] != 1 {
		t.Errorf("AgentTagCounts = %v, %v", ac, err)
	}
	if ac, err := d.AgentTagCounts(nil); err != nil || len(ac) != 0 {
		t.Errorf("empty = %v, %v", ac, err)
	}
	// 空入参不发查询 → DB 关闭也返回空
	if err := d.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if ac, err := d.AgentTagCounts(nil); err != nil || len(ac) != 0 {
		t.Errorf("closed empty = %v, %v", ac, err)
	}
}

func TestAgentTagsQueries(t *testing.T) {
	d := testDB(t)
	defer d.Close()

	if err := d.UpsertAgent(&Agent{ID: "a1", Hostname: "h1"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := d.UpsertAgent(&Agent{ID: "a2", Hostname: "h2"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// 名称序（创建顺序反过来，断言按 name 排序）
	b, _ := d.CreateTag("beta", "b")
	a, _ := d.CreateTag("alpha", "a")
	if err := d.SetAgentTags("a1", []string{b.ID, a.ID}); err != nil {
		t.Fatalf("assign: %v", err)
	}
	if err := d.SetAgentTags("a2", []string{a.ID}); err != nil {
		t.Fatalf("assign: %v", err)
	}

	// AgentTags：单机、按名称排序
	tags, err := d.AgentTags("a1")
	if err != nil || len(tags) != 2 || tags[0].Name != "alpha" || tags[1].Name != "beta" {
		t.Fatalf("AgentTags = %+v, %v", tags, err)
	}
	// 无关联 → 空
	if tags, err := d.AgentTags("ghost"); err != nil || len(tags) != 0 {
		t.Errorf("ghost = %v, %v", tags, err)
	}

	// TagsForAgents：批量取（含空入参短路）
	m, err := d.TagsForAgents([]string{"a1", "a2"})
	if err != nil || len(m["a1"]) != 2 || len(m["a2"]) != 1 {
		t.Fatalf("TagsForAgents = %v, %v", m, err)
	}
	if m["a1"][0].Color != "a" {
		t.Errorf("color not carried: %+v", m["a1"][0])
	}
	if m, err := d.TagsForAgents(nil); err != nil || len(m) != 0 {
		t.Errorf("empty = %v, %v", m, err)
	}

	// AgentIDsByTag：按标签反查机器
	ids, err := d.AgentIDsByTag(a.ID)
	if err != nil || len(ids) != 2 {
		t.Errorf("AgentIDsByTag = %v, %v", ids, err)
	}

	// 查询失败（DB 关闭）→ 错误透传
	if err := d.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := d.AgentTags("a1"); err == nil {
		t.Error("closed AgentTags should error")
	}
	if _, err := d.TagsForAgents([]string{"a1"}); err == nil {
		t.Error("closed TagsForAgents should error")
	}
	if _, err := d.AgentIDsByTag(a.ID); err == nil {
		t.Error("closed AgentIDsByTag should error")
	}
}

func TestSetAgentTags(t *testing.T) {
	d := testDB(t)
	defer d.Close()

	if err := d.UpsertAgent(&Agent{ID: "a1", Hostname: "h1"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	t1, _ := d.CreateTag("prod", "")
	t2, _ := d.CreateTag("db", "")

	// 覆盖设置：重复 ID 去重、空串忽略
	if err := d.SetAgentTags("a1", []string{t1.ID, t1.ID, " ", t2.ID}); err != nil {
		t.Fatalf("SetAgentTags: %v", err)
	}
	ids := d.mustTagIDs(t, t1.ID)
	if len(ids) != 1 || ids[0] != "a1" {
		t.Errorf("t1 ids = %v", ids)
	}
	// 幂等：同集合重复调用无副作用
	if err := d.SetAgentTags("a1", []string{t1.ID, t2.ID}); err != nil {
		t.Fatalf("idempotent: %v", err)
	}
	if got, _ := d.AgentTags("a1"); len(got) != 2 {
		t.Errorf("after idempotent = %d tags", len(got))
	}
	// 未知 tag → ErrTagNotFound，且不落地悬空关联
	if err := d.SetAgentTags("a1", []string{"ghost"}); !errors.Is(err, ErrTagNotFound) {
		t.Errorf("unknown tag = %v, want ErrTagNotFound", err)
	}
	if got, _ := d.AgentTags("a1"); len(got) != 2 {
		t.Errorf("failed set must not clobber: %d tags", len(got))
	}
	// 空集合 → 清空
	if err := d.SetAgentTags("a1", nil); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if got, _ := d.AgentTags("a1"); len(got) != 0 {
		t.Errorf("cleared = %d tags", len(got))
	}
	// agent 不存在 → ErrNotFound
	if err := d.SetAgentTags("ghost", []string{t1.ID}); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing agent = %v, want ErrNotFound", err)
	}
	// 关闭后设置 → 通用错误
	if err := d.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := d.SetAgentTags("a1", nil); err == nil || errors.Is(err, ErrNotFound) {
		t.Errorf("closed db = %v, want generic error", err)
	}
}

func TestClearAgentTagsAndDeleteAgentCascade(t *testing.T) {
	d := testDB(t)
	defer d.Close()

	if err := d.UpsertAgent(&Agent{ID: "a1", Hostname: "h1"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	tag, _ := d.CreateTag("prod", "")
	if err := d.SetAgentTags("a1", []string{tag.ID}); err != nil {
		t.Fatalf("assign: %v", err)
	}
	// agent 删除：连带摘关联（tag.go 的 ClearAgentTags 被 storage.DeleteAgent 调用）
	if err := d.DeleteAgent("a1"); err != nil {
		t.Fatalf("DeleteAgent: %v", err)
	}
	if ids := d.mustTagIDs(t, tag.ID); len(ids) != 0 {
		t.Errorf("assignments left after agent delete: %v", ids)
	}
	// 清不存在的 agent 关联是无害空操作
	if err := d.ClearAgentTags("ghost"); err != nil {
		t.Errorf("clear ghost = %v", err)
	}
	// 关闭后清理 → 错误透传
	if err := d.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := d.ClearAgentTags("a1"); err == nil {
		t.Error("closed db should error")
	}
}

// TestDeleteTagAssignTriggerFail 删除标签时清理关联行失败 → 事务中止错误透传
// （SQLite trigger 在 DELETE 上注入与约束无关的失败）
func TestDeleteTagAssignTriggerFail(t *testing.T) {
	d := testDB(t)
	defer d.Close()

	if err := d.UpsertAgent(&Agent{ID: "a1", Hostname: "h1"}); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	tag, _ := d.CreateTag("prod", "")
	if err := d.SetAgentTags("a1", []string{tag.ID}); err != nil {
		t.Fatalf("assign: %v", err)
	}
	if err := d.db.Exec(`CREATE TRIGGER assign_boom BEFORE DELETE ON agent_tag_assignments
		BEGIN SELECT RAISE(ABORT, 'assign delete boom'); END`).Error; err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	err := d.DeleteTag(tag.ID)
	if err == nil || !strings.Contains(err.Error(), "assign delete boom") {
		t.Errorf("DeleteTag = %v, want trigger error", err)
	}
}

// TestSetAgentTagsTxTriggerFail 覆盖式重写时清旧行失败 → 事务错误透传
func TestSetAgentTagsTxTriggerFail(t *testing.T) {
	d := testDB(t)
	defer d.Close()

	if err := d.UpsertAgent(&Agent{ID: "a1", Hostname: "h1"}); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	t1, _ := d.CreateTag("prod", "")
	if err := d.SetAgentTags("a1", []string{t1.ID}); err != nil {
		t.Fatalf("assign: %v", err)
	}
	if err := d.db.Exec(`CREATE TRIGGER assign_boom BEFORE DELETE ON agent_tag_assignments
		BEGIN SELECT RAISE(ABORT, 'assign delete boom'); END`).Error; err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	// 再次覆盖设置 → 事务内 DELETE 旧行被 trigger 打断
	if err := d.SetAgentTags("a1", []string{t1.ID}); err == nil ||
		!strings.Contains(err.Error(), "assign delete boom") {
		t.Errorf("SetAgentTags = %v, want trigger error", err)
	}
}

// TestSetAgentTagsCountError 存在性校验的 Count 查询失败（表被删）→ 错误透传，
// 且发生在 GetAgent 通过之后（非 agent 缺失路径）
func TestSetAgentTagsCountError(t *testing.T) {
	d := testDB(t)
	defer d.Close()

	if err := d.UpsertAgent(&Agent{ID: "a1", Hostname: "h1"}); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	// GetAgent 查 agents 表仍成功；随后对 agent_tags 的 Count 直接报错
	if err := d.db.Exec("DROP TABLE agent_tags").Error; err != nil {
		t.Fatalf("drop: %v", err)
	}
	if err := d.SetAgentTags("a1", []string{"t-x"}); err == nil ||
		errors.Is(err, ErrTagNotFound) || errors.Is(err, ErrNotFound) {
		t.Errorf("count fail = %v, want generic error", err)
	}
}

// TestListTagQueriesError 统计/列表查询的错误分支（DB 关闭后仍非空入参）
func TestListTagQueriesError(t *testing.T) {
	d := testDB(t)
	defer d.Close()

	if err := d.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := d.ListTags(); err == nil {
		t.Error("closed ListTags should error")
	}
	if _, err := d.TagCounts(); err == nil {
		t.Error("closed TagCounts should error")
	}
	if _, err := d.AgentTagCounts([]string{"a1"}); err == nil {
		t.Error("closed AgentTagCounts should error")
	}
	if err := d.DeleteTag("x"); err == nil {
		t.Error("closed DeleteTag should error")
	}
}

// TestUpdateTagClashQueryError UpdateTag 改名查重语句本身的驱动级失败：
// gorm Before 回调对「WHERE 带 <>」（仅查重语句命中）注入错误，
// 模拟 GetTag 成功后、查重阶段 DB 异常的防御分支 → 错误透传
func TestUpdateTagClashQueryError(t *testing.T) {
	d := testDB(t)
	defer d.Close()

	tag, _ := d.CreateTag("prod", "")
	if err := d.db.Callback().Query().Before("gorm:query").Register("test:clasherr", func(db *gorm.DB) {
		if c, ok := db.Statement.Clauses["WHERE"]; ok {
			if w, ok := c.Expression.(clause.Where); ok &&
				strings.Contains(fmt.Sprintf("%v", w.Exprs), "<>") {
				db.AddError(errors.New("clash query boom"))
			}
		}
	}); err != nil {
		t.Fatalf("register cb: %v", err)
	}
	err := d.UpdateTag(tag.ID, "renamed", "")
	if err == nil || !strings.Contains(err.Error(), "clash query boom") {
		t.Errorf("UpdateTag = %v, want injected error", err)
	}
}

// decodeHexInto 测试内把标签 ID（hex）解回字节（randRead 注入用）
func decodeHexInto(s string, out []byte) (int, error) {
	if len(s) != len(out)*2 {
		return 0, errors.New("bad id length")
	}
	for i := 0; i < len(out); i++ {
		var v byte
		for j := 0; j < 2; j++ {
			c := s[i*2+j]
			var nibble byte
			switch {
			case c >= '0' && c <= '9':
				nibble = c - '0'
			case c >= 'a' && c <= 'f':
				nibble = c - 'a' + 10
			default:
				return 0, errors.New("bad hex")
			}
			v = v<<4 | nibble
		}
		out[i] = v
	}
	return len(out), nil
}
