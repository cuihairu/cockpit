package pve

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ============ VM Operations Tests ============

func TestSuspendVM(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api2/json/nodes/pve1/qemu/100/status/suspend" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data": null}`))
	}))
	defer server.Close()

	client := NewClient(Config{Endpoint: server.URL, TokenID: "t", TokenSecret: "s"})
	if err := client.SuspendVM("pve1", 100); err != nil {
		t.Errorf("SuspendVM() error = %v", err)
	}
}

func TestResumeVM(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api2/json/nodes/pve1/qemu/100/status/resume" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data": null}`))
	}))
	defer server.Close()

	client := NewClient(Config{Endpoint: server.URL, TokenID: "t", TokenSecret: "s"})
	if err := client.ResumeVM("pve1", 100); err != nil {
		t.Errorf("ResumeVM() error = %v", err)
	}
}

func TestSuspendVMDefaultNode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data": null}`))
	}))
	defer server.Close()

	client := NewClient(Config{Endpoint: server.URL, TokenID: "t", TokenSecret: "s", Node: "pve1"})
	if err := client.SuspendVM("", 100); err != nil {
		t.Errorf("SuspendVM() with default node error = %v", err)
	}
}

func TestSuspendVMNoNode(t *testing.T) {
	client := NewClient(Config{Endpoint: "http://x", TokenID: "t", TokenSecret: "s"})
	if err := client.SuspendVM("", 100); err == nil {
		t.Error("SuspendVM() without node should return error")
	}
}

func TestResumeVMNoNode(t *testing.T) {
	client := NewClient(Config{Endpoint: "http://x", TokenID: "t", TokenSecret: "s"})
	if err := client.ResumeVM("", 100); err == nil {
		t.Error("ResumeVM() without node should return error")
	}
}

func TestStartVMNoNode(t *testing.T) {
	client := NewClient(Config{Endpoint: "http://x", TokenID: "t", TokenSecret: "s"})
	if err := client.StartVM("", 100); err == nil {
		t.Error("StartVM() without node should return error")
	}
}

func TestStopVMNoNode(t *testing.T) {
	client := NewClient(Config{Endpoint: "http://x", TokenID: "t", TokenSecret: "s"})
	if err := client.StopVM("", 100); err == nil {
		t.Error("StopVM() without node should return error")
	}
}

func TestRestartVMNoNode(t *testing.T) {
	client := NewClient(Config{Endpoint: "http://x", TokenID: "t", TokenSecret: "s"})
	if err := client.RestartVM("", 100); err == nil {
		t.Error("RestartVM() without node should return error")
	}
}

func TestGetVMNoNode(t *testing.T) {
	client := NewClient(Config{Endpoint: "http://x", TokenID: "t", TokenSecret: "s"})
	_, err := client.GetVM("", 100)
	if err == nil {
		t.Error("GetVM() without node should return error")
	}
}

func TestStartVMDefaultNode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expected := "/api2/json/nodes/mynode/qemu/200/status/start"
		if r.URL.Path != expected {
			t.Errorf("path = %s, want %s", r.URL.Path, expected)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data": null}`))
	}))
	defer server.Close()

	client := NewClient(Config{Endpoint: server.URL, TokenID: "t", TokenSecret: "s", Node: "mynode"})
	if err := client.StartVM("", 200); err != nil {
		t.Errorf("StartVM() with default node error = %v", err)
	}
}

// ============ Container Operations Tests ============

