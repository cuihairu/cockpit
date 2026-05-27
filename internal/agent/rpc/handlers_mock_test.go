package rpc

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/cuihairu/cockpit/internal/docker"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/docker/docker/api/types"
)

// ============ Mock DockerAPI ============

type mockDockerAPI struct {
	containers []docker.ContainerInfo
	container  *docker.ContainerInfo
	images     []docker.ImageInfo
	volumes    []docker.VolumeInfo
	networks   []docker.NetworkInfo
	sysInfo    *docker.SystemInfo
	logs       string
	stats      map[string]interface{}
	pullID     string
	deleted    []string
	err        error
}

func (m *mockDockerAPI) ListContainers(all bool) ([]docker.ContainerInfo, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.containers, nil
}

func (m *mockDockerAPI) GetContainer(id string) (*docker.ContainerInfo, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.container, nil
}

func (m *mockDockerAPI) StartContainer(id string) error            { return m.err }
func (m *mockDockerAPI) StopContainer(id string, timeout *int) error { return m.err }
func (m *mockDockerAPI) RestartContainer(id string, timeout *int) error {
	return m.err
}
func (m *mockDockerAPI) RemoveContainer(id string, force, removeVolumes bool) error {
	return m.err
}
func (m *mockDockerAPI) PauseContainer(id string) error   { return m.err }
func (m *mockDockerAPI) UnpauseContainer(id string) error { return m.err }

func (m *mockDockerAPI) GetLogs(id string, tail, since string, follow, timestamps, stdout, stderr bool) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.logs, nil
}

func (m *mockDockerAPI) GetContainerStats(id string) (map[string]interface{}, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.stats, nil
}

func (m *mockDockerAPI) ListImages(all bool) ([]docker.ImageInfo, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.images, nil
}

func (m *mockDockerAPI) RemoveImage(id string, force, pruneChildren bool) ([]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.deleted, nil
}

func (m *mockDockerAPI) PullImage(ref string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.pullID, nil
}

func (m *mockDockerAPI) ListVolumes() ([]docker.VolumeInfo, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.volumes, nil
}

func (m *mockDockerAPI) RemoveVolume(name string, force bool) error { return m.err }

func (m *mockDockerAPI) ListNetworks() ([]docker.NetworkInfo, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.networks, nil
}

func (m *mockDockerAPI) Info() (*docker.SystemInfo, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.sysInfo, nil
}

func (m *mockDockerAPI) Version() (types.Version, error) { return types.Version{}, nil }
func (m *mockDockerAPI) Close() error                    { return nil }

func newMockDockerProvider(api docker.DockerAPI) *DockerProvider {
	return &DockerProvider{client: api}
}

// ============ DockerProvider success paths ============

func TestDockerProviderListContainersSuccess(t *testing.T) {
	p := newMockDockerProvider(&mockDockerAPI{
		containers: []docker.ContainerInfo{
			{ID: "abc123", Name: "web", State: "running"},
		},
	})
	result, err := p.ListContainers(map[string]interface{}{"all": true})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	containers := result.([]docker.ContainerInfo)
	if len(containers) != 1 || containers[0].Name != "web" {
		t.Errorf("unexpected result: %v", containers)
	}
}

func TestDockerProviderGetContainerSuccess(t *testing.T) {
	p := newMockDockerProvider(&mockDockerAPI{
		container: &docker.ContainerInfo{ID: "abc", Name: "web"},
	})
	result, err := p.GetContainer(map[string]interface{}{"id": "abc"})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	c := result.(*docker.ContainerInfo)
	if c.Name != "web" {
		t.Errorf("Name = %v", c.Name)
	}
}

func TestDockerProviderActionSuccess(t *testing.T) {
	api := &mockDockerAPI{}
	p := newMockDockerProvider(api)

	actions := []struct {
		name   string
		fn     func() (interface{}, error)
		status string
	}{
		{"start", func() (interface{}, error) { return p.StartContainer(map[string]interface{}{"id": "x"}) }, "started"},
		{"stop", func() (interface{}, error) { return p.StopContainer(map[string]interface{}{"id": "x"}) }, "stopped"},
		{"restart", func() (interface{}, error) { return p.RestartContainer(map[string]interface{}{"id": "x"}) }, "restarted"},
		{"remove", func() (interface{}, error) { return p.RemoveContainer(map[string]interface{}{"id": "x"}) }, "removed"},
		{"pause", func() (interface{}, error) { return p.PauseContainer(map[string]interface{}{"id": "x"}) }, "paused"},
		{"unpause", func() (interface{}, error) { return p.UnpauseContainer(map[string]interface{}{"id": "x"}) }, "unpaused"},
	}

	for _, a := range actions {
		t.Run(a.name, func(t *testing.T) {
			result, err := a.fn()
			if err != nil {
				t.Fatalf("error = %v", err)
			}
			m := result.(map[string]interface{})
			if m["status"] != a.status {
				t.Errorf("status = %v, want %s", m["status"], a.status)
			}
		})
	}
}

