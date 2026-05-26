package docker

import (
	"errors"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
)

// mockDockerClient implements DockerAPI for testing
type mockDockerClient struct {
	containers     []ContainerInfo
	containersErr  error
	containerDetail *ContainerInfo
	containerErr   error
	startErr       error
	stopErr        error
	restartErr     error
	removeErr      error
	pauseErr       error
	unpauseErr     error
	logsResult     string
	logsErr        error
	statsResult    map[string]interface{}
	statsErr       error
	images         []ImageInfo
	imagesErr      error
	removeImgResult []string
	removeImgErr   error
	pullResult     string
	pullErr        error
	volumes        []VolumeInfo
	volumesErr     error
	removeVolErr   error
	networks       []NetworkInfo
	networksErr    error
	sysInfo        *SystemInfo
	sysInfoErr     error
	versionResult  types.Version
	versionErr     error
	closeErr       error
}

func (m *mockDockerClient) ListContainers(all bool) ([]ContainerInfo, error) {
	return m.containers, m.containersErr
}

func (m *mockDockerClient) GetContainer(id string) (*ContainerInfo, error) {
	return m.containerDetail, m.containerErr
}

func (m *mockDockerClient) StartContainer(id string) error {
	return m.startErr
}

func (m *mockDockerClient) StopContainer(id string, timeout *int) error {
	return m.stopErr
}

func (m *mockDockerClient) RestartContainer(id string, timeout *int) error {
	return m.restartErr
}

func (m *mockDockerClient) RemoveContainer(id string, force, removeVolumes bool) error {
	return m.removeErr
}

func (m *mockDockerClient) PauseContainer(id string) error {
	return m.pauseErr
}

func (m *mockDockerClient) UnpauseContainer(id string) error {
	return m.unpauseErr
}

func (m *mockDockerClient) GetLogs(id string, tail, since string, follow, timestamps, stdout, stderr bool) (string, error) {
	return m.logsResult, m.logsErr
}

func (m *mockDockerClient) GetContainerStats(id string) (map[string]interface{}, error) {
	return m.statsResult, m.statsErr
}

func (m *mockDockerClient) ListImages(all bool) ([]ImageInfo, error) {
	return m.images, m.imagesErr
}

func (m *mockDockerClient) RemoveImage(id string, force, pruneChildren bool) ([]string, error) {
	return m.removeImgResult, m.removeImgErr
}

func (m *mockDockerClient) PullImage(ref string) (string, error) {
	return m.pullResult, m.pullErr
}

func (m *mockDockerClient) ListVolumes() ([]VolumeInfo, error) {
	return m.volumes, m.volumesErr
}

func (m *mockDockerClient) RemoveVolume(name string, force bool) error {
	return m.removeVolErr
}

func (m *mockDockerClient) ListNetworks() ([]NetworkInfo, error) {
	return m.networks, m.networksErr
}

func (m *mockDockerClient) Info() (*SystemInfo, error) {
	return m.sysInfo, m.sysInfoErr
}

func (m *mockDockerClient) Version() (types.Version, error) {
	return m.versionResult, m.versionErr
}

func (m *mockDockerClient) Close() error {
	return m.closeErr
}

// Verify mock implements interface
var _ DockerAPI = (*mockDockerClient)(nil)

// ============ Config Tests ============

func TestConfigDefaults(t *testing.T) {
	cfg := Config{}
	if cfg.Host != "" {
		t.Error("Host should be empty by default")
	}
	if cfg.Timeout != 0 {
		t.Error("Timeout should be 0 by default")
	}
}

func TestConfigWithValues(t *testing.T) {
	cfg := Config{
		Host:    "unix:///var/run/docker.sock",
		Timeout: 30 * time.Second,
	}
	if cfg.Host != "unix:///var/run/docker.sock" {
		t.Errorf("Host = %v, want unix:///var/run/docker.sock", cfg.Host)
	}
	if cfg.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", cfg.Timeout)
	}
}

