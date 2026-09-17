import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Alert, Badge, Card, Collapse, Descriptions, Empty, Space, Spin, Table, Tag, Tooltip, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { api } from '@/services/api'
import type { OverlayPeer, OverlayStatus, OverlayTool } from '@/types'

// Overlay 组网观测：ZeroTier/Tailscale/WireGuard/frp 运行态只读
// （见 docs/guide/overlay-design.md）。server 纯转发不落库，
// 「全网总览」由前端逐 agent 拉取后聚合——同节点多路径可见时
// 按延迟最低合并并保留来源徽标。

const TOOL_LABELS: Record<string, string> = {
  zerotier: 'ZeroTier',
  tailscale: 'Tailscale',
  wireguard: 'WireGuard',
  frp: 'frp',
}

const STATUS_COLORS: Record<string, string> = {
  ok: 'success',
  degraded: 'warning',
  error: 'error',
  unavailable: 'default',
}

const STATUS_LABELS: Record<string, string> = {
  ok: '正常',
  degraded: '降级',
  error: '错误',
  unavailable: '未安装',
}

const formatTime = (iso?: string) => {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toLocaleString()
}

// toolPeers 展平工具的 peers（含 wg 接口内 peers，来源标注接口名）
const toolPeers = (tool: OverlayTool): { peer: OverlayPeer; from?: string }[] => {
  const out: { peer: OverlayPeer; from?: string }[] = [...(tool.peers ?? []).map((p) => ({ peer: p }))]
  for (const iface of tool.interfaces ?? []) {
    for (const p of iface.peers ?? []) {
      out.push({ peer: p, from: iface.name })
    }
  }
  return out
}

// 全网总览行：同一对端（tool+id）跨 agent 合并，保留最低延迟
interface MeshRow {
  key: string
  tool: string
  id: string
  name: string
  agents: string[]
  ips: string[]
  latencyMs?: number
  online: boolean
  version: string
  detail: string
}

const buildMeshRows = (statuses: { agent: string; status: OverlayStatus }[]): MeshRow[] => {
  const byKey = new Map<string, MeshRow>()
  for (const { agent, status } of statuses) {
    for (const tool of status.tools ?? []) {
      if (tool.status === 'unavailable') continue
      for (const { peer, from } of toolPeers(tool)) {
        const key = `${tool.tool}:${peer.id}`
        const row = byKey.get(key) ?? {
          key,
          tool: tool.tool,
          id: peer.id,
          name: peer.name ?? '',
          agents: [],
          ips: [],
          latencyMs: undefined,
          online: false,
          version: peer.version ?? '',
          detail: '',
        }
        if (!row.agents.includes(agent)) row.agents.push(agent)
        if (!row.online && peer.online) row.online = true
        for (const ip of peer.virtualIps ?? []) {
          if (!row.ips.includes(ip)) row.ips.push(ip)
        }
        if (peer.latencyMs && (row.latencyMs === undefined || peer.latencyMs < row.latencyMs)) {
          row.latencyMs = peer.latencyMs
        }
        if (!row.name && peer.name) row.name = peer.name
        if (!row.version && peer.version) row.version = peer.version
        if (from && !row.detail) row.detail = from
        byKey.set(key, row)
      }
    }
  }
  return [...byKey.values()].sort((a, b) => Number(b.online) - Number(a.online) || a.tool.localeCompare(b.tool))
}

const meshColumns: ColumnsType<MeshRow> = [
  {
    title: '在线',
    dataIndex: 'online',
    key: 'online',
    width: 80,
    render: (v: boolean) => <Badge status={v ? 'success' : 'error'} text={v ? '在线' : '离线'} />,
  },
  {
    title: '节点',
    key: 'node',
    render: (_, r) => (
      <Space direction="vertical" size={0}>
        <Typography.Text strong>{r.name || r.id}</Typography.Text>
        {r.name && <Typography.Text type="secondary" code style={{ fontSize: 12 }}>{r.id}</Typography.Text>}
      </Space>
    ),
  },
  {
    title: '工具',
    dataIndex: 'tool',
    key: 'tool',
    width: 110,
    render: (v: string) => <Tag>{TOOL_LABELS[v] ?? v}</Tag>,
  },
  {
    title: '虚拟 IP',
    dataIndex: 'ips',
    key: 'ips',
    render: (ips: string[]) =>
      ips.length ? (
        <Typography.Text code style={{ fontSize: 12 }}>
          {ips.join(', ')}
        </Typography.Text>
      ) : (
        '—'
      ),
  },
  {
    title: '延迟',
    dataIndex: 'latencyMs',
    key: 'latencyMs',
    width: 90,
    render: (v?: number) => (v !== undefined ? `${v}ms` : '—'),
  },
  {
    title: '版本',
    dataIndex: 'version',
    key: 'version',
    width: 110,
    render: (v: string) => v || '—',
  },
  {
    title: '可见于',
    dataIndex: 'agents',
    key: 'agents',
    width: 160,
    render: (agents: string[]) => <Tag color="blue">{agents.join(', ')}</Tag>,
  },
]

