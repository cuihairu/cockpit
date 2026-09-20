package server

// rbac.go 路由权限判定层（rbac-design.md 笔 2）：全局中间件形态，挂在
// AuditMiddleware 内层（403 也被审计记录）、各 auth 挂点外层——自身解析
// Bearer token 取角色，无/坏 token 放行给内层 auth 出 401（认证优先于
// 鉴权）。每请求查库（D6），角色缺失 fail-closed 403；隐含规则同 resource
// 下 write ⊇ read、admin ⊇ write。

import (
	"net/http"
	"strings"

	"github.com/cuihairu/cockpit/internal/auth"
)

// resourceRule path 前缀 → resource；fixed 非空时忽略 method 推导恒用该
// action：audit/logs 的动作全是读（导出/检索也是 POST）；terminal 只有
// write（GET 列表同样按 write 门禁，D11 语义）。
type resourceRule struct {
	prefix string
	res    string
	fixed  string
}

// resourceRules 有序：长前缀在前，先命中先赢。子路径归并决策：
// probe/notification → alerts（拨测配置含告警阈值）、metrics → inventory
// （基础设施观测）、proxies → proxy、server-backups → backup、
// remote/desktop/vnc → terminal。
var resourceRules = []resourceRule{
	{"/api/docker", "docker", ""},
	{"/api/stacks", "stack", ""},
	{"/api/backups", "backup", ""},
	{"/api/server-backups", "backup", ""},
	{"/api/probe", "alerts", ""},
	{"/api/notification", "alerts", ""},
	{"/api/proxies", "proxy", ""},
	{"/api/metrics", "inventory", ""},
	{"/api/admin/audit", "audit", "read"},
	{"/api/remote", "terminal", "write"},
	{"/api/desktop", "terminal", "write"},
	{"/api/vnc", "terminal", "write"},
	{"/api/recordings", "recordings", ""},
	{"/api/drift/config", "drift", ""},
	{"/api/inventory/consistency", "inventory", ""},
	{"/api/smart/config", "smart", ""},
	{"/api/nas/config", "nas", ""},
	{"/api/logs/search", "logs", "read"},
	{"/api/dns", "dns", ""},
	{"/api/domains", "dns", ""},
	{"/api/overlay/cloud", "overlay", ""},
	{"/api/ddns", "ddns", ""},
	{"/api/acme", "acme", ""},
	{"/api/resources/", "inventory", ""},
	{"/api/alerts", "alerts", ""},
	{"/api/agents", "inventory", ""},
}

// agentSubResources /api/agents/{id}/{sub}/... 的 sub → resource
// （裸 /api/agents/{id} 详情走 inventory）
var agentSubResources = map[string]string{
	"files/":    "files",
	"proxy/":    "proxy",
	"cron/":     "cron",
	"services/": "services",
	"nas/":      "nas",
	"logs/":     "logs",
	"drift/":    "drift",
	"domains":   "dns", // D7 只读清单/片段
	"overlay/":  "overlay",
	"smart/":    "smart",
}

// requiredPerms 返回 path+method 所需权限点与是否归 RBAC 管：
// governed=false（非 /api/ 或未登记路径）放行，未登记路径由 handler 404
// 兜底；perms 空切片 = 登录即可（/me、/status、认证流程）。
func requiredPerms(path, method string) (perms []string, governed bool) {
	if !strings.HasPrefix(path, "/api/") {
		return nil, false
	}
	switch {
	case path == "/api/status",
		path == "/api/me" || strings.HasPrefix(path, "/api/me/"),
		strings.HasPrefix(path, "/api/auth/"):
		return nil, true
	case path == "/api/settings":
		return []string{"settings:admin"}, true
	case path == "/api/users" || strings.HasPrefix(path, "/api/users/"):
		return []string{"users:admin"}, true
	}
	// acme 签发/部署归 acme:admin（D3 约定：签发/吊销是高危动作）
	if method == http.MethodPost && strings.HasPrefix(path, "/api/acme/") &&
		(strings.HasSuffix(path, "/issue") || strings.HasSuffix(path, "/deploy")) {
		return []string{"acme:admin"}, true
	}
	// 域名绑定写操作同时要 dns 与 proxy 写权限（domain-binding D9：
	// 联动了谁就要谁的权限；读沿用 dns:read）
	if path == "/api/domains" || strings.HasPrefix(path, "/api/domains/") {
		if !isReadMethod(method) {
			return []string{"dns:write", "proxy:write"}, true
		}
	}
	if rest, ok := strings.CutPrefix(path, "/api/agents/"); ok && rest != "" {
		// agent 密钥重置是管理动作
		if strings.HasSuffix(rest, "/secret") {
			return []string{"inventory:write"}, true
		}
		for sub, res := range agentSubResources {
			if strings.Contains(rest, "/"+sub) {
				return actionPerm(res, method, ""), true
			}
		}
		return actionPerm("inventory", method, ""), true
	}
	for _, rule := range resourceRules {
		if strings.HasPrefix(path, rule.prefix) {
			return actionPerm(rule.res, method, rule.fixed), true
		}
	}
	return nil, false
}

func isReadMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead
}

func actionPerm(res, method, fixed string) []string {
	action := fixed
	if action == "" {
		if isReadMethod(method) {
			action = "read"
		} else {
			action = "write"
		}
	}
	return []string{res + ":" + action}
}

// roleCovers 权限清单是否覆盖需求（隐含规则见文件头）
func roleCovers(have []string, need string) bool {
	nRes, nAct, ok := strings.Cut(need, ":")
	if !ok {
		return false
	}
	rank := map[string]int{"read": 1, "write": 2, "admin": 3}
	want := rank[nAct]
	if want == 0 {
		return false
	}
	for _, p := range have {
		hRes, hAct, _ := strings.Cut(p, ":")
		if hRes == nRes && rank[hAct] >= want {
			return true
		}
	}
	return false
}

// userHasPerm 当前请求用户（context）是否持有权限点（每请求查库）。
// RBAC 拦截之外的细粒度判定用（如改密码免验旧密码的条件）。
func (s *Server) userHasPerm(r *http.Request, perm string) bool {
	user, ok := auth.GetUserFromContext(r)
	if !ok {
		return false
	}
	role, err := s.db.GetRole(user.Role)
	if err != nil {
		return false
	}
	return roleCovers(role.Permissions, perm)
}

// RBACMiddleware 全局权限判定（AuditMiddleware 内层：403 也进审计链）
func (s *Server) RBACMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		perms, governed := requiredPerms(r.URL.Path, r.Method)
		if governed && len(perms) > 0 && !s.permitted(w, r, perms) {
			return
		}
		next.ServeHTTP(w, r)
	})
}

// permitted 校验 Bearer 角色覆盖 perms。无/坏 token 放行（内层 auth 出
// 401）；角色在角色表缺失 fail-closed 403；权限不足 403。
func (s *Server) permitted(w http.ResponseWriter, r *http.Request, perms []string) bool {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return true
	}
	claims, err := s.authService().ValidateToken(strings.TrimPrefix(header, "Bearer "))
	if err != nil {
		return true
	}
	role, err := s.db.GetRole(claims.Role)
	if err != nil {
		http.Error(w, `{"error":"Unknown role"}`, http.StatusForbidden)
		return false
	}
	for _, need := range perms {
		if !roleCovers(role.Permissions, need) {
			http.Error(w, `{"error":"Insufficient permissions"}`, http.StatusForbidden)
			return false
		}
	}
	return true
}