func TestNewClientNoDaemon(t *testing.T) {
	cfg := Config{
		Host:    "tcp://127.0.0.1:9999",
		Timeout: 1 * time.Second,
	}
	_, err := NewClient(cfg)
	if err == nil {
		t.Error("NewClient() should return error when Docker daemon is not available")
	}
}

// ============ ContainerInfo Tests ============

func TestContainerInfoStruct(t *testing.T) {
	info := ContainerInfo{
		ID:      "container-123",
		Name:    "test-container",
		Image:   "nginx:latest",
		ImageID: "sha256:abc123",
		State:   "running",
		Status:  "Up 2 hours",
		Labels:  map[string]string{"app": "test"},
		Created: 1234567890,
	}
	if info.ID != "container-123" {
		t.Errorf("ID = %v, want container-123", info.ID)
	}
	if info.Name != "test-container" {
		t.Errorf("Name = %v, want test-container", info.Name)
	}
	if info.State != "running" {
		t.Errorf("State = %v, want running", info.State)
	}
	if len(info.Labels) != 1 {
		t.Errorf("Labels length = %d, want 1", len(info.Labels))
	}
}

func TestContainerInfoEmpty(t *testing.T) {
	info := ContainerInfo{}
	if info.ID != "" {
		t.Error("ID should be empty")
	}
	if info.Name != "" {
		t.Error("Name should be empty")
	}
	if len(info.Labels) != 0 {
		t.Error("Labels should be empty")
	}
}

// ============ ImageInfo Tests ============

func TestImageInfoStruct(t *testing.T) {
	info := ImageInfo{
		ID:       "sha256:abc",
		RepoTags: []string{"nginx:latest", "nginx:1.25"},
		Size:     1024 * 1024 * 100,
		Created:  1234567890,
	}
	if info.ID != "sha256:abc" {
		t.Errorf("ID = %v", info.ID)
	}
	if len(info.RepoTags) != 2 {
		t.Errorf("RepoTags length = %d, want 2", len(info.RepoTags))
	}
	if info.Size != 1024*1024*100 {
		t.Errorf("Size = %v", info.Size)
	}
}

// ============ VolumeInfo Tests ============

func TestVolumeInfoStruct(t *testing.T) {
	info := VolumeInfo{
		Name:       "my-volume",
		Driver:     "local",
		Mountpoint: "/var/lib/docker/volumes/my-volume",
		Labels:     map[string]string{"env": "prod"},
	}
	if info.Name != "my-volume" {
		t.Errorf("Name = %v", info.Name)
	}
	if info.Driver != "local" {
		t.Errorf("Driver = %v", info.Driver)
	}
	if info.Labels["env"] != "prod" {
		t.Errorf("Labels[env] = %v", info.Labels["env"])
	}
}

// ============ NetworkInfo Tests ============

func TestNetworkInfoStruct(t *testing.T) {
	info := NetworkInfo{
		ID:     "net-123",
		Name:   "bridge",
		Driver: "bridge",
	}
	if info.ID != "net-123" {
		t.Errorf("ID = %v", info.ID)
	}
	if info.Name != "bridge" {
		t.Errorf("Name = %v", info.Name)
	}
}

// ============ SystemInfo Tests ============

func TestSystemInfoStruct(t *testing.T) {
	info := SystemInfo{
		Containers:        10,
		ContainersRunning: 5,
		ContainersPaused:  1,
		ContainersStopped: 4,
		Images:            20,
		Driver:            "overlay2",
		OperatingSystem:   "Ubuntu 22.04",
		Architecture:      "x86_64",
		CPUs:              8,
		Memory:            16 * 1024 * 1024 * 1024,
		ServerVersion:     "24.0.7",
		KernelVersion:     "5.15.0",
	}
	if info.Containers != 10 {
		t.Errorf("Containers = %d, want 10", info.Containers)
	}
	if info.CPUs != 8 {
		t.Errorf("CPUs = %d, want 8", info.CPUs)
	}
	if info.Driver != "overlay2" {
		t.Errorf("Driver = %v", info.Driver)
	}
}

