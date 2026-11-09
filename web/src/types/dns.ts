// ========== DNS 管理（Cloudflare，见 dns-design.md） ==========

// DNS 配置探测（只返回布尔，不含 token）
export interface DNSStatus {
  configured: boolean
  provider: string
}

// DNS 托管区（in_cmdb = 是否已登记在「资源 → 域名」）
export interface DNSZone {
  id: string
  name: string
  status: string
  name_servers: string[]
  in_cmdb: boolean
}

// 孤儿台账行摘要（dns-design M3 D20）：DNS 来源、但所属 zone 已不在
// provider 列表的 Domain 台账行，由用户经「移除登记」处置
export interface DNSOrphan {
  id: string
  domain: string
  zone_id: string
}

// zones 端点返回（data + 反向对账 orphans）
export interface DNSZonesResult {
  zones: DNSZone[]
  orphans: DNSOrphan[]
}

// DNS 记录
export interface DNSRecord {
  id: string
  type: string
  name: string
  content: string
  ttl: number
  proxied: boolean
  locked: boolean
}

// 创建/更新入参
export interface DNSRecordInput {
  type: string
  name: string
  content: string
  ttl: number
  proxied: boolean
}

// 记录分页结果
export interface DNSRecordsPage {
  records: DNSRecord[]
  page: number
  total_pages: number
}

// 批量导出结果（dns-design M4 D22：便携 RecordInput 形态，无 provider
// id/locked；文件可直接作导入输入，zone 归属由请求 URL 决定）
export interface DNSExport {
  zone_id: string
  provider: string
  count: number
  records: DNSRecordInput[]
}

// 导入单条失败明细（M4 D24：带下标，单条失败不中断整批）
export interface DNSImportFailedItem {
  index: number
  name: string
  error: string
}

// 批量导入结果（created/updated/skipped/failed 四分类）
export interface DNSImportResult {
  total: number
  created: number
  updated: number
  skipped: number
  failed: DNSImportFailedItem[]
}
