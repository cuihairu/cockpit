import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Alert,
  Badge,
  Button,
  Card,
  Collapse,
  Descriptions,
  Empty,
  Input,
  message,
  Modal,
  Popconfirm,
  Segmented,
  Space,
  Spin,
  Switch,
  Table,
  Tag,
  Tooltip,
  Typography,
} from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { api } from '@/services/api'
import { PermGuard } from '@/components/PermGuard'
import type {
  Agent,
  OverlayAgentIdentity,
  OverlayCloudDevice,
  OverlayCloudMember,
  OverlayPeer,
  OverlayStatus,
  OverlayTool,
} from '@/types'

// Overlay 组网：M1 运行态只读观测 + M2 云端管理面（见 docs/guide/overlay-design.md）。
// 观测：server 纯转发不落库，「全网总览」由前端逐 agent 拉取后聚合——同节点
// 多路径可见时按延迟最低合并并保留来源徽标。
// 云端管理：server 直连 ZeroTier Central / Tailscale 控制面，managed 徽标由
// server 依 agent 上报身份对照（D17），前端只做展示与变更操作。

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

// agent 上报的虚拟网身份（D15/D16：注册时快照，重连刷新）
const agentIdentity = (a: Agent): OverlayAgentIdentity | undefined => {
  const cap = (a.capabilities ?? []).find((c) => c.type === 'overlay')
  const id = cap?.metadata?.identity
  return id && typeof id === 'object' ? (id as OverlayAgentIdentity) : undefined
}

// 身份 chip：本机 ZeroTier node id / Tailscale device id（与云端 member/device id 同键）
const IdentityChips: React.FC<{ identity?: OverlayAgentIdentity }> = ({ identity }) => {
  if (!identity) return null
  const chips: string[] = []
  if (identity.nodeId) chips.push(`ZeroTier ${identity.nodeId}`)
  if (identity.id) chips.push(`Tailscale ${identity.id}`)
  if (chips.length === 0) return null
  return (
    <Space wrap size={4}>
      <Typography.Text type="secondary" style={{ fontSize: 12 }}>
        本机身份（云端对照用）：
      </Typography.Text>
      {chips.map((c) => (
        <Tag key={c} style={{ fontSize: 12 }}>
          {c}
        </Tag>
      ))}
    </Space>
  )
}

// ============ 云端管理（M2，D11-D17） ============

// ManagedTag managed 徽标：面板内有 agent 上报同 id 身份
const ManagedTag: React.FC<{ managed: boolean }> = ({ managed }) =>
  managed ? (
    <Tooltip title="面板内有 Agent 上报了该身份（capability metadata.identity）">
      <Tag color="green">面板纳管</Tag>
    </Tooltip>
  ) : (
    <Tooltip title="云端可见但面板内没有对应 Agent——未纳管设备，或该主机 Agent 未升级到上报身份的版本">
      <Tag color="orange">未纳管</Tag>
    </Tooltip>
  )