// 单 agent 详情面板：工具卡 + 网络与 peers
const AgentOverlayPanel: React.FC<{ agentId: string }> = ({ agentId }) => {
  const { data, isLoading, isError } = useQuery({
    queryKey: ['overlay-status', agentId],
    queryFn: () => api.getOverlayStatus(agentId),
    staleTime: 30_000,
  })

  if (isLoading) {
    return (
      <div style={{ textAlign: 'center', padding: 24 }}>
        <Spin />
      </div>
    )
  }
  if (isError || !data) {
    return <Alert type="warning" showIcon message="该主机状态获取失败" />
  }

  const active = (data.tools ?? []).filter((t) => t.status !== 'unavailable')
  if (active.length === 0) {
    return <Empty description="未检测到组网工具" />
  }

  return (
    <Space direction="vertical" size={12} style={{ width: '100%' }}>
      {active.map((tool) => (
        <Card
          key={tool.tool}
          size="small"
          title={
            <Space>
              <span>{TOOL_LABELS[tool.tool] ?? tool.tool}</span>
              <Tag color={STATUS_COLORS[tool.status]}>{STATUS_LABELS[tool.status] ?? tool.status}</Tag>
              {tool.version && <Typography.Text type="secondary" style={{ fontSize: 12 }}>v{tool.version}</Typography.Text>}
            </Space>
          }
          extra={tool.error ? <Tooltip title={tool.error}><Typography.Text type="warning" style={{ fontSize: 12 }}>详情</Typography.Text></Tooltip> : undefined}
        >
          {(tool.networks?.length ?? 0) > 0 && (
            <Descriptions
              size="small"
              column={1}
              style={{ marginBottom: tool.peers?.length || tool.interfaces?.length ? 8 : 0 }}
              items={tool.networks!.map((n) => ({
                key: n.id,
                label: n.name || n.id,
                children: (
                  <Space wrap size={8}>
                    <Tag color={n.status === 'OK' || n.online ? 'success' : 'warning'}>{n.status ?? '—'}</Tag>
                    {(n.ips ?? []).map((ip) => (
                      <Typography.Text key={ip} code style={{ fontSize: 12 }}>{ip}</Typography.Text>
                    ))}
                  </Space>
                ),
              }))}
            />
          )}
          {(tool.interfaces?.length ?? 0) > 0 && (
            <Descriptions
              size="small"
              column={1}
              items={tool.interfaces!.map((i) => ({
                key: i.name,
                label: i.name,
                children: `${i.peerCount} 个 peer${i.listenPort ? ` · 监听 ${i.listenPort}` : ''}`,
              }))}
            />
          )}
          {toolPeers(tool).length > 0 && (
            <Table
              size="small"
              rowKey={(p) => `${tool.tool}:${p.peer.id}:${p.from ?? ''}`}
              dataSource={toolPeers(tool)}
              pagination={{ pageSize: 8, hideOnSinglePage: true }}
              columns={[
                { title: '对端', render: (_: unknown, r) => r.peer.name || r.peer.id, width: 180 },
                {
                  title: '地址',
                  render: (_: unknown, r) => (
                    <Typography.Text code style={{ fontSize: 12 }}>
                      {r.peer.virtualIps?.join(', ') || r.peer.endpoint || '—'}
                    </Typography.Text>
                  ),
                },
                {
                  title: '在线',
                  dataIndex: ['peer', 'online'],
                  width: 80,
                  render: (v: boolean) => <Badge status={v ? 'success' : 'error'} />,
                },
                { title: '延迟', render: (_: unknown, r) => (r.peer.latencyMs ? `${r.peer.latencyMs}ms` : '—'), width: 80 },
                {
                  title: '最近握手',
                  dataIndex: ['peer', 'lastHandshake'],
                  width: 160,
                  render: (v?: string) => <span style={{ fontSize: 12 }}>{formatTime(v)}</span>,
                },
              ]}
            />
          )}
        </Card>
      ))}
    </Space>
  )
}

const Network = () => {
  const { data: agents } = useQuery({ queryKey: ['agents'], queryFn: () => api.getAgents() })

  // 带 overlay capability 的在线 agent
  const overlayAgents = useMemo(
    () =>
      (agents ?? []).filter(
        (a) => (a.capabilities ?? []).some((c) => c.type === 'overlay') && a.status !== 'offline',
      ),
    [agents],
  )

  const statuses = useQuery({
    queryKey: ['overlay-mesh', overlayAgents.map((a) => a.id).join(',')],
    queryFn: async () => {
      const results = await Promise.all(
        overlayAgents.map(async (a) => ({ agent: a.hostname || a.id, status: await api.getOverlayStatus(a.id) })),
      )
      return results
    },
    enabled: overlayAgents.length > 0,
    staleTime: 30_000,
    retry: false,
  })

  const meshRows = useMemo(
    () => (statuses.data ? buildMeshRows(statuses.data) : []),
    [statuses.data],
  )

  const panelItems = overlayAgents.map((a) => ({
    key: a.id,
    label: `${a.hostname || a.id}${a.region ? ` · ${a.region}` : ''}`,
    children: <AgentOverlayPanel agentId={a.id} />,
  }))

  return (
    <div style={{ padding: 24 }}>
      <Card title="组网观测" style={{ marginBottom: 16 }}>
        <Alert
          type="info"
          showIcon
          style={{ marginBottom: 16 }}
          message="自动发现各主机上的 ZeroTier / Tailscale / WireGuard / frp 并汇总运行态，跨地域节点一屏可见。"
        />
        {overlayAgents.length === 0 ? (
          <Empty description="暂无带组网能力（overlay）的在线主机——在目标主机安装 ZeroTier / Tailscale / WireGuard / frp 任一工具并运行 Agent 即可" />
        ) : statuses.isLoading ? (
          <div style={{ textAlign: 'center', padding: 32 }}>
            <Spin tip="正在收集各主机组网状态…" />
          </div>
        ) : (
          <Table<MeshRow>
            rowKey="key"
            columns={meshColumns}
            dataSource={meshRows}
            pagination={{ pageSize: 15, hideOnSinglePage: true }}
            size="small"
            locale={{ emptyText: '各主机暂无对端节点' }}
          />
        )}
      </Card>

      {overlayAgents.length > 0 && (
        <Card title="按主机查看">
          <Collapse items={panelItems} />
        </Card>
      )}
    </div>
  )
}

export default Network
