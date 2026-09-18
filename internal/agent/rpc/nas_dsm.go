package rpc

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
)

// ============ NAS M2：DSM（群晖）Provider ============
//
// 经 agent 内网 HTTP 访问 DSM webapi（见 nas-design.md D2/D3b）：登录拿
// sid → load_info（池+卷）→ list_share（共享）→ 登出（best-effort）。
// 只读不写；凭据来自 agent 环境变量 COCKPIT_NAS_TARGETS，绝不进快照/
// 日志/错误消息；单 target 失败降级不拖垮快照。

// NasTarget 一台网络 NAS 的连接配置（COCKPIT_NAS_TARGETS 数组元素）
type NasTarget struct {
	Name        string `json:"name"`
	Type        string `json:"type"` // dsm（truenas/omv 后续）
	Addr        string `json:"addr"` // 必须带协议，如 https://192.168.1.10:5001
	Username    string `json:"username"`
	Password    string `json:"password"`
	InsecureTLS bool   `json:"insecureTls"` // 自签证书常态，默认建议 true
}

// nasTargetsEnv 网络 NAS 凭据来源（D2：不落库、不进 config.yaml）
const nasTargetsEnv = "COCKPIT_NAS_TARGETS"

// parseNasTargets 解析 targets JSON：非法整体忽略（log），单条缺字段丢弃
func parseNasTargets(raw string) []NasTarget {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var targets []NasTarget
	if err := json.Unmarshal([]byte(raw), &targets); err != nil {
		log.Printf("nas targets: invalid JSON in %s, ignored", nasTargetsEnv)
		return nil
	}
	out := make([]NasTarget, 0, len(targets))
	for _, t := range targets {
		if t.Name == "" || t.Addr == "" || t.Username == "" || t.Password == "" {
			log.Printf("nas target: drop incomplete entry (need name/addr/username/password)")
			continue
		}
		if !strings.HasPrefix(t.Addr, "http://") && !strings.HasPrefix(t.Addr, "https://") {
			log.Printf("nas target %s: addr must start with http(s)://, dropped", t.Name)
			continue
		}
		out = append(out, t)
	}
	return out
}

// nasDsmClientFactory HTTP client 工厂，测试注入（自签证书场景跳过校验）
var nasDsmClientFactory = func(insecure bool) *http.Client {
	transport := &http.Transport{}
	if insecure {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // 家用 NAS 自签常态，D2 明示
	}
	return &http.Client{Timeout: nasCmdTimeout, Transport: transport}
}

// nasDsmSession 一次 DSM 会话：登录后顺序取数，退出时登出
type nasDsmSession struct {
	target NasTarget
	client *http.Client
	sid    string
}

// dsmEntry 调一次 entry.cgi 并解出 data 段（success:false 返回错误）
func (s *nasDsmSession) dsmEntry(ctx context.Context, params url.Values) (json.RawMessage, error) {
	base := strings.TrimSuffix(s.target.Addr, "/") + "/webapi/entry.cgi"
	q := params
	q.Set("_sid", s.sid)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	var body struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
		Error   struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&body); err != nil {
		return nil, err
	}
	if !body.Success {
		return nil, fmt.Errorf("dsm error code %d", body.Error.Code)
	}
	return body.Data, nil
}

// dsmLogin 登录取 sid（密码只经 url.Values 编码，不出现在错误里）
func dsmLogin(ctx context.Context, t NasTarget, client *http.Client) (*nasDsmSession, error) {
	s := &nasDsmSession{target: t, client: client}
	q := url.Values{
		"api":     {"SYNO.API.Auth"},
		"version": {"6"},
		"method":  {"login"},
		"account": {t.Username},
		"passwd":  {t.Password},
		"session": {"StorageManager"},
		"format":  {"sid"},
	}
	// sid 未取到前不带 _sid（dsmEntry 会带空值，DSM 忽略空 _sid）
	data, err := s.dsmEntry(ctx, q)
	if err != nil {
		return nil, err
	}
	var d struct {
		SID string `json:"sid"`
	}
	if err := json.Unmarshal(data, &d); err != nil || d.SID == "" {
		return nil, fmt.Errorf("login response missing sid")
	}
	s.sid = d.SID
	return s, nil
}

// dsmLogout 登出（best-effort，D3b）
func (s *nasDsmSession) dsmLogout() {
	q := url.Values{
		"api":     {"SYNO.API.Auth"},
		"version": {"6"},
		"method":  {"logout"},
		"session": {"StorageManager"},
	}
	_, _ = s.dsmEntry(context.Background(), q)
}

