package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/agent"
)

// 服务安装参数面（svcargs.go，平台无关故在 linux CI 覆盖）：
// `service install` 入参 → StartCmd → svcArgsFrom → 注册进服务的命令行
// → SCM 重启时再解析回来。往返一致性是安装路径的口径不变量——任一环的参数名
// 或顺序漂移都只在真机装机时暴露。

func TestCovSvcBindAllFields(t *testing.T) {
	cmd, err := svcBind([]string{
		"-server", "wss://example.com:9000/ws",
		"-id", "win-agent-1",
		"-secret", "s3cr3t",
		"-region", "cn-bj",
		"-zone", "z1",
		"-labels", "env=prod,services=[docker,k8s]",
		"-ssh-keys", `C:\keys`,
	})
	if err != nil {
		t.Fatalf("svcBind error: %v", err)
	}
	want := agent.StartCmd{
		Version: version,
		Server:  "wss://example.com:9000/ws",
		ID:      "win-agent-1",
		Secret:  "s3cr3t",
		Region:  "cn-bj",
		Zone:    "z1",
		Labels:  "env=prod,services=[docker,k8s]",
		SSHKeys: `C:\keys`,
	}
	if !reflect.DeepEqual(*cmd, want) {
		t.Fatalf("got %+v want %+v", *cmd, want)
	}
	if err := cmd.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestCovSvcBindParseError(t *testing.T) {
	// 未知 flag：svcBind 原样返回 flag 错误（svcInstall 据此打印「参数错误」并返回 1）
	cmd, err := svcBind([]string{"-bogus", "1"})
	if err == nil || cmd != nil {
		t.Fatalf("cmd=%+v err=%v", cmd, err)
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("err should name the flag: %v", err)
	}
}

// 留空 -server 不在 svcBind 拦（Validate 在 svcInstall 里调，语义与 handleStart 一致）
func TestCovSvcBindServerValidatedLater(t *testing.T) {
	cmd, err := svcBind([]string{"-id", "a1"})
	if err != nil {
		t.Fatalf("svcBind error: %v", err)
	}
	if err := cmd.Validate(); err == nil || !strings.Contains(err.Error(), "-server") {
		t.Fatalf("Validate should reject empty server, got %v", err)
	}
}

func TestCovSvcArgsFromMinimal(t *testing.T) {
	// 只带 Server：命令行收敛到 4 段，不产空值 flag
	args := svcArgsFrom(&agent.StartCmd{Server: "ws://h:9000/ws"})
	want := []string{"service", "run", "-server", "ws://h:9000/ws"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("got %v want %v", args, want)
	}
}

func TestCovSvcArgsFromFull(t *testing.T) {
	args := svcArgsFrom(&agent.StartCmd{
		Server:  "ws://h:9000/ws",
		ID:      "a1",
		Secret:  "sec",
		Region:  "r",
		Zone:    "z",
		Labels:  "k=v",
		SSHKeys: "/ssh",
	})
	want := []string{
		"service", "run",
		"-server", "ws://h:9000/ws",
		"-id", "a1",
		"-secret", "sec",
		"-region", "r",
		"-zone", "z",
		"-labels", "k=v",
		"-ssh-keys", "/ssh",
	}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("got %v want %v", args, want)
	}
}

// 往返不变量：install 绑定的入参经 svcArgsFrom 落到服务命令行，剥掉
// `service run` 动词后重新绑定必须还原同一个 StartCmd（SCM 每次重启走这条路）
func TestCovSvcArgsRoundTrip(t *testing.T) {
	in := []string{
		"-server", "wss://example.com:9000/ws",
		"-id", "win-agent-1",
		"-secret", "s3cr3t",
		"-region", "cn-bj",
		"-zone", "z1",
		"-labels", "env=prod,services=[docker,k8s]",
		"-ssh-keys", `C:\ProgramData\keys`,
	}
	first, err := svcBind(in)
	if err != nil {
		t.Fatalf("svcBind error: %v", err)
	}
	back, err := svcBind(svcStripVerbs(svcArgsFrom(first)))
	if err != nil {
		t.Fatalf("re-bind error: %v", err)
	}
	if !reflect.DeepEqual(*first, *back) {
		t.Fatalf("round trip drift:\n first %+v\n back  %+v", *first, *back)
	}
	// 二次还原稳定（不再漂移出新的 flag 段）
	if !reflect.DeepEqual(svcArgsFrom(first), svcArgsFrom(back)) {
		t.Fatalf("args not stable across round trips")
	}
}

func TestCovSvcStripVerbs(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"service run 前缀（本仓注册形态）", []string{"service", "run", "-server", "x"}, []string{"-server", "x"}},
		{"start 前缀（install.ps1 存量直挂形态）", []string{"start", "-server", "x"}, []string{"-server", "x"}},
		{"连续动词", []string{"service", "run", "service", "-server", "x"}, []string{"-server", "x"}},
		{"已剥净", []string{"-server", "x"}, []string{"-server", "x"}},
		{"空", nil, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := svcStripVerbs(c.in); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %v want %v", got, c.want)
			}
		})
	}
}

// svcStripVerbs 不吃后续位置的动词：`start -labels service` 的值原样保留
func TestCovSvcStripVerbsOnlyLeading(t *testing.T) {
	got := svcStripVerbs([]string{"start", "-labels", "svc=service", "-server", "x"})
	want := []string{"-labels", "svc=service", "-server", "x"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}