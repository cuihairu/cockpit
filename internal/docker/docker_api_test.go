package docker

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/system"
	"github.com/docker/docker/api/types/volume"
	dockerclient "github.com/docker/docker/client"
)

// newTestClient creates a Client backed by an httptest server
func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	cli, err := dockerclient.NewClientWithOpts(
		dockerclient.WithHost(ts.URL),
		dockerclient.WithVersion("1.43"),
	)
	if err != nil {
		t.Fatalf("create docker client: %v", err)
	}
	t.Cleanup(func() { cli.Close() })

	return &Client{cli: cli}
}

// ============ Container Tests ============

func TestClientListContainers(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/containers/json") {
			json.NewEncoder(w).Encode([]container.Summary{
				{
					ID:      "container-1",
					Names:   []string{"/web"},
					Image:   "nginx:latest",
					ImageID: "sha256:abc",
					State:   "running",
					Status:  "Up 2 hours",
					Labels:  map[string]string{"app": "web"},
					Created: 1234567890,
				},
				{
					ID:    "container-2",
					Names: []string{},
					Image: "alpine:latest",
					State: "exited",
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	containers, err := c.ListContainers(true)
	if err != nil {
		t.Fatalf("ListContainers() error = %v", err)
	}
	if len(containers) != 2 {
		t.Fatalf("count = %d, want 2", len(containers))
	}
	if containers[0].ID != "container-1" {
		t.Errorf("containers[0].ID = %v", containers[0].ID)
	}
	if containers[0].Name != "/web" {
		t.Errorf("containers[0].Name = %v", containers[0].Name)
	}
	if containers[1].Name != "" {
		t.Errorf("containers[1].Name should be empty for no names, got %v", containers[1].Name)
	}
}

func TestClientListContainersError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, "internal error")
	})

	_, err := c.ListContainers(true)
	if err == nil {
		t.Error("ListContainers() should return error on 500")
	}
}

func TestClientGetContainer(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/containers/abc") {
			running := true
			json.NewEncoder(w).Encode(container.InspectResponse{
				ContainerJSONBase: &container.ContainerJSONBase{
					ID:      "abc",
					Name:   "test-container",
					Image:   "sha256:img123",
					Created: "2024-01-15T10:30:00Z",
					State:   &container.State{Status: "running", Running: running},
				},
				Config: &container.Config{
					Image:   "nginx:latest",
					Labels:  map[string]string{"env": "test"},
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	cnt, err := c.GetContainer("abc")
	if err != nil {
		t.Fatalf("GetContainer() error = %v", err)
	}
	if cnt.ID != "abc" {
		t.Errorf("ID = %v", cnt.ID)
	}
	if cnt.Name != "test-container" {
		t.Errorf("Name = %v", cnt.Name)
	}
	if cnt.Status != "running" {
		t.Errorf("Status = %v, want running", cnt.Status)
	}
	if cnt.Image != "nginx:latest" {
		t.Errorf("Image = %v", cnt.Image)
	}
	if cnt.Labels["env"] != "test" {
		t.Errorf("Labels[env] = %v", cnt.Labels["env"])
	}
}

func TestClientGetContainerPaused(t *testing.T) {
	paused := true
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(container.InspectResponse{
			ContainerJSONBase: &container.ContainerJSONBase{
				ID: "abc",
				State: &container.State{
					Status: "paused",
					Paused: paused,
				},
			},
			Config: &container.Config{Image: "test"},
		})
	})

	cnt, err := c.GetContainer("abc")
	if err != nil {
		t.Fatalf("GetContainer() error = %v", err)
	}
	if cnt.Status != "paused" {
		t.Errorf("Status = %v, want paused", cnt.Status)
	}
}

func TestClientGetContainerDead(t *testing.T) {
	dead := true
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(container.InspectResponse{
			ContainerJSONBase: &container.ContainerJSONBase{
				ID: "abc",
				State: &container.State{
					Status: "dead",
					Dead:   dead,
				},
			},
			Config: &container.Config{Image: "test"},
		})
	})

	cnt, err := c.GetContainer("abc")
	if err != nil {
		t.Fatalf("GetContainer() error = %v", err)
	}
	if cnt.Status != "dead" {
		t.Errorf("Status = %v, want dead", cnt.Status)
	}
}

func TestClientGetContainerRestarting(t *testing.T) {
	restarting := true
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(container.InspectResponse{
			ContainerJSONBase: &container.ContainerJSONBase{
				ID: "abc",
				State: &container.State{
					Status:     "restarting",
					Restarting: restarting,
				},
			},
			Config: &container.Config{Image: "test"},
		})
	})

	cnt, err := c.GetContainer("abc")
	if err != nil {
		t.Fatalf("GetContainer() error = %v", err)
	}
	if cnt.Status != "restarting" {
		t.Errorf("Status = %v, want restarting", cnt.Status)
	}
}