const CloudPanel: React.FC = () => {
  const queryClient = useQueryClient()
  const { data, isLoading, isError } = useQuery({
    queryKey: ['overlay-cloud'],
    queryFn: () => api.getOverlayCloud(),
    staleTime: 30_000,
  })
  const refresh = () => queryClient.invalidateQueries({ queryKey: ['overlay-cloud'] })

  const ztAuthz = useMutation({
    mutationFn: (v: { networkId: string; memberId: string; authorized: boolean }) =>
      api.setOverlayZTMemberAuthorized(v.networkId, v.memberId, v.authorized),
    onSuccess: (_v, vars) => {
      message.success(vars.authorized ? '已授权' : '已取消授权')
      refresh()
    },
    onError: (e) => message.error(`授权失败：${String(e)}`),
  })
  const ztRemove = useMutation({
    mutationFn: (v: { networkId: string; memberId: string }) => api.removeOverlayZTMember(v.networkId, v.memberId),
    onSuccess: () => {
      message.success('已除名（设备重新加入后可再授权）')
      refresh()
    },
    onError: (e) => message.error(`除名失败：${String(e)}`),
  })
  const tsAuthorize = useMutation({
    mutationFn: (deviceId: string) => api.authorizeOverlayTSDevice(deviceId),
    onSuccess: () => {
      message.success('已授权')
      refresh()
    },
    onError: (e) => message.error(`授权失败：${String(e)}`),
  })
  const tsRemove = useMutation({
    mutationFn: (deviceId: string) => api.removeOverlayTSDevice(deviceId),
    onSuccess: () => {
      message.success('设备已删除')
      refresh()
      setTsDeleteTarget(null)
    },
    onError: (e) => message.error(`删除失败：${String(e)}`),
  })

  // Tailscale 删除确认（破坏性高于 ZT 除名：设备需重新登录）
  const [tsDeleteTarget, setTsDeleteTarget] = useState<OverlayCloudDevice | null>(null)
  const [tsDeleteInput, setTsDeleteInput] = useState('')
  const tsDeleteName = tsDeleteTarget?.name || tsDeleteTarget?.id || ''

  if (isLoading) {
    return (
      <div style={{ textAlign: 'center', padding: 32 }}>
        <Spin tip="正在拉取云端成员…" />
      </div>
    )
  }
  if (isError || !data) {
    return <Alert type="error" showIcon message="云端成员获取失败" />
  }

  const members = (data.zerotier.networks ?? []).flatMap((n) => n.members ?? [])
  const devices = data.tailscale.devices ?? []
  const unmanaged =
    members.filter((m) => !m.managed).length + devices.filter((d) => !d.managed).length

  const memberColumns = (networkId: string): ColumnsType<OverlayCloudMember> => [
    {
      title: '成员',
      key: 'member',
      render: (_, m) => (
        <Space direction="vertical" size={0}>
          <Typography.Text strong>{m.name || m.id}</Typography.Text>
          <Typography.Text type="secondary" code style={{ fontSize: 12 }}>
            {m.id}
          </Typography.Text>
        </Space>
      ),
    },
    {
      title: '授权',
      dataIndex: 'authorized',
      width: 90,
      render: (v: boolean, m) => (
        <PermGuard perm="overlay:write" fallback={<Switch size="small" checked={v} disabled />}>
          <Switch
            size="small"
            checked={v}
            loading={ztAuthz.isPending && ztAuthz.variables?.memberId === m.id}
            onChange={(checked) => ztAuthz.mutate({ networkId, memberId: m.id, authorized: checked })}
          />
        </PermGuard>
      ),
    },
    {
      title: '在线',
      dataIndex: 'online',
      width: 80,
      render: (v: boolean) => <Badge status={v ? 'success' : 'default'} text={v ? '在线' : '离线'} />,
    },
    {
      title: '虚拟 IP',
      dataIndex: 'ips',
      render: (ips?: string[]) =>
        ips?.length ? (
          <Typography.Text code style={{ fontSize: 12 }}>
            {ips.join(', ')}
          </Typography.Text>
        ) : (
          '—'
        ),
    },
    { title: '版本', dataIndex: 'version', width: 100, render: (v?: string) => v || '—' },
    {
      title: '最后在线',
      dataIndex: 'lastSeen',
      width: 160,
      render: (v?: string) => <span style={{ fontSize: 12 }}>{formatTime(v)}</span>,
    },
    { title: '对照', key: 'managed', width: 100, render: (_, m) => <ManagedTag managed={m.managed} /> },
    {
      title: '操作',
      key: 'action',
      width: 90,
      render: (_, m) => (
        <PermGuard perm="overlay:write">
          <Popconfirm
            title="除名该成员？"
            description="成员重新加入网络后可再次授权。"
            onConfirm={() => ztRemove.mutate({ networkId, memberId: m.id })}
          >
            <Button size="small" danger loading={ztRemove.isPending && ztRemove.variables?.memberId === m.id}>
              除名
            </Button>
          </Popconfirm>
        </PermGuard>
      ),
    },
  ]

  const deviceColumns: ColumnsType<OverlayCloudDevice> = [
    {
      title: '设备',
      key: 'device',
      render: (_, d) => (
        <Space direction="vertical" size={0}>
          <Typography.Text strong>{d.name || d.id}</Typography.Text>
          <Typography.Text type="secondary" code style={{ fontSize: 12 }}>
            {d.id}
          </Typography.Text>
        </Space>
      ),
    },
    {
      title: '地址',
      dataIndex: 'addresses',
      render: (v?: string[]) =>
        v?.length ? (
          <Typography.Text code style={{ fontSize: 12 }}>
            {v.join(', ')}
          </Typography.Text>
        ) : (
          '—'
        ),
    },
    { title: '归属', dataIndex: 'user', render: (v?: string) => v || '—' },
    { title: '系统', dataIndex: 'os', width: 90, render: (v?: string) => v || '—' },
    {
      title: '在线',
      dataIndex: 'online',
      width: 80,
      render: (v: boolean) => <Badge status={v ? 'success' : 'default'} text={v ? '在线' : '离线'} />,
    },
    {
      title: '授权',
      dataIndex: 'authorized',
      width: 110,
      render: (v: boolean, d) =>
        v ? (
          <Tag color="success">已授权</Tag>
        ) : (
          <PermGuard perm="overlay:write">
            <Button
              size="small"
              type="primary"
              loading={tsAuthorize.isPending && tsAuthorize.variables === d.id}
              onClick={() => tsAuthorize.mutate(d.id)}
            >
              授权
            </Button>
          </PermGuard>
        ),
    },
    {
      title: '密钥过期',
      dataIndex: 'keyExpiry',
      width: 150,
      render: (v?: string) => {
        if (!v) return '—'
        const expired = new Date(v).getTime() < Date.now()
        return (
          <span style={{ fontSize: 12, color: expired ? '#cf1322' : undefined }}>
            {formatTime(v)}
            {expired ? '（已过期）' : ''}
          </span>
        )
      },
    },
    { title: '对照', key: 'managed', width: 100, render: (_, d) => <ManagedTag managed={d.managed} /> },
    {
      title: '操作',
      key: 'action',
      width: 90,
      render: (_, d) => (
        <PermGuard perm="overlay:write">
          <Button
            size="small"
            danger
            onClick={() => {
              setTsDeleteTarget(d)
              setTsDeleteInput('')
            }}
          >
            删除
          </Button>
        </PermGuard>
      ),
    },
  ]

  return (
    <Space direction="vertical" size={16} style={{ width: '100%' }}>
      <Alert
        type={unmanaged > 0 ? 'warning' : 'info'}
        showIcon
        message={
          unmanaged > 0
            ? `云端共 ${members.length + devices.length} 台设备，其中 ${unmanaged} 台未纳管——云端可见但面板内无对应 Agent 上报身份。`
            : `云端共 ${members.length + devices.length} 台设备，全部与面板 Agent 身份对上。`
        }
      />

      {/* ZeroTier Central */}
      {!data.zerotier.configured ? (
        <Card size="small" title="ZeroTier Central（未配置）">
          <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
            在 config.yaml 设置 <Typography.Text code>overlay.zerotier.api_token</Typography.Text>
            ，或设置环境变量 <Typography.Text code>ZEROTIER_API_TOKEN</Typography.Text>
            （优先）。token 在 ZeroTier Central「Account → Generate Token」生成，member id
            与面板 Agent 上报的 node id 天然同键对照。
          </Typography.Paragraph>
        </Card>
      ) : (
        <Card size="small" title="ZeroTier Central">
          {data.zerotier.error && (
            <Alert type="warning" showIcon style={{ marginBottom: 12 }} message={`ZeroTier API 错误：${data.zerotier.error}`} />
          )}
          {(data.zerotier.networks?.length ?? 0) === 0 ? (
            <Empty description="无网络或拉取失败" />
          ) : (
            <Collapse
              defaultActiveKey={data.zerotier.networks!.map((n) => n.id)}
              items={data.zerotier.networks!.map((n) => ({
                key: n.id,
                label: `${n.name || n.id}（${n.members?.length ?? 0} 台成员）`,
                children: (
                  <Table<OverlayCloudMember>
                    rowKey="id"
                    size="small"
                    columns={memberColumns(n.id)}
                    dataSource={n.members ?? []}
                    pagination={false}
                    locale={{ emptyText: '暂无成员' }}
                  />
                ),
              }))}
            />
          )}
        </Card>
      )}

      {/* Tailscale */}
      {!data.tailscale.configured ? (
        <Card size="small" title="Tailscale（未配置）">
          <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
            在 config.yaml 设置 <Typography.Text code>overlay.tailscale.api_token</Typography.Text>
            ，或设置环境变量 <Typography.Text code>TAILSCALE_API_TOKEN</Typography.Text>
            （优先）；可选 <Typography.Text code>overlay.tailscale.tailnet</Typography.Text>{' '}
            指定 tailnet（默认 <Typography.Text code>-</Typography.Text> 即 token 所属默认
            tailnet）。token 在 Tailscale 管理台「Settings → Personal settings → Keys」生成。
          </Typography.Paragraph>
        </Card>
      ) : (
        <Card
          size="small"
          title={`Tailscale${data.tailscale.tailnet ? `（tailnet: ${data.tailscale.tailnet}）` : ''}`}
        >
          {data.tailscale.error && (
            <Alert type="warning" showIcon style={{ marginBottom: 12 }} message={`Tailscale API 错误：${data.tailscale.error}`} />
          )}
          <Table<OverlayCloudDevice>
            rowKey="id"
            size="small"
            columns={deviceColumns}
            dataSource={devices}
            pagination={{ pageSize: 15, hideOnSinglePage: true }}
            locale={{ emptyText: '暂无设备' }}
          />
        </Card>
      )}

      <Modal
        title="删除 Tailscale 设备"
        open={!!tsDeleteTarget}
        confirmLoading={tsRemove.isPending}
        onCancel={() => setTsDeleteTarget(null)}
        onOk={() => tsDeleteTarget && tsRemove.mutate(tsDeleteTarget.id)}
        okText="删除"
        okButtonProps={{ danger: true, disabled: tsDeleteInput !== tsDeleteName }}
      >
        <Typography.Paragraph type="warning">
          删除后该设备需重新登录才能回到网络。输入设备名 <Typography.Text strong>{tsDeleteName}</Typography.Text>{' '}
          以确认：
        </Typography.Paragraph>
        <Input value={tsDeleteInput} onChange={(e) => setTsDeleteInput(e.target.value)} placeholder={tsDeleteName} />
      </Modal>
    </Space>
  )
}

