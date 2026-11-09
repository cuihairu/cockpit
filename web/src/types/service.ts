// ============ systemd 服务管理（见 docs/guide/service-design.md） ============

// 一个 *.service unit 的观测快照
export interface ServiceUnit {
  name: string
  description: string
  loadState: string
  activeState: string
  subState: string
  unitFileState: string // enabled/disabled/static/... （自启态）
  preset: string
}

// service.list 返回
export interface ServiceListResult {
  services: ServiceUnit[]
}

// service.status：概览
export interface ServiceStatus {
  systemState: string
  total: number
  active: number
  failed: number
  enabled: number
}

// 服务管理动作（白名单，双端同规则；mask/unmask 仅 systemd 后端）
export type ServiceActionName = 'start' | 'stop' | 'restart' | 'reload' | 'enable' | 'disable' | 'mask' | 'unmask'

// service.unitfile 返回（D13）：systemctl cat 有效视图
export interface ServiceUnitFile {
  name: string
  fragmentPath: string
  content: string
}

// service.unitsave 返回：包管文件保存时 path 为 /etc 覆盖位
export interface ServiceUnitFileSaveResult {
  name: string
  path: string
  reloaded: boolean
}
