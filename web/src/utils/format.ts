// 通用格式化：字节/时长/百分比。全站唯一实现，页面不得本地重复定义
// （规则：B 整数、KB/MB 一位小数、GB/TB 两位小数）。

export function formatBytes(n: number): string {
  if (!n) return '0 B'
  if (n < 1024) return `${n} B`
  if (n < 1024 ** 2) return `${(n / 1024).toFixed(1)} KB`
  if (n < 1024 ** 3) return `${(n / 1024 ** 2).toFixed(1)} MB`
  if (n < 1024 ** 4) return `${(n / 1024 ** 3).toFixed(2)} GB`
  return `${(n / 1024 ** 4).toFixed(2)} TB`
}

// 毫秒 → 紧凑时长（35s / 12m05s / 1h30m）。0 值语义（如「进行中」）由调用方兜底。
export function formatDuration(ms: number): string {
  const s = Math.round(ms / 1000)
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m}m${s % 60}s`
  return `${Math.floor(m / 60)}h${m % 60}m`
}

// 秒 → 中文运行时长（3天 4小时 / 5小时 12分钟）
export function formatUptime(seconds: number): string {
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  if (days > 0) return `${days}天 ${hours}小时`
  if (hours > 0) return `${hours}小时 ${minutes}分钟`
  return `${minutes}分钟`
}

export function formatPercent(value: number): string {
  return `${value.toFixed(1)}%`
}

// Unix 秒（Agent.lastSeen）→ 离线时长（1 分钟 / 3 小时 / 2 天）。
// 刚离线按 1 分钟起计（不出「0 分钟」）；lastSee 缺失/为 0 返回空串，
// 由调用方决定是否拼接到「离线」标签后。
export function formatOfflineDuration(lastSeen: number | undefined): string {
  // Number() 归一：容忍字符串形态的时间戳（'0' 字符串是 truthy，不能走 !lastSeen）
  const ts = Number(lastSeen)
  if (!ts) return ''
  const seconds = Math.max(0, Date.now() / 1000 - ts)
  const minutes = Math.max(1, Math.floor(seconds / 60))
  if (minutes < 60) return `${minutes} 分钟`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours} 小时`
  return `${Math.floor(hours / 24)} 天`
}
