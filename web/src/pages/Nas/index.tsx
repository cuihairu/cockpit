import { useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Alert,
  Badge,
  Button,
  Card,
  Collapse,
  Empty,
  InputNumber,
  Space,
  Spin,
  Switch,
  Table,
  Tooltip,
  Typography,
  message,
} from 'antd'
import { DatabaseOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { api } from '@/services/api'
import type { Agent, NasMount, NasPool, NasShare, NasStatus } from '@/types'
import { getApiErrorMessage } from '@/utils/apiError'

// NAS 存储观测：各主机存储池/挂载/共享只读总览 + server 定时巡检告警
// （见 docs/guide/nas-design.md）。server 纯转发不落库，总览由前端
// 逐 agent 拉取后聚合（磁盘健康页同款）。

const POOL_META: Record<NasPool['state'], { label: string; status: 'success' | 'error' | 'warning' | 'processing' | 'default' }> = {
  healthy: { label: '健康', status: 'success' },
  degraded: { label: '降级', status: 'error' },
  resync: { label: '同步中', status: 'processing' },
  failed: { label: '故障', status: 'error' },
  unknown: { label: '未知', status: 'default' },
}

const KIND_LABEL: Record<NasPool['kind'], string> = {
  mdadm: 'mdadm',
  zfs: 'ZFS',
  lvm: 'LVM',
}

const SHARE_PROTOCOL_LABEL: Record<NasShare['protocol'], string> = {
  smb: 'SMB',
  nfs: 'NFS',
}

const formatGB = (gb?: number) => {
  if (gb === undefined || gb <= 0) return '—'
  return gb >= 1024 ? `${(gb / 1024).toFixed(1)} TB` : `${Math.round(gb)} GB`
}

// 行模型：单主机面板不带 agent，总览跨主机平铺时填主机名
interface PoolRow extends NasPool {
  key: string
  agent?: string
}

interface MountRow extends NasMount {
  key: string
  agent?: string
}

interface ShareRow extends NasShare {
  key: string
  agent?: string
}

// 池异常严重度：failed > degraded/resync > unknown > healthy
const poolSeverity = (state: NasPool['state']) =>
  state === 'failed' ? 0 : state === 'degraded' || state === 'resync' ? 1 : state === 'unknown' ? 2 : 3

// 单主机详情（Collapse 内）：池/挂载/共享三段
const AgentNasPanel: React.FC<{ agentId: string }> = ({ agentId }) => {
  const { data, isLoading, isError } = useQuery({
    queryKey: ['nas-status', agentId],
    queryFn: () => api.getNASStatus(agentId),
    staleTime: 30_000,
    retry: false,
  })

  if (isLoading) {
    return (
      <div style={{ textAlign: 'center', padding: 24 }}>
        <Spin />
      </div>
    )
  }
  if (isError || !data) {
    return <Alert type="warning" showIcon message="该主机存储状态获取失败" />
  }
  if (!data.available) {
    return <Empty description="未发现可观测的存储（mdadm/ZFS/LVM/SMB/NFS 均无数据）" />
  }

  const poolRows: PoolRow[] = (data.pools ?? []).map((p) => ({ ...p, key: `${p.kind}:${p.name}` }))
  const mountRows: MountRow[] = (data.mounts ?? []).map((m) => ({ ...m, key: `${m.device}:${m.mountPath}` }))
  const shareRows: ShareRow[] = (data.shares ?? []).map((s) => ({ ...s, key: `${s.protocol}:${s.path}` }))

  return (
    <Space direction="vertical" style={{ width: '100%' }} size={16}>
      <PoolTable dataSource={poolRows} showAgent={false} />
      <MountTable dataSource={mountRows} showAgent={false} />
      {shareRows.length > 0 && <ShareTable dataSource={shareRows} showAgent={false} />}
    </Space>
  )
}

// 池表（总览与单主机共用）
const PoolTable: React.FC<{ dataSource: PoolRow[]; showAgent?: boolean }> = ({
  dataSource,
  showAgent = true,
}) => {
  const columns: ColumnsType<PoolRow> = [
    {
      title: '状态',
      dataIndex: 'state',
      width: 100,
      render: (_: unknown, r) => {
        const meta = POOL_META[r.state] ?? POOL_META.unknown
        const badge = <Badge status={meta.status} text={meta.label} />
        return r.detail && r.state !== 'healthy' ? (
          <Tooltip title={r.detail}>
            <span>{badge}</span>
          </Tooltip>
        ) : (
          badge
        )
      },
    },
    ...(showAgent ? [{ title: '主机', dataIndex: 'agent', width: 150, ellipsis: true } as const] : []),
    { title: '存储池', dataIndex: 'name', width: 140, render: (v: string) => <Typography.Text code>{v}</Typography.Text> },
    { title: '类型', dataIndex: 'kind', width: 90, render: (v: NasPool['kind']) => KIND_LABEL[v] ?? v },
    { title: '容量', dataIndex: 'totalGB', width: 110, render: (v?: number) => formatGB(v) },
    { title: '成员盘', dataIndex: 'devices', ellipsis: true, render: (v?: string[]) => (v && v.length > 0 ? v.join('、') : '—') },
  ]
  return (
    <Table<PoolRow>
      size="small"
      rowKey="key"
      dataSource={dataSource}
      pagination={false}
      columns={columns}
      locale={{ emptyText: '未发现存储池' }}
    />
  )
}

const MountTable: React.FC<{ dataSource: MountRow[]; warnPct?: number; showAgent?: boolean }> = ({
  dataSource,
  warnPct,
  showAgent = true,
}) => {
  const columns: ColumnsType<MountRow> = [
    ...(showAgent ? [{ title: '主机', dataIndex: 'agent', width: 150, ellipsis: true } as const] : []),
    { title: '挂载点', dataIndex: 'mountPath', width: 200, render: (v: string) => <Typography.Text code>{v}</Typography.Text> },
    { title: '设备', dataIndex: 'device', width: 160, ellipsis: true, render: (v: string) => <Typography.Text code>{v}</Typography.Text> },
    { title: '文件系统', dataIndex: 'fsType', width: 100 },
    { title: '容量', dataIndex: 'totalGB', width: 110, render: (v?: number) => formatGB(v) },
    {
      title: '已用',
      key: 'usage',
      width: 180,
      render: (_: unknown, r) => {
        if (!r.totalGB || r.totalGB <= 0) return '—'
        const pct = Math.round((r.usedGB ?? 0) / r.totalGB * 100)
        const hot = warnPct !== undefined && pct >= warnPct
        return (
          <span style={hot ? { color: '#cf1322', fontWeight: 600 } : undefined}>
            {pct}%（{formatGB(r.usedGB)}）
          </span>
        )
      },
    },
  ]
  return (
    <Table<MountRow>
      size="small"
      rowKey="key"
      dataSource={dataSource}
      pagination={false}
      columns={columns}
      locale={{ emptyText: '未发现挂载点' }}
    />
  )
}

const ShareTable: React.FC<{ dataSource: ShareRow[]; showAgent?: boolean }> = ({
  dataSource,
  showAgent = true,
}) => {
  const columns: ColumnsType<ShareRow> = [
    ...(showAgent ? [{ title: '主机', dataIndex: 'agent', width: 150, ellipsis: true } as const] : []),
    { title: '协议', dataIndex: 'protocol', width: 80, render: (v: NasShare['protocol']) => SHARE_PROTOCOL_LABEL[v] ?? v },
    { title: '名称', dataIndex: 'name', width: 160, render: (v: string) => <Typography.Text code>{v}</Typography.Text> },
    { title: '路径', dataIndex: 'path', ellipsis: true, render: (v: string) => <Typography.Text code>{v}</Typography.Text> },
    { title: '说明', dataIndex: 'comment', ellipsis: true, render: (v?: string) => v || '—' },
    { title: '允许主机', dataIndex: 'hosts', ellipsis: true, render: (v?: string) => v || '—' },
  ]
  return (
    <Table<ShareRow>
      size="small"
      rowKey="key"
      dataSource={dataSource}
      pagination={false}
      columns={columns}
      locale={{ emptyText: '未发现网络共享' }}
    />
  )
}

const Nas = () => {
  const { data: agents = [] } = useQuery({ queryKey: ['agents'], queryFn: () => api.getAgents() })
  const queryClient = useQueryClient()

  // 巡检设置编辑态：改动才落本地，未编辑从服务端派生（与 Disk 页同模式）
  const [scanEdit, setScanEdit] = useState<{ on?: boolean; minutes?: number; warnPct?: number } | null>(null)
  const [savingScan, setSavingScan] = useState(false)

  const { data: scanCfg } = useQuery({ queryKey: ['nas-config'], queryFn: () => api.getNASConfig() })

  const savedInterval = scanCfg?.scan_interval_seconds ?? 0
  const scanOn = scanEdit?.on ?? savedInterval > 0
  const scanMinutes = scanEdit?.minutes ?? (savedInterval > 0 ? Math.round(savedInterval / 60) : 30)
  const savedWarnPct = scanCfg?.usage_warn_percent ?? 80
  const warnPct = scanEdit?.warnPct ?? savedWarnPct

  const saveScanConfig = async () => {
    const seconds = scanOn ? scanMinutes * 60 : 0
    if (scanOn && (scanMinutes < 5 || scanMinutes > 1440)) {
      message.warning('间隔需在 5～1440 分钟之间')
      return
    }
    if (warnPct < (scanCfg?.usageMin ?? 50) || warnPct > (scanCfg?.usageMax ?? 99)) {
      message.warning(`容量阈值需在 ${scanCfg?.usageMin ?? 50}～${scanCfg?.usageMax ?? 99} 之间`)
      return
    }
    setSavingScan(true)
    try {
      await api.putNASConfig(seconds, warnPct)
      setScanEdit(null)
      await queryClient.invalidateQueries({ queryKey: ['nas-config'] })
      message.success('巡检设置已保存')
    } catch (err) {
      message.error(getApiErrorMessage(err, '保存失败'))
    } finally {
      setSavingScan(false)
    }
  }

  // 带 nas capability 的在线 agent
  const nasAgents = useMemo(
    () =>
      agents.filter(
        (a) => a.status !== 'offline' && (a.capabilities ?? []).some((c) => c.type === 'nas'),
      ),
    [agents],
  )

  const statuses = useQuery({
    queryKey: ['nas-overview', nasAgents.map((a) => a.id).join(',')],
    queryFn: async () => {
      const results = await Promise.allSettled(
        nasAgents.map(async (a) => ({ agent: a, status: await api.getNASStatus(a.id) })),
      )
      return results
        .filter((r) => r.status === 'fulfilled')
        .map((r) => (r as PromiseFulfilledResult<{ agent: Agent; status: NasStatus }>).value)
    },
    enabled: nasAgents.length > 0,
    staleTime: 30_000,
    retry: false,
  })

  const { poolRows, mountRows, shareRows } = useMemo(() => {
    const data = statuses.data ?? []
    const poolRows: PoolRow[] = []
    const mountRows: MountRow[] = []
    const shareRows: ShareRow[] = []
    for (const { agent, status } of data) {
      const host = agent.hostname || agent.id
      for (const p of status.pools ?? []) {
        poolRows.push({ ...p, key: `${agent.id}:${p.kind}:${p.name}`, agent: host })
      }
      for (const m of status.mounts ?? []) {
        mountRows.push({ ...m, key: `${agent.id}:${m.device}:${m.mountPath}`, agent: host })
      }
      for (const s of status.shares ?? []) {
        shareRows.push({ ...s, key: `${agent.id}:${s.protocol}:${s.path}`, agent: host })
      }
    }
    // 异常池排前面
    poolRows.sort(
      (a, b) =>
        poolSeverity(a.state) - poolSeverity(b.state) || (a.agent ?? '').localeCompare(b.agent ?? ''),
    )
    return { poolRows, mountRows, shareRows }
  }, [statuses.data])

  const panelItems = nasAgents.map((a) => ({
    key: a.id,
    label: a.hostname || a.id,
    children: <AgentNasPanel agentId={a.id} />,
  }))

  const failedCount = poolRows.filter((r) => r.state === 'failed').length
  const degradedCount = poolRows.filter((r) => r.state === 'degraded' || r.state === 'resync').length
  const hotMountCount = mountRows.filter((m) => {
    if (!m.totalGB || m.totalGB <= 0) return false
    return Math.round(((m.usedGB ?? 0) / m.totalGB) * 100) >= warnPct
  }).length

  return (
    <div style={{ padding: 24 }}>
      <Card
        title={
          <Space>
            <DatabaseOutlined />
            存储池
          </Space>
        }
      >
        <Space wrap style={{ marginBottom: 16 }} align="center">
          <Typography.Text strong>自动巡检</Typography.Text>
          <Tooltip title="开启后 server 定时收集各主机存储状态，池降级/故障或容量超阈值即产生告警（同一异常未处理期间只提醒一次）">
            <Switch checked={scanOn} onChange={(on) => setScanEdit((e) => ({ ...e, on }))} loading={!scanCfg} />
          </Tooltip>
          {scanOn && (
            <InputNumber
              min={5}
              max={1440}
              value={scanMinutes}
              onChange={(v) => setScanEdit((e) => ({ ...e, minutes: v ?? 30 }))}
              addonAfter="分钟"
              style={{ width: 130 }}
            />
          )}
          <InputNumber
            min={scanCfg?.usageMin ?? 50}
            max={scanCfg?.usageMax ?? 99}
            value={warnPct}
            onChange={(v) => setScanEdit((e) => ({ ...e, warnPct: v ?? 80 }))}
            addonAfter="% 容量告警"
            style={{ width: 160 }}
          />
          <Button size="small" onClick={() => void saveScanConfig()} loading={savingScan}>
            保存
          </Button>
        </Space>
        <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginBottom: 16 }}>
          观测来源为本机 mdadm（/proc/mdstat）、ZFS、LVM 与 SMB/NFS 导出，只读不落库；池降级或成员盘故障意味着冗余已受损，出现即建议备份数据。
        </Typography.Text>

        {nasAgents.length === 0 ? (
          <Empty description="暂无支持存储观测的在线主机——装有 mdadm/ZFS/LVM 或 Samba/NFS 的主机运行 Agent 即可" />
        ) : statuses.isLoading ? (
          <div style={{ textAlign: 'center', padding: 32 }}>
            <Spin tip="正在收集各主机存储状态…" />
          </div>
        ) : (
          <>
            {(failedCount > 0 || degradedCount > 0 || hotMountCount > 0) && (
              <Alert
                type={failedCount > 0 ? 'error' : 'warning'}
                showIcon
                style={{ marginBottom: 16 }}
                message={
                  failedCount > 0
                    ? `${failedCount} 个存储池故障，数据访问可能中断，请立即检查`
                    : [
                        degradedCount > 0 && `${degradedCount} 个存储池降级/同步中`,
                        hotMountCount > 0 && `${hotMountCount} 个挂载点容量超过 ${warnPct}%`,
                      ]
                        .filter(Boolean)
                        .join('，')
                }
              />
            )}
            <PoolTable dataSource={poolRows} />
          </>
        )}
      </Card>

      {mountRows.length > 0 && (
        <Card title="挂载点容量" style={{ marginTop: 16 }}>
          <MountTable dataSource={mountRows} warnPct={warnPct} />
        </Card>
      )}

      {shareRows.length > 0 && (
        <Card title="网络共享" style={{ marginTop: 16 }}>
          <ShareTable dataSource={shareRows} />
        </Card>
      )}

      {nasAgents.length > 0 && (
        <Card title="按主机查看" style={{ marginTop: 16 }}>
          <Collapse items={panelItems} />
        </Card>
      )}
    </div>
  )
}

export default Nas
