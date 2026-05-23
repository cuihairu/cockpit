package docker

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
)

// Client Docker client wrapper
type Client struct {
	cli *client.Client
}

// Config Docker configuration
type Config struct {
	Host    string
	Timeout time.Duration
}

// NewClient creates Docker client
func NewClient(cfg Config) (*Client, error) {
	opts := []client.Opt{}

	if cfg.Host != "" {
		opts = append(opts, client.WithHost(cfg.Host))
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	cli, err := client.NewClientWithOpts(append(opts, client.WithAPIVersionNegotiation())...)
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = cli.Ping(ctx)
	if err != nil {
		return nil, fmt.Errorf("connect to docker daemon: %w", err)
	}

	return &Client{cli: cli}, nil
}

// Close closes client
func (c *Client) Close() error {
	return c.cli.Close()
}

// ============ Containers ============

// ContainerInfo container information
type ContainerInfo struct {
	ID      string
	Name    string
	Image   string
	ImageID string
	State   string
	Status  string
	Labels  map[string]string
	Created int64
}

// ListContainers lists containers
func (c *Client) ListContainers(all bool) ([]ContainerInfo, error) {
	ctx := context.Background()

	containers, err := c.cli.ContainerList(ctx, container.ListOptions{All: all})
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}

	result := make([]ContainerInfo, len(containers))
	for i, cnt := range containers {
		name := ""
		if len(cnt.Names) > 0 {
			name = cnt.Names[0]
		}
		result[i] = ContainerInfo{
			ID:      cnt.ID,
			Name:    name,
			Image:   cnt.Image,
			ImageID: cnt.ImageID,
			State:   cnt.State,
			Status:  cnt.Status,
			Labels:  cnt.Labels,
			Created: cnt.Created,
		}
	}

	return result, nil
}

// GetContainer gets container details
func (c *Client) GetContainer(id string) (*ContainerInfo, error) {
	ctx := context.Background()

	cnt, err := c.cli.ContainerInspect(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("inspect container: %w", err)
	}

	// Parse created time
	var created int64
	if cnt.Created != "" {
		if t, err := time.Parse(time.RFC3339Nano, cnt.Created); err == nil {
			created = t.Unix()
		}
	}

	// Build status string
	status := cnt.State.Status
	if cnt.State.Running {
		status = "running"
	} else if cnt.State.Paused {
		status = "paused"
	} else if cnt.State.Restarting {
		status = "restarting"
	} else if cnt.State.Dead {
		status = "dead"
	} else {
		status = "exited"
	}

	return &ContainerInfo{
		ID:      cnt.ID,
		Name:    cnt.Name,
		Image:   cnt.Config.Image,
		ImageID: cnt.Image,
		State:   cnt.State.Status,
		Status:  status,
		Labels:  cnt.Config.Labels,
		Created: created,
	}, nil
}

// StartContainer starts container
func (c *Client) StartContainer(id string) error {
	ctx := context.Background()
	if err := c.cli.ContainerStart(ctx, id, container.StartOptions{}); err != nil {
		return fmt.Errorf("start container: %w", err)
	}
	return nil
}

// StopContainer stops container
func (c *Client) StopContainer(id string, timeout *int) error {
	ctx := context.Background()
	if err := c.cli.ContainerStop(ctx, id, container.StopOptions{Timeout: timeout}); err != nil {
		return fmt.Errorf("stop container: %w", err)
	}
	return nil
}

// RestartContainer restarts container
func (c *Client) RestartContainer(id string, timeout *int) error {
	ctx := context.Background()
	if err := c.cli.ContainerRestart(ctx, id, container.StopOptions{Timeout: timeout}); err != nil {
		return fmt.Errorf("restart container: %w", err)
	}
	return nil
}

// RemoveContainer removes container
func (c *Client) RemoveContainer(id string, force, removeVolumes bool) error {
	ctx := context.Background()
	if err := c.cli.ContainerRemove(ctx, id, container.RemoveOptions{
		Force:         force,
		RemoveVolumes: removeVolumes,
	}); err != nil {
		return fmt.Errorf("remove container: %w", err)
	}
	return nil
}

// PauseContainer pauses container
func (c *Client) PauseContainer(id string) error {
	ctx := context.Background()
	if err := c.cli.ContainerPause(ctx, id); err != nil {
		return fmt.Errorf("pause container: %w", err)
	}
	return nil
}

// UnpauseContainer resumes container
func (c *Client) UnpauseContainer(id string) error {
	ctx := context.Background()
	if err := c.cli.ContainerUnpause(ctx, id); err != nil {
		return fmt.Errorf("unpause container: %w", err)
	}
	return nil
}

// GetLogs gets container logs
func (c *Client) GetLogs(id string, tail, since string, follow, timestamps, stdout, stderr bool) (string, error) {
	ctx := context.Background()

	options := container.LogsOptions{
		ShowStdout: stdout,
		ShowStderr: stderr,
		Follow:     follow,
		Timestamps: timestamps,
		Tail:       tail,
		Since:      since,
	}

	reader, err := c.cli.ContainerLogs(ctx, id, options)
	if err != nil {
		return "", fmt.Errorf("get logs: %w", err)
	}
	defer reader.Close()

	// Read logs (max 1MB)
	buf := make([]byte, 1024*1024)
	n, err := reader.Read(buf)
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("read logs: %w", err)
	}

	return string(buf[:n]), nil
}

