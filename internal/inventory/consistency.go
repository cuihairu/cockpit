package inventory

import (
	"sort"
	"strings"

	"github.com/cuihairu/cockpit/internal/storage"
)

// CMDB 一致性比对（drift-design.md M6 D28-D29）：inventory 声明（期望态）
// vs agent 实报（实际态，注册时覆写 DB 行的 hostname/IP）。纯函数无 IO，
// 声明态由调用方从 YAML 加载。

// 一致性四态
const (
	ConsistencyOK           = "ok"
	ConsistencyMismatch     = "mismatch"
	ConsistencyUnregistered = "unregistered" // 声明了但库里没有（未上线/声明先于装机）
	ConsistencyUndeclared   = "undeclared"   // 库里有但 YAML 没声明（CMDB 完整性缺口）
)

// AgentFacts 一侧的 hostname/IP 事实快照
type AgentFacts struct {
	Hostname string `json:"hostname,omitempty"`
	IP       string `json:"ip,omitempty"`
}

// ConsistencyMismatch 字段级不一致明细
type ConsistencyFieldMismatch struct {
	Field    string `json:"field"` // hostname | ip
	Declared string `json:"declared"`
	Actual   string `json:"actual"`
}

// AgentConsistency 单 agent 一致性结果
type AgentConsistency struct {
	ID       string                     `json:"id"`
	Status   string                     `json:"status"`
	Declared *AgentFacts                `json:"declared,omitempty"`
	Actual   *AgentFacts                `json:"actual,omitempty"`
	Mismatch []ConsistencyFieldMismatch `json:"mismatch,omitempty"`
}

// ConsistencySummary 四态计数
type ConsistencySummary struct {
	Total        int `json:"total"`
	OK           int `json:"ok"`
	Mismatch     int `json:"mismatch"`
	Unregistered int `json:"unregistered"`
	Undeclared   int `json:"undeclared"`
}

// ConsistencyReport 完整一致性报告
type ConsistencyReport struct {
	Agents  []AgentConsistency `json:"agents"`
	Summary ConsistencySummary `json:"summary"`
}

// CompareAgents 比对声明与实报（D29）：仅声明非空才比对；hostname 大小写
// 不敏感（DNS 语义），IP 精确串匹配；结果按 id 稳定排序
func CompareAgents(inv *Inventory, agents []*storage.Agent) *ConsistencyReport {
	report := &ConsistencyReport{Agents: []AgentConsistency{}}
	declaredAgents := inv.GetAgents()

	actualByID := make(map[string]*storage.Agent, len(agents))
	for _, a := range agents {
		actualByID[a.ID] = a
	}

	seen := make(map[string]bool, len(declaredAgents))
	for id, loc := range declaredAgents {
		seen[id] = true
		declared := &AgentFacts{Hostname: strings.TrimSpace(loc.Hostname), IP: strings.TrimSpace(loc.IP)}
		ac := AgentConsistency{ID: id, Declared: declared}
		actual, exists := actualByID[id]
		if !exists {
			ac.Status = ConsistencyUnregistered
			report.Agents = append(report.Agents, ac)
			report.Summary.Unregistered++
			continue
		}
		ac.Actual = &AgentFacts{Hostname: strings.TrimSpace(actual.Hostname), IP: strings.TrimSpace(actual.IP)}
		if declared.Hostname != "" && !strings.EqualFold(declared.Hostname, ac.Actual.Hostname) {
			ac.Mismatch = append(ac.Mismatch, ConsistencyFieldMismatch{
				Field: "hostname", Declared: declared.Hostname, Actual: ac.Actual.Hostname,
			})
		}
		if declared.IP != "" && declared.IP != ac.Actual.IP {
			ac.Mismatch = append(ac.Mismatch, ConsistencyFieldMismatch{
				Field: "ip", Declared: declared.IP, Actual: ac.Actual.IP,
			})
		}
		if len(ac.Mismatch) > 0 {
			ac.Status = ConsistencyMismatch
			report.Summary.Mismatch++
		} else {
			ac.Status = ConsistencyOK
			report.Summary.OK++
		}
		report.Agents = append(report.Agents, ac)
	}

	for _, a := range agents {
		if seen[a.ID] {
			continue
		}
		report.Agents = append(report.Agents, AgentConsistency{
			ID:     a.ID,
			Status: ConsistencyUndeclared,
			Actual: &AgentFacts{Hostname: strings.TrimSpace(a.Hostname), IP: strings.TrimSpace(a.IP)},
		})
		report.Summary.Undeclared++
	}

	sort.Slice(report.Agents, func(i, j int) bool { return report.Agents[i].ID < report.Agents[j].ID })
	report.Summary.Total = len(report.Agents)
	return report
}