// ============ 本机侧管理（M3，D21-D30） ============

// 加入表单占位/提示按工具区分（D30）
const JOIN_HINTS: Record<string, { placeholder: string; hint: string }> = {
  zerotier: {
    placeholder: '16 位十六进制网络 ID',
    hint: '形如 8056c2e21c000001，在 ZeroTier Central 网络页可查。',
  },
  tailscale: {
    placeholder: 'tailnet 名或 -（不校验归属）',
    hint: 'Tailscale 无按网络加入的 CLI 动作：此处执行 tailscale up，需该机已完成登录（未登录会被拒绝并给引导）。',
  },
}

// ToolCard 单工具卡：观测内容 + M3 加入/离开动作。wireguard/frp 无加入
// 语义不出口（D25）；离开就近放在各自的网络行（ZT 逐网、TS 卡级 down）
const ToolCard: React.FC<{ agentId: string; tool: OverlayTool }> = ({ agentId, tool }) => {
  const queryClient = useQueryClient()
  const [joinOpen, setJoinOpen] = useState(false)
  const [netId, setNetId] = useState('')
  const manageable = tool.tool === 'zerotier' || tool.tool === 'tailscale'

  const change = useMutation({
    mutationFn: (v: { verb: 'join' | 'leave'; netId: string }) =>
      v.verb === 'join'
        ? api.joinAgentOverlayNetwork(agentId, v.netId, tool.tool)
        : api.leaveAgentOverlayNetwork(agentId, v.netId, tool.tool),
    onSuccess: (resp, vars) => {
      // D26：响应携带最新快照与身份——快照直接入缓存免二次请求，
      // agents 列表失效以刷新身份 chip
      if (resp?.status) queryClient.setQueryData(['overlay-status', agentId], resp.status)
      message.success(vars.verb === 'join' ? '已发起加入' : '已离开网络')
      queryClient.invalidateQueries({ queryKey: ['overlay-status', agentId] })
      queryClient.invalidateQueries({ queryKey: ['agents'] })
    },
    onError: (e) => message.error(`操作失败：${String(e)}`),
  })

  const submitJoin = () => {
    const id = netId.trim()
    if (!id) return
    change.mutate({ verb: 'join', netId: id })
    setJoinOpen(false)
    setNetId('')
  }

  const leaveButton = (netIdValue: string, name: string, description: string) => (
    <Popconfirm
      title={`离开 ${name}？`}
      description={description}
      okText="离开"
      onConfirm={() => change.mutate({ verb: 'leave', netId: netIdValue })}
    >
      <Button
        size="small"
        danger
        loading={change.isPending && change.variables?.verb === 'leave' && change.variables?.netId === netIdValue}
      >
        离开
      </Button>
    </Popconfirm>
  )

  const extra = manageable ? (
    <PermGuard perm="overlay:admin">
      <Space size={8}>
        <Button
          size="small"
          type="primary"
          ghost
          onClick={() => {
            setNetId('')
            setJoinOpen(true)
          }}
        >
          加入网络
        </Button>
        {tool.tool === 'tailscale' &&
          leaveButton('-', 'tailnet', '执行 tailscale down：断开连接但保留登录态，再次 up 即可回来。')}
      </Space>
    </PermGuard>
  ) : undefined

  return (
    <Card
      size="small"
      title={
        <Space>
          <span>{TOOL_LABELS[tool.tool] ?? tool.tool}</span>
          <Tag color={STATUS_COLORS[tool.status]}>{STATUS_LABELS[tool.status] ?? tool.status}</Tag>
          {tool.version && <Typography.Text type="secondary" style={{ fontSize: 12 }}>v{tool.version}</Typography.Text>}
        </Space>
      }
      extra={tool.error ? <Tooltip title={tool.error}><Typography.Text type="warning" style={{ fontSize: 12 }}>详情</Typography.Text></Tooltip> : extra}
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
                {/* ZeroTier 逐网络离开（D30）：网络 id 已知，Popconfirm 二次确认 */}
                {tool.tool === 'zerotier' && (
                  <PermGuard perm="overlay:admin">
                    {leaveButton(n.id, n.name || n.id, '离开后该主机将失去此网络的虚拟 IP。')}
                  </PermGuard>
                )}
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
      {manageable && (
        <Modal
          title={`加入网络（${TOOL_LABELS[tool.tool]}）`}
          open={joinOpen}
          onOk={submitJoin}
          okText="加入"
          cancelText="取消"
          okButtonProps={{ disabled: !netId.trim() }}
          onCancel={() => setJoinOpen(false)}
        >
          <Space direction="vertical" style={{ width: '100%' }} size={8}>
            <Input
              value={netId}
              onChange={(e) => setNetId(e.target.value)}
              onPressEnter={submitJoin}
              placeholder={JOIN_HINTS[tool.tool]?.placeholder}
              allowClear
            />
            <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
              {JOIN_HINTS[tool.tool]?.hint}
            </Typography.Paragraph>
          </Space>
        </Modal>
      )}
    </Card>
  )
}

