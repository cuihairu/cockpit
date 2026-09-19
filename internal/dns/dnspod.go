package dns

// dnspod.go DNSPod（腾讯云）provider：手写 form 客户端直连 dnsapi.cn，
// M2 D12。与 cloudflare.go 同构（net/http 手写、httptest 可测）；错误只含
// status.code/message，token 不出现（D7）。Zone.ID 用域名本身（D14）。

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// dnsSubDomain 全名转子域语义（两家 API 共用，D14）：www.example.com→www、
// example.com 或 @→@；大小写不敏感，带尾点剥掉。输入不是 zone 的子域时
// 原样返回，交给 API 报错。
func dnsSubDomain(name, zone string) string {
	n := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), ".")
	z := strings.ToLower(strings.TrimSpace(zone))
	if n == z || n == "" {
		return "@"
	}
	return strings.TrimSuffix(n, "."+z)
}

// dnsFullName 子域语义转全名（@→zone、www→www.zone），List 输出用
func dnsFullName(sub, zone string) string {
	if sub == "@" || sub == "" {
		return zone
	}
	return sub + "." + zone
}

// dnspodMXContent 拆 MX/SRV content：首词优先级（DNSPod 独立 mx 参数，D15）。
// SRV 剩余三段（weight port target）整体作为 value。
func dnspodMXContent(recordType, content string) (priority string, value string, err error) {
	if recordType != "MX" && recordType != "SRV" {
		return "", content, nil
	}
	want := 2
	if recordType == "SRV" {
		want = 4
	}
	parts := strings.SplitN(content, " ", want)
	if len(parts) != want {
		return "", "", fmt.Errorf("%s content must have %d fields, got %q", strings.ToLower(recordType), want, content)
	}
	return parts[0], strings.Join(parts[1:], " "), nil
}

// dnspodJoinMX 读侧拼回 "priority value" 形态
func dnspodJoinMX(recordType string, priority int, value string) string {
	if (recordType == "MX" || recordType == "SRV") && priority > 0 {
		return strconv.Itoa(priority) + " " + value
	}
	return value
}

const (
	dnspodDefaultBase = "https://dnsapi.cn"
	dnspodPageSize    = 50
	dnspodDomainBatch = 400 // Domain.List 单页上限（个人场景不翻页）
)

// New 按 dns.provider 构造 DNS 管理客户端（M2 D11：与 ACME D13 共用同一
// provider 键与凭据）。空 = cloudflare 向后兼容；凭据缺失返回 nil
// （server 侧统一 503 引导语义）；未知 provider 同样返回 nil。
func New(provider, cloudflareToken, dnspodToken, aliAccessKey, aliSecretKey string) Provider {
	switch provider {
	case "", "cloudflare": // 空 = cloudflare（向后兼容）
		return NewCloudflare(cloudflareToken)
	case "dnspod":
		return NewDNSPod(dnspodToken)
	case "alidns":
		return NewAliDNS(aliAccessKey, aliSecretKey)
	default:
		return nil
	}
}

// dnspodProvider DNSPod 实现（login_token 为 "ID,Token" 合并格式）
type dnspodProvider struct {
	loginToken string
	base       string
	client     *http.Client
}

// NewDNSPod 创建 DNSPod 客户端；token 为空返回 nil（未配置语义）
func NewDNSPod(loginToken string) Provider {
	if loginToken == "" {
		return nil
	}
	return &dnspodProvider{
		loginToken: loginToken,
		base:       dnspodDefaultBase,
		client:     &http.Client{Timeout: 15 * time.Second},
	}
}

// NewDNSPodWithBase 测试用：指定 base URL（httptest mock）
func NewDNSPodWithBase(loginToken, base string) Provider {
	return &dnspodProvider{
		loginToken: loginToken,
		base:       base,
		client:     &http.Client{Timeout: 5 * time.Second},
	}
}

