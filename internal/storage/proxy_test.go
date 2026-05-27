package storage

import (
	"testing"
)

func TestCreateProxy(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	proxy := &Proxy{
		ID:         "proxy-1",
		Name:       "SSH Proxy",
		AgentID:    "agent-1",
		ProxyType:  "tcp",
		RemotePort: 2222,
		Target:     "192.168.1.1:22",
		Enabled:    true,
	}
	if err := db.CreateProxy(proxy); err != nil {
		t.Fatalf("CreateProxy() error = %v", err)
	}
}

func TestGetProxy(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	db.CreateProxy(&Proxy{
		ID: "p1", Name: "Test", AgentID: "a1",
		ProxyType: "tcp", RemotePort: 8080, Target: "10.0.0.1:80",
	})

	proxy, err := db.GetProxy("p1")
	if err != nil {
		t.Fatalf("GetProxy() error = %v", err)
	}
	if proxy.Name != "Test" {
		t.Errorf("Name = %v, want Test", proxy.Name)
	}
	if proxy.RemotePort != 8080 {
		t.Errorf("RemotePort = %d, want 8080", proxy.RemotePort)
	}
}

func TestGetProxyNotFound(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	_, err := db.GetProxy("nonexistent")
	if err == nil {
		t.Error("GetProxy(nonexistent) should return error")
	}
}

func TestListProxies(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	db.CreateProxy(&Proxy{ID: "p1", Name: "A", AgentID: "agent-1", ProxyType: "tcp", RemotePort: 1, Target: "t1"})
	db.CreateProxy(&Proxy{ID: "p2", Name: "B", AgentID: "agent-1", ProxyType: "tcp", RemotePort: 2, Target: "t2"})
	db.CreateProxy(&Proxy{ID: "p3", Name: "C", AgentID: "agent-2", ProxyType: "tcp", RemotePort: 3, Target: "t3"})

	all, err := db.ListProxies("")
	if err != nil {
		t.Fatalf("ListProxies() error = %v", err)
	}
	if len(all) != 3 {
		t.Errorf("ListProxies('') count = %d, want 3", len(all))
	}

	filtered, _ := db.ListProxies("agent-1")
	if len(filtered) != 2 {
		t.Errorf("ListProxies(agent-1) count = %d, want 2", len(filtered))
	}
}

func TestListEnabledProxies(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	db.CreateProxy(&Proxy{ID: "p1", Name: "On", AgentID: "a1", ProxyType: "tcp", RemotePort: 1, Target: "t", Enabled: true})
	p2 := &Proxy{ID: "p2", Name: "Off", AgentID: "a1", ProxyType: "tcp", RemotePort: 2, Target: "t", Enabled: true}
	db.CreateProxy(p2)
	// GORM skips zero-value bool on Create, so disable via Update
	db.db.Model(&Proxy{}).Where("id = ?", "p2").Update("enabled", false)

	enabled, err := db.ListEnabledProxies()
	if err != nil {
		t.Fatalf("ListEnabledProxies() error = %v", err)
	}
	if len(enabled) != 1 {
		t.Errorf("ListEnabledProxies() count = %d, want 1", len(enabled))
	}
	if enabled[0].Name != "On" {
		t.Errorf("Name = %v, want On", enabled[0].Name)
	}
}

func TestUpdateProxy(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	proxy := &Proxy{ID: "p1", Name: "Old", AgentID: "a1", ProxyType: "tcp", RemotePort: 8080, Target: "old:80"}
	db.CreateProxy(proxy)

	proxy.Name = "New"
	proxy.Target = "new:80"
	if err := db.UpdateProxy(proxy); err != nil {
		t.Fatalf("UpdateProxy() error = %v", err)
	}

	got, _ := db.GetProxy("p1")
	if got.Name != "New" {
		t.Errorf("Name = %v, want New", got.Name)
	}
	if got.Target != "new:80" {
		t.Errorf("Target = %v, want new:80", got.Target)
	}
}

func TestDeleteProxy(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	db.CreateProxy(&Proxy{ID: "p1", Name: "T", AgentID: "a1", ProxyType: "tcp", RemotePort: 1, Target: "t"})

	if err := db.DeleteProxy("p1"); err != nil {
		t.Fatalf("DeleteProxy() error = %v", err)
	}

	_, err := db.GetProxy("p1")
	if err == nil {
		t.Error("GetProxy() after delete should return error")
	}
}

func TestGetProxyByRemotePort(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	db.CreateProxy(&Proxy{ID: "p1", Name: "A", AgentID: "a1", ProxyType: "tcp", RemotePort: 2222, Target: "t", Enabled: true})
	p2 := &Proxy{ID: "p2", Name: "B", AgentID: "a1", ProxyType: "tcp", RemotePort: 3333, Target: "t", Enabled: true}
	db.CreateProxy(p2)
	db.db.Model(&Proxy{}).Where("id = ?", "p2").Update("enabled", false)

	proxy, err := db.GetProxyByRemotePort(2222)
	if err != nil {
		t.Fatalf("GetProxyByRemotePort() error = %v", err)
	}
	if proxy.ID != "p1" {
		t.Errorf("ID = %v, want p1", proxy.ID)
	}

	// Disabled proxy should not be found
	_, err = db.GetProxyByRemotePort(3333)
	if err == nil {
		t.Error("GetProxyByRemotePort(3333) should not find disabled proxy")
	}
}

func TestProxyTableName(t *testing.T) {
	p := Proxy{}
	if p.TableName() != "proxies" {
		t.Errorf("TableName() = %v, want proxies", p.TableName())
	}
}
