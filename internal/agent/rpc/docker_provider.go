package rpc

import (
	"fmt"
	"time"

	"github.com/cuihairu/cockpit/internal/docker"
)

// ============ Docker Provider ============

type DockerProvider struct {
	client docker.DockerAPI
}

// NewDockerProvider 创建 Docker Provider
func NewDockerProvider(host string) (*DockerProvider, error) {
	cfg := docker.Config{
		Host:    host,
		Timeout: 30 * time.Second,
	}
	client, err := docker.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}
	return &DockerProvider{client: client}, nil
}

func (p *DockerProvider) Type() string { return "docker" }

func (p *DockerProvider) Call(action string, params map[string]interface{}) (interface{}, error) {
	switch action {
	case "containers.list":
		return p.ListContainers(params)
	case "containers.get":
		return p.GetContainer(params)
	case "containers.start":
		return p.StartContainer(params)
	case "containers.stop":
		return p.StopContainer(params)
	case "containers.restart":
		return p.RestartContainer(params)
	case "containers.remove":
		return p.RemoveContainer(params)
	case "containers.pause":
		return p.PauseContainer(params)
	case "containers.unpause":
		return p.UnpauseContainer(params)
	case "containers.logs":
		return p.GetLogs(params)
	case "containers.stats":
		return p.GetStats(params)
	case "images.list":
		return p.ListImages(params)
	case "images.remove":
		return p.RemoveImage(params)
	case "images.pull":
		return p.PullImage(params)
	case "volumes.list":
		return p.ListVolumes(params)
	case "volumes.remove":
		return p.RemoveVolume(params)
	case "networks.list":
		return p.ListNetworks(params)
	case "system.info":
		return p.GetSystemInfo(params)
	default:
		return nil, fmt.Errorf("unknown docker action: %s", action)
	}
}

func (p *DockerProvider) ListContainers(params map[string]interface{}) (interface{}, error) {
	all, _ := params["all"].(bool)
	containers, err := p.client.ListContainers(all)
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	return containers, nil
}

func (p *DockerProvider) GetContainer(params map[string]interface{}) (interface{}, error) {
	id, ok := params["id"].(string)
	if !ok {
		return nil, fmt.Errorf("id required")
	}
	container, err := p.client.GetContainer(id)
	if err != nil {
		return nil, fmt.Errorf("get container: %w", err)
	}
	return container, nil
}

func (p *DockerProvider) StartContainer(params map[string]interface{}) (interface{}, error) {
	id, ok := params["id"].(string)
	if !ok {
		return nil, fmt.Errorf("id required")
	}
	if err := p.client.StartContainer(id); err != nil {
		return nil, fmt.Errorf("start container: %w", err)
	}
	return map[string]interface{}{"status": "started", "id": id}, nil
}

func (p *DockerProvider) StopContainer(params map[string]interface{}) (interface{}, error) {
	id, ok := params["id"].(string)
	if !ok {
		return nil, fmt.Errorf("id required")
	}
	var timeout *int
	if t, ok := params["timeout"].(int); ok {
		timeout = &t
	}
	if err := p.client.StopContainer(id, timeout); err != nil {
		return nil, fmt.Errorf("stop container: %w", err)
	}
	return map[string]interface{}{"status": "stopped", "id": id}, nil
}

func (p *DockerProvider) RestartContainer(params map[string]interface{}) (interface{}, error) {
	id, ok := params["id"].(string)
	if !ok {
		return nil, fmt.Errorf("id required")
	}
	var timeout *int
	if t, ok := params["timeout"].(int); ok {
		timeout = &t
	}
	if err := p.client.RestartContainer(id, timeout); err != nil {
		return nil, fmt.Errorf("restart container: %w", err)
	}
	return map[string]interface{}{"status": "restarted", "id": id}, nil
}

func (p *DockerProvider) RemoveContainer(params map[string]interface{}) (interface{}, error) {
	id, ok := params["id"].(string)
	if !ok {
		return nil, fmt.Errorf("id required")
	}
	force, _ := params["force"].(bool)
	removeVolumes, _ := params["volumes"].(bool)
	if err := p.client.RemoveContainer(id, force, removeVolumes); err != nil {
		return nil, fmt.Errorf("remove container: %w", err)
	}
	return map[string]interface{}{"status": "removed", "id": id}, nil
}