// dsmLoadInfo load_info 的消费字段（DSM 版本间字段有出入，取不到的留零值）
type dsmLoadInfo struct {
	Pools []struct {
		ID        string   `json:"id"`
		Status    string   `json:"status"`
		Devices   []string `json:"devices"`
		TotalSize int64    `json:"total_size"`
		UsedSize  int64    `json:"used_size"`
	} `json:"pools"`
	Volumes []struct {
		Volume    string `json:"volume"`
		Status    string `json:"status"`
		FsType    string `json:"fs_type"`
		TotalSize int64  `json:"total_size"`
		UsedSize  int64  `json:"used_size"`
		PoolID    string `json:"pool_id"`
	} `json:"volumes"`
}

// dsmListShare list_share 的消费字段
type dsmListShare struct {
	Shares []struct {
		Name string `json:"name"`
		Path string `json:"path"`
	} `json:"shares"`
}

// dsmPoolState DSM 池状态 → 统一 State（D3b 映射表）
func dsmPoolState(status string) string {
	switch {
	case status == "Normal":
		return "healthy"
	case status == "Degrade" || status == "Degraded":
		return "degraded"
	case strings.HasPrefix(status, "Crash"):
		return "failed"
	case strings.HasPrefix(status, "Resync") || strings.HasPrefix(status, "Migrat"):
		return "resync"
	default:
		return "unknown"
	}
}

// bytesToGB 字节 → GB（十进制 GB，与 df 侧口径一致）
func bytesToGB(b int64) float64 {
	return float64(b) / 1e9
}

// dsmSnapshot 拉一台 DSM 的池/卷/共享（调用方负责 Host 打标与失败降级）
func dsmSnapshot(ctx context.Context, t NasTarget) ([]NasPool, []NasMount, []NasShare, error) {
	client := nasDsmClientFactory(t.InsecureTLS)
	sess, err := dsmLogin(ctx, t, client)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("target %s login failed: %w", t.Name, err)
	}
	defer sess.dsmLogout()

	var info dsmLoadInfo
	infoData, err := sess.dsmEntry(ctx, url.Values{
		"api": {"SYNO.Storage.CGI.Storage"}, "version": {"2"}, "method": {"load_info"},
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("target %s load_info failed: %w", t.Name, err)
	}
	if err := json.Unmarshal(infoData, &info); err != nil {
		return nil, nil, nil, fmt.Errorf("target %s load_info decode failed: %w", t.Name, err)
	}

	var shareList dsmListShare
	shareData, err := sess.dsmEntry(ctx, url.Values{
		"api": {"SYNO.FileStation.List"}, "version": {"2"}, "method": {"list_share"},
	})
	if err != nil {
		// 共享拿不到不致命：池/卷已足够观测
		log.Printf("nas dsm target %s list_share failed: %v", t.Name, err)
	} else if err := json.Unmarshal(shareData, &shareList); err != nil {
		log.Printf("nas dsm target %s list_share decode failed: %v", t.Name, err)
	}

	pools := make([]NasPool, 0, len(info.Pools))
	for _, p := range info.Pools {
		pools = append(pools, NasPool{
			Name:    p.ID,
			Kind:    "dsm",
			State:   dsmPoolState(p.Status),
			TotalGB: bytesToGB(p.TotalSize),
			UsedGB:  bytesToGB(p.UsedSize),
			Devices: p.Devices,
			Detail:  p.Status,
			Host:    t.Name,
		})
	}
	mounts := make([]NasMount, 0, len(info.Volumes))
	for _, v := range info.Volumes {
		mounts = append(mounts, NasMount{
			Device:    v.PoolID,
			MountPath: v.Volume,
			FsType:    v.FsType,
			TotalGB:   bytesToGB(v.TotalSize),
			UsedGB:    bytesToGB(v.UsedSize),
			Host:      t.Name,
		})
	}
	shares := make([]NasShare, 0, len(shareList.Shares))
	for _, sh := range shareList.Shares {
		shares = append(shares, NasShare{
			Protocol: "smb",
			Name:     sh.Name,
			Path:     sh.Path,
			Hosts:    "",
			Host:     t.Name,
		})
	}
	return pools, mounts, shares, nil
}

// snapshotFromTargets 合并全部已实现类型的 target（单 target 失败降级）
// 返回 false 表示没有任何 target 成功产出
func (p *NasProvider) snapshotFromTargets(ctx context.Context, snap *NasSnapshot, sources *[]string) bool {
	anyOK := false
	for _, t := range p.targets {
		if t.Type != "dsm" {
			continue // truenas/omv 未实现，忽略
		}
		pools, mounts, shares, err := dsmSnapshot(ctx, t)
		if err != nil {
			log.Printf("nas scan: %v", err) // 错误消息只含 target 名与错误码，无凭据
			continue
		}
		anyOK = true
		*sources = append(*sources, "dsm")
		snap.Pools = append(snap.Pools, pools...)
		snap.Mounts = append(snap.Mounts, mounts...)
		snap.Shares = append(snap.Shares, shares...)
	}
	return anyOK
}
