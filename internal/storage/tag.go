package storage

import (
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

// 服务器标签（2026-10-02）：服务端持久化、跨设备一致的分类维度。
//
// 与 Agent.Labels（agent 自报的 key-value，如 env=prod）的区别：标签是
// 人在控制台打的分类（可命名/改色/删除），与 agent 实现无关；前者随注册
// 覆盖，后者独立于 agent 生命周期（删标签不删服务器）。
//
// 关联用显式中间表 AgentTagAssignment 而非 GORM many2many：读写两侧都
// 要按 tag 反查 agent（列表打标签数、按标签筛机器），显式表可直查。
type AgentTag struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"uniqueIndex;size:64;not null" json:"name"`
	Color     string    `gorm:"size:32" json:"color"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// AgentTagAssignment 标签与服务器的关联行（复合主键 = 一台机器一个标签至多一行）
type AgentTagAssignment struct {
	AgentID string    `gorm:"primaryKey" json:"agentId"`
	TagID   string    `gorm:"primaryKey;index" json:"tagId"`
	// TaggedAt 打标签时刻
	TaggedAt time.Time `json:"taggedAt"`
}

// ErrTagNotFound 标签不存在
var ErrTagNotFound = errors.New("tag not found")

// ErrTagNameTaken 标签名重复（Name 唯一索引）
var ErrTagNameTaken = errors.New("tag name already exists")

// newTagID 生成标签 ID（16 字节 hex）。复用 remote_credential.go 的 randRead 注入点。
func newTagID() (string, error) {
	b := make([]byte, 16)
	if _, err := randRead(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// NormalizeTagName 标签名归一：去首尾空白并压到 64 字符（与列宽一致）。
func NormalizeTagName(name string) string {
	name = strings.TrimSpace(name)
	if len(name) > 64 {
		name = strings.TrimSpace(name[:64])
	}
	return name
}

// CreateTag 新建标签。名称重复返回 ErrTagNameTaken（供 handler 转 409）。
func (d *DB) CreateTag(name, color string) (*AgentTag, error) {
	name = NormalizeTagName(name)
	if name == "" {
		return nil, errors.New("tag name is required")
	}

	// 先查重给出可辨识错误；并发下仍可能撞唯一索引，调用方按错误文本兜底
	var existing AgentTag
	if err := d.db.First(&existing, "name = ?", name).Error; err == nil {
		return nil, ErrTagNameTaken
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	id, err := newTagID()
	if err != nil {
		return nil, err
	}

	tag := &AgentTag{ID: id, Name: name, Color: strings.TrimSpace(color)}
	if err := d.db.Create(tag).Error; err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, ErrTagNameTaken
		}
		return nil, err
	}
	return tag, nil
}

// UpdateTag 重命名/改色。两者都可选（空串表示不改）。
func (d *DB) UpdateTag(id, name, color string) error {
	tag, err := d.GetTag(id)
	if err != nil {
		return err
	}

	if n := NormalizeTagName(name); n != "" && n != tag.Name {
		var clash AgentTag
		if err := d.db.First(&clash, "name = ? AND id <> ?", n, id).Error; err == nil {
			return ErrTagNameTaken
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		tag.Name = n
	}
	if c := strings.TrimSpace(color); c != "" {
		tag.Color = c
	}

	return d.db.Save(tag).Error
}

// GetTag 取单个标签
func (d *DB) GetTag(id string) (*AgentTag, error) {
	var tag AgentTag
	err := d.db.First(&tag, "id = ?", id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, ErrTagNotFound
	}
	return &tag, err
}

// DeleteTag 删标签及其关联行。服务器本身不受影响（只摘关联）。
func (d *DB) DeleteTag(id string) error {
	if _, err := d.GetTag(id); err != nil {
		return err
	}
	return d.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tag_id = ?", id).Delete(&AgentTagAssignment{}).Error; err != nil {
			return err
		}
		return tx.Delete(&AgentTag{}, "id = ?", id).Error
	})
}

// ListTags 标签列表（按创建时间），附带每标签挂载的服务器数。
func (d *DB) ListTags() ([]*AgentTag, error) {
	var tags []*AgentTag
	if err := d.db.Order("created_at").Find(&tags).Error; err != nil {
		return nil, err
	}
	return tags, nil
}