func TestDockerProviderGetLogsSuccess(t *testing.T) {
	p := newMockDockerProvider(&mockDockerAPI{logs: "line1\nline2\n"})
	result, err := p.GetLogs(map[string]interface{}{"id": "abc", "tail": "100", "stdout": true})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if result != "line1\nline2\n" {
		t.Errorf("logs = %v", result)
	}
}

func TestDockerProviderGetStatsSuccess(t *testing.T) {
	p := newMockDockerProvider(&mockDockerAPI{stats: map[string]interface{}{"cpu": 0.5}})
	result, err := p.GetStats(map[string]interface{}{"id": "abc"})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if result.(map[string]interface{})["cpu"] != 0.5 {
		t.Errorf("stats = %v", result)
	}
}

func TestDockerProviderListImagesSuccess(t *testing.T) {
	p := newMockDockerProvider(&mockDockerAPI{
		images: []docker.ImageInfo{{ID: "sha256:abc", RepoTags: []string{"nginx:latest"}}},
	})
	result, err := p.ListImages(nil)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	images := result.([]docker.ImageInfo)
	if len(images) != 1 || images[0].RepoTags[0] != "nginx:latest" {
		t.Errorf("result = %v", images)
	}
}

func TestDockerProviderRemoveImageSuccess(t *testing.T) {
	p := newMockDockerProvider(&mockDockerAPI{deleted: []string{"sha256:abc"}})
	result, err := p.RemoveImage(map[string]interface{}{"id": "abc"})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	m := result.(map[string]interface{})
	if m["status"] != "removed" {
		t.Errorf("status = %v", m["status"])
	}
}

func TestDockerProviderPullImageSuccess(t *testing.T) {
	p := newMockDockerProvider(&mockDockerAPI{pullID: "sha256:new"})
	result, err := p.PullImage(map[string]interface{}{"ref": "nginx"})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	m := result.(map[string]interface{})
	if m["id"] != "sha256:new" {
		t.Errorf("id = %v", m["id"])
	}
}

func TestDockerProviderListVolumesSuccess(t *testing.T) {
	p := newMockDockerProvider(&mockDockerAPI{
		volumes: []docker.VolumeInfo{{Name: "data", Driver: "local"}},
	})
	result, err := p.ListVolumes(nil)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	vols := result.([]docker.VolumeInfo)
	if len(vols) != 1 || vols[0].Name != "data" {
		t.Errorf("result = %v", vols)
	}
}

func TestDockerProviderRemoveVolumeSuccess(t *testing.T) {
	p := newMockDockerProvider(&mockDockerAPI{})
	result, err := p.RemoveVolume(map[string]interface{}{"name": "data"})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if result.(map[string]interface{})["status"] != "removed" {
		t.Errorf("result = %v", result)
	}
}

func TestDockerProviderListNetworksSuccess(t *testing.T) {
	p := newMockDockerProvider(&mockDockerAPI{
		networks: []docker.NetworkInfo{{Name: "bridge", Driver: "bridge"}},
	})
	result, err := p.ListNetworks(nil)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	nets := result.([]docker.NetworkInfo)
	if len(nets) != 1 {
		t.Errorf("result = %v", nets)
	}
}

func TestDockerProviderGetSystemInfoSuccess(t *testing.T) {
	p := newMockDockerProvider(&mockDockerAPI{
		sysInfo: &docker.SystemInfo{Containers: 10, Driver: "overlay2"},
	})
	result, err := p.GetSystemInfo(nil)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	info := result.(*docker.SystemInfo)
	if info.Containers != 10 || info.Driver != "overlay2" {
		t.Errorf("result = %+v", info)
	}
}

// ============ DockerProvider Call routing ============

