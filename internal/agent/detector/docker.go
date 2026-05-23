package detector

import (
	"net"
	"os"
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
	// 1. 检查环境变量
	socketPath := os.Getenv("DOCKER_HOST")
	if socketPath == "" {
		// 尝试常见 socket 路径
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

	// 2. 测试指定的 socket
	if d.testSocket(socketPath) {
		return &protocol.Capability{
			Type:     "docker-api",
			Endpoint: socketPath,
		}, nil
	}

	return nil, nil
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
