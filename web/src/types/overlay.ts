// ========== Overlay 组网观测（见 overlay-design.md） ==========

// 组网对端节点（工具间字段取可用子集）
export interface OverlayPeer {
  id: string
  name?: string
  virtualIps?: string[]
  version?: string
  latencyMs?: number
  online: boolean
  endpoint?: string
  relay?: string
  role?: string
  lastHandshake?: string
}

// 本机加入的网络（ZeroTier/Tailscale）
export interface OverlayNetwork {
  id: string
  name?: string
  status?: string
  type?: string
  dev?: string
  online?: boolean
  ips?: string[]
}

// WireGuard 接口
export interface OverlayWGInterface {
  name: string
  listenPort?: string
  peerCount: number
  peers: OverlayPeer[]
}

// 单个组网工具的观测快照
export interface OverlayTool {
  tool: 'zerotier' | 'tailscale' | 'wireguard' | 'frp'
  status: 'ok' | 'degraded' | 'error' | 'unavailable'
  version?: string
  error?: string
  networks?: OverlayNetwork[]
  peers?: OverlayPeer[]
  interfaces?: OverlayWGInterface[]
  extra?: Record<string, unknown>
}

// overlay.status 返回
export interface OverlayStatus {
  tools: OverlayTool[]
}

// ========== Overlay 本机侧管理（M3，见 overlay-design.md D21-D30） ==========

// 单工具 daemon/systemd 单元状态（D27）；非 systemd 平台 unit 字段缺席
export interface OverlayDaemonTool {
  tool: 'zerotier' | 'tailscale'
  installed: boolean
  missingGuide?: string
  unitExists?: boolean
  active?: boolean
  enabled?: boolean
  version?: string
}

// overlay.daemon 返回
export interface OverlayDaemonStatus {
  tools: OverlayDaemonTool[]
}

// ========== Overlay 云管理面（M2，见 overlay-design.md D13/D17） ==========

// agent 上报的虚拟网身份（overlay capability metadata.identity，D15/D16）
export interface OverlayAgentIdentity {
  // zerotier 段
  nodeId?: string
  networks?: { id: string; name?: string; status?: string; addresses?: string[] }[]
  // tailscale 段
  id?: string
  hostName?: string
  dnsName?: string
  addresses?: string[]
}

// overlay.join/leave 响应（D26：携带刷新后的 status 快照与身份，
// 前端免二次请求）
export interface OverlayChangeResult {
  status: OverlayStatus
  identity?: OverlayAgentIdentity
}

// overlay.service 响应
export interface OverlayServiceResult {
  tool: string
  unit: string
  action: string
}

// 云端成员（ZeroTier Central）；managed = 面板内有 agent 上报同 id 身份（D17）
export interface OverlayCloudMember {
  id: string
  name?: string
  authorized: boolean
  online: boolean
  ips?: string[]
  version?: string
  lastSeen?: string
  managed: boolean
}

// 云端网络及其成员
export interface OverlayCloudNetwork {
  id: string
  name?: string
  members: OverlayCloudMember[]
}

// 云端设备（Tailscale）
export interface OverlayCloudDevice {
  id: string
  name?: string
  addresses?: string[]
  user?: string
  os?: string
  authorized: boolean
  online: boolean
  keyExpiry?: string
  lastSeen?: string
  managed: boolean
}

// GET /api/overlay/cloud 返回（未配置段 configured:false，失败段 error 非空）
export interface OverlayCloudStatus {
  zerotier: { configured: boolean; error?: string; networks?: OverlayCloudNetwork[] }
  tailscale: { configured: boolean; tailnet?: string; error?: string; devices?: OverlayCloudDevice[] }
}