// ============ Mock Client Tests ============

func TestMockListContainers(t *testing.T) {
	mock := &mockDockerClient{
		containers: []ContainerInfo{
			{ID: "abc", Name: "web", State: "running"},
			{ID: "def", Name: "db", State: "exited"},
		},
	}

	containers, err := mock.ListContainers(true)
	if err != nil {
		t.Fatalf("ListContainers() error = %v", err)
	}
	if len(containers) != 2 {
		t.Fatalf("ListContainers() count = %d, want 2", len(containers))
	}
	if containers[0].Name != "web" {
		t.Errorf("containers[0].Name = %v, want web", containers[0].Name)
	}
}

func TestMockListContainersError(t *testing.T) {
	mock := &mockDockerClient{
		containersErr: errors.New("docker daemon not available"),
	}

	_, err := mock.ListContainers(true)
	if err == nil {
		t.Error("ListContainers() should return error")
	}
}

func TestMockGetContainer(t *testing.T) {
	mock := &mockDockerClient{
		containerDetail: &ContainerInfo{
			ID:    "abc123",
			Name:  "test-container",
			State: "running",
		},
	}

	container, err := mock.GetContainer("abc123")
	if err != nil {
		t.Fatalf("GetContainer() error = %v", err)
	}
	if container.ID != "abc123" {
		t.Errorf("ID = %v, want abc123", container.ID)
	}
}

func TestMockGetContainerNotFound(t *testing.T) {
	mock := &mockDockerClient{
		containerErr: errors.New("container not found"),
	}

	_, err := mock.GetContainer("nonexistent")
	if err == nil {
		t.Error("GetContainer() should return error for non-existent container")
	}
}

func TestMockStartContainer(t *testing.T) {
	mock := &mockDockerClient{}

	err := mock.StartContainer("abc123")
	if err != nil {
		t.Errorf("StartContainer() error = %v", err)
	}
}

func TestMockStartContainerError(t *testing.T) {
	mock := &mockDockerClient{
		startErr: errors.New("already running"),
	}

	err := mock.StartContainer("abc123")
	if err == nil {
		t.Error("StartContainer() should return error")
	}
}

func TestMockStopContainer(t *testing.T) {
	mock := &mockDockerClient{}

	timeout := 10
	err := mock.StopContainer("abc123", &timeout)
	if err != nil {
		t.Errorf("StopContainer() error = %v", err)
	}
}

func TestMockRestartContainer(t *testing.T) {
	mock := &mockDockerClient{}

	err := mock.RestartContainer("abc123", nil)
	if err != nil {
		t.Errorf("RestartContainer() error = %v", err)
	}
}

func TestMockRemoveContainer(t *testing.T) {
	mock := &mockDockerClient{}

	err := mock.RemoveContainer("abc123", true, false)
	if err != nil {
		t.Errorf("RemoveContainer() error = %v", err)
	}
}

func TestMockPauseUnpause(t *testing.T) {
	mock := &mockDockerClient{}

	err := mock.PauseContainer("abc123")
	if err != nil {
		t.Errorf("PauseContainer() error = %v", err)
	}

	err = mock.UnpauseContainer("abc123")
	if err != nil {
		t.Errorf("UnpauseContainer() error = %v", err)
	}
}

func TestMockGetLogs(t *testing.T) {
	mock := &mockDockerClient{
		logsResult: "line1\nline2\nline3\n",
	}

	logs, err := mock.GetLogs("abc123", "100", "", false, false, true, true)
	if err != nil {
		t.Fatalf("GetLogs() error = %v", err)
	}
	if logs != "line1\nline2\nline3\n" {
		t.Errorf("Logs = %q, want multi-line output", logs)
	}
}