func (p *DockerProvider) PauseContainer(params map[string]interface{}) (interface{}, error) {
	id, ok := params["id"].(string)
	if !ok {
		return nil, fmt.Errorf("id required")
	}
	if err := p.client.PauseContainer(id); err != nil {
		return nil, fmt.Errorf("pause container: %w", err)
	}
	return map[string]interface{}{"status": "paused", "id": id}, nil
}

func (p *DockerProvider) UnpauseContainer(params map[string]interface{}) (interface{}, error) {
	id, ok := params["id"].(string)
	if !ok {
		return nil, fmt.Errorf("id required")
	}
	if err := p.client.UnpauseContainer(id); err != nil {
		return nil, fmt.Errorf("unpause container: %w", err)
	}
	return map[string]interface{}{"status": "unpaused", "id": id}, nil
}

func (p *DockerProvider) GetLogs(params map[string]interface{}) (interface{}, error) {
	id, ok := params["id"].(string)
	if !ok {
		return nil, fmt.Errorf("id required")
	}
	tail, _ := params["tail"].(string)
	since, _ := params["since"].(string)
	follow, _ := params["follow"].(bool)
	timestamps, _ := params["timestamps"].(bool)
	stdout, _ := params["stdout"].(bool)
	stderr, _ := params["stderr"].(bool)

	logs, err := p.client.GetLogs(id, tail, since, follow, timestamps, stdout, stderr)
	if err != nil {
		return nil, fmt.Errorf("get logs: %w", err)
	}
	return logs, nil
}

func (p *DockerProvider) GetStats(params map[string]interface{}) (interface{}, error) {
	id, ok := params["id"].(string)
	if !ok {
		return nil, fmt.Errorf("id required")
	}
	stats, err := p.client.GetContainerStats(id)
	if err != nil {
		return nil, fmt.Errorf("get stats: %w", err)
	}
	return stats, nil
}

func (p *DockerProvider) ListImages(params map[string]interface{}) (interface{}, error) {
	all, _ := params["all"].(bool)
	images, err := p.client.ListImages(all)
	if err != nil {
		return nil, fmt.Errorf("list images: %w", err)
	}
	return images, nil
}

func (p *DockerProvider) RemoveImage(params map[string]interface{}) (interface{}, error) {
	id, ok := params["id"].(string)
	if !ok {
		return nil, fmt.Errorf("id required")
	}
	force, _ := params["force"].(bool)
	pruneChildren, _ := params["prune_children"].(bool)
	deleted, err := p.client.RemoveImage(id, force, pruneChildren)
	if err != nil {
		return nil, fmt.Errorf("remove image: %w", err)
	}
	return map[string]interface{}{"status": "removed", "deleted": deleted}, nil
}

func (p *DockerProvider) PullImage(params map[string]interface{}) (interface{}, error) {
	ref, ok := params["ref"].(string)
	if !ok {
		return nil, fmt.Errorf("ref required")
	}
	imageID, err := p.client.PullImage(ref)
	if err != nil {
		return nil, fmt.Errorf("pull image: %w", err)
	}
	return map[string]interface{}{"status": "pulled", "id": imageID}, nil
}

func (p *DockerProvider) ListVolumes(params map[string]interface{}) (interface{}, error) {
	volumes, err := p.client.ListVolumes()
	if err != nil {
		return nil, fmt.Errorf("list volumes: %w", err)
	}
	return volumes, nil
}

func (p *DockerProvider) RemoveVolume(params map[string]interface{}) (interface{}, error) {
	name, ok := params["name"].(string)
	if !ok {
		return nil, fmt.Errorf("name required")
	}
	force, _ := params["force"].(bool)
	if err := p.client.RemoveVolume(name, force); err != nil {
		return nil, fmt.Errorf("remove volume: %w", err)
	}
	return map[string]interface{}{"status": "removed", "name": name}, nil
}

func (p *DockerProvider) ListNetworks(params map[string]interface{}) (interface{}, error) {
	networks, err := p.client.ListNetworks()
	if err != nil {
		return nil, fmt.Errorf("list networks: %w", err)
	}
	return networks, nil
}

func (p *DockerProvider) GetSystemInfo(params map[string]interface{}) (interface{}, error) {
	info, err := p.client.Info()
	if err != nil {
		return nil, fmt.Errorf("get system info: %w", err)
	}
	return info, nil
}
