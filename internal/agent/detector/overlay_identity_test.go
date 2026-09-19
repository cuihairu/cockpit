package detector

import (
	"testing"
)

// overlay 身份提取测试（M2-B，D15/D16）：经由 fakeBinDir PATH 桩注入
// 假 CLI 输出，断言白名单字段与失败静默语义。

const ztInfoJSON = `{"address":"7f3d0a9b12","online":true,"version":"1.14.0"}`

const ztNetworksJSON = `[{"id":"8056c2e21c","name":"home","status":"OK","type":"private",
  "assignedAddresses":["10.147.20.5/24","fd80::1/88"]},
 {"id":"deadbeef00","name":"office","status":"ACCESS_DENIED","assignedAddresses":[]}]`

const tsStatusJSON = `{"Version":"1.80.2","Self":{"ID":"1234567890","HostName":"nas",
  "DNSName":"nas.tail-scale.ts.net.","TailscaleIPs":["100.64.0.2","fd7a:115c:a1e0::2"]},
  "Peer":{}}`

func TestOverlayIdentityZeroTier(t *testing.T) {
	fakeBinDir(t, map[string]string{
		"zerotier-cli": `if [ "$2" = "info" ]; then echo '` + ztInfoJSON + `'; else echo '` + ztNetworksJSON + `'; fi`,
	})

	id, ok := zerotierIdentity()
	if !ok {
		t.Fatal("zerotierIdentity() should succeed with stubbed CLI")
	}
	if id["nodeId"] != "7f3d0a9b12" {
		t.Errorf("nodeId = %v, want 7f3d0a9b12", id["nodeId"])
	}
	networks, ok := id["networks"].([]map[string]any)
	if !ok || len(networks) != 2 {
		t.Fatalf("networks = %v, want 2 entries", id["networks"])
	}
	if networks[0]["id"] != "8056c2e21c" || networks[0]["name"] != "home" || networks[0]["status"] != "OK" {
		t.Errorf("network[0] = %v", networks[0])
	}
	// D17 匹配口径：CIDR 后缀剥离，对齐云端 ipAssignments 裸地址
	addrs, ok := networks[0]["addresses"].([]string)
	if !ok || len(addrs) != 2 || addrs[0] != "10.147.20.5" || addrs[1] != "fd80::1" {
		t.Errorf("addresses = %v, want CIDR-stripped [10.147.20.5 fd80::1]", networks[0]["addresses"])
	}
}

func TestOverlayIdentityZeroTierInfoFailSilent(t *testing.T) {
	fakeBinDir(t, map[string]string{
		"zerotier-cli": "exit 1",
	})
	if id, ok := zerotierIdentity(); ok || id != nil {
		t.Errorf("zerotierIdentity() = (%v, %v), want absent on CLI failure", id, ok)
	}
}

func TestOverlayIdentityZeroTierInfoBadJSON(t *testing.T) {
	fakeBinDir(t, map[string]string{
		"zerotier-cli": `echo 'not-json'`,
	})
	if _, ok := zerotierIdentity(); ok {
		t.Error("zerotierIdentity() should be absent on invalid JSON")
	}
}

func TestOverlayIdentityZeroTierListNetworksFail(t *testing.T) {
	// info 成功 listnetworks 失败 → nodeId 保留，networks 段缺席（D16 逐步降级）
	fakeBinDir(t, map[string]string{
		"zerotier-cli": `if [ "$2" = "info" ]; then echo '` + ztInfoJSON + `'; else exit 1; fi`,
	})
	id, ok := zerotierIdentity()
	if !ok || id["nodeId"] != "7f3d0a9b12" {
		t.Fatalf("zerotierIdentity() = (%v, %v), want nodeId kept", id, ok)
	}
	if _, has := id["networks"]; has {
		t.Errorf("networks should be absent when listnetworks fails, got %v", id["networks"])
	}
}