// GetContainerStats gets container stats
func (c *Client) GetContainerStats(id string) (map[string]interface{}, error) {
	ctx := context.Background()

	stats, err := c.cli.ContainerStats(ctx, id, false)
	if err != nil {
		return nil, fmt.Errorf("get container stats: %w", err)
	}
	defer stats.Body.Close()

	// Return basic stats info
	result := map[string]interface{}{
		"id": id,
	}

	// Read stats body to ensure connection is complete
	buf := make([]byte, 1024)
	stats.Body.Read(buf)

	return result, nil
}

// ============ Images ============

// ImageInfo image information
type ImageInfo struct {
	ID       string
	RepoTags []string
	Size     int64
	Created  int64
}

// ListImages lists images
func (c *Client) ListImages(all bool) ([]ImageInfo, error) {
	ctx := context.Background()

	images, err := c.cli.ImageList(ctx, image.ListOptions{All: all})
	if err != nil {
		return nil, fmt.Errorf("list images: %w", err)
	}

	result := make([]ImageInfo, len(images))
	for i, img := range images {
		result[i] = ImageInfo{
			ID:       img.ID,
			RepoTags: img.RepoTags,
			Size:     img.Size,
			Created:  img.Created,
		}
	}

	return result, nil
}

// RemoveImage removes image
func (c *Client) RemoveImage(id string, force, pruneChildren bool) ([]string, error) {
	ctx := context.Background()

	resp, err := c.cli.ImageRemove(ctx, id, image.RemoveOptions{
		Force:         force,
		PruneChildren: pruneChildren,
	})
	if err != nil {
		return nil, fmt.Errorf("remove image: %w", err)
	}

	var deleted []string
	for _, r := range resp {
		if r.Untagged != "" {
			deleted = append(deleted, r.Untagged)
		}
		if r.Deleted != "" {
			deleted = append(deleted, r.Deleted)
		}
	}

	return deleted, nil
}

// PullImage pulls image
func (c *Client) PullImage(ref string) (string, error) {
	ctx := context.Background()

	reader, err := c.cli.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		return "", fmt.Errorf("pull image: %w", err)
	}
	defer reader.Close()

	// Wait for pull to complete
	buf := make([]byte, 1024)
	for {
		_, err := reader.Read(buf)
		if err != nil {
			break
		}
	}

	// Get image info
	inspect, err := c.cli.ImageInspect(ctx, ref)
	if err != nil {
		return "", fmt.Errorf("inspect image: %w", err)
	}

	return inspect.ID, nil
}

// ============ Volumes ============

// VolumeInfo volume information
type VolumeInfo struct {
	Name       string
	Driver     string
	Mountpoint string
	Labels     map[string]string
}

// ListVolumes lists volumes
func (c *Client) ListVolumes() ([]VolumeInfo, error) {
	ctx := context.Background()

	volumes, err := c.cli.VolumeList(ctx, volume.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list volumes: %w", err)
	}

	result := make([]VolumeInfo, len(volumes.Volumes))
	for i, vol := range volumes.Volumes {
		labels := vol.Labels
		if labels == nil {
			labels = make(map[string]string)
		}
		result[i] = VolumeInfo{
			Name:       vol.Name,
			Driver:     vol.Driver,
			Mountpoint: vol.Mountpoint,
			Labels:     labels,
		}
	}

	return result, nil
}

// RemoveVolume removes volume
func (c *Client) RemoveVolume(name string, force bool) error {
	ctx := context.Background()
	if err := c.cli.VolumeRemove(ctx, name, force); err != nil {
		return fmt.Errorf("remove volume: %w", err)
	}
	return nil
}

// ============ Networks ============

// NetworkInfo network information
type NetworkInfo struct {
	ID     string
	Name   string
	Driver string
}

// ListNetworks lists networks
func (c *Client) ListNetworks() ([]NetworkInfo, error) {
	ctx := context.Background()

	networks, err := c.cli.NetworkList(ctx, network.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list networks: %w", err)
	}

	result := make([]NetworkInfo, len(networks))
	for i, net := range networks {
		result[i] = NetworkInfo{
			ID:     net.ID,
			Name:   net.Name,
			Driver: net.Driver,
		}
	}

	return result, nil
}

// ============ System ============

// SystemInfo system information
type SystemInfo struct {
	Containers        int
	ContainersRunning int
	ContainersPaused  int
	ContainersStopped int
	Images            int
	Driver            string
	OperatingSystem   string
	Architecture      string
	CPUs              int
	Memory            int64
	ServerVersion     string
	KernelVersion     string
}

// Info gets system information
func (c *Client) Info() (*SystemInfo, error) {
	ctx := context.Background()

	info, err := c.cli.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("get info: %w", err)
	}

	return &SystemInfo{
		Containers:        info.Containers,
		ContainersRunning: info.ContainersRunning,
		ContainersPaused:  info.ContainersPaused,
		ContainersStopped: info.ContainersStopped,
		Images:            info.Images,
		Driver:            info.Driver,
		OperatingSystem:   info.OperatingSystem,
		Architecture:      info.Architecture,
		CPUs:              info.NCPU,
		Memory:            info.MemTotal,
		ServerVersion:     info.ServerVersion,
		KernelVersion:     info.KernelVersion,
	}, nil
}

// Version gets version information
func (c *Client) Version() (types.Version, error) {
	ctx := context.Background()
	ver, err := c.cli.ServerVersion(ctx)
	if err != nil {
		return types.Version{}, fmt.Errorf("get version: %w", err)
	}
	return ver, nil
}