func TestClientGetContainerExited(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(container.InspectResponse{
			ContainerJSONBase: &container.ContainerJSONBase{
				ID: "abc",
				State: &container.State{
					Status: "exited",
				},
			},
			Config: &container.Config{Image: "test"},
		})
	})

	cnt, err := c.GetContainer("abc")
	if err != nil {
		t.Fatalf("GetContainer() error = %v", err)
	}
	if cnt.Status != "exited" {
		t.Errorf("Status = %v, want exited", cnt.Status)
	}
}

func TestClientGetContainerNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	_, err := c.GetContainer("nonexistent")
	if err == nil {
		t.Error("GetContainer() should return error for 404")
	}
}

func TestClientStartContainer(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/start") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	if err := c.StartContainer("abc"); err != nil {
		t.Errorf("StartContainer() error = %v", err)
	}
}

func TestClientStartContainerError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	if err := c.StartContainer("abc"); err == nil {
		t.Error("StartContainer() should return error on 500")
	}
}

func TestClientStopContainer(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	timeout := 10
	if err := c.StopContainer("abc", &timeout); err != nil {
		t.Errorf("StopContainer() error = %v", err)
	}
}

func TestClientRestartContainer(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	if err := c.RestartContainer("abc", nil); err != nil {
		t.Errorf("RestartContainer() error = %v", err)
	}
}

func TestClientRemoveContainer(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	if err := c.RemoveContainer("abc", true, false); err != nil {
		t.Errorf("RemoveContainer() error = %v", err)
	}
}

func TestClientPauseContainer(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	if err := c.PauseContainer("abc"); err != nil {
		t.Errorf("PauseContainer() error = %v", err)
	}
}

func TestClientUnpauseContainer(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	if err := c.UnpauseContainer("abc"); err != nil {
		t.Errorf("UnpauseContainer() error = %v", err)
	}
}

func TestClientGetLogs(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		// Docker log stream: 8-byte header + payload per frame
		header := []byte{1, 0, 0, 0, 0, 0, 0, 5}
		w.Write(header)
		w.Write([]byte("hello"))
	})

	logs, err := c.GetLogs("abc", "100", "", false, false, true, false)
	if err != nil {
		t.Fatalf("GetLogs() error = %v", err)
	}
	if logs == "" {
		t.Error("GetLogs() should return log content")
	}
}