func TestGetContainer(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			if r.URL.Path != "/api2/json/nodes/pve1/lxc/200/status/current" {
				t.Errorf("unexpected status path: %s", r.URL.Path)
			}
			resp := map[string]interface{}{
				"data": Container{VMID: 200, Name: "ct-test", Status: "running"},
			}
			json.NewEncoder(w).Encode(resp)
		} else {
			if r.URL.Path != "/api2/json/nodes/pve1/lxc/200/config" {
				t.Errorf("unexpected config path: %s", r.URL.Path)
			}
			resp := map[string]interface{}{
				"data": map[string]string{"cores": "2", "memory": "2048"},
			}
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	client := NewClient(Config{Endpoint: server.URL, TokenID: "t", TokenSecret: "s"})
	ct, err := client.GetContainer("pve1", 200)
	if err != nil {
		t.Fatalf("GetContainer() error = %v", err)
	}
	if ct.Container.VMID != 200 {
		t.Errorf("VMID = %d, want 200", ct.Container.VMID)
	}
	if ct.Config["cores"] != "2" {
		t.Errorf("Config cores = %v", ct.Config["cores"])
	}
}

func TestGetContainerNoNode(t *testing.T) {
	client := NewClient(Config{Endpoint: "http://x", TokenID: "t", TokenSecret: "s"})
	_, err := client.GetContainer("", 200)
	if err == nil {
		t.Error("GetContainer() without node should return error")
	}
}

func TestStartContainer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api2/json/nodes/pve1/lxc/200/status/start" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data": null}`))
	}))
	defer server.Close()

	client := NewClient(Config{Endpoint: server.URL, TokenID: "t", TokenSecret: "s"})
	if err := client.StartContainer("pve1", 200); err != nil {
		t.Errorf("StartContainer() error = %v", err)
	}
}

func TestStopContainer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api2/json/nodes/pve1/lxc/200/status/stop" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data": null}`))
	}))
	defer server.Close()

	client := NewClient(Config{Endpoint: server.URL, TokenID: "t", TokenSecret: "s"})
	if err := client.StopContainer("pve1", 200); err != nil {
		t.Errorf("StopContainer() error = %v", err)
	}
}

func TestRestartContainer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api2/json/nodes/pve1/lxc/200/status/reboot" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data": null}`))
	}))
	defer server.Close()

	client := NewClient(Config{Endpoint: server.URL, TokenID: "t", TokenSecret: "s"})
	if err := client.RestartContainer("pve1", 200); err != nil {
		t.Errorf("RestartContainer() error = %v", err)
	}
}

func TestShutdownContainer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api2/json/nodes/pve1/lxc/200/status/shutdown" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data": null}`))
	}))
	defer server.Close()

	client := NewClient(Config{Endpoint: server.URL, TokenID: "t", TokenSecret: "s"})
	if err := client.ShutdownContainer("pve1", 200); err != nil {
		t.Errorf("ShutdownContainer() error = %v", err)
	}
}

func TestContainerNoNodeErrors(t *testing.T) {
	client := NewClient(Config{Endpoint: "http://x", TokenID: "t", TokenSecret: "s"})

	if err := client.StartContainer("", 100); err == nil {
		t.Error("StartContainer() without node should error")
	}
	if err := client.StopContainer("", 100); err == nil {
		t.Error("StopContainer() without node should error")
	}
	if err := client.RestartContainer("", 100); err == nil {
		t.Error("RestartContainer() without node should error")
	}
	if err := client.ShutdownContainer("", 100); err == nil {
		t.Error("ShutdownContainer() without node should error")
	}
	if _, err := client.ListContainers(""); err == nil {
		t.Error("ListContainers() without node should error")
	}
}

func TestListContainersDefaultNode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"data": []Container{{VMID: 100, Name: "ct-1", Status: "running"}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{Endpoint: server.URL, TokenID: "t", TokenSecret: "s", Node: "pve1"})
	cts, err := client.ListContainers("")
	if err != nil {
		t.Fatalf("ListContainers() error = %v", err)
	}
	if len(cts) != 1 {
		t.Errorf("len = %d, want 1", len(cts))
	}
}

// ============ Storage Tests ============