// DaemonCard daemon/systemd 单元状态卡（D27/D30）：安装态 + 运行/自启徽标 +
// 缺装引导 + 启停/自启按钮。daemon 归 systemd，agent 只管理不宿主（D22）。
const DAEMON_ACTIONS: { action: string; label: string; danger?: boolean }[] = [
  { action: 'start', label: '启动' },
  { action: 'stop', label: '停止', danger: true },
  { action: 'enable', label: '开机自启' },
  { action: 'disable', label: '取消自启' },
]

const DaemonCard: React.FC<{ agentId: string }> = ({ agentId }) => {
  const queryClient = useQueryClient()
  const { data, isLoading, isError } = useQuery({
    queryKey: ['overlay-daemon', agentId],
    queryFn: () => api.getOverlayDaemon(agentId),
    staleTime: 30_000,
  })
  const service = useMutation({
    mutationFn: (v: { tool: string; action: string }) => api.agentOverlayService(agentId, v.tool, v.action),
    onSuccess: (r) => {
      message.success(`${r.unit} ${r.action} 完成`)
      queryClient.invalidateQueries({ queryKey: ['overlay-daemon', agentId] })
    },
    onError: (e) => message.error(`服务操作失败：${String(e)}`),
  })

  if (isLoading) {
    return (
      <Card size="small" title="Daemon 服务">
        <Spin />
      </Card>
    )
  }
  if (isError || !data) {
    return <Alert type="warning" showIcon message="Daemon 状态获取失败" />
  }
  const tools = data.tools ?? []
  if (tools.length === 0) return null

  return (
    <Card size="small" title="Daemon 服务">
      <Space direction="vertical" size={10} style={{ width: '100%' }}>
        {tools.map((t) => (
          <div key={t.tool}>
            <Space wrap size={8}>
              <Typography.Text strong>{TOOL_LABELS[t.tool] ?? t.tool}</Typography.Text>
              {t.installed ? (
                <>
                  {t.version && <Tag>v{t.version}</Tag>}
                  {t.unitExists === undefined ? (
                    <Tag>非 systemd 平台</Tag>
                  ) : t.unitExists ? (
                    <>
                      <Badge status={t.active ? 'success' : 'error'} text={t.active ? '运行中' : '已停止'} />
                      <Badge status={t.enabled ? 'processing' : 'default'} text={t.enabled ? '开机自启' : '未自启'} />
                    </>
                  ) : (
                    <Tag color="warning">systemd 单元缺失</Tag>
                  )}
                </>
              ) : (
                <Tag color="default">未安装</Tag>
              )}
              {t.installed && t.unitExists && (
                <PermGuard perm="overlay:admin">
                  <Space size={4}>
                    {DAEMON_ACTIONS.map((a) => (
                      <Button
                        key={a.action}
                        size="small"
                        danger={a.danger}
                        loading={
                          service.isPending &&
                          service.variables?.tool === t.tool &&
                          service.variables?.action === a.action
                        }
                        onClick={() => service.mutate({ tool: t.tool, action: a.action })}
                      >
                        {a.label}
                      </Button>
                    ))}
                  </Space>
                </PermGuard>
              )}
            </Space>
            {!t.installed && t.missingGuide && (
              <Alert type="warning" showIcon style={{ marginTop: 8 }} message={t.missingGuide} />
            )}
          </div>
        ))}
      </Space>
    </Card>
  )
}

