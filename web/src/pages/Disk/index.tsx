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
import { HddOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { api } from '@/services/api'
import type { Agent, SmartDevice, SmartStatus } from '@/types'
import { getApiErrorMessage } from '@/utils/apiError'
import { PermGuard } from '@/components/PermGuard'

// 磁盘健康（SMART）：各主机磁盘只读观测 + server 定时巡检告警
// （见 docs/guide/disk-health-design.md）。server 纯转发不落库，
// 总览由前端逐 agent 拉取后聚合。

const HEALTH_META: Record<SmartDevice['health'], { label: string; status: 'success' | 'error' | 'default' }> = {
  passed: { label: '健康', status: 'success' },
  failed: { label: 'FAILED', status: 'error' },
  unknown: { label: '未知', status: 'default' },
}

const formatSize = (bytes?: number) => {
  if (!bytes) return '—'
  const gb = bytes / 1024 ** 3
  return gb >= 1000 ? `${(gb / 1024).toFixed(1)} TB` : `${Math.round(gb)} GB`
}

const formatHours = (h?: number) => {
  if (h === undefined) return '—'
  if (h < 24) return `${h} 小时`
  return `${Math.floor(h / 24)} 天 ${h % 24} 小时`
}

// 总览行：一块盘（跨主机平铺，主机列区分）
interface DiskRow extends SmartDevice {
  key: string
  agent: string
}

const buildDiskRows = (statuses: { agent: Agent; status: SmartStatus }[]): DiskRow[] => {
  const rows: DiskRow[] = []
  for (const { agent, status } of statuses) {
    for (const dev of status.devices ?? []) {
      rows.push({
        ...dev,
        key: `${agent.id}:${dev.name}`,
        agent: agent.hostname || agent.id,
      })
    }
  }
  // 异常盘排前面：failed > unknown > passed，同级按扇区数倒序
  const severity = (r: DiskRow) =>
    r.health === 'failed' ? 0 : r.health === 'unknown' ? 1 : 2
  const badSectors = (r: DiskRow) =>
    (r.reallocatedSectors ?? 0) + (r.pendingSectors ?? 0) + (r.mediaErrors ?? 0)
  return rows.sort(
    (a, b) => severity(a) - severity(b) || badSectors(b) - badSectors(a) || a.agent.localeCompare(b.agent),
  )
}

// 健康徽标：unknown 时 Tooltip 说明原因（权限/不支持 SMART）
const HealthCell: React.FC<{ dev: SmartDevice }> = ({ dev }) => {
  const meta = HEALTH_META[dev.health] ?? HEALTH_META.unknown
  const badge = <Badge status={meta.status} text={meta.label} />
  if (dev.health === 'unknown' && dev.error) {
    return (
      <Tooltip title={dev.error}>
        <span>{badge}</span>
      </Tooltip>
    )
  }
  return badge
}

const SECTOR_CELL_STYLE = { fontSize: 12 }

// 单主机详情（Collapse 内）：与总览同构，少一列主机
const AgentDiskPanel: React.FC<{ agentId: string }> = ({ agentId }) => {
  const { data, isLoading, isError } = useQuery({
    queryKey: ['smart-status', agentId],
    queryFn: () => api.getSmartStatus(agentId),
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
    return <Alert type="warning" showIcon message="该主机磁盘状态获取失败" />
  }
  if (!data.available || (data.devices ?? []).length === 0) {
    return <Empty description="未发现可读 SMART 的磁盘（需 root 权限运行 Agent）" />
  }

  return (
    <Table<SmartDevice>
      size="small"
      rowKey="name"
      dataSource={data.devices}
      pagination={false}
      columns={[
        {
          title: '健康',
          dataIndex: 'health',
          width: 90,
          render: (_: unknown, dev) => <HealthCell dev={dev} />,
        },
        { title: '设备', dataIndex: 'name', width: 140, render: (v: string) => <Typography.Text code>{v}</Typography.Text> },
        { title: '型号', dataIndex: 'model', ellipsis: true, render: (v: string) => v || '—' },
        { title: '容量', dataIndex: 'sizeBytes', width: 100, render: (v?: number) => formatSize(v) },
        { title: '温度', dataIndex: 'temperatureC', width: 80, render: (v?: number) => (v !== undefined ? `${v}°C` : '—') },
      ]}
    />
  )
}

const Disk = () => {
  const { data: agents = [] } = useQuery({ queryKey: ['agents'], queryFn: () => api.getAgents() })

  // 巡检设置编辑态：改动才落本地，未编辑从服务端派生（与 Drift 页同模式）
  const [scanEdit, setScanEdit] = useState<{ on?: boolean; minutes?: number } | null>(null)
  const [savingScan, setSavingScan] = useState(false)
  const queryClient = useQueryClient()

  const { data: scanCfg } = useQuery({ queryKey: ['smart-config'], queryFn: () => api.getSmartConfig() })

  const savedInterval = scanCfg?.scan_interval_seconds ?? 0
  const scanOn = scanEdit?.on ?? savedInterval > 0
  const scanMinutes = scanEdit?.minutes ?? (savedInterval > 0 ? Math.round(savedInterval / 60) : 60)

  const saveScanConfig = async () => {
    const seconds = scanOn ? scanMinutes * 60 : 0
    if (scanOn && (scanMinutes < 5 || scanMinutes > 1440)) {
      message.warning('间隔需在 5～1440 分钟之间')
      return
    }
    setSavingScan(true)
    try {
      await api.putSmartConfig(seconds)
      setScanEdit(null)
      await queryClient.invalidateQueries({ queryKey: ['smart-config'] })
      message.success(scanOn ? '已开启自动巡检' : '已关闭自动巡检')
    } catch (err) {
      message.error(getApiErrorMessage(err, '保存失败'))
    } finally {
      setSavingScan(false)
    }
  }

  // 带 hardware-monitor + smart 标志的在线 agent（D1）
  const smartAgents = useMemo(
    () =>
      agents.filter(
        (a) =>
          a.status !== 'offline' &&
          (a.capabilities ?? []).some(
            (c) => c.type === 'hardware-monitor' && (c.metadata as Record<string, unknown> | undefined)?.smart === true,
          ),
      ),
    [agents],
  )

  const statuses = useQuery({
    queryKey: ['smart-overview', smartAgents.map((a) => a.id).join(',')],
    queryFn: async () => {
      const results = await Promise.allSettled(
        smartAgents.map(async (a) => ({ agent: a, status: await api.getSmartStatus(a.id) })),
      )
      return results.filter((r) => r.status === 'fulfilled').map((r) => (r as PromiseFulfilledResult<{ agent: Agent; status: SmartStatus }>).value)
    },
    enabled: smartAgents.length > 0,
    staleTime: 30_000,
    retry: false,
  })

  const diskRows = useMemo(
    () => (statuses.data ? buildDiskRows(statuses.data) : []),
    [statuses.data],
  )

  const overviewColumns: ColumnsType<DiskRow> = [
    {
      title: '健康',
      key: 'health',
      width: 100,
      render: (_: unknown, r) => <HealthCell dev={r} />,
    },
    { title: '主机', dataIndex: 'agent', width: 150, ellipsis: true },
    {
      title: '设备',
      dataIndex: 'name',
      width: 140,
      render: (v: string) => <Typography.Text code>{v}</Typography.Text>,
    },
    { title: '型号', dataIndex: 'model', ellipsis: true, render: (v: string) => v || '—' },
    { title: '容量', dataIndex: 'sizeBytes', width: 100, render: (v?: number) => formatSize(v) },
    { title: '温度', dataIndex: 'temperatureC', width: 80, render: (v?: number) => (v !== undefined ? `${v}°C` : '—') },
    { title: '通电', dataIndex: 'powerOnHours', width: 120, render: (v?: number) => formatHours(v) },
    {
      title: '重映射',
      dataIndex: 'reallocatedSectors',
      width: 90,
      render: (v?: number) =>
        v !== undefined && v > 0 ? <span style={{ color: '#faad14', ...SECTOR_CELL_STYLE }}>{v}</span> : <span style={SECTOR_CELL_STYLE}>0</span>,
    },
    {
      title: '待定扇区',
      dataIndex: 'pendingSectors',
      width: 90,
      render: (v?: number) =>
        v !== undefined && v > 0 ? <span style={{ color: '#faad14', ...SECTOR_CELL_STYLE }}>{v}</span> : <span style={SECTOR_CELL_STYLE}>0</span>,
    },
    {
      title: '介质错误',
      dataIndex: 'mediaErrors',
      width: 90,
      render: (v?: number) =>
        v !== undefined && v > 0 ? <span style={{ color: '#faad14', ...SECTOR_CELL_STYLE }}>{v}</span> : <span style={SECTOR_CELL_STYLE}>0</span>,
    },
  ]

  const panelItems = smartAgents.map((a) => ({
    key: a.id,
    label: a.hostname || a.id,
    children: <AgentDiskPanel agentId={a.id} />,
  }))

  const failedCount = diskRows.filter((r) => r.health === 'failed').length
  const warnCount = diskRows.filter(
    (r) => r.health === 'passed' && ((r.reallocatedSectors ?? 0) > 0 || (r.pendingSectors ?? 0) > 0 || (r.mediaErrors ?? 0) > 0),
  ).length

  return (
    <div style={{ padding: 24 }}>
      <Card
        title={
          <Space>
            <HddOutlined />
            磁盘健康
          </Space>
        }
      >
        <Space wrap style={{ marginBottom: 16 }} align="center">
          <Typography.Text strong>自动巡检</Typography.Text>
          <Tooltip title="开启后 server 定时扫描全部支持 SMART 的主机，发现盘 FAILED 或扇区异常即产生告警（同一主机同级未处理期间只提醒一次）">
            <Switch
              checked={scanOn}
              onChange={(on) => setScanEdit((e) => ({ ...e, on }))}
              loading={!scanCfg}
            />
          </Tooltip>
          {scanOn && (
            <InputNumber
              min={5}
              max={1440}
              value={scanMinutes}
              onChange={(v) => setScanEdit((e) => ({ ...e, minutes: v ?? 5 }))}
              addonAfter="分钟"
              style={{ width: 130 }}
            />
          )}
          <PermGuard perm="smart:write">
            <Button size="small" onClick={() => void saveScanConfig()} loading={savingScan}>
              保存
            </Button>
          </PermGuard>
        </Space>
        <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginBottom: 16 }}>
          巡检依赖 smartctl 读取盘的 SMART 数据，Agent 需以 root 运行；重映射/待定扇区增长是盘失效前的典型征兆，出现即建议备份数据。
        </Typography.Text>

        {smartAgents.length === 0 ? (
          <Empty description="暂无支持 SMART 的在线主机——在目标主机安装 smartmontools 并运行 Agent 即可" />
        ) : statuses.isLoading ? (
          <div style={{ textAlign: 'center', padding: 32 }}>
            <Spin tip="正在收集各主机磁盘状态…" />
          </div>
        ) : (
          <>
            {(failedCount > 0 || warnCount > 0) && (
              <Alert
                type={failedCount > 0 ? 'error' : 'warning'}
                showIcon
                style={{ marginBottom: 16 }}
                message={
                  failedCount > 0
                    ? `${failedCount} 块盘 SMART 判定 FAILED，请立即备份并更换`
                    : `${warnCount} 块盘出现重映射/待定扇区或介质错误，建议尽快备份`
                }
              />
            )}
            <Table<DiskRow>
              rowKey="key"
              columns={overviewColumns}
              dataSource={diskRows}
              pagination={{ pageSize: 15, hideOnSinglePage: true }}
              size="small"
              locale={{ emptyText: '各主机暂无磁盘读数' }}
              style={{ marginBottom: 16 }}
            />
          </>
        )}
      </Card>

      {smartAgents.length > 0 && (
        <Card title="按主机查看" style={{ marginTop: 16 }}>
          <Collapse items={panelItems} />
        </Card>
      )}
    </div>
  )
}

export default Disk
