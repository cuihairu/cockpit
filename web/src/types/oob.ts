// ========== 带外兜底转发编排（oob-openwrt-bootstrap.md 片C，字段对齐
// internal/server/api_oob_redirect.go 与 internal/oob/redirect） ==========

/** 下发转发规则规格（server oobRedirectSpec / agent redirect.Spec）。 */
export interface OobRedirectSpec {
  label: string
  proto: string
  src_zone: string
  dst_zone: string
  src_ip?: string
  dst_ip: string
  src_port?: number
  dst_port: number
  ttl_seconds: number
}

/** 下发成功返回：规则名与过期时刻由 agent 侧生成（过期账进规则名）。 */
export interface OobRedirectApplyResult {
  name: string
  expiry: number
  ttl_seconds: number
}

/** 设备当前转发规则（ubus 原样字段；name 是展示与拆除锚点）。 */
export interface OobRedirectRule {
  name: string
  [key: string]: unknown
}

export interface OobRedirectListResult {
  redirects: OobRedirectRule[]
}

/** 清扫返回：实际拆除的 cockpit 规则名清单。 */
export interface OobRedirectSweepResult {
  removed: string[]
}