func TestMockGetContainerStats(t *testing.T) {
	mock := &mockDockerClient{
		statsResult: map[string]interface{}{
			"id":    "abc123",
			"cpu":   25.5,
			"memory": 1024,
		},
	}

	stats, err := mock.GetContainerStats("abc123")
	if err != nil {
		t.Fatalf("GetContainerStats() error = %v", err)
	}
	if stats["id"] != "abc123" {
		t.Errorf("Stats id = %v", stats["id"])
	}
}

func TestMockListImages(t *testing.T) {
	mock := &mockDockerClient{
		images: []ImageInfo{
			{ID: "sha256:abc", RepoTags: []string{"nginx:latest"}, Size: 100000},
			{ID: "sha256:def", RepoTags: []string{"redis:7"}, Size: 50000},
		},
	}

	images, err := mock.ListImages(false)
	if err != nil {
		t.Fatalf("ListImages() error = %v", err)
	}
	if len(images) != 2 {
		t.Fatalf("ListImages() count = %d, want 2", len(images))
	}
	if images[0].RepoTags[0] != "nginx:latest" {
		t.Errorf("images[0].RepoTags[0] = %v", images[0].RepoTags[0])
	}
}

func TestMockRemoveImage(t *testing.T) {
	mock := &mockDockerClient{
		removeImgResult: []string{"sha256:abc", "nginx:latest"},
	}

	deleted, err := mock.RemoveImage("sha256:abc", true, false)
	if err != nil {
		t.Fatalf("RemoveImage() error = %v", err)
	}
	if len(deleted) != 2 {
		t.Fatalf("RemoveImage() count = %d, want 2", len(deleted))
	}
}

func TestMockPullImage(t *testing.T) {
	mock := &mockDockerClient{
		pullResult: "sha256:newimage123",
	}

	id, err := mock.PullImage("nginx:latest")
	if err != nil {
		t.Fatalf("PullImage() error = %v", err)
	}
	if id != "sha256:newimage123" {
		t.Errorf("PullImage() id = %v", id)
	}
}

func TestMockListVolumes(t *testing.T) {
	mock := &mockDockerClient{
		volumes: []VolumeInfo{
			{Name: "vol1", Driver: "local"},
			{Name: "vol2", Driver: "nfs"},
		},
	}

	volumes, err := mock.ListVolumes()
	if err != nil {
		t.Fatalf("ListVolumes() error = %v", err)
	}
	if len(volumes) != 2 {
		t.Fatalf("ListVolumes() count = %d, want 2", len(volumes))
	}
}

func TestMockRemoveVolume(t *testing.T) {
	mock := &mockDockerClient{}

	err := mock.RemoveVolume("vol1", false)
	if err != nil {
		t.Errorf("RemoveVolume() error = %v", err)
	}
}

func TestMockListNetworks(t *testing.T) {
	mock := &mockDockerClient{
		networks: []NetworkInfo{
			{ID: "net1", Name: "bridge", Driver: "bridge"},
			{ID: "net2", Name: "host", Driver: "host"},
		},
	}

	networks, err := mock.ListNetworks()
	if err != nil {
		t.Fatalf("ListNetworks() error = %v", err)
	}
	if len(networks) != 2 {
		t.Fatalf("ListNetworks() count = %d, want 2", len(networks))
	}
}

func TestMockInfo(t *testing.T) {
	mock := &mockDockerClient{
		sysInfo: &SystemInfo{
			Containers:        5,
			ContainersRunning: 3,
			Images:            10,
			Driver:            "overlay2",
			ServerVersion:     "24.0.7",
		},
	}

	info, err := mock.Info()
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if info.Containers != 5 {
		t.Errorf("Containers = %d, want 5", info.Containers)
	}
	if info.Driver != "overlay2" {
		t.Errorf("Driver = %v", info.Driver)
	}
}

func TestMockVersion(t *testing.T) {
	mock := &mockDockerClient{
		versionResult: types.Version{
			Version: "24.0.7",
			APIVersion: "1.43",
		},
	}

	ver, err := mock.Version()
	if err != nil {
		t.Fatalf("Version() error = %v", err)
	}
	if ver.Version != "24.0.7" {
		t.Errorf("Version = %v", ver.Version)
	}
}