// 单 agent 详情面板：身份 chip + daemon 卡 + 工具卡（观测 + M3 动作）
const AgentOverlayPanel: React.FC<{ agentId: string; identity?: OverlayAgentIdentity }> = ({
  agentId,
  identity,
}) => {
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

  return (
    <Space direction="vertical" size={12} style={{ width: '100%' }}>
      <IdentityChips identity={identity} />
      {/* daemon 卡不受 status 快照影响——未安装工具的引导也在这里（D30） */}
      <DaemonCard agentId={agentId} />
      {active.length === 0 ? (
        <Empty description="未检测到组网工具" />
      ) : (
        active.map((tool) => <ToolCard key={tool.tool} agentId={agentId} tool={tool} />)
      )}
    </Space>
  )
}

const Network = () => {
  const [view, setView] = useState<'mesh' | 'cloud'>('mesh')
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
    enabled: view === 'mesh' && overlayAgents.length > 0,
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
    children: <AgentOverlayPanel agentId={a.id} identity={agentIdentity(a)} />,
  }))

  return (
    <div style={{ padding: 24 }}>
      <Card
        title={
          <Space size={16}>
            <span>组网</span>
            <Segmented
              value={view}
              onChange={(v) => setView(v as 'mesh' | 'cloud')}
              options={[
                { label: '运行态观测', value: 'mesh' },
                { label: '云端管理', value: 'cloud' },
              ]}
            />
          </Space>
        }
        style={{ marginBottom: 16 }}
        extra={
          view === 'mesh' && (
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              自动发现 ZeroTier / Tailscale / WireGuard / frp，跨地域节点一屏可见
            </Typography.Text>
          )
        }
      >
        {view === 'mesh' ? (
          <>
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
          </>
        ) : (
          <CloudPanel />
        )}
      </Card>

      {view === 'mesh' && overlayAgents.length > 0 && (
        <Card title="按主机查看">
          <Collapse items={panelItems} />
        </Card>
      )}
    </div>
  )
}

export default Network
