package dns

// alidns.go 阿里云云解析 provider：手写 RPC V1 签名客户端直连
// alidns.aliyuncs.com（GET query），M2 D13。SDK（tea 栈）难注入 httptest
// endpoint，手写签名 ~40 行换全链路可测性与三 client 同构。
// Zone.ID 用域名本身（D14）；MX/SRV Priority 独立参数（D15）。

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

const (
	aliDefaultBase = "https://alidns.aliyuncs.com"
	aliAPIVersion  = "2015-01-09"
	aliPageSize    = 50
)

// aliProvider 阿里云实现（RPC V1 签名）
type aliProvider struct {
	accessKey string
	secretKey string
	base      string
	client    *http.Client
}

// NewAliDNS 创建阿里云客户端；任一密钥为空返回 nil（未配置语义）
func NewAliDNS(accessKey, secretKey string) Provider {
	if accessKey == "" || secretKey == "" {
		return nil
	}
	return &aliProvider{
		accessKey: accessKey,
		secretKey: secretKey,
		base:      aliDefaultBase,
		client:    &http.Client{Timeout: 15 * time.Second},
	}
}

// NewAliDNSWithBase 测试用：指定 base URL（httptest server）
func NewAliDNSWithBase(accessKey, secretKey, base string) Provider {
	return &aliProvider{
		accessKey: accessKey,
		secretKey: secretKey,
		base:      base,
		client:    &http.Client{Timeout: 5 * time.Second},
	}
}

// do 构造公共参数、签名并发 GET；业务数据由调用方解。非 200 都归一为
// error（错误消息只含阿里云返回的 Code/Message，绝不拼 AccessKey/secret，D7）
func (p *aliProvider) do(ctx context.Context, action string, params url.Values) ([]byte, error) {
	if params == nil {
		params = url.Values{}
	}
	params.Set("Action", action)
	params.Set("Format", "json")
	params.Set("Version", aliAPIVersion)
	params.Set("AccessKeyId", p.accessKey)
	params.Set("SignatureMethod", "HMAC-SHA1")
	params.Set("SignatureVersion", "1.0")
	params.Set("SignatureNonce", aliNonce())
	params.Set("Timestamp", time.Now().UTC().Format("2006-01-02T15:04:05Z"))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.base+"/?"+aliSignQuery(p.secretKey, params), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("alidns request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read alidns response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		// RPC 错误响应：{"RequestId","Code","Message",...}
		var e struct {
			Code    string `json:"Code"`
			Message string `json:"Message"`
		}
		_ = json.Unmarshal(body, &e)
		if e.Message == "" {
			e.Message = strings.TrimSpace(string(body))
		}
		if e.Code != "" {
			return nil, fmt.Errorf("alidns error %s: %s", e.Code, e.Message)
		}
		return nil, fmt.Errorf("alidns error: %s", e.Message)
	}
	return body, nil
}

// ListZones 拉取域名列表（单页 50，足够个人场景）；Zone.ID 即域名（D14）
func (p *aliProvider) ListZones(ctx context.Context) ([]Zone, error) {
	body, err := p.do(ctx, "DescribeDomains", url.Values{
		"PageSize":   {"50"},
		"PageNumber": {"1"},
	})
	if err != nil {
		return nil, err
	}
	var resp struct {
		Domains struct {
			Domain []struct {
				DomainName string `json:"DomainName"`
			} `json:"Domain"`
		} `json:"Domains"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal domains: %w", err)
	}
	zones := make([]Zone, 0, len(resp.Domains.Domain))
	for _, d := range resp.Domains.Domain {
		zones = append(zones, Zone{ID: d.DomainName, Name: d.DomainName, Status: "active"})
	}
	return zones, nil
}

// ListRecords 分页拉取记录；DescribeDomainRecords 无精确 type 过滤参数，
// recordType 在 client 侧过滤（TotalPage 仍按全集算，可能翻到空页）
func (p *aliProvider) ListRecords(ctx context.Context, zoneID, recordType string, page int) (*RecordsPage, error) {
	if page < 1 {
		page = 1
	}
	body, err := p.do(ctx, "DescribeDomainRecords", url.Values{
		"DomainName": {zoneID},
		"PageNumber": {strconv.Itoa(page)},
		"PageSize":   {strconv.Itoa(aliPageSize)},
	})
	if err != nil {
		return nil, err
	}
	var resp struct {
		Records struct {
			TotalCount int         `json:"TotalCount"`
			Record     []aliRecord `json:"Record"`
		} `json:"Records"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal records: %w", err)
	}
	out := &RecordsPage{Records: make([]Record, 0, len(resp.Records.Record)), Page: page}
	for _, r := range resp.Records.Record {
		if recordType != "" && r.Type != strings.ToUpper(recordType) {
			continue
		}
		out.Records = append(out.Records, r.toRecord(zoneID))
	}
	out.TotalPage = (resp.Records.TotalCount + aliPageSize - 1) / aliPageSize
	if out.TotalPage < 1 {
		out.TotalPage = 1
	}
	return out, nil
}