func TestListStorage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api2/json/nodes/pve1/storage" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		resp := map[string]interface{}{
			"data": []Storage{
				{Storage: "local", Type: "dir", Total: 100000, Used: 50000, Avail: 50000},
				{Storage: "ceph", Type: "rbd", Total: 500000, Used: 200000, Avail: 300000},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{Endpoint: server.URL, TokenID: "t", TokenSecret: "s"})
	storages, err := client.ListStorage("pve1")
	if err != nil {
		t.Fatalf("ListStorage() error = %v", err)
	}
	if len(storages) != 2 {
		t.Errorf("len = %d, want 2", len(storages))
	}
	if storages[0].Storage != "local" {
		t.Errorf("storages[0].Storage = %v", storages[0].Storage)
	}
}

func TestListStorageNoNode(t *testing.T) {
	client := NewClient(Config{Endpoint: "http://x", TokenID: "t", TokenSecret: "s"})
	_, err := client.ListStorage("")
	if err == nil {
		t.Error("ListStorage() without node should error")
	}
}

func TestGetStorage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expected := "/api2/json/nodes/pve1/storage/local/status"
		if r.URL.Path != expected {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		resp := map[string]interface{}{
			"data": StorageStatus{Used: 50000, Avail: 50000, Total: 100000},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{Endpoint: server.URL, TokenID: "t", TokenSecret: "s"})
	status, err := client.GetStorage("pve1", "local")
	if err != nil {
		t.Fatalf("GetStorage() error = %v", err)
	}
	if status.Total != 100000 {
		t.Errorf("Total = %d, want 100000", status.Total)
	}
}

func TestGetStorageNoNode(t *testing.T) {
	client := NewClient(Config{Endpoint: "http://x", TokenID: "t", TokenSecret: "s"})
	_, err := client.GetStorage("", "local")
	if err == nil {
		t.Error("GetStorage() without node should error")
	}
}

// ============ Snapshot Tests ============

func TestListVMSnapshots(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api2/json/nodes/pve1/qemu/100/snapshot" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		resp := map[string]interface{}{
			"data": []Snapshot{
				{Name: "snap1", Snaptime: 1000000, Description: "before update"},
				{Name: "snap2", Snaptime: 2000000},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{Endpoint: server.URL, TokenID: "t", TokenSecret: "s"})
	snaps, err := client.ListVMSnapshots("pve1", 100)
	if err != nil {
		t.Fatalf("ListVMSnapshots() error = %v", err)
	}
	if len(snaps) != 2 {
		t.Errorf("len = %d, want 2", len(snaps))
	}
	if snaps[0].Name != "snap1" {
		t.Errorf("snaps[0].Name = %v", snaps[0].Name)
	}
}

func TestListVMSnapshotsNoNode(t *testing.T) {
	client := NewClient(Config{Endpoint: "http://x", TokenID: "t", TokenSecret: "s"})
	_, err := client.ListVMSnapshots("", 100)
	if err == nil {
		t.Error("ListVMSnapshots() without node should error")
	}
}

func TestCreateVMSnapshot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api2/json/nodes/pve1/qemu/100/snapshot" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data": null}`))
	}))
	defer server.Close()

	client := NewClient(Config{Endpoint: server.URL, TokenID: "t", TokenSecret: "s"})
	if err := client.CreateVMSnapshot("pve1", 100, "backup", "before upgrade"); err != nil {
		t.Errorf("CreateVMSnapshot() error = %v", err)
	}
}

func TestCreateVMSnapshotNoName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data": null}`))
	}))
	defer server.Close()

	client := NewClient(Config{Endpoint: server.URL, TokenID: "t", TokenSecret: "s"})
	if err := client.CreateVMSnapshot("pve1", 100, "", ""); err != nil {
		t.Errorf("CreateVMSnapshot() with empty params error = %v", err)
	}
}

func TestCreateVMSnapshotNoNode(t *testing.T) {
	client := NewClient(Config{Endpoint: "http://x", TokenID: "t", TokenSecret: "s"})
	if err := client.CreateVMSnapshot("", 100, "snap", ""); err == nil {
		t.Error("CreateVMSnapshot() without node should error")
	}
}

