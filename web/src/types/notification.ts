// 通知域类型
// 通知渠道摘要（不含 token/secret 凭据）
export interface NotificationChannelSummary {
  channel: string // herald / ntfy / webhook / telegram
  target: string
}

// 通知事件开关（config.yaml notification.events 透传）
export interface NotificationEventRow {
  type: string // service.down / service.up / certificate.expired ...
  enabled: boolean
}

// 通知服务状态
export interface NotificationStatus {
  enabled: boolean
  channels: NotificationChannelSummary[]
  events: NotificationEventRow[]
}

// 单渠道投递结果
export interface NotificationSendResult {
  channel: string
  target?: string
  ok: boolean
  error?: string
}
