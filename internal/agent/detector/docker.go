package detector

import (
	"net"
	"os"
	"strings"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
)

func init() {
	Register(&DockerDetector{})
}

// DockerDetector Docker API 检测器
type DockerDetector struct{}

// Name 检测器名称
func (d *DockerDetector) Name() string {
	return "docker-api"
}

// Priority 检测优先级
func (d *DockerDetector) Priority() int {
	return 20
}

// Detect 检测 Docker API
func (d *DockerDetector) Detect() (*protocol.Capability, error) {
	// 1. 检查环境变量（兼容标准 unix:// 格式与裸 socket 路径）
	if raw := os.Getenv("DOCKER_HOST"); raw != "" {
		socketPath, endpoint := normalizeUnixHost(raw)
		if d.testSocket(socketPath) {
			return &protocol.Capability{
				Type:     "docker-api",
				Endpoint: endpoint,
			}, nil
		}
		return nil, nil
	}

	// 2. 尝试常见 socket 路径
	candidates := []string{
		"/var/run/docker.sock",
		"/run/docker.sock",
	}
	for _, path := range candidates {
		if d.testSocket(path) {
			return &protocol.Capability{
				Type:     "docker-api",
				Endpoint: "unix://" + path,
			}, nil
		}
	}
	return nil, nil
}

// normalizeUnixHost 把 DOCKER_HOST 值规范化为 (socket 路径, endpoint)：
//
//	unix:///run/docker.sock → ("/run/docker.sock", "unix:///run/docker.sock")
//	/run/docker.sock        → ("/run/docker.sock", "unix:///run/docker.sock")
//
// endpoint 统一带 unix:// 前缀：docker 库的 WithHost 要求 scheme，
// 裸路径直接传入会解析失败。tcp:// 等远程格式暂不在此检测。
func normalizeUnixHost(raw string) (string, string) {
	if strings.HasPrefix(raw, "unix://") {
		path := strings.TrimPrefix(raw, "unix://")
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		return path, "unix://" + path
	}
	return raw, "unix://" + raw
}

// testSocket 测试 Docker socket 是否可连接
func (d *DockerDetector) testSocket(path string) bool {
	// 检查文件是否存在
	if _, err := os.Stat(path); err != nil {
		return false
	}

	// 尝试连接
	conn, err := net.DialTimeout("unix", path, 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()

	return true
}
