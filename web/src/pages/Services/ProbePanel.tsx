import { useQuery } from '@tanstack/react-query'
import { Alert, Button, Card, Empty, Table, Tag, Tooltip, Typography } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import dayjs from 'dayjs'
import type { ColumnsType } from 'antd/es/table'
import { api } from '@/services/api'
import type { ProbeTargetSnapshot, ProbeWindow } from '@/types'

// 目标探活面板（服务检测 agent B6，docs/design/service-detect-agent.md）：
// 探针 agent（cockpit-probe-agent）上报的目标观测快照与故障窗口回查，
// 数据为 server 聚合的全局视图（跨全部 agent）。与上方的服务自愈面板
// （HealthPanel，按主机 systemd 单元）分区呈现——两套探测语义并存不混用。

// 三态 → Tag（healthy/faulty/unknown 与 core/healthprobe 一致）
const STATE_META: Record<string, { color: string; label: string }> = {
  healthy: { color: 'green', label: '正常' },
  faulty: { color: 'red', label: '故障' },
  unknown: { color: 'default', label: '冷区' },
}

const stateTag = (state: string) => {
  const meta = STATE_META[state] ?? { color: 'default', label: state || '未知' }
  return <Tag color={meta.color}>{meta.label}</Tag>
}

const fmtTime = (v: string) => (v ? dayjs(v).format('MM-DD HH:mm:ss') : '—')

// 故障持续时长：已关窗 = 结束-开始，进行中 = 现在-开始（随 30s 轮询推进）
const fmtDuration = (start: string, end: string | null) => {
  if (!start) return '—'
  const ms = dayjs(end ?? undefined).diff(dayjs(start))
  if (!Number.isFinite(ms) || ms < 0) return '—'
  const s = Math.floor(ms / 1000)
  const d = Math.floor(s / 86400)
  const h = Math.floor((s % 86400) / 3600)
  const m = Math.floor((s % 3600) / 60)
  if (d > 0) return `${d}d${h}h`
  if (h > 0) return `${h}h${m}m`
  if (m > 0) return `${m}m${s % 60}s`
  return `${s}s`
}

const ProbePanel = () => {
  const targetsQuery = useQuery({
    queryKey: ['probe-targets'],
    queryFn: () => api.getProbeTargets(),
    refetchInterval: 30_000,
  })
  const windowsQuery = useQuery({
    queryKey: ['probe-windows'],
    queryFn: () => api.getProbeWindows({ limit: 50 }),
    refetchInterval: 30_000,
  })
  // 主机名映射（与 Services 页共用 ['agents'] 缓存，不重复发请求）
  const agentsQuery = useQuery({ queryKey: ['agents'], queryFn: () => api.getAgents() })

  const hostnameOf = (agentId: string) => {
    const a = (agentsQuery.data ?? []).find((x) => x.id === agentId)
    if (a?.hostname) return a.hostname
    return agentId
  }

  const targets = targetsQuery.data?.targets ?? []
  const windows = windowsQuery.data?.windows ?? []
  const failed = targetsQuery.isError || windowsQuery.isError

  const targetColumns: ColumnsType<ProbeTargetSnapshot> = [
    { title: '目标', dataIndex: 'target', width: 200, ellipsis: true },
    { title: 'Agent', width: 160, render: (_, r) => hostnameOf(r.agentId), ellipsis: true },
    { title: '状态', dataIndex: 'state', width: 90, render: (v: string) => stateTag(v) },
    {
      title: '状态起自',
      dataIndex: 'since',
      width: 150,
      render: fmtTime,
    },
    {
      title: '最近探测',
      dataIndex: 'lastChecked',
      width: 150,
      render: fmtTime,
    },
    {
      title: '最近错误',
      dataIndex: 'lastError',
      ellipsis: true,
      render: (v: string) =>
        v ? (
          <Tooltip title={v}>
            <Typography.Text type="danger" code style={{ fontSize: 12 }}>
              {v}
            </Typography.Text>
          </Tooltip>
        ) : (
          '—'
        ),
    },
  ]

  const windowColumns: ColumnsType<ProbeWindow> = [
    { title: '目标', dataIndex: 'target', width: 200, ellipsis: true },
    { title: 'Agent', width: 160, render: (_, r) => hostnameOf(r.agentId), ellipsis: true },
    { title: '开始', dataIndex: 'startedAt', width: 150, render: fmtTime },
    {
      title: '结束',
      dataIndex: 'endedAt',
      width: 150,
      render: (v: string | null) =>
        v ? (
          fmtTime(v)
        ) : (
          <Tag color="red" data-testid="window-open">
            故障中
          </Tag>
        ),
    },
    {
      title: '持续',
      width: 110,
      render: (_, r) => fmtDuration(r.startedAt, r.endedAt),
    },
    {
      title: '最近错误',
      dataIndex: 'lastError',
      ellipsis: true,
      render: (v: string) =>
        v ? (
          <Tooltip title={v}>
            <Typography.Text type="danger" code style={{ fontSize: 12 }}>
              {v}
            </Typography.Text>
          </Tooltip>
        ) : (
          '—'
        ),
    },
  ]

  const refresh = () => {
    void targetsQuery.refetch()
    void windowsQuery.refetch()
  }

  return (
    <Card
      title="目标探活（探针 agent）"
      extra={
        <Button icon={<ReloadOutlined />} onClick={refresh} size="small" aria-label="刷新探活数据" />
      }
    >
      {failed && (
        <Alert
          type="warning"
          showIcon
          message="探活数据加载失败"
          description="探针目标快照或故障窗口回查失败，请稍后重试"
          style={{ marginBottom: 12 }}
        />
      )}
      <Table<ProbeTargetSnapshot>
        rowKey="id"
        size="small"
        columns={targetColumns}
        dataSource={targets}
        loading={targetsQuery.isLoading}
        pagination={false}
        locale={{
          emptyText: (
            <Empty
              description="暂无探针目标——部署 cockpit-probe-agent 并配置 probe.yaml 后自动上报"
              image={Empty.PRESENTED_IMAGE_SIMPLE}
            />
          ),
        }}
      />
      <Typography.Title level={5} style={{ marginTop: 16 }}>
        近期故障窗口
      </Typography.Title>
      <Table<ProbeWindow>
        rowKey="id"
        size="small"
        columns={windowColumns}
        dataSource={windows}
        loading={windowsQuery.isLoading}
        pagination={{ pageSize: 10, hideOnSinglePage: true, showSizeChanger: false }}
        locale={{ emptyText: '暂无故障窗口记录' }}
      />
    </Card>
  )
}

export default ProbePanel
