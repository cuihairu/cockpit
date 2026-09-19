package overlay

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// ============ Tailscale（api.tailscale.com/api/v2）============

// TailscaleClient Tailscale API v2 客户端（D12：tailnet 空 = "-" 默认简写）
type TailscaleClient struct {
	token   string
	tailnet string
	base    string
	http    *http.Client
}

// NewTailscale 创建 Tailscale 客户端（生产端点）；tailnet 空取 "-"
func NewTailscale(token, tailnet string) *TailscaleClient {
	if tailnet == "" {
		tailnet = "-"
	}
	return &TailscaleClient{token: token, tailnet: tailnet, base: "https://api.tailscale.com/api/v2", http: &http.Client{Timeout: cloudTimeout}}
}

// NewTailscaleWithBase 创建指定端点的客户端（httptest 注入用，D20）
func NewTailscaleWithBase(token, tailnet, base string) *TailscaleClient {
	c := NewTailscale(token, tailnet)
	c.base = base
	return c
}

// TailnetName 生效的 tailnet 标识（web 展示用）
func (c *TailscaleClient) TailnetName() string { return c.tailnet }

// TailscaleDevice 设备（白名单构造，D20）
type TailscaleDevice struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Addresses  []string `json:"addresses,omitempty"`
	User       string   `json:"user,omitempty"`
	OS         string   `json:"os,omitempty"`
	Authorized bool     `json:"authorized"`
	Online     bool     `json:"online"`
	KeyExpiry  string   `json:"keyExpiry,omitempty"`
	LastSeen   string   `json:"lastSeen,omitempty"`
}

// Devices 拉取 tailnet 全部设备
func (c *TailscaleClient) Devices(ctx context.Context) ([]TailscaleDevice, error) {
	data, err := doRequest(ctx, c.http, http.MethodGet,
		c.base+"/tailnet/"+c.tailnet+"/devices", c.token, nil)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Devices []struct {
			ID         string   `json:"id"`
			Name       string   `json:"name"`
			Addresses  []string `json:"addresses"`
			User       string   `json:"user"`
			OS         string   `json:"os"`
			Authorized bool     `json:"authorized"`
			Online     bool     `json:"online"`
			KeyExpiry  string   `json:"keyExpiry"`
			LastSeen   string   `json:"lastSeen"`
		} `json:"devices"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("invalid tailscale device list: %w", err)
	}
	out := make([]TailscaleDevice, 0, len(doc.Devices))
	for _, d := range doc.Devices {
		out = append(out, TailscaleDevice{
			ID:         d.ID,
			Name:       d.Name,
			Addresses:  d.Addresses,
			User:       d.User,
			OS:         d.OS,
			Authorized: d.Authorized,
			Online:     d.Online,
			KeyExpiry:  d.KeyExpiry,
			LastSeen:   d.LastSeen,
		})
	}
	return out, nil
}

// AuthorizeDevice 授权设备（POST /device/{id}/authorize，无请求体）
func (c *TailscaleClient) AuthorizeDevice(ctx context.Context, deviceID string) error {
	_, err := doRequest(ctx, c.http, http.MethodPost,
		c.base+"/device/"+deviceID+"/authorize", c.token, nil)
	return err
}

// DeleteDevice 删除设备（设备需重新登录才能回到 tailnet，破坏性高于 ZT 除名）
func (c *TailscaleClient) DeleteDevice(ctx context.Context, deviceID string) error {
	_, err := doRequest(ctx, c.http, http.MethodDelete,
		c.base+"/device/"+deviceID, c.token, nil)
	return err
}