func TestMockClose(t *testing.T) {
	mock := &mockDockerClient{}

	err := mock.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestMockCloseError(t *testing.T) {
	mock := &mockDockerClient{
		closeErr: errors.New("connection error"),
	}

	err := mock.Close()
	if err == nil {
		t.Error("Close() should return error")
	}
}

// ============ All Operations Error Path Tests ============

func TestMockAllErrors(t *testing.T) {
	testErr := errors.New("test error")
	mock := &mockDockerClient{
		containersErr:  testErr,
		containerErr:   testErr,
		startErr:       testErr,
		stopErr:        testErr,
		restartErr:     testErr,
		removeErr:      testErr,
		pauseErr:       testErr,
		unpauseErr:     testErr,
		logsErr:        testErr,
		statsErr:       testErr,
		imagesErr:      testErr,
		removeImgErr:   testErr,
		pullErr:        testErr,
		volumesErr:     testErr,
		removeVolErr:   testErr,
		networksErr:    testErr,
		sysInfoErr:     testErr,
		versionErr:     testErr,
	}

	if _, err := mock.ListContainers(true); err == nil {
		t.Error("ListContainers should fail")
	}
	if _, err := mock.GetContainer("x"); err == nil {
		t.Error("GetContainer should fail")
	}
	if err := mock.StartContainer("x"); err == nil {
		t.Error("StartContainer should fail")
	}
	if err := mock.StopContainer("x", nil); err == nil {
		t.Error("StopContainer should fail")
	}
	if err := mock.RestartContainer("x", nil); err == nil {
		t.Error("RestartContainer should fail")
	}
	if err := mock.RemoveContainer("x", false, false); err == nil {
		t.Error("RemoveContainer should fail")
	}
	if err := mock.PauseContainer("x"); err == nil {
		t.Error("PauseContainer should fail")
	}
	if err := mock.UnpauseContainer("x"); err == nil {
		t.Error("UnpauseContainer should fail")
	}
	if _, err := mock.GetLogs("x", "", "", false, false, true, false); err == nil {
		t.Error("GetLogs should fail")
	}
	if _, err := mock.GetContainerStats("x"); err == nil {
		t.Error("GetContainerStats should fail")
	}
	if _, err := mock.ListImages(false); err == nil {
		t.Error("ListImages should fail")
	}
	if _, err := mock.RemoveImage("x", false, false); err == nil {
		t.Error("RemoveImage should fail")
	}
	if _, err := mock.PullImage("x"); err == nil {
		t.Error("PullImage should fail")
	}
	if _, err := mock.ListVolumes(); err == nil {
		t.Error("ListVolumes should fail")
	}
	if err := mock.RemoveVolume("x", false); err == nil {
		t.Error("RemoveVolume should fail")
	}
	if _, err := mock.ListNetworks(); err == nil {
		t.Error("ListNetworks should fail")
	}
	if _, err := mock.Info(); err == nil {
		t.Error("Info should fail")
	}
	if _, err := mock.Version(); err == nil {
		t.Error("Version should fail")
	}
}

// ============ Empty Results Tests ============

func TestMockEmptyResults(t *testing.T) {
	mock := &mockDockerClient{}

	containers, err := mock.ListContainers(true)
	if err != nil || len(containers) != 0 {
		t.Error("Empty ListContainers should return empty slice")
	}

	images, err := mock.ListImages(false)
	if err != nil || len(images) != 0 {
		t.Error("Empty ListImages should return empty slice")
	}

	volumes, err := mock.ListVolumes()
	if err != nil || len(volumes) != 0 {
		t.Error("Empty ListVolumes should return empty slice")
	}

	networks, err := mock.ListNetworks()
	if err != nil || len(networks) != 0 {
		t.Error("Empty ListNetworks should return empty slice")
	}

	logs, err := mock.GetLogs("x", "100", "", false, false, true, true)
	if err != nil || logs != "" {
		t.Error("Empty GetLogs should return empty string")
	}
}
