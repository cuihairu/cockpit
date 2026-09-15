import { Tooltip } from 'antd'
import type { ProbeResult } from '@/types'

// HeartbeatBar 心跳条：最近 N 轮探测结果的可视化（Uptime Kuma 式）。
// 数据按 checkedAt 倒序传入（API 约定），内部反转为时间升序、左旧右新。
// 颜色语义：绿=正常，红=失败，橙=降级/临期，灰=无数据。

const HEARTBEAT_COLORS: Record<string, string> = {
  up: '#52c41a',
  active: '#52c41a',
  valid: '#52c41a',
  down: '#ff4d4f',
  error: '#ff4d4f',
  expired: '#ff4d4f',
  degraded: '#faad14',
  expiring: '#faad14',
}

// unknown 占位色与空槽位色（亮暗主题通用）
const UNKNOWN_COLOR = '#bfbfbf'
const EMPTY_COLOR = 'var(--hb-empty, rgba(128, 128, 128, 0.25))'

const statusLabel: Record<string, string> = {
  up: '正常',
  active: '正常',
  valid: '有效',
  down: '宕机',
  error: '错误',
  expired: '已过期',
  degraded: '降级',
  expiring: '即将过期',
}

const formatDot = (d: Date) =>
  `${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')} ` +
  `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`

interface HeartbeatBarProps {
  results: ProbeResult[]
  slots?: number
  loading?: boolean
}

export const HeartbeatBar: React.FC<HeartbeatBarProps> = ({ results, slots = 30, loading }) => {
  if (loading) {
    return <span style={{ color: UNKNOWN_COLOR, fontSize: 12 }}>…</span>
  }
  if (!results || results.length === 0) {
    return <span style={{ color: UNKNOWN_COLOR, fontSize: 12 }}>暂无记录</span>
  }

  // API 倒序 → 升序展示；只取最近 slots 条
  const recent = [...results].slice(0, slots).reverse()
  const empty = Math.max(0, slots - recent.length)

  const cellStyle = (color: string): React.CSSProperties => ({
    width: 6,
    height: 16,
    borderRadius: 2,
    background: color,
    flex: '0 0 auto',
  })

  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 2 }} aria-label="最近探测记录">
      {Array.from({ length: empty }, (_, i) => (
        <Tooltip key={`e${i}`} title="暂无记录">
          <div style={cellStyle(EMPTY_COLOR)} />
        </Tooltip>
      ))}
      {recent.map((r) => {
        const color = HEARTBEAT_COLORS[r.status] || UNKNOWN_COLOR
        const label = statusLabel[r.status] || r.status
        const tip = `${formatDot(new Date(r.checkedAt))} · ${label}` +
          (r.latencyMs ? ` · ${r.latencyMs}ms` : '') +
          (r.message ? ` · ${r.message}` : '')
        return (
          <Tooltip key={r.id} title={tip}>
            <div style={cellStyle(color)} />
          </Tooltip>
        )
      })}
    </div>
  )
}
