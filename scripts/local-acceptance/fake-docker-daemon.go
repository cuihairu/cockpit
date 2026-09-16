//go:build acceptance

// fake-docker-daemon 在 unix socket 上模拟 Docker Engine API 的最小子集，
// 供 Cockpit Compose Stack 本地验收使用（scripts/local-acceptance/README.md）。
//
// 仅实现 stack provider / docker provider 需要的端点：
//
//	GET /_ping                 存活探测（返回 API-Version 头）
//	GET /version               版本协商
//	GET /containers/json       容器列表（汇总状态目录里的 *.json）
//
// 容器状态由 fake-docker CLI 在 up/down 时写入状态目录（每个 compose
// project 一个 <project>.json，内容为容器数组），使「compose up 之后
// stack.status 能看到容器」的联动真实走通。
//
// 环境变量：
//
//	FAKE_DOCKER_SOCK         unix socket 路径（必填）
//	FAKE_COMPOSE_STATE_DIR   容器状态目录（必填）
package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const fakeAPIVersion = "1.51"

func main() {
	sock := os.Getenv("FAKE_DOCKER_SOCK")
	stateDir := os.Getenv("FAKE_COMPOSE_STATE_DIR")
	if sock == "" || stateDir == "" {
		fmt.Fprintln(os.Stderr, "fake-docker-daemon: FAKE_DOCKER_SOCK and FAKE_COMPOSE_STATE_DIR are required")
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Dir(sock), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "fake-docker-daemon: mkdir: %v\n", err)
		os.Exit(1)
	}
	if err := os.Remove(sock); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "fake-docker-daemon: remove old socket: %v\n", err)
		os.Exit(1)
	}

	ln, err := net.Listen("unix", sock)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fake-docker-daemon: listen: %v\n", err)
		os.Exit(1)
	}
	// docker 客户端以当前用户连接，socket 需可读写
	if err := os.Chmod(sock, 0o666); err != nil {
		fmt.Fprintf(os.Stderr, "fake-docker-daemon: chmod socket: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("fake-docker-daemon listening on %s (state: %s)\n", sock, stateDir)

	handler := func(w http.ResponseWriter, r *http.Request) {
		p := stripAPIVersion(r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch p {
		case "/_ping":
			w.Header().Set("API-Version", fakeAPIVersion)
			w.Header().Set("Ostype", "linux")
			w.Header().Set("Builder-Version", "2")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		case "/version":
			writeJSON(w, map[string]interface{}{
				"Version":       "27.3.1-fake",
				"ApiVersion":    fakeAPIVersion,
				"MinAPIVersion": "1.24",
				"Os":            "linux",
				"Arch":          "amd64",
				"Components": []map[string]interface{}{
					{"Name": "Engine", "Version": "27.3.1-fake"},
				},
			})
		case "/info":
			writeJSON(w, map[string]interface{}{
				"ID":            "fake-daemon",
				"Name":          "cockpit-acceptance",
				"OSType":        "linux",
				"ServerVersion": "27.3.1-fake",
			})
		case "/containers/json":
			listContainers(w, stateDir, r.URL.Query().Get("all") == "true" || r.URL.Query().Get("all") == "1")
		default:
			http.Error(w, `{"message":"fake-daemon: not implemented: `+p+`"}`, http.StatusNotFound)
		}
	}
	if err := http.Serve(ln, http.HandlerFunc(handler)); err != nil {
		fmt.Fprintf(os.Stderr, "fake-docker-daemon: serve: %v\n", err)
		os.Exit(1)
	}
}

// stripAPIVersion 去掉 moby 客户端版本协商产生的 /v1.xx 前缀
func stripAPIVersion(p string) string {
	if strings.HasPrefix(p, "/v") {
		if i := strings.Index(p[1:], "/"); i >= 0 {
			return p[i+1:]
		}
	}
	return p
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(v)
}

// listContainers 汇总状态目录里的 <project>.json（每个文件是容器数组）。
// all=false 时只返回 State=running 的容器。
func listContainers(w http.ResponseWriter, stateDir string, all bool) {
	files, err := filepath.Glob(filepath.Join(stateDir, "*.json"))
	if err != nil {
		http.Error(w, `{"message":"fake-daemon: list state: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	sort.Strings(files)
	out := make([]map[string]interface{}, 0)
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var items []map[string]interface{}
		if err := json.Unmarshal(raw, &items); err != nil {
			fmt.Fprintf(os.Stderr, "fake-docker-daemon: bad state file %s: %v\n", f, err)
			continue
		}
		for _, it := range items {
			if !all {
				if state, _ := it["State"].(string); state != "running" {
					continue
				}
			}
			out = append(out, it)
		}
	}
	writeJSON(w, out)
}
