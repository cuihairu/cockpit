package agent

// cov_startcmd_test.go 覆盖 StartCmd 的参数解析与错误分支。
// 注意：StartCmd.Run 的成功路径会真启动 agent，这里只测 Validate 失败与
// agent 启动失败（连不上 server）两条不产生真实 agent 的路径。

import (
	"flag"
	"strings"
	"testing"
)

func TestCovAgentStartBind(t *testing.T) {
	c := &StartCmd{}
	fs := flag.NewFlagSet("agent-start", flag.ContinueOnError)
	c.Bind(fs)
	for _, name := range []string{"server", "id", "secret", "region", "zone", "labels"} {
		if fs.Lookup(name) == nil {
			t.Errorf("flag -%s not bound", name)
		}
	}
}

func TestCovAgentStartBindWithUsage(t *testing.T) {
	c := &StartCmd{}
	fs := flag.NewFlagSet("agent-start", flag.ContinueOnError)
	c.BindWithUsage(fs, StartUsage{
		Server: "custom server usage",
		ID:     "custom id usage",
		Secret: "custom secret usage",
		Region: "custom region usage",
		Zone:   "custom zone usage",
		Labels: "custom labels usage",
	})
	if got := fs.Lookup("server").Usage; got != "custom server usage" {
		t.Errorf("server usage = %q", got)
	}

	// 空描述回退默认文案
	fs2 := flag.NewFlagSet("agent-start", flag.ContinueOnError)
	c.BindWithUsage(fs2, StartUsage{})
	if got := fs2.Lookup("server").Usage; got != defaultStartUsage.Server {
		t.Errorf("fallback server usage = %q", got)
	}
	if got := fs2.Lookup("labels").Usage; got != defaultStartUsage.Labels {
		t.Errorf("fallback labels usage = %q", got)
	}
}

func TestCovAgentStartRunValidateError(t *testing.T) {
	if err := (&StartCmd{}).Run(); err == nil {
		t.Fatal("missing -server should error")
	}
}

func TestCovAgentStartRunAgentConnectError(t *testing.T) {
	// 不可达地址：agent.Start() 连接失败即返回，不产生真实 agent 进程
	cmd := &StartCmd{Server: "ws://127.0.0.1:1", ID: "cov-agent", Secret: "s"}
	err := cmd.Run()
	if err == nil {
		t.Fatal("unreachable server should error")
	}
	if !strings.Contains(err.Error(), "connect failed") {
		t.Errorf("err = %v, want transparent connect error", err)
	}
}

func TestCovParseLabelsEdgeCases(t *testing.T) {
	if got := parseLabels(""); len(got) != 0 {
		t.Errorf("empty labels should yield empty map, got %v", got)
	}
	// 空片段被跳过
	got := parseLabels("a=1,, ,b=2")
	if got["a"] != 1 || got["b"] != 2 || len(got) != 2 {
		t.Errorf("labels = %#v", got)
	}
	// 空数组值 → []string{}
	arr, ok := parseLabels("list=[]")["list"].([]string)
	if !ok || len(arr) != 0 {
		t.Errorf("empty list = %#v", parseLabels("list=[]")["list"])
	}
	// 布尔 false 字面量 → bool(false)，而非字符串 "false"
	if f, ok := parseLabels("flag=false")["flag"].(bool); !ok || f {
		t.Errorf("flag=false = %#v, want bool false", parseLabels("flag=false")["flag"])
	}
}
