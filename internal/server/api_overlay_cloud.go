package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/overlay"
	"github.com/cuihairu/cockpit/internal/protocol"
)

// Overlay 云管理面 API（见 docs/guide/overlay-design.md M2，D11-D19）。
// server 直连 ZeroTier Central / Tailscale 控制面，agent 不参与；
// 云端成员/设备与 registry 内 agent 上报身份对照出 managed 标注（D17/D18）。
//
//	GET    /api/overlay/cloud                                          列表+对照（浏览不审计）
//	POST   /api/overlay/cloud/zerotier/networks/{nwid}/members/{mid}   授权/取消（审计 overlay_authz）
//	DELETE /api/overlay/cloud/zerotier/networks/{nwid}/members/{mid}   除名（审计 overlay_remove）
//	POST   /api/overlay/cloud/tailscale/devices/{devid}/authorize      授权（审计 overlay_authz）
//	DELETE /api/overlay/cloud/tailscale/devices/{devid}                除名（审计 overlay_remove）

// D19 ID 白名单（云 API 是出站调用，不校验会被当作请求路径注入）
var (
	ztNetworkIDRe = regexp.MustCompile(`^[0-9a-f]{16}$`)
	ztMemberIDRe  = regexp.MustCompile(`^[0-9a-f]{10}$`)
	tsDeviceIDRe  = regexp.MustCompile(`^[0-9]{1,20}$`)
)

// overlayCloudMemberOut / 设备行 = 云客户端白名单结构 + managed 标注
type overlayCloudMemberOut struct {
	overlay.ZeroTierMember
	Managed bool `json:"managed"`
}

type overlayCloudDeviceOut struct {
	overlay.TailscaleDevice
	Managed bool `json:"managed"`
}

