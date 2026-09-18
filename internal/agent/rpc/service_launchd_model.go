package rpc

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// ============ macOS launchd 服务模型（无 build tag，跨平台可测）============
//
// service-design.md D10：LaunchDaemons 与 systemd/Windows SCM 共用
// ServiceUnit 观测模型。本文件放 plist 轻量解析、launchctl list 输出解析
// 与映射/校验纯函数（Linux CI 可测）；真执行在 service_launchd_darwin.go，
// 非 darwin 编译为 stub。

// LaunchdService plist 采集条目（一个 LaunchDaemon 的安装项）
type LaunchdService struct {
	Label     string // plist Label 键（操作目标，≠文件名）
	Path      string // plist 路径（→ ServiceUnit.Description）
	RunAtLoad bool   // 开机/加载时自启（launchd 的「enabled」语义）
	Disabled  bool   // disabled DB 之外的 plist 级禁用
}

// LaunchdRuntime launchctl list 的一行（运行态）
type LaunchdRuntime struct {
	PID    string // "-" 表示从未运行
	Status string // 上次退出码，"-" 或数字
}

var (
	// launchdLabelRe label 反向域名惯例字符集（不含 / 与空格，D10.4）
	launchdLabelRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]{0,254}$`)
	// launchdActions launchd 后端动词白名单（无 reload，D10.3）
	launchdActions = map[string]bool{
		"start": true, "stop": true, "restart": true,
		"enable": true, "disable": true,
	}
)

// validateLaunchdUnit label 与动作白名单校验（与 systemd/Windows 校验三足鼎立）
func validateLaunchdUnit(label, action string) error {
	if !launchdLabelRe.MatchString(label) {
		return fmt.Errorf("invalid launchd label %q", label)
	}
	if !launchdActions[action] {
		return fmt.Errorf("unsupported action %q (allowed: start stop restart enable disable)", action)
	}
	return nil
}

// launchdActiveState launchctl list 的 PID/Status → 运行态映射（D10.2）：
// PID 在即 active；上次退出码非 0 → failed；从未运行/正常退出 → inactive
func launchdActiveState(pid, status string) string {
	if pid != "" && pid != "-" {
		return "active"
	}
	if status != "" && status != "-" && status != "0" {
		return "failed"
	}
	return "inactive"
}

// launchdToUnit DTO → 统一 ServiceUnit（D10.2 映射表）：launchd 无描述与
// 细分态概念，Description 用 plist 路径、Preset 空
func launchdToUnit(svc LaunchdService, rt *LaunchdRuntime) ServiceUnit {
	active := "inactive"
	if rt != nil {
		active = launchdActiveState(rt.PID, rt.Status)
	}
	unitFile := "disabled"
	if svc.RunAtLoad && !svc.Disabled {
		unitFile = "enabled"
	}
	return ServiceUnit{
		Name:          svc.Label,
		Description:   svc.Path,
		LoadState:     "loaded",
		ActiveState:   active,
		UnitFileState: unitFile,
	}
}

// parseLaunchctlList 解析 launchctl list 输出（三列 PID/Status/Label，
// 无表头；Label 可能含点，列间空白切分）
func parseLaunchctlList(out string) map[string]LaunchdRuntime {
	m := map[string]LaunchdRuntime{}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 3 {
			continue
		}
		m[fields[2]] = LaunchdRuntime{PID: fields[0], Status: fields[1]}
	}
	return m
}

// parsePlistDaemon 轻量解析 plist 顶层 dict 的 Label/RunAtLoad/Disabled
// 三键（launchd 观测的全部所需），不引入第三方 plist 库（D10.2）：
// plist 是 key/值交替的 XML，复杂值（array/dict）整树跳过
func parsePlistDaemon(data []byte) (LaunchdService, error) {
	var svc LaunchdService
	dec := xml.NewDecoder(bytes.NewReader(data))
	state := 0 // 0 idle / 1 收 key 名 / 2 收标量值
	key := ""
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return svc, nil
		}
		if err != nil {
			return svc, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch {
			case state == 1:
				return svc, fmt.Errorf("unexpected <%s> inside key", t.Name.Local)
			case state == 2:
				switch t.Name.Local {
				case "true":
					setPlistKey(&svc, key, true)
					state = 0
				case "false":
					setPlistKey(&svc, key, false)
					state = 0
				case "array", "dict":
					if err := dec.Skip(); err != nil {
						return svc, err
					}
					state = 0
				}
				// string/integer/real/date/data：保持 state=2 等 CharData
			case t.Name.Local == "key":
				state = 1
			}
		case xml.CharData:
			txt := strings.TrimSpace(string(t))
			if txt == "" {
				continue
			}
			switch state {
			case 1:
				key = txt
				state = 2
			case 2:
				if key == "Label" {
					svc.Label = txt
				}
				state = 0
			}
		case xml.EndElement:
			// 标量值元素结束（如空 <string></string>）；key 元素的结束
			// 在 state=1→2 转换后到达，不在此复位
			if state == 2 && t.Name.Local != "key" {
				state = 0
			}
		}
	}
}

func setPlistKey(svc *LaunchdService, key string, v bool) {
	switch key {
	case "RunAtLoad":
		svc.RunAtLoad = v
	case "Disabled":
		svc.Disabled = v
	}
}