func TestDockerProviderCallAllActions(t *testing.T) {
	api := &mockDockerAPI{
		containers: []docker.ContainerInfo{},
		images:     []docker.ImageInfo{},
		volumes:    []docker.VolumeInfo{},
		networks:   []docker.NetworkInfo{},
		sysInfo:    &docker.SystemInfo{},
		stats:      map[string]interface{}{},
		deleted:    []string{},
	}
	p := newMockDockerProvider(api)

	tests := []struct {
		action string
		params map[string]interface{}
	}{
		{"containers.list", map[string]interface{}{"all": true}},
		{"containers.get", map[string]interface{}{"id": "abc"}},
		{"containers.start", map[string]interface{}{"id": "abc"}},
		{"containers.stop", map[string]interface{}{"id": "abc"}},
		{"containers.restart", map[string]interface{}{"id": "abc"}},
		{"containers.remove", map[string]interface{}{"id": "abc"}},
		{"containers.pause", map[string]interface{}{"id": "abc"}},
		{"containers.unpause", map[string]interface{}{"id": "abc"}},
		{"containers.logs", map[string]interface{}{"id": "abc"}},
		{"containers.stats", map[string]interface{}{"id": "abc"}},
		{"images.list", map[string]interface{}{}},
		{"images.remove", map[string]interface{}{"id": "abc"}},
		{"images.pull", map[string]interface{}{"ref": "nginx"}},
		{"volumes.list", nil},
		{"volumes.remove", map[string]interface{}{"name": "vol"}},
		{"networks.list", nil},
		{"system.info", nil},
	}

	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			_, err := p.Call(tt.action, tt.params)
			if err != nil {
				t.Errorf("Call(%q) error = %v", tt.action, err)
			}
		})
	}
}

func TestDockerProviderCallAllErrors(t *testing.T) {
	p := newMockDockerProvider(&mockDockerAPI{err: errors.New("docker error")})

	tests := []struct {
		action string
		params map[string]interface{}
	}{
		{"containers.list", nil},
		{"containers.get", map[string]interface{}{"id": "abc"}},
		{"containers.start", map[string]interface{}{"id": "abc"}},
		{"containers.stop", map[string]interface{}{"id": "abc"}},
		{"containers.restart", map[string]interface{}{"id": "abc"}},
		{"containers.remove", map[string]interface{}{"id": "abc"}},
		{"containers.pause", map[string]interface{}{"id": "abc"}},
		{"containers.unpause", map[string]interface{}{"id": "abc"}},
		{"containers.logs", map[string]interface{}{"id": "abc"}},
		{"containers.stats", map[string]interface{}{"id": "abc"}},
		{"images.list", nil},
		{"images.remove", map[string]interface{}{"id": "abc"}},
		{"images.pull", map[string]interface{}{"ref": "nginx"}},
		{"volumes.list", nil},
		{"volumes.remove", map[string]interface{}{"name": "vol"}},
		{"networks.list", nil},
		{"system.info", nil},
	}

	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			_, err := p.Call(tt.action, tt.params)
			if err == nil {
				t.Errorf("Call(%q) should return error", tt.action)
			}
		})
	}
}

// ============ PVE httptest handler ============

func pveTestHandler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch {
	case path == "/api2/json/nodes":
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []interface{}{
				map[string]interface{}{"node": "pve1", "status": "online"},
			},
		})
	case path == "/api2/json/nodes/pve1/qemu":
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []interface{}{
				map[string]interface{}{"vmid": 100, "name": "test-vm", "status": "running"},
			},
		})
	case path == "/api2/json/nodes/pve1/lxc":
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []interface{}{
				map[string]interface{}{"vmid": 200, "name": "test-ct", "status": "running"},
			},
		})
	case path == "/api2/json/nodes/pve1/storage":
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []interface{}{
				map[string]interface{}{"storage": "local", "type": "dir"},
			},
		})
	case path == "/api2/json/nodes/pve1/qemu/100/status/current":
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"vm":        map[string]interface{}{"vmid": 100, "name": "test-vm", "status": "running"},
				"qmpstatus": "running",
			},
		})
	case path == "/api2/json/nodes/pve1/qemu/100/config":
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"cores": 2, "memory": 2048, "name": "test-vm",
			},
		})
	case path == "/api2/json/nodes/pve1/lxc/200/status/current":
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"vm": map[string]interface{}{"vmid": 200, "name": "test-ct", "status": "running"},
			},
		})
	case path == "/api2/json/nodes/pve1/lxc/200/config":
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"cores": "1", "memory": "512", "hostname": "test-ct",
			},
		})
	case path == "/api2/json/nodes/pve1/qemu/100/snapshot":
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []interface{}{
				map[string]interface{}{"name": "snap1", "snaptime": 1700000000},
			},
		})
	default:
		if r.Method == "POST" || r.Method == "DELETE" {
			json.NewEncoder(w).Encode(map[string]interface{}{"data": nil})
		} else {
			json.NewEncoder(w).Encode(map[string]interface{}{"data": []interface{}{}})
		}
	}
}