// aliRecord 阿里云记录条目（RR 为子域语义；Priority 仅 MX/SRV 有值）
type aliRecord struct {
	RecordId string `json:"RecordId"`
	Type     string `json:"Type"`
	RR       string `json:"RR"`
	Value    string `json:"Value"`
	TTL      int64  `json:"TTL"`
	Priority *int64 `json:"Priority"`
}

func (r aliRecord) toRecord(zoneID string) Record {
	rec := Record{
		ID:      r.RecordId,
		Type:    r.Type,
		Name:    dnsFullName(r.RR, zoneID),
		Content: r.Value,
		TTL:     int(r.TTL),
	}
	if (r.Type == "MX" || r.Type == "SRV") && r.Priority != nil && *r.Priority > 0 {
		rec.Content = strconv.FormatInt(*r.Priority, 10) + " " + r.Value
	}
	return rec
}

// aliRecordValue 拆 MX/SRV content：首词优先级进 Priority 参数（阿里云
// SRV value 为 "weight port target" 三段，D15）
func aliRecordValue(params url.Values, in RecordInput) error {
	if in.Type != "MX" && in.Type != "SRV" {
		params.Set("Value", in.Content)
		return nil
	}
	priority, value, err := dnspodMXContent(in.Type, in.Content)
	if err != nil {
		return err
	}
	params.Set("Value", value)
	params.Set("Priority", priority)
	return nil
}

// CreateRecord 创建记录
func (p *aliProvider) CreateRecord(ctx context.Context, zoneID string, input RecordInput) (*Record, error) {
	input, err := normalizeInput(input)
	if err != nil {
		return nil, err
	}
	params := url.Values{
		"DomainName": {zoneID},
		"RR":         {dnsSubDomain(input.Name, zoneID)},
		"Type":       {input.Type},
	}
	if err := aliRecordValue(params, input); err != nil {
		return nil, err
	}
	if input.TTL > 1 {
		params.Set("TTL", strconv.Itoa(input.TTL))
	}
	body, err := p.do(ctx, "AddDomainRecord", params)
	if err != nil {
		return nil, err
	}
	var resp struct {
		RecordId string `json:"RecordId"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal record: %w", err)
	}
	return &Record{ID: resp.RecordId, Type: input.Type, Name: input.Name, Content: input.Content, TTL: input.TTL}, nil
}

// UpdateRecord 更新记录（全量字段）
func (p *aliProvider) UpdateRecord(ctx context.Context, zoneID, recordID string, input RecordInput) (*Record, error) {
	input, err := normalizeInput(input)
	if err != nil {
		return nil, err
	}
	params := url.Values{
		"RecordId": {recordID},
		"RR":       {dnsSubDomain(input.Name, zoneID)},
		"Type":     {input.Type},
	}
	if err := aliRecordValue(params, input); err != nil {
		return nil, err
	}
	if input.TTL > 1 {
		params.Set("TTL", strconv.Itoa(input.TTL))
	}
	if _, err := p.do(ctx, "UpdateDomainRecord", params); err != nil {
		return nil, err
	}
	return &Record{ID: recordID, Type: input.Type, Name: input.Name, Content: input.Content, TTL: input.TTL}, nil
}

// DeleteRecord 删除记录
func (p *aliProvider) DeleteRecord(ctx context.Context, zoneID, recordID string) error {
	_, err := p.do(ctx, "DeleteDomainRecord", url.Values{"RecordId": {recordID}})
	return err
}