// handleOverlayCloud 分发 /api/overlay/cloud 及子路径
func (s *Server) handleOverlayCloud(w http.ResponseWriter, r *http.Request) {
	sub := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/overlay/cloud"), "/")
	switch {
	case sub == "":
		if r.Method != http.MethodGet {
			s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.handleOverlayCloudList(w, r)
	case strings.HasPrefix(sub, "zerotier/networks/"):
		s.handleOverlayZTMember(w, r, strings.TrimPrefix(sub, "zerotier/networks/"))
	case strings.HasPrefix(sub, "tailscale/devices/"):
		s.handleOverlayTSDevice(w, r, strings.TrimPrefix(sub, "tailscale/devices/"))
	default:
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
	}
}

// handleOverlayCloudList GET /api/overlay/cloud——两 provider 一次拉齐，
// 未配置段 configured:false，单 provider 失败降级不整体 5xx（D13）
func (s *Server) handleOverlayCloudList(w http.ResponseWriter, r *http.Request) {
	ztIDs, tsIDs := s.overlayIdentitySets()

	resp := map[string]interface{}{}
	if s.overlayZT == nil {
		resp["zerotier"] = map[string]interface{}{"configured": false}
	} else {
		seg := map[string]interface{}{"configured": true, "error": ""}
		networks, err := s.overlayZT.Networks(r.Context())
		if err != nil {
			seg["error"] = overlayCloudErrSummary(err)
			networks = nil
		}
		out := make([]map[string]interface{}, 0, len(networks))
		for _, n := range networks {
			members := make([]overlayCloudMemberOut, 0, len(n.Members))
			for _, m := range n.Members {
				members = append(members, overlayCloudMemberOut{ZeroTierMember: m, Managed: ztIDs[m.ID]})
			}
			out = append(out, map[string]interface{}{
				"id": n.ID, "name": n.Name, "members": members,
			})
		}
		seg["networks"] = out
		resp["zerotier"] = seg
	}

	if s.overlayTS == nil {
		resp["tailscale"] = map[string]interface{}{"configured": false}
	} else {
		seg := map[string]interface{}{"configured": true, "error": "", "tailnet": s.overlayTS.TailnetName()}
		devices, err := s.overlayTS.Devices(r.Context())
		if err != nil {
			seg["error"] = overlayCloudErrSummary(err)
			devices = nil
		}
		out := make([]overlayCloudDeviceOut, 0, len(devices))
		for _, d := range devices {
			out = append(out, overlayCloudDeviceOut{TailscaleDevice: d, Managed: tsIDs[d.ID]})
		}
		seg["devices"] = out
		resp["tailscale"] = seg
	}

	s.writeJSON(w, http.StatusOK, resp)
}

// handleOverlayZTMember /zerotier/networks/{nwid}/members/{mid}
func (s *Server) handleOverlayZTMember(w http.ResponseWriter, r *http.Request, sub string) {
	// sub 形态：{nwid}/members/{mid}
	parts := strings.Split(strings.Trim(sub, "/"), "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] != "members" || parts[2] == "" {
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
		return
	}
	networkID, memberID := parts[0], parts[2]
	if !ztNetworkIDRe.MatchString(networkID) {
		s.handleError(w, r, http.StatusBadRequest, "invalid zerotier network id")
		return
	}
	if !ztMemberIDRe.MatchString(memberID) {
		s.handleError(w, r, http.StatusBadRequest, "invalid zerotier member id")
		return
	}
	if s.overlayZT == nil {
		s.overlayCloudNotConfigured(w, r, "zerotier")
		return
	}

	switch r.Method {
	case http.MethodPost:
		var body struct {
			Authorized *bool `json:"authorized"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil || body.Authorized == nil {
			s.handleError(w, r, http.StatusBadRequest, "body must be {\"authorized\": true|false}")
			return
		}
		if err := s.overlayZT.SetMemberAuthorized(r.Context(), networkID, memberID, *body.Authorized); err != nil {
			s.handleOverlayCloudUpstream(w, r, err)
			return
		}
		s.auditOverlayCloud(r, "overlay_authz",
			fmt.Sprintf("zerotier/%s/%s", networkID, memberID),
			map[string]interface{}{"provider": "zerotier", "authorized": *body.Authorized})
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
	case http.MethodDelete:
		if err := s.overlayZT.DeleteMember(r.Context(), networkID, memberID); err != nil {
			s.handleOverlayCloudUpstream(w, r, err)
			return
		}
		s.auditOverlayCloud(r, "overlay_remove",
			fmt.Sprintf("zerotier/%s/%s", networkID, memberID),
			map[string]interface{}{"provider": "zerotier"})
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
	default:
		s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleOverlayTSDevice /tailscale/devices/{devid}[/authorize]
func (s *Server) handleOverlayTSDevice(w http.ResponseWriter, r *http.Request, sub string) {
	parts := strings.Split(strings.Trim(sub, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
		return
	}
	deviceID := parts[0]
	authorize := len(parts) == 2 && parts[1] == "authorize"
	if len(parts) > 2 || (len(parts) == 2 && !authorize) {
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
		return
	}
	if !tsDeviceIDRe.MatchString(deviceID) {
		s.handleError(w, r, http.StatusBadRequest, "invalid tailscale device id")
		return
	}
	if s.overlayTS == nil {
		s.overlayCloudNotConfigured(w, r, "tailscale")
		return
	}

	switch {
	case r.Method == http.MethodPost && authorize:
		if err := s.overlayTS.AuthorizeDevice(r.Context(), deviceID); err != nil {
			s.handleOverlayCloudUpstream(w, r, err)
			return
		}
		s.auditOverlayCloud(r, "overlay_authz", "tailscale/"+deviceID,
			map[string]interface{}{"provider": "tailscale", "authorized": true})
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
	case r.Method == http.MethodDelete && !authorize:
		if err := s.overlayTS.DeleteDevice(r.Context(), deviceID); err != nil {
			s.handleOverlayCloudUpstream(w, r, err)
			return
		}
		s.auditOverlayCloud(r, "overlay_remove", "tailscale/"+deviceID,
			map[string]interface{}{"provider": "tailscale"})
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
	default:
		s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// overlayIdentitySets 从 registry 内全部 agent 的 overlay capability metadata
// 收集虚拟网身份，作为 CMDB 对照匹配键（D17/D18）。metadata 经 JSON 反序列化，
// 逐层防御性断言，形状不符一律忽略。
func (s *Server) overlayIdentitySets() (ztNodeIDs, tsDeviceIDs map[string]bool) {
	ztNodeIDs = map[string]bool{}
	tsDeviceIDs = map[string]bool{}
	if s.registry == nil {
		return ztNodeIDs, tsDeviceIDs
	}
	for _, agent := range s.registry.List() {
		nodeID, deviceID := overlayIdentityFromCapabilities(agent.GetCapabilities())
		if nodeID != "" {
			ztNodeIDs[nodeID] = true
		}
		if deviceID != "" {
			tsDeviceIDs[deviceID] = true
		}
	}
	return ztNodeIDs, tsDeviceIDs
}

// overlayIdentityFromCapabilities 提取单 agent 的 zerotier nodeId 与
// tailscale device id（D15 identity 结构，缺失/畸形返回空串）
func overlayIdentityFromCapabilities(caps []protocol.Capability) (nodeID, deviceID string) {
	for _, c := range caps {
		if c.Type != "overlay" {
			continue
		}
		identity, ok := c.Metadata["identity"].(map[string]interface{})
		if !ok {
			continue
		}
		if zt, ok := identity["zerotier"].(map[string]interface{}); ok {
			nodeID, _ = zt["nodeId"].(string)
		}
		if ts, ok := identity["tailscale"].(map[string]interface{}); ok {
			deviceID, _ = ts["id"].(string)
		}
	}
	return nodeID, deviceID
}

// overlayCloudNotConfigured 变更操作在 provider 未配置时统一 503 + 引导
// （指名缺哪个键，不含凭据值，D12）
func (s *Server) overlayCloudNotConfigured(w http.ResponseWriter, r *http.Request, provider string) {
	var msg string
	if provider == "zerotier" {
		msg = "Overlay cloud management not configured: set overlay.zerotier.api_token in config.yaml or ZEROTIER_API_TOKEN env"
	} else {
		msg = "Overlay cloud management not configured: set overlay.tailscale.api_token in config.yaml or TAILSCALE_API_TOKEN env"
	}
	s.handleError(w, r, http.StatusServiceUnavailable, msg)
}

// overlayCloudErrSummary 云端错误的展示摘要（UpstreamError 已截断 body，
// token 在请求头不回显，D12）
func overlayCloudErrSummary(err error) string {
	return err.Error()
}

// handleOverlayCloudUpstream 云端错误映射为网关错误（同 DNS 502 口径），
// 摘要不含请求上下文
func (s *Server) handleOverlayCloudUpstream(w http.ResponseWriter, r *http.Request, err error) {
	s.handleError(w, r, http.StatusBadGateway, overlayCloudErrSummary(err))
}

// auditOverlayCloud 云管理面变更审计（D14；details 只记布尔/名称，绝无 token）
func (s *Server) auditOverlayCloud(r *http.Request, action, resourceID string, details interface{}) {
	username := ""
	if userInfo, ok := auth.GetUserFromContext(r); ok {
		username = userInfo.Username
	}
	s.audit.LogResource(username, action, audit.ResourceOverlayMember,
		resourceID, details, s.getClientIP(r), r.UserAgent())
}