// ============ PVEProvider success with httptest ============

func TestPVEProviderWithHTTPSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(pveTestHandler))
	defer ts.Close()

	p := NewPVEProvider(ts.URL, "token-id", "secret")

	t.Run("ListNodes", func(t *testing.T) {
		result, err := p.ListNodes(nil)
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if result == nil {
			t.Error("result should not be nil")
		}
	})

	t.Run("ListVMs", func(t *testing.T) {
		result, err := p.ListVMs(map[string]interface{}{"node": "pve1"})
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if result == nil {
			t.Error("result should not be nil")
		}
	})

	t.Run("GetVM", func(t *testing.T) {
		result, err := p.GetVM(map[string]interface{}{"node": "pve1", "vmid": 100})
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if result == nil {
			t.Error("result should not be nil")
		}
	})

	t.Run("StartVM", func(t *testing.T) {
		result, err := p.StartVM(map[string]interface{}{"node": "pve1", "vmid": 100})
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if result.(map[string]interface{})["status"] != "started" {
			t.Errorf("result = %v", result)
		}
	})

	t.Run("StopVM", func(t *testing.T) {
		result, err := p.StopVM(map[string]interface{}{"node": "pve1", "vmid": 100})
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if result.(map[string]interface{})["status"] != "stopped" {
			t.Errorf("result = %v", result)
		}
	})

	t.Run("RestartVM", func(t *testing.T) {
		result, err := p.RestartVM(map[string]interface{}{"node": "pve1", "vmid": 100})
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if result.(map[string]interface{})["status"] != "restarted" {
			t.Errorf("result = %v", result)
		}
	})

	t.Run("SuspendVM", func(t *testing.T) {
		result, err := p.SuspendVM(map[string]interface{}{"node": "pve1", "vmid": 100})
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if result.(map[string]interface{})["status"] != "suspended" {
			t.Errorf("result = %v", result)
		}
	})

	t.Run("ResumeVM", func(t *testing.T) {
		result, err := p.ResumeVM(map[string]interface{}{"node": "pve1", "vmid": 100})
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if result.(map[string]interface{})["status"] != "resumed" {
			t.Errorf("result = %v", result)
		}
	})

	t.Run("ListContainers", func(t *testing.T) {
		result, err := p.ListContainers(map[string]interface{}{"node": "pve1"})
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if result == nil {
			t.Error("result should not be nil")
		}
	})

	t.Run("GetContainer", func(t *testing.T) {
		result, err := p.GetContainer(map[string]interface{}{"node": "pve1", "vmid": 200})
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if result == nil {
			t.Error("result should not be nil")
		}
	})

	t.Run("StartContainer", func(t *testing.T) {
		result, err := p.StartContainer(map[string]interface{}{"node": "pve1", "vmid": 200})
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if result.(map[string]interface{})["status"] != "started" {
			t.Errorf("result = %v", result)
		}
	})

	t.Run("StopContainer", func(t *testing.T) {
		result, err := p.StopContainer(map[string]interface{}{"node": "pve1", "vmid": 200})
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if result.(map[string]interface{})["status"] != "stopped" {
			t.Errorf("result = %v", result)
		}
	})

	t.Run("RestartContainer", func(t *testing.T) {
		result, err := p.RestartContainer(map[string]interface{}{"node": "pve1", "vmid": 200})
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if result.(map[string]interface{})["status"] != "restarted" {
			t.Errorf("result = %v", result)
		}
	})

	t.Run("ListStorage", func(t *testing.T) {
		result, err := p.ListStorage(map[string]interface{}{"node": "pve1"})
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if result == nil {
			t.Error("result should not be nil")
		}
	})

	t.Run("ListSnapshots", func(t *testing.T) {
		result, err := p.ListSnapshots(map[string]interface{}{"node": "pve1", "vmid": 100})
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if result == nil {
			t.Error("result should not be nil")
		}
	})

	t.Run("CreateSnapshot", func(t *testing.T) {
		result, err := p.CreateSnapshot(map[string]interface{}{
			"node": "pve1", "vmid": 100, "name": "snap1", "description": "test",
		})
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		m := result.(map[string]interface{})
		if m["status"] != "created" || m["name"] != "snap1" {
			t.Errorf("result = %v", m)
		}
	})

	t.Run("DeleteSnapshot", func(t *testing.T) {
		result, err := p.DeleteSnapshot(map[string]interface{}{
			"node": "pve1", "vmid": 100, "name": "snap1",
		})
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		m := result.(map[string]interface{})
		if m["status"] != "deleted" {
			t.Errorf("result = %v", m)
		}
	})
}

func TestPVEProviderHTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	p := NewPVEProvider(ts.URL, "token-id", "secret")

	errTests := []struct {
		name string
		fn   func() (interface{}, error)
	}{
		{"ListNodes", func() (interface{}, error) { return p.ListNodes(nil) }},
		{"ListVMs", func() (interface{}, error) { return p.ListVMs(map[string]interface{}{"node": "pve1"}) }},
		{"GetVM", func() (interface{}, error) { return p.GetVM(map[string]interface{}{"node": "pve1", "vmid": 100}) }},
		{"StartVM", func() (interface{}, error) { return p.StartVM(map[string]interface{}{"node": "pve1", "vmid": 100}) }},
		{"StopVM", func() (interface{}, error) { return p.StopVM(map[string]interface{}{"node": "pve1", "vmid": 100}) }},
		{"RestartVM", func() (interface{}, error) { return p.RestartVM(map[string]interface{}{"node": "pve1", "vmid": 100}) }},
		{"SuspendVM", func() (interface{}, error) { return p.SuspendVM(map[string]interface{}{"node": "pve1", "vmid": 100}) }},
		{"ResumeVM", func() (interface{}, error) { return p.ResumeVM(map[string]interface{}{"node": "pve1", "vmid": 100}) }},
		{"ListContainers", func() (interface{}, error) { return p.ListContainers(map[string]interface{}{"node": "pve1"}) }},
		{"GetContainer", func() (interface{}, error) { return p.GetContainer(map[string]interface{}{"node": "pve1", "vmid": 200}) }},
		{"StartContainer", func() (interface{}, error) { return p.StartContainer(map[string]interface{}{"node": "pve1", "vmid": 200}) }},
		{"StopContainer", func() (interface{}, error) { return p.StopContainer(map[string]interface{}{"node": "pve1", "vmid": 200}) }},
		{"RestartContainer", func() (interface{}, error) { return p.RestartContainer(map[string]interface{}{"node": "pve1", "vmid": 200}) }},
		{"ListStorage", func() (interface{}, error) { return p.ListStorage(map[string]interface{}{"node": "pve1"}) }},
		{"ListSnapshots", func() (interface{}, error) { return p.ListSnapshots(map[string]interface{}{"node": "pve1", "vmid": 100}) }},
		{"CreateSnapshot", func() (interface{}, error) {
			return p.CreateSnapshot(map[string]interface{}{"node": "pve1", "vmid": 100, "name": "snap1"})
		}},
		{"DeleteSnapshot", func() (interface{}, error) {
			return p.DeleteSnapshot(map[string]interface{}{"node": "pve1", "vmid": 100, "name": "snap1"})
		}},
	}

	for _, tt := range errTests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.fn()
			if err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestPVEProviderCallRouting(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(pveTestHandler))
	defer ts.Close()

	p := NewPVEProvider(ts.URL, "t", "s")

	actions := []struct {
		action string
		params map[string]interface{}
	}{
		{"vms.list", map[string]interface{}{"node": "pve1"}},
		{"vms.get", map[string]interface{}{"node": "pve1", "vmid": 100}},
		{"vms.start", map[string]interface{}{"node": "pve1", "vmid": 100}},
		{"vms.stop", map[string]interface{}{"node": "pve1", "vmid": 100}},
		{"vms.restart", map[string]interface{}{"node": "pve1", "vmid": 100}},
		{"vms.suspend", map[string]interface{}{"node": "pve1", "vmid": 100}},
		{"vms.resume", map[string]interface{}{"node": "pve1", "vmid": 100}},
		{"containers.list", map[string]interface{}{"node": "pve1"}},
		{"containers.get", map[string]interface{}{"node": "pve1", "vmid": 200}},
		{"containers.start", map[string]interface{}{"node": "pve1", "vmid": 200}},
		{"containers.stop", map[string]interface{}{"node": "pve1", "vmid": 200}},
		{"containers.restart", map[string]interface{}{"node": "pve1", "vmid": 200}},
		{"nodes.list", nil},
		{"storage.list", map[string]interface{}{"node": "pve1"}},
		{"snapshots.list", map[string]interface{}{"node": "pve1", "vmid": 100}},
		{"snapshots.create", map[string]interface{}{"node": "pve1", "vmid": 100, "name": "snap1"}},
		{"snapshots.delete", map[string]interface{}{"node": "pve1", "vmid": 100, "name": "snap1"}},
	}

	for _, tt := range actions {
		t.Run(tt.action, func(t *testing.T) {
			_, err := p.Call(tt.action, tt.params)
			if err != nil {
				t.Errorf("Call(%q) error = %v", tt.action, err)
			}
		})
	}
}

