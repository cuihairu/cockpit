// Package overlay 提供组网云管理面 API 客户端（M2：ZeroTier Central /
// Tailscale）。server 直连外部控制面（与 DNS 管理、ACME 同款出站路径），
// Agent 不参与。见 docs/guide/overlay-design.md M2（D11/D20）。
package overlay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// cloudTimeout 云 API 单请求超时（与 dns client 同口径）
const cloudTimeout = 15 * time.Second

// UpstreamError 云端非 2xx 响应。Body 原文不透传给最终用户（含云端诊断，
// 可能回显请求上下文），只取摘要。
type UpstreamError struct {
	StatusCode int
	Body       string
}

func (e *UpstreamError) Error() string {
	summary := e.Body
	if len(summary) > 200 {
		summary = summary[:200]
	}
	if summary == "" {
		return fmt.Sprintf("overlay cloud api status %d", e.StatusCode)
	}
	return fmt.Sprintf("overlay cloud api status %d: %s", e.StatusCode, summary)
}

// doRequest 统一请求执行：Bearer 头、状态码检查、响应体读取上限。
// body 为 nil 时发无体请求。
func doRequest(ctx context.Context, client *http.Client, method, url, token string, body []byte) ([]byte, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &UpstreamError{StatusCode: resp.StatusCode, Body: string(data)}
	}
	return data, nil
}

// ============ ZeroTier Central（api.zerotier.com/api/v1）============

// ZeroTierClient ZeroTier Central API v1 客户端（D11：账号级 token 管全网成员）
type ZeroTierClient struct {
	token string
	base  string
	http  *http.Client
}

// NewZeroTier 创建 ZeroTier Central 客户端（生产端点）
func NewZeroTier(token string) *ZeroTierClient {
	return &ZeroTierClient{token: token, base: "https://api.zerotier.com/api/v1", http: &http.Client{Timeout: cloudTimeout}}
}

// NewZeroTierWithBase 创建指定端点的客户端（httptest 注入用，D20）
func NewZeroTierWithBase(token, base string) *ZeroTierClient {
	return &ZeroTierClient{token: token, base: base, http: &http.Client{Timeout: cloudTimeout}}
}

// ZeroTierMember 网络成员（白名单构造，D20）
type ZeroTierMember struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Authorized bool     `json:"authorized"`
	Online     bool     `json:"online"`
	IPs        []string `json:"ips,omitempty"`
	Version    string   `json:"version,omitempty"`
	LastSeen   string   `json:"lastSeen,omitempty"`
}

// ZeroTierNetwork 网络及其成员
type ZeroTierNetwork struct {
	ID      string           `json:"id"`
	Name    string           `json:"name"`
	Members []ZeroTierMember `json:"members"`
}

// Networks 拉取账号下全部网络与成员（Central /network + 逐网 /member）。
// 单网成员拉取失败降级为空成员表并继续其余网络。
func (c *ZeroTierClient) Networks(ctx context.Context) ([]ZeroTierNetwork, error) {
	data, err := doRequest(ctx, c.http, http.MethodGet, c.base+"/network", c.token, nil)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		ID     string `json:"id"`
		Config struct {
			Name string `json:"name"`
		} `json:"config"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid zerotier network list: %w", err)
	}

	networks := make([]ZeroTierNetwork, 0, len(raw))
	for _, n := range raw {
		net := ZeroTierNetwork{ID: n.ID, Name: n.Config.Name}
		if members, err := c.members(ctx, n.ID); err == nil {
			net.Members = members
		}
		networks = append(networks, net)
	}
	return networks, nil
}

// members 拉取单网成员列表并白名单映射
func (c *ZeroTierClient) members(ctx context.Context, networkID string) ([]ZeroTierMember, error) {
	data, err := doRequest(ctx, c.http, http.MethodGet, c.base+"/network/"+networkID+"/member", c.token, nil)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		Authorized bool   `json:"authorized"`
		Online     bool   `json:"online"`
		LastSeen   int64  `json:"lastSeen"`
		Version    string `json:"version"`
		Config     struct {
			IPAssignments []string `json:"ipAssignments"`
		} `json:"config"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid zerotier member list: %w", err)
	}
	out := make([]ZeroTierMember, 0, len(raw))
	for _, m := range raw {
		// Central lastSeen 为毫秒时间戳，归一 RFC3339（与 M1 peers 口径一致）
		lastSeen := ""
		if m.LastSeen > 0 {
			lastSeen = time.UnixMilli(m.LastSeen).UTC().Format(time.RFC3339)
		}
		out = append(out, ZeroTierMember{
			ID:         m.ID,
			Name:       m.Name,
			Authorized: m.Authorized,
			Online:     m.Online,
			IPs:        m.Config.IPAssignments,
			Version:    m.Version,
			LastSeen:   lastSeen,
		})
	}
	return out, nil
}

// SetMemberAuthorized 授权/取消授权成员（POST body {authorized}）
func (c *ZeroTierClient) SetMemberAuthorized(ctx context.Context, networkID, memberID string, authorized bool) error {
	body, _ := json.Marshal(map[string]bool{"authorized": authorized})
	_, err := doRequest(ctx, c.http, http.MethodPost,
		c.base+"/network/"+networkID+"/member/"+memberID, c.token, body)
	return err
}

// DeleteMember 除名成员（设备重新 join 后可再授权，member id = node id 不变）
func (c *ZeroTierClient) DeleteMember(ctx context.Context, networkID, memberID string) error {
	_, err := doRequest(ctx, c.http, http.MethodDelete,
		c.base+"/network/"+networkID+"/member/"+memberID, c.token, nil)
	return err
}