func TestOverlayIdentityTailscale(t *testing.T) {
	fakeBinDir(t, map[string]string{
		"tailscale": `if [ "$2" = "--json" ]; then echo '` + tsStatusJSON + `'; fi`,
	})

	id, ok := tailscaleIdentity()
	if !ok {
		t.Fatal("tailscaleIdentity() should succeed with stubbed CLI")
	}
	if id["id"] != "1234567890" {
		t.Errorf("id = %v, want 1234567890", id["id"])
	}
	if id["hostName"] != "nas" || id["dnsName"] != "nas.tail-scale.ts.net." {
		t.Errorf("identity = %v", id)
	}
	addrs, ok := id["addresses"].([]string)
	if !ok || len(addrs) != 2 || addrs[0] != "100.64.0.2" {
		t.Errorf("addresses = %v, want [100.64.0.2 …]", id["addresses"])
	}
}

func TestOverlayIdentityTailscaleSelfNull(t *testing.T) {
	// 未登录：Self 为 null → 身份缺席
	fakeBinDir(t, map[string]string{
		"tailscale": `echo '{"Version":"1.80.2","Self":null,"Peer":{}}'`,
	})
	if _, ok := tailscaleIdentity(); ok {
		t.Error("tailscaleIdentity() should be absent when Self is null")
	}
}

func TestOverlayIdentityTailscaleFailSilent(t *testing.T) {
	fakeBinDir(t, map[string]string{
		"tailscale": "exit 1",
	})
	if _, ok := tailscaleIdentity(); ok {
		t.Error("tailscaleIdentity() should be absent on CLI failure")
	}
}

func TestExtractIdentityBothTools(t *testing.T) {
	fakeBinDir(t, map[string]string{
		"zerotier-cli": `if [ "$2" = "info" ]; then echo '` + ztInfoJSON + `'; else echo '[]'; fi`,
		"tailscale":    `echo '` + tsStatusJSON + `'`,
	})

	id := extractIdentity(map[string]any{"zerotier": true, "tailscale": true})
	if id == nil {
		t.Fatal("extractIdentity() should return both identities")
	}
	if _, ok := id["zerotier"]; !ok {
		t.Error("identity missing zerotier")
	}
	if _, ok := id["tailscale"]; !ok {
		t.Error("identity missing tailscale")
	}
}

func TestExtractIdentityNoneAvailable(t *testing.T) {
	// 两工具都缺席 → identity 键不写入 metadata
	if id := extractIdentity(map[string]any{"wireguard": true, "frp": true}); id != nil {
		t.Errorf("extractIdentity() = %v, want nil without zerotier/tailscale", id)
	}
	// zerotier CLI 存在但 daemon 不可达 → 同样静默缺席
	fakeBinDir(t, map[string]string{
		"zerotier-cli": "exit 1",
	})
	if id := extractIdentity(map[string]any{"zerotier": true}); id != nil {
		t.Errorf("extractIdentity() = %v, want nil when CLI fails", id)
	}
}

func TestOverlayDetectorDetectWithIdentity(t *testing.T) {
	// Detect() 集成：ZT + TS 桩就位 → capability metadata 携带 identity
	fakeBinDir(t, map[string]string{
		"zerotier-cli": `if [ "$2" = "info" ]; then echo '` + ztInfoJSON + `'; else echo '` + ztNetworksJSON + `'; fi`,
		"tailscale":    `echo '` + tsStatusJSON + `'`,
	})

	d := &OverlayDetector{}
	cap, err := d.Detect()
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if cap == nil {
		t.Fatal("Detect() should report overlay capability")
	}
	identity, ok := cap.Metadata["identity"].(map[string]any)
	if !ok {
		t.Fatalf("metadata identity = %v, want map", cap.Metadata["identity"])
	}
	if _, ok := identity["zerotier"]; !ok {
		t.Error("identity missing zerotier")
	}
	if _, ok := identity["tailscale"]; !ok {
		t.Error("identity missing tailscale")
	}
	// 工具特征位（环境相关，只断言本次桩涉及的两个）
	if cap.Metadata["zerotier"] != true || cap.Metadata["tailscale"] != true {
		t.Errorf("unexpected feature flags: %v", cap.Metadata)
	}
}
