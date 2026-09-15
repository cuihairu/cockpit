// Package dns 提供 DNS 服务商 API 客户端（M1：Cloudflare）。
// server 直连外部 API（与 probe 探测、通知渠道同款出站路径），Agent 不参与。
// 见 docs/guide/dns-design.md。
package dns

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Zone DNS 托管区（Cloudflare 侧事实源，不落库）
type Zone struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Status      string   `json:"status"`
	NameServers []string `json:"name_servers"`
}

// Record DNS 记录
type Record struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Proxied bool   `json:"proxied"`
	Locked  bool   `json:"locked"`
}

// RecordInput 创建/更新入参（TTL 空 = 1 = auto）
type RecordInput struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Proxied bool   `json:"proxied"`
}

// RecordsPage 记录分页结果
type RecordsPage struct {
	Records   []Record `json:"records"`
	Page      int      `json:"page"`
	TotalPage int      `json:"total_pages"`
}

// Provider DNS 服务商抽象（后续 DNSPod/阿里云只加实现）
type Provider interface {
	ListZones(ctx context.Context) ([]Zone, error)
	ListRecords(ctx context.Context, zoneID, recordType string, page int) (*RecordsPage, error)
	CreateRecord(ctx context.Context, zoneID string, input RecordInput) (*Record, error)
	UpdateRecord(ctx context.Context, zoneID, recordID string, input RecordInput) (*Record, error)
	DeleteRecord(ctx context.Context, zoneID, recordID string) error
}

// AllowedTypes 记录类型白名单（D8）
var AllowedTypes = map[string]bool{
	"A": true, "AAAA": true, "CNAME": true, "MX": true,
	"TXT": true, "NS": true, "SRV": true, "CAA": true,
}

// ValidateInput 双端同规则的入参校验（server 侧在此，Web 表单同规则）
func ValidateInput(in RecordInput) error {
	in.Type = strings.ToUpper(strings.TrimSpace(in.Type))
	in.Name = strings.TrimSpace(in.Name)
	in.Content = strings.TrimSpace(in.Content)
	if !AllowedTypes[in.Type] {
		return fmt.Errorf("unsupported record type %q", in.Type)
	}
	if in.Name == "" || in.Content == "" {
		return fmt.Errorf("name and content are required")
	}
	// TTL 0/1 = auto（未填按 1 发）；其余必须 >= 60
	if in.TTL < 0 || (in.TTL > 1 && in.TTL < 60) {
		return fmt.Errorf("ttl must be 1 (auto) or >= 60")
	}
	return nil
}

// cloudflareProvider Cloudflare API v4 实现
type cloudflareProvider struct {
	token  string
	base   string
	client *http.Client
}

// NewCloudflare 创建 Cloudflare 客户端；token 为空返回 nil（未配置语义）
func NewCloudflare(token string) Provider {
	if token == "" {
		return nil
	}
	return &cloudflareProvider{
		token:  token,
		base:   "https://api.cloudflare.com/client/v4",
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

// NewCloudflareWithBase 测试用：指定 base URL（httptest mock）
func NewCloudflareWithBase(token, base string) Provider {
	return &cloudflareProvider{
		token:  token,
		base:   base,
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

// cfEnvelope Cloudflare 统一响应壳
type cfEnvelope struct {
	Success bool `json:"success"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
	Result json.RawMessage `json:"result"`
	// result_info（仅列表接口）
	ResultInfo *struct {
		Page       int `json:"page"`
		TotalPages int `json:"total_pages"`
	} `json:"result_info"`
}

// do 发请求并解包 envelope；非 2xx 或 success=false 都归一为 error
// （错误消息只含 Cloudflare 返回的 code/message，绝不拼 token，D7）
func (p *cloudflareProvider) do(ctx context.Context, method, path string, body interface{}) (*cfEnvelope, error) {
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.base+path, reader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cloudflare request: %w", err)
	}
	defer resp.Body.Close()
	var env cfEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, fmt.Errorf("decode cloudflare response (status %d): %w", resp.StatusCode, err)
	}
	if !env.Success {
		if len(env.Errors) > 0 {
			return nil, fmt.Errorf("cloudflare error %d: %s", env.Errors[0].Code, env.Errors[0].Message)
		}
		return nil, fmt.Errorf("cloudflare error (status %d)", resp.StatusCode)
	}
	return &env, nil
}

// ListZones 拉取 zone 列表（per_page=50 单页，足够个人场景）
func (p *cloudflareProvider) ListZones(ctx context.Context) ([]Zone, error) {
	env, err := p.do(ctx, http.MethodGet, "/zones?per_page=50", nil)
	if err != nil {
		return nil, err
	}
	var zones []Zone
	if err := json.Unmarshal(env.Result, &zones); err != nil {
		return nil, fmt.Errorf("unmarshal zones: %w", err)
	}
	return zones, nil
}

// ListRecords 分页拉取记录（recordType 可选过滤）
func (p *cloudflareProvider) ListRecords(ctx context.Context, zoneID, recordType string, page int) (*RecordsPage, error) {
	if page < 1 {
		page = 1
	}
	path := fmt.Sprintf("/zones/%s/dns_records?per_page=50&page=%d", zoneID, page)
	if recordType != "" {
		path += "&type=" + strings.ToUpper(recordType)
	}
	env, err := p.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var records []Record
	if err := json.Unmarshal(env.Result, &records); err != nil {
		return nil, fmt.Errorf("unmarshal records: %w", err)
	}
	out := &RecordsPage{Records: records, Page: page}
	if env.ResultInfo != nil {
		out.TotalPage = env.ResultInfo.TotalPages
	}
	return out, nil
}

// normalizeInput 校验并归一化（type 大写、去首尾空白）；TTL=0 视为 1（auto）
func normalizeInput(in RecordInput) (RecordInput, error) {
	in.Type = strings.ToUpper(strings.TrimSpace(in.Type))
	in.Name = strings.TrimSpace(in.Name)
	in.Content = strings.TrimSpace(in.Content)
	if err := ValidateInput(in); err != nil {
		return in, err
	}
	if in.TTL == 0 {
		in.TTL = 1
	}
	return in, nil
}

// CreateRecord 创建记录
func (p *cloudflareProvider) CreateRecord(ctx context.Context, zoneID string, input RecordInput) (*Record, error) {
	input, err := normalizeInput(input)
	if err != nil {
		return nil, err
	}
	env, err := p.do(ctx, http.MethodPost, "/zones/"+zoneID+"/dns_records", input)
	if err != nil {
		return nil, err
	}
	var rec Record
	if err := json.Unmarshal(env.Result, &rec); err != nil {
		return nil, fmt.Errorf("unmarshal record: %w", err)
	}
	return &rec, nil
}

// UpdateRecord 更新记录（全量字段）
func (p *cloudflareProvider) UpdateRecord(ctx context.Context, zoneID, recordID string, input RecordInput) (*Record, error) {
	input, err := normalizeInput(input)
	if err != nil {
		return nil, err
	}
	env, err := p.do(ctx, http.MethodPut, "/zones/"+zoneID+"/dns_records/"+recordID, input)
	if err != nil {
		return nil, err
	}
	var rec Record
	if err := json.Unmarshal(env.Result, &rec); err != nil {
		return nil, fmt.Errorf("unmarshal record: %w", err)
	}
	return &rec, nil
}

// DeleteRecord 删除记录
func (p *cloudflareProvider) DeleteRecord(ctx context.Context, zoneID, recordID string) error {
	_, err := p.do(ctx, http.MethodDelete, "/zones/"+zoneID+"/dns_records/"+recordID, nil)
	return err
}