func TestDeleteVMSnapshot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		expected := "/api2/json/nodes/pve1/qemu/100/snapshot/backup"
		if r.URL.Path != expected {
			t.Errorf("unexpected path: %s, want %s", r.URL.Path, expected)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data": null}`))
	}))
	defer server.Close()

	client := NewClient(Config{Endpoint: server.URL, TokenID: "t", TokenSecret: "s"})
	if err := client.DeleteVMSnapshot("pve1", 100, "backup"); err != nil {
		t.Errorf("DeleteVMSnapshot() error = %v", err)
	}
}

func TestDeleteVMSnapshotNoNode(t *testing.T) {
	client := NewClient(Config{Endpoint: "http://x", TokenID: "t", TokenSecret: "s"})
	if err := client.DeleteVMSnapshot("", 100, "snap"); err == nil {
		t.Error("DeleteVMSnapshot() without node should error")
	}
}

// ============ Version Tests ============

func TestVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api2/json/version" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		resp := map[string]interface{}{
			"data": VersionInfo{Version: "8.1.3", Release: "8.1", Repoid: "abc123"},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{Endpoint: server.URL, TokenID: "t", TokenSecret: "s"})
	ver, err := client.Version()
	if err != nil {
		t.Fatalf("Version() error = %v", err)
	}
	if ver.Version != "8.1.3" {
		t.Errorf("Version = %v, want 8.1.3", ver.Version)
	}
}

// ============ GetVMID Tests ============

func TestGetVMID(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		want  int
		err   bool
	}{
		{"int", 100, 100, false},
		{"int64", int64(200), 200, false},
		{"float64", float64(300.7), 300, false},
		{"string", "400", 400, false},
		{"invalid string", "abc", 0, true},
		{"invalid type", []int{1}, 0, true},
		{"nil", nil, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetVMID(tt.input)
			if tt.err {
				if err == nil {
					t.Error("expected error")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if got != tt.want {
					t.Errorf("GetVMID() = %d, want %d", got, tt.want)
				}
			}
		})
	}
}

// ============ Error Path Tests ============

func TestVMOperationsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"errors": "internal error"}`))
	}))
	defer server.Close()

	client := NewClient(Config{Endpoint: server.URL, TokenID: "t", TokenSecret: "s", Node: "pve1"})

	if err := client.StartVM("", 100); err == nil {
		t.Error("StartVM() should fail on 500")
	}
	if err := client.StopVM("", 100); err == nil {
		t.Error("StopVM() should fail on 500")
	}
	if err := client.RestartVM("", 100); err == nil {
		t.Error("RestartVM() should fail on 500")
	}
	if err := client.SuspendVM("", 100); err == nil {
		t.Error("SuspendVM() should fail on 500")
	}
	if err := client.ResumeVM("", 100); err == nil {
		t.Error("ResumeVM() should fail on 500")
	}
	if err := client.StartContainer("", 100); err == nil {
		t.Error("StartContainer() should fail on 500")
	}
	if err := client.StopContainer("", 100); err == nil {
		t.Error("StopContainer() should fail on 500")
	}
}

func TestStorageOperationsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintf(w, `{"errors": "permission denied"}`)
	}))
	defer server.Close()

	client := NewClient(Config{Endpoint: server.URL, TokenID: "t", TokenSecret: "s", Node: "pve1"})

	if _, err := client.ListStorage(""); err == nil {
		t.Error("ListStorage() should fail on 403")
	}
	if _, err := client.GetStorage("", "local"); err == nil {
		t.Error("GetStorage() should fail on 403")
	}
	if _, err := client.ListVMSnapshots("", 100); err == nil {
		t.Error("ListVMSnapshots() should fail on 403")
	}
	if err := client.CreateVMSnapshot("", 100, "snap", ""); err == nil {
		t.Error("CreateVMSnapshot() should fail on 403")
	}
	if err := client.DeleteVMSnapshot("", 100, "snap"); err == nil {
		t.Error("DeleteVMSnapshot() should fail on 403")
	}
	if _, err := client.Version(); err == nil {
		t.Error("Version() should fail on 403")
	}
}

func TestConnectionRefused(t *testing.T) {
	client := NewClient(Config{
		Endpoint:    "http://127.0.0.1:1",
		TokenID:     "t",
		TokenSecret: "s",
	})

	_, err := client.ListNodes()
	if err == nil {
		t.Error("ListNodes() should fail on connection refused")
	}
}

func TestInvalidJSONParse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	}))
	defer server.Close()

	client := NewClient(Config{Endpoint: server.URL, TokenID: "t", TokenSecret: "s", Node: "pve1"})

	if _, err := client.ListVMs(""); err == nil {
		t.Error("ListVMs() should fail on invalid JSON")
	}
	if _, err := client.ListContainers(""); err == nil {
		t.Error("ListContainers() should fail on invalid JSON")
	}
	if _, err := client.ListStorage(""); err == nil {
		t.Error("ListStorage() should fail on invalid JSON")
	}
	if _, err := client.ListVMSnapshots("", 100); err == nil {
		t.Error("ListVMSnapshots() should fail on invalid JSON")
	}
	if _, err := client.Version(); err == nil {
		t.Error("Version() should fail on invalid JSON")
	}
	if _, err := client.GetNodeStatus("pve1"); err == nil {
		t.Error("GetNodeStatus() should fail on invalid JSON")
	}
	if _, err := client.GetStorage("", "local"); err == nil {
		t.Error("GetStorage() should fail on invalid JSON")
	}
}

func TestGetContainerAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"errors": "not found"}`))
	}))
	defer server.Close()

	client := NewClient(Config{Endpoint: server.URL, TokenID: "t", TokenSecret: "s"})
	_, err := client.GetContainer("pve1", 999)
	if err == nil {
		t.Error("GetContainer() should fail on 404")
	}
}

func TestGetVMAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"errors": "not found"}`))
	}))
	defer server.Close()

	client := NewClient(Config{Endpoint: server.URL, TokenID: "t", TokenSecret: "s"})
	_, err := client.GetVM("pve1", 999)
	if err == nil {
		t.Error("GetVM() should fail on 404")
	}
}

// ============ Default Node for Container Operations ============

func TestContainerDefaultNode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data": null}`))
	}))
	defer server.Close()

	client := NewClient(Config{Endpoint: server.URL, TokenID: "t", TokenSecret: "s", Node: "default-node"})

	if err := client.StartContainer("", 100); err != nil {
		t.Errorf("StartContainer() error = %v", err)
	}
	if err := client.StopContainer("", 100); err != nil {
		t.Errorf("StopContainer() error = %v", err)
	}
	if err := client.RestartContainer("", 100); err != nil {
		t.Errorf("RestartContainer() error = %v", err)
	}
	if err := client.ShutdownContainer("", 100); err != nil {
		t.Errorf("ShutdownContainer() error = %v", err)
	}
}

func TestSnapshotDefaultNode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{"data": []Snapshot{}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{Endpoint: server.URL, TokenID: "t", TokenSecret: "s", Node: "pve1"})

	snaps, err := client.ListVMSnapshots("", 100)
	if err != nil {
		t.Fatalf("ListVMSnapshots() error = %v", err)
	}
	if len(snaps) != 0 {
		t.Errorf("len = %d, want 0", len(snaps))
	}
}

func TestStorageDefaultNode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{"data": []Storage{}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{Endpoint: server.URL, TokenID: "t", TokenSecret: "s", Node: "pve1"})

	storages, err := client.ListStorage("")
	if err != nil {
		t.Fatalf("ListStorage() error = %v", err)
	}
	if len(storages) != 0 {
		t.Errorf("len = %d, want 0", len(storages))
	}
}