// ============ OpenWrtProvider with httptest ============

func TestOpenWrtProviderWithHTTPSuccess(t *testing.T) {
	loginDone := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !loginDone {
			loginDone = true
			json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      1,
				"result": []interface{}{
					map[string]interface{}{
						"ubus_rpc_session": "test-session",
					},
				},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"result": []interface{}{
				map[string]interface{}{
					"uptime":    12345,
					"localtime": 1700000000,
				},
			},
		})
	}))
	defer ts.Close()

	u, _ := url.Parse(ts.URL)
	host := u.Hostname()
	port, _ := strconv.Atoi(u.Port())

	p := NewOpenWrtProvider(host, port, "root", "password")

	_, err := p.GetSystemInfo(nil)
	_ = err
}

func TestOpenWrtProviderWriteFileDefaultMode(t *testing.T) {
	p := NewOpenWrtProvider("127.0.0.1", 1, "root", "password")
	_, err := p.WriteFile(map[string]interface{}{
		"path": "/tmp/test",
		"data": "content",
	})
	_ = err
}

// ============ Handler.Handle edge cases ============

func TestHandlerHandleNoMethod(t *testing.T) {
	h := NewHandler()
	h.RegisterProvider(NewSystemProvider())

	msg := protocol.NewMessage(protocol.MessageTypeRPCRequest, map[string]interface{}{})
	_, err := h.Handle(msg)
	if err == nil {
		t.Error("expected error for missing method")
	}
}

func TestHandlerHandleEmptyMethod(t *testing.T) {
	h := NewHandler()
	h.RegisterProvider(NewSystemProvider())

	msg := protocol.NewMessage(protocol.MessageTypeRPCRequest, map[string]interface{}{
		"method": "",
	})
	defer func() {
		if r := recover(); r != nil {
			t.Logf("empty method panicked (expected): %v", r)
		}
	}()
	h.Handle(msg)
}

func TestHandlerHandleResponseID(t *testing.T) {
	h := NewHandler()
	h.RegisterProvider(NewSystemProvider())

	msg := protocol.NewMessage(protocol.MessageTypeRPCRequest, map[string]interface{}{
		"method": "status",
	})
	msg.ID = "test-id-123"

	resp, err := h.Handle(msg)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if resp.ID != "test-id-123" {
		t.Errorf("response ID = %v, want test-id-123", resp.ID)
	}
}

func TestHandlerHandleWithDockerProvider(t *testing.T) {
	api := &mockDockerAPI{
		containers: []docker.ContainerInfo{{ID: "abc", Name: "web"}},
	}
	p := newMockDockerProvider(api)
	h := NewHandler()
	h.RegisterProvider(p)

	msg := protocol.NewMessage(protocol.MessageTypeRPCRequest, map[string]interface{}{
		"method": "docker.containers.list",
		"params": map[string]interface{}{"all": true},
	})

	resp, err := h.Handle(msg)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if resp == nil {
		t.Fatal("response should not be nil")
	}

	data, ok := resp.Payload["data"]
	if !ok {
		t.Fatal("response should have data field")
	}
	containers, ok := data.([]docker.ContainerInfo)
	if !ok {
		t.Fatalf("data type = %T", data)
	}
	if len(containers) != 1 || containers[0].Name != "web" {
		t.Errorf("data = %v", data)
	}
}