func TestClientGetContainerStats(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"cpu_stats":{},"memory_stats":{}}`))
	})

	stats, err := c.GetContainerStats("abc")
	if err != nil {
		t.Fatalf("GetContainerStats() error = %v", err)
	}
	if stats["id"] != "abc" {
		t.Errorf("stats[id] = %v, want abc", stats["id"])
	}
}

// ============ Image Tests ============

func TestClientListImages(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/images/json") {
			json.NewEncoder(w).Encode([]image.Summary{
				{
					ID:       "sha256:img1",
					RepoTags: []string{"nginx:latest"},
					Size:     100000,
					Created:  1234567890,
				},
				{
					ID:       "sha256:img2",
					RepoTags: []string{"redis:7"},
					Size:     50000,
					Created:  1234567800,
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	images, err := c.ListImages(false)
	if err != nil {
		t.Fatalf("ListImages() error = %v", err)
	}
	if len(images) != 2 {
		t.Fatalf("count = %d, want 2", len(images))
	}
	if images[0].RepoTags[0] != "nginx:latest" {
		t.Errorf("images[0].RepoTags[0] = %v", images[0].RepoTags[0])
	}
	if images[1].Size != 50000 {
		t.Errorf("images[1].Size = %d, want 50000", images[1].Size)
	}
}

func TestClientListImagesError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := c.ListImages(false)
	if err == nil {
		t.Error("ListImages() should return error on 500")
	}
}

func TestClientRemoveImage(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			json.NewEncoder(w).Encode([]image.DeleteResponse{
				{Untagged: "nginx:latest"},
				{Deleted: "sha256:abc"},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	deleted, err := c.RemoveImage("sha256:abc", true, false)
	if err != nil {
		t.Fatalf("RemoveImage() error = %v", err)
	}
	if len(deleted) != 2 {
		t.Fatalf("count = %d, want 2", len(deleted))
	}
}

func TestClientRemoveImageError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	_, err := c.RemoveImage("nonexistent", false, false)
	if err == nil {
		t.Error("RemoveImage() should return error on 404")
	}
}

func TestClientPullImage(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("fromImage") != "" || r.Method == "POST" {
			// Image pull stream
			w.Write([]byte(`{"status":"Pull complete"}` + "\n"))
			return
		}
		if strings.Contains(r.URL.Path, "/images/") {
			// Image inspect
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	// PullImage will fail at ImageInspect but that's ok - we test the pull path
	_, err := c.PullImage("nginx:latest")
	// ImageInspect fails, so we expect an error
	if err == nil {
		t.Error("PullImage() should fail when inspect not available")
	}
}

// ============ Volume Tests ============

func TestClientListVolumes(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/volumes") {
			json.NewEncoder(w).Encode(volume.ListResponse{
				Volumes: []*volume.Volume{
					{
						Name:       "vol1",
						Driver:     "local",
						Mountpoint: "/var/lib/docker/volumes/vol1",
						Labels:     map[string]string{"env": "prod"},
					},
					{
						Name:       "vol2",
						Driver:     "nfs",
						Mountpoint: "/mnt/nfs/vol2",
					},
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	volumes, err := c.ListVolumes()
	if err != nil {
		t.Fatalf("ListVolumes() error = %v", err)
	}
	if len(volumes) != 2 {
		t.Fatalf("count = %d, want 2", len(volumes))
	}
	if volumes[0].Name != "vol1" {
		t.Errorf("volumes[0].Name = %v", volumes[0].Name)
	}
	if volumes[1].Driver != "nfs" {
		t.Errorf("volumes[1].Driver = %v", volumes[1].Driver)
	}
	if volumes[0].Labels["env"] != "prod" {
		t.Errorf("volumes[0].Labels[env] = %v", volumes[0].Labels["env"])
	}
}

func TestClientListVolumesNilLabels(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(volume.ListResponse{
			Volumes: []*volume.Volume{
				{
					Name:       "vol-nolabel",
					Driver:     "local",
					Mountpoint: "/var/lib/docker/volumes/vol-nolabel",
					Labels:     nil,
				},
			},
		})
	})

	volumes, err := c.ListVolumes()
	if err != nil {
		t.Fatalf("ListVolumes() error = %v", err)
	}
	if len(volumes) != 1 {
		t.Fatalf("count = %d, want 1", len(volumes))
	}
	if volumes[0].Labels == nil {
		t.Error("Labels should not be nil (converted to empty map)")
	}
}

func TestClientListVolumesError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := c.ListVolumes()
	if err == nil {
		t.Error("ListVolumes() should return error on 500")
	}
}

func TestClientRemoveVolume(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	if err := c.RemoveVolume("vol1", false); err != nil {
		t.Errorf("RemoveVolume() error = %v", err)
	}
}

func TestClientRemoveVolumeError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	if err := c.RemoveVolume("nonexistent", false); err == nil {
		t.Error("RemoveVolume() should return error on 404")
	}
}

// ============ Network Tests ============

func TestClientListNetworks(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/networks") {
			json.NewEncoder(w).Encode([]network.Inspect{
				{ID: "net1", Name: "bridge", Driver: "bridge"},
				{ID: "net2", Name: "host", Driver: "host"},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	networks, err := c.ListNetworks()
	if err != nil {
		t.Fatalf("ListNetworks() error = %v", err)
	}
	if len(networks) != 2 {
		t.Fatalf("count = %d, want 2", len(networks))
	}
	if networks[0].Name != "bridge" {
		t.Errorf("networks[0].Name = %v", networks[0].Name)
	}
	if networks[1].Driver != "host" {
		t.Errorf("networks[1].Driver = %v", networks[1].Driver)
	}
}

func TestClientListNetworksError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := c.ListNetworks()
	if err == nil {
		t.Error("ListNetworks() should return error on 500")
	}
}

// ============ System Tests ============

func TestClientInfo(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/info") {
			json.NewEncoder(w).Encode(system.Info{
				Containers:        10,
				ContainersRunning: 5,
				ContainersPaused:  1,
				ContainersStopped: 4,
				Images:            20,
				Driver:            "overlay2",
				OperatingSystem:   "Ubuntu 22.04",
				Architecture:      "x86_64",
				NCPU:              8,
				MemTotal:          16 * 1024 * 1024 * 1024,
				ServerVersion:     "24.0.7",
				KernelVersion:     "5.15.0",
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	info, err := c.Info()
	if err != nil {
		t.Fatalf("Info() error = %v", err)
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
	if info.ServerVersion != "24.0.7" {
		t.Errorf("ServerVersion = %v", info.ServerVersion)
	}
}

func TestClientInfoError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := c.Info()
	if err == nil {
		t.Error("Info() should return error on 500")
	}
}

func TestClientVersion(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/version") {
			json.NewEncoder(w).Encode(types.Version{
				Version:    "24.0.7",
				APIVersion: "1.43",
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	ver, err := c.Version()
	if err != nil {
		t.Fatalf("Version() error = %v", err)
	}
	if ver.Version != "24.0.7" {
		t.Errorf("Version = %v", ver.Version)
	}
	if ver.APIVersion != "1.43" {
		t.Errorf("APIVersion = %v", ver.APIVersion)
	}
}

func TestClientVersionError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := c.Version()
	if err == nil {
		t.Error("Version() should return error on 500")
	}
}

func TestClientClose(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {})
	if err := c.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}