// dpEnvelope DNSPod 统一响应壳；list 接口的业务数据用 RawMessage 二次解
type dpEnvelope struct {
	Status struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"status"`
	Domains json.RawMessage `json:"domains"`
	Records json.RawMessage `json:"records"`
	Info    struct {
		RecordTotal int `json:"record_total"`
	} `json:"info"`
	Record json.RawMessage `json:"record"`
}

// dpRecord DNSPod 记录条目（id 为字符串；mx 承载 MX/SRV 优先级）
type dpRecord struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
	TTL   int    `json:"ttl"`
	MX    int    `json:"mx"`
}

func (r dpRecord) toRecord(zoneID string) Record {
	return Record{
		ID:      r.ID,
		Type:    r.Type,
		Name:    dnsFullName(r.Name, zoneID),
		Content: dnspodJoinMX(r.Type, r.MX, r.Value),
		TTL:     r.TTL,
	}
}

// do 发请求并按 status.code 判成功；业务数据由调用方从 envelope 解
func (p *dnspodProvider) do(ctx context.Context, action string, form url.Values) (*dpEnvelope, error) {
	if form == nil {
		form = url.Values{}
	}
	form.Set("login_token", p.loginToken)
	form.Set("format", "json")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.base+"/"+action, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("dnspod request: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read dnspod response: %w", err)
	}
	var env dpEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("decode dnspod response (status %d): %w", resp.StatusCode, err)
	}
	if env.Status.Code != "1" {
		// DNSPod 偶发返回无 status 的响应（网关层），兜底报文
		if env.Status.Code == "" {
			return nil, fmt.Errorf("dnspod error (status %d): %s", resp.StatusCode, strings.TrimSpace(string(raw)))
		}
		return nil, fmt.Errorf("dnspod error %s: %s", env.Status.Code, env.Status.Message)
	}
	return &env, nil
}

// ListZones 拉取域名列表（单页 400，足够个人场景）；Zone.ID 即域名（D14）
func (p *dnspodProvider) ListZones(ctx context.Context) ([]Zone, error) {
	env, err := p.do(ctx, "Domain.List", url.Values{
		"offset": {"0"},
		"length": {strconv.Itoa(dnspodDomainBatch)},
	})
	if err != nil {
		return nil, err
	}
	var domains []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(env.Domains, &domains); err != nil {
		return nil, fmt.Errorf("unmarshal domains: %w", err)
	}
	zones := make([]Zone, 0, len(domains))
	for _, d := range domains {
		zones = append(zones, Zone{ID: d.Name, Name: d.Name, Status: "active"})
	}
	return zones, nil
}

// ListRecords 分页拉取记录（offset/length 分页 + record_type 服务端过滤）
func (p *dnspodProvider) ListRecords(ctx context.Context, zoneID, recordType string, page int) (*RecordsPage, error) {
	if page < 1 {
		page = 1
	}
	form := url.Values{
		"domain": {zoneID},
		"offset": {strconv.Itoa((page - 1) * dnspodPageSize)},
		"length": {strconv.Itoa(dnspodPageSize)},
	}
	if recordType != "" {
		form.Set("record_type", strings.ToUpper(recordType))
	}
	env, err := p.do(ctx, "Record.List", form)
	if err != nil {
		return nil, err
	}
	var records []dpRecord
	if err := json.Unmarshal(env.Records, &records); err != nil {
		return nil, fmt.Errorf("unmarshal records: %w", err)
	}
	out := &RecordsPage{Records: make([]Record, 0, len(records)), Page: page}
	for _, r := range records {
		out.Records = append(out.Records, r.toRecord(zoneID))
	}
	out.TotalPage = (env.Info.RecordTotal + dnspodPageSize - 1) / dnspodPageSize
	if out.TotalPage < 1 {
		out.TotalPage = 1
	}
	return out, nil
}

// dnspodRecordForm Create/Modify 的公共表单：子域 + 类型 + 默认线路 +
// MX/SRV 拆装 + TTL（1=auto 时省略，由 API 取默认，D15）
func dnspodRecordForm(zoneID string, in RecordInput, form url.Values) error {
	form.Set("domain", zoneID)
	form.Set("sub_domain", dnsSubDomain(in.Name, zoneID))
	form.Set("record_type", in.Type)
	form.Set("record_line", "默认") // 智能线路编辑固定默认（M2 不做线路）
	priority, value, err := dnspodMXContent(in.Type, in.Content)
	if err != nil {
		return err
	}
	form.Set("value", value)
	if priority != "" {
		form.Set("mx", priority)
	}
	if in.TTL > 1 {
		form.Set("ttl", strconv.Itoa(in.TTL))
	}
	return nil
}

// CreateRecord 创建记录
func (p *dnspodProvider) CreateRecord(ctx context.Context, zoneID string, input RecordInput) (*Record, error) {
	input, err := normalizeInput(input)
	if err != nil {
		return nil, err
	}
	form := url.Values{}
	if err := dnspodRecordForm(zoneID, input, form); err != nil {
		return nil, err
	}
	env, err := p.do(ctx, "Record.Create", form)
	if err != nil {
		return nil, err
	}
	var r dpRecord
	if err := json.Unmarshal(env.Record, &r); err != nil {
		return nil, fmt.Errorf("unmarshal record: %w", err)
	}
	rec := r.toRecord(zoneID)
	rec.TTL = input.TTL // 省略 ttl 参数时 API 返回默认值，回显请求语义
	return &rec, nil
}

// UpdateRecord 更新记录（全量字段）
func (p *dnspodProvider) UpdateRecord(ctx context.Context, zoneID, recordID string, input RecordInput) (*Record, error) {
	input, err := normalizeInput(input)
	if err != nil {
		return nil, err
	}
	form := url.Values{"record_id": {recordID}}
	if err := dnspodRecordForm(zoneID, input, form); err != nil {
		return nil, err
	}
	if _, err := p.do(ctx, "Record.Modify", form); err != nil {
		return nil, err
	}
	return &Record{ID: recordID, Type: input.Type, Name: input.Name, Content: input.Content, TTL: input.TTL}, nil
}

// DeleteRecord 删除记录
func (p *dnspodProvider) DeleteRecord(ctx context.Context, zoneID, recordID string) error {
	_, err := p.do(ctx, "Record.Remove", url.Values{
		"domain":    {zoneID},
		"record_id": {recordID},
	})
	return err
}