// TagCounts 标签 → 挂载服务器数
func (d *DB) TagCounts() (map[string]int, error) {
	type row struct {
		TagID string
		N     int
	}
	var rows []row
	if err := d.db.Model(&AgentTagAssignment{}).
		Select("tag_id, count(*) as n").
		Group("tag_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	counts := make(map[string]int, len(rows))
	for _, r := range rows {
		counts[r.TagID] = r.N
	}
	return counts, nil
}

// AgentTagCounts 按 agent 统计标签数（AgentID → 标签数）
func (d *DB) AgentTagCounts(agentIDs []string) (map[string]int, error) {
	if len(agentIDs) == 0 {
		return map[string]int{}, nil
	}
	type row struct {
		AgentID string
		N       int
	}
	var rows []row
	if err := d.db.Model(&AgentTagAssignment{}).
		Select("agent_id, count(*) as n").
		Where("agent_id IN ?", agentIDs).
		Group("agent_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	counts := make(map[string]int, len(rows))
	for _, r := range rows {
		counts[r.AgentID] = r.N
	}
	return counts, nil
}

// AgentTags 取一台服务器的标签
func (d *DB) AgentTags(agentID string) ([]*AgentTag, error) {
	var tags []*AgentTag
	err := d.db.Model(&AgentTag{}).
		Joins("JOIN agent_tag_assignments a ON a.tag_id = agent_tags.id").
		Where("a.agent_id = ?", agentID).
		Order("agent_tags.name").
		Find(&tags).Error
	return tags, err
}

// TagsForAgents 批量取多台服务器的标签（agentID → 标签列表）。
// Agent 列表接口用它避免 N+1 查询。
func (d *DB) TagsForAgents(agentIDs []string) (map[string][]*AgentTag, error) {
	result := make(map[string][]*AgentTag, len(agentIDs))
	if len(agentIDs) == 0 {
		return result, nil
	}

	type row struct {
		AgentID string
		ID      string
		Name    string
		Color   string
	}
	var rows []row
	err := d.db.Model(&AgentTagAssignment{}).
		Select("agent_tag_assignments.agent_id AS agent_id, agent_tags.id AS id, agent_tags.name AS name, agent_tags.color AS color").
		Joins("JOIN agent_tags ON agent_tags.id = agent_tag_assignments.tag_id").
		Where("agent_tag_assignments.agent_id IN ?", agentIDs).
		Order("agent_tags.name").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		result[r.AgentID] = append(result[r.AgentID], &AgentTag{ID: r.ID, Name: r.Name, Color: r.Color})
	}
	return result, nil
}

// AgentIDsByTag 按标签取服务器 ID（标签筛选的存储侧支撑）
func (d *DB) AgentIDsByTag(tagID string) ([]string, error) {
	var ids []string
	err := d.db.Model(&AgentTagAssignment{}).
		Where("tag_id = ?", tagID).
		Pluck("agent_id", &ids).Error
	return ids, err
}

// SetAgentTags 覆盖式设置一台服务器的标签集合（幂等：重复调用同集合无副作用）。
// 不存在的 tagID 返回 ErrTagNotFound，避免静默写出悬空关联。
func (d *DB) SetAgentTags(agentID string, tagIDs []string) error {
	if _, err := d.GetAgent(agentID); err != nil {
		return err
	}

	seen := make(map[string]bool, len(tagIDs))
	unique := make([]string, 0, len(tagIDs))
	for _, id := range tagIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		unique = append(unique, id)
	}

	if len(unique) > 0 {
		var found int64
		if err := d.db.Model(&AgentTag{}).Where("id IN ?", unique).Count(&found).Error; err != nil {
			return err
		}
		if int(found) != len(unique) {
			return ErrTagNotFound
		}
	}

	now := time.Now()
	return d.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("agent_id = ?", agentID).Delete(&AgentTagAssignment{}).Error; err != nil {
			return err
		}
		if len(unique) == 0 {
			return nil
		}
		rows := make([]AgentTagAssignment, 0, len(unique))
		for _, id := range unique {
			rows = append(rows, AgentTagAssignment{AgentID: agentID, TagID: id, TaggedAt: now})
		}
		return tx.Create(&rows).Error
	})
}

// ClearAgentTags 摘掉某台服务器的全部标签（agent 删除时调用，
// 避免标签行随服务器消失而悬空）
func (d *DB) ClearAgentTags(agentID string) error {
	return d.db.Where("agent_id = ?", agentID).Delete(&AgentTagAssignment{}).Error
}