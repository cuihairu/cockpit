package rpc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// basicAuth Basic 认证 header 值（凭据只进请求头，不进错误消息）
func basicAuth(username, password string) string {
	return base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
}

// ============ NAS M2：TrueNAS Provider ============
//
// 经 agent 内网 HTTP 访问 TrueNAS REST v2.0（见 nas-design.md D3c）：
// Basic Auth（无会话状态）→ pool（池）+ dataset 顶层（挂载容量）+
// sharing/smb、sharing/nfs（共享）。只读不写；凭据/降级/Host 纪律与
// DSM 同（D2/D3b），消费端零改动。

// flexInt 兼容 number/string 两种 JSON 数字（TrueNAS 部分版本的
// used/available 是字符串数字）
type flexInt int64

func (f *flexInt) UnmarshalJSON(data []byte) error {
	s := strings.TrimSpace(string(data))
	if s == `""` || s == "null" {
		return nil
	}
	if len(s) >= 2 && s[0] == '"' {
		s = strings.Trim(s, `"`)
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return fmt.Errorf("not a number: %s", data)
	}
	*f = flexInt(n)
	return nil
}

// tnPool /api/v2.0/pool 的消费字段
type tnPool struct {
	Name      string  `json:"name"`
	Status    string  `json:"status"` // ONLINE | DEGRADED | FAULTED | OFFLINE | UNAVAIL | REMOVED
	Size      flexInt `json:"size"`
	Allocated flexInt `json:"allocated"`
	Topology  struct {
		Data []struct {
			Type   string `json:"type"`
			Disk   string `json:"disk"`
			Status string `json:"status"`
		} `json:"data"`
	} `json:"topology"`
	Scan *struct {
		Function string `json:"function"` // RESILVER | SCRUB | ""
		State    string `json:"state"`    // SCANNING | FINISHED | CANCELED | ""
	} `json:"scan"`
}

// tnDataset /api/v2.0/dataset 的消费字段
type tnDataset struct {
	Name       string  `json:"name"` // 顶层如 tank，子级 tank/data
	Type       string  `json:"type"` // FILESYSTEM | VOLUME | BLOCKDEV
	Mounted    bool    `json:"mounted"`
	Mountpoint string  `json:"mountpoint"`
	Used       flexInt `json:"used"`
	Available  flexInt `json:"available"`
}

// tnSmbShare /api/v2.0/sharing/smb 的消费字段
type tnSmbShare struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Comment string `json:"comment"`
}

// tnNfsShare /api/v2.0/sharing/nfs 的消费字段（paths/networks/hosts 都是数组）
type tnNfsShare struct {
	Paths    []string `json:"paths"`
	Networks []string `json:"networks"`
	Hosts    []string `json:"hosts"`
	Comment  string   `json:"comment"`
}

// truenasPoolState ZFS 池状态 → 统一 State；scanning = scan.state
// SCANNING（resilver/scrub 进行中）→ resync（D3c）
func truenasPoolState(status string, scanning bool) string {
	if scanning {
		return "resync"
	}
	switch status {
	case "ONLINE":
		return "healthy"
	case "DEGRADED":
		return "degraded"
	case "FAULTED", "OFFLINE", "UNAVAIL", "REMOVED":
		return "failed"
	default:
		return "unknown"
	}
}

// truenasSnapshot 拉一台 TrueNAS 的池/顶层挂载/共享（调用方负责 Host 打标）
func truenasSnapshot(ctx context.Context, t NasTarget) ([]NasPool, []NasMount, []NasShare, error) {
	client := nasHTTPClientFactory(t.InsecureTLS)
	headers := map[string]string{"Authorization": "Basic " + basicAuth(t.Username, t.Password)}
	get := func(path string, out interface{}) error {
		url := strings.TrimSuffix(t.Addr, "/") + "/api/v2.0/" + path
		body, err := nasHTTPGet(ctx, client, url, headers)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("decode %s: %w", path, err)
		}
		return nil
	}

	var pools []tnPool
	if err := get("pool", &pools); err != nil {
		return nil, nil, nil, fmt.Errorf("target %s pool failed: %w", t.Name, err)
	}
	// dataset/共享拿不到不致命：池已足够观测（容量在池级有 size/allocated）
	var datasets []tnDataset
	if err := get("dataset?limit=0", &datasets); err != nil {
		datasets = nil
	}
	var smbShares []tnSmbShare
	if err := get("sharing/smb", &smbShares); err != nil {
		smbShares = nil
	}
	var nfsShares []tnNfsShare
	if err := get("sharing/nfs", &nfsShares); err != nil {
		nfsShares = nil
	}

	outPools := make([]NasPool, 0, len(pools))
	for _, p := range pools {
		var devices []string
		for _, d := range p.Topology.Data {
			if d.Disk != "" {
				devices = append(devices, d.Disk)
			}
		}
		scanning := p.Scan != nil && p.Scan.State == "SCANNING"
		detail := p.Status
		if scanning && p.Scan.Function != "" {
			detail = p.Status + " (" + strings.ToLower(p.Scan.Function) + ")"
		}
		outPools = append(outPools, NasPool{
			Name:    p.Name,
			Kind:    "truenas",
			State:   truenasPoolState(p.Status, scanning),
			TotalGB: bytesToGB(int64(p.Size)),
			UsedGB:  bytesToGB(int64(p.Allocated)),
			Devices: devices,
			Detail:  detail,
			Host:    t.Name,
		})
	}
	// 顶层 FILESYSTEM 数据集 = 每池一个挂载点（子数据集配额不取，防噪声）
	outMounts := make([]NasMount, 0, len(datasets))
	for _, d := range datasets {
		if d.Type != "FILESYSTEM" || !d.Mounted || strings.Contains(d.Name, "/") {
			continue
		}
		outMounts = append(outMounts, NasMount{
			Device:    d.Name,
			MountPath: d.Mountpoint,
			FsType:    "zfs",
			TotalGB:   bytesToGB(int64(d.Used) + int64(d.Available)),
			UsedGB:    bytesToGB(int64(d.Used)),
			Host:      t.Name,
		})
	}
	outShares := make([]NasShare, 0, len(smbShares)+len(nfsShares))
	for _, s := range smbShares {
		outShares = append(outShares, NasShare{
			Protocol: "smb", Name: s.Name, Path: s.Path, Comment: s.Comment, Host: t.Name,
		})
	}
	for _, n := range nfsShares {
		for _, path := range n.Paths {
			hosts := strings.Join(n.Networks, ",")
			if len(n.Hosts) > 0 {
				if hosts != "" {
					hosts += ","
				}
				hosts += strings.Join(n.Hosts, ",")
			}
			outShares = append(outShares, NasShare{
				Protocol: "nfs", Name: strings.TrimPrefix(path, "/mnt/"), Path: path,
				Comment: n.Comment, Hosts: hosts, Host: t.Name,
			})
		}
	}
	return outPools, outMounts, outShares, nil
}
