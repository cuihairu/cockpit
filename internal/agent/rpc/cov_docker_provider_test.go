package rpc

// 覆盖率补充测试：docker_provider.go 参数校验分支与构造失败路径。

import (
	"testing"
)

func TestCovDockerProviderUnknownAction(t *testing.T) {
	p := newMockDockerProvider(&mockDockerAPI{})
	if p.Type() != "docker" {
		t.Fatalf("type = %q", p.Type())
	}
	if _, err := p.Call("bogus.action", nil); err == nil || err.Error() != "unknown docker action: bogus.action" {
		t.Errorf("unknown action err = %v", err)
	}
}

func TestCovDockerProviderMissingParams(t *testing.T) {
	p := newMockDockerProvider(&mockDockerAPI{})
	cases := []struct {
		desc   string
		action string
		params map[string]interface{}
	}{
		{"get no id", "containers.get", map[string]interface{}{}},
		{"get wrong id type", "containers.get", map[string]interface{}{"id": 42}},
		{"start no id", "containers.start", map[string]interface{}{}},
		{"stop no id", "containers.stop", map[string]interface{}{}},
		{"restart no id", "containers.restart", map[string]interface{}{}},
		{"remove no id", "containers.remove", map[string]interface{}{}},
		{"pause no id", "containers.pause", map[string]interface{}{}},
		{"unpause no id", "containers.unpause", map[string]interface{}{}},
		{"logs no id", "containers.logs", map[string]interface{}{}},
		{"stats no id", "containers.stats", map[string]interface{}{}},
		{"image remove no id", "images.remove", map[string]interface{}{}},
		{"image pull no ref", "images.pull", map[string]interface{}{}},
		{"volume remove no name", "volumes.remove", map[string]interface{}{}},
	}
	for _, c := range cases {
		if _, err := p.Call(c.action, c.params); err == nil {
			t.Errorf("%s: should fail", c.desc)
		}
	}
}

func TestCovDockerProviderTimeoutParam(t *testing.T) {
	// stop / restart 携带 timeout（int 类型命中 *int 分支）
	p := newMockDockerProvider(&mockDockerAPI{})
	if _, err := p.StopContainer(map[string]interface{}{"id": "x", "timeout": 5}); err != nil {
		t.Errorf("stop with timeout: %v", err)
	}
	if _, err := p.RestartContainer(map[string]interface{}{"id": "x", "timeout": 5}); err != nil {
		t.Errorf("restart with timeout: %v", err)
	}
	// remove / pull / remove image 参数透传成功路径
	if _, err := p.RemoveContainer(map[string]interface{}{"id": "x", "force": true, "volumes": true}); err != nil {
		t.Errorf("remove: %v", err)
	}
	if _, err := p.RemoveImage(map[string]interface{}{"id": "x", "force": true, "prune_children": true}); err != nil {
		t.Errorf("remove image: %v", err)
	}
	if _, err := p.RemoveVolume(map[string]interface{}{"name": "v", "force": true}); err != nil {
		t.Errorf("remove volume: %v", err)
	}
	// GetLogs 参数透传（tail/since/follow/timestamps/stdout/stderr）
	if _, err := p.GetLogs(map[string]interface{}{
		"id": "x", "tail": "100", "since": "1h", "follow": true,
		"timestamps": true, "stdout": true, "stderr": true,
	}); err != nil {
		t.Errorf("get logs: %v", err)
	}
	if _, err := p.ListContainers(map[string]interface{}{"all": true}); err != nil {
		t.Errorf("list containers: %v", err)
	}
	if _, err := p.ListImages(map[string]interface{}{"all": true}); err != nil {
		t.Errorf("list images: %v", err)
	}
}

func TestCovNewDockerProviderInvalidHost(t *testing.T) {
	// 非法 host → client 构造/连接失败
	if _, err := NewDockerProvider("bogus-scheme://!!"); err == nil {
		t.Error("invalid docker host should fail")
	}
}
