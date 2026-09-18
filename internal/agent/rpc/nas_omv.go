package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
)

// ============ NAS M2：OMV（OpenMediaVault）Provider ============
//
// 经 agent 内网访问 OMV JSON-RPC（见 nas-design.md D3d）：POST rpc.php，
// body {"service","method","params"}，响应包装 {"response":...,"error":...}；
// 认证走请求头 X-Openmediavault-Sessionid（非 Cookie 非 body）。凭据来自
// COCKPIT_NAS_TARGETS，绝不进快照/日志/错误消息；单 target 失败降级。

// omvEntry 调一次 rpc.php 并解出 response 段（error 非空返回错误，
// message 截断防长 trace 进日志）
func omvEntry(ctx context.Context, client *http.Client, addr, sid, service, method string, params interface{}) (json.RawMessage, error) {
	body, err := json.Marshal(map[string]interface{}{
		"service": service, "method": method, "params": params,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimSuffix(addr, "/")+"/rpc.php", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if sid != "" {
		req.Header.Set("X-Openmediavault-Sessionid", sid)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Response json.RawMessage `json:"response"`
		Error    *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}
	if parsed.Error != nil {
		msg := parsed.Error.Message
		if len(msg) > 100 {
			msg = msg[:100]
		}
		return nil, fmt.Errorf("omv error %d: %s", parsed.Error.Code, msg)
	}
	return parsed.Response, nil
}

// omvLogin 登录取 sessionid（密码只在请求 body，不进错误消息）
func omvLogin(ctx context.Context, t NasTarget, client *http.Client) (string, error) {
	resp, err := omvEntry(ctx, client, t.Addr, "", "session", "login",
		map[string]string{"username": t.Username, "password": t.Password})
	if err != nil {
		return "", err
	}
	var d struct {
		SessionID string `json:"sessionid"`
	}
	if err := json.Unmarshal(resp, &d); err != nil || d.SessionID == "" {
		// 2FA 开启的账号返回 challengeRequired 无 sessionid，等同失败（D3d）
		return "", fmt.Errorf("login response missing sessionid")
	}
	return d.SessionID, nil
}

// omvFilesystem enumerateFilesystems 条目：容量字段是 binary_format
// 字符串（"1.50 GiB"，未挂载 "-1"），见 parseBinarySize
type omvFilesystem struct {
	Devicefile string `json:"devicefile"`
	Type       string `json:"type"`
	Mounted    bool   `json:"mounted"`
	Mountpoint string `json:"mountpoint"`
	Size       string `json:"size"`
	Used       string `json:"used"`
}

// omvShareList getShareList 的 {total, data} 包装；SMB/NFS 条目字段有
// 交集，统一 struct 各取所需（NFS 无 enable 恒 false、SMB 无 client）
type omvShareList struct {
	Data []struct {
		Sharedfoldername string `json:"sharedfoldername"`
		Comment          string `json:"comment"`
		Enable           bool   `json:"enable"`
		Client           string `json:"client"`
		Hostsallow       string `json:"hostsallow"`
	} `json:"data"`
}

// omvBinaryUnits binary_format 单位 → 字节数（IEC 二进制单位）
var omvBinaryUnits = map[string]float64{
	"B":   1,
	"KIB": 1024,
	"MIB": 1024 * 1024,
	"GIB": 1024 * 1024 * 1024,
	"TIB": 1024 * 1024 * 1024 * 1024,
	"PIB": 1024 * 1024 * 1024 * 1024 * 1024,
}

// parseBinarySize 解析 OMV binary_format 输出（"1.50 GiB"）为十进制 GB
// （与 df 侧口径一致）；非法/负值/未知单位 → 0（前端显示 —）
func parseBinarySize(s string) float64 {
	fields := strings.Fields(strings.TrimSpace(s))
	if len(fields) != 2 {
		return 0
	}
	v, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || v < 0 {
		return 0
	}
	unit, ok := omvBinaryUnits[strings.ToUpper(fields[1])]
	if !ok {
		return 0
	}
	return v * unit / 1e9
}

// omvSnapshot 拉一台 OMV 的文件系统/共享（调用方负责 Host 打标与失败降级）。
// Pools 置空：OMV 无统一存储池概念（D3d），底层 mdadm 由 OS 层呈现
func omvSnapshot(ctx context.Context, t NasTarget) ([]NasMount, []NasShare, error) {
	client := nasHTTPClientFactory(t.InsecureTLS)
	sid, err := omvLogin(ctx, t, client)
	if err != nil {
		return nil, nil, fmt.Errorf("target %s login failed: %w", t.Name, err)
	}
	defer func() {
		_, _ = omvEntry(context.Background(), client, t.Addr, sid, "session", "logout", struct{}{})
	}()

	fsData, err := omvEntry(ctx, client, t.Addr, sid, "filesystemmgmt", "enumerateFilesystems", struct{}{})
	if err != nil {
		return nil, nil, fmt.Errorf("target %s enumerateFilesystems failed: %w", t.Name, err)
	}
	var fss []omvFilesystem
	if err := json.Unmarshal(fsData, &fss); err != nil {
		return nil, nil, fmt.Errorf("target %s enumerateFilesystems decode failed: %w", t.Name, err)
	}

	mounts := make([]NasMount, 0, len(fss))
	for _, fs := range fss {
		// mounted + mountpoint 兜底过滤 swap/未挂载条目（enumerateFilesystems 无类型过滤）
		if !fs.Mounted || fs.Mountpoint == "" {
			continue
		}
		mounts = append(mounts, NasMount{
			Device:    fs.Devicefile,
			MountPath: fs.Mountpoint,
			FsType:    fs.Type,
			TotalGB:   parseBinarySize(fs.Size),
			UsedGB:    parseBinarySize(fs.Used),
			Host:      t.Name,
		})
	}

	shares := make([]NasShare, 0)
	for _, spec := range []struct {
		service, method string
		protocol        string
	}{
		{"smb", "getShareList", "smb"},
		{"nfs", "getShareList", "nfs"},
	} {
		shareData, err := omvEntry(ctx, client, t.Addr, sid, spec.service, spec.method,
			map[string]int{"start": 0, "limit": -1})
		if err != nil {
			// 共享拿不到不致命：文件系统已足够观测
			log.Printf("nas omv target %s %s.%s failed: %v", t.Name, spec.service, spec.method, err)
			continue
		}
		var list omvShareList
		if err := json.Unmarshal(shareData, &list); err != nil {
			log.Printf("nas omv target %s %s decode failed: %v", t.Name, spec.service, err)
			continue
		}
		for _, sh := range list.Data {
			if spec.protocol == "smb" && !sh.Enable {
				continue // SMB 仅取启用项；NFS schema 无 enable 字段全量保留
			}
			hosts := sh.Hostsallow
			if spec.protocol == "nfs" {
				hosts = sh.Client
			}
			shares = append(shares, NasShare{
				Protocol: spec.protocol,
				Name:     sh.Sharedfoldername,
				Comment:  sh.Comment,
				Hosts:    hosts,
				Host:     t.Name,
			})
		}
	}
	return mounts, shares, nil
}
