import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Alert, Badge, Button, Card, Descriptions, Empty, Input, Select, Space, Table, Tag, Tooltip, Typography, message } from 'antd'
import { PoweroffOutlined, ReloadOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { api } from '@/services/api'
import type { ServiceActionName, ServiceUnit } from '@/types'
import { getApiErrorMessage } from '@/utils/apiError'

// 服务管理（见 docs/guide/service-design.md）：systemd 与 Windows SCM 双后端
// 统一观测 + 动作操作（capability type=service，metadata.backend 区分）。
// 事实源在 agent 侧，server 纯转发不落库；动作经白名单校验并记审计日志。

// systemctl 动词中文名与语义提示
const ACTION_LABELS: Record<ServiceActionName, string> = {
  start: '启动',
  stop: '停止',
  restart: '重启',
  reload: '重载',
  enable: '设自启',
  disable: '停自启',
}

// activeState → 徽标样式
const ACTIVE_META: Record<string, { status: 'success' | 'processing' | 'error' | 'default' | 'warning'; label: string }> = {
  active: { status: 'success', label: 'active' },
  activating: { status: 'processing', label: 'activating' },
  failed: { status: 'error', label: 'failed' },
  inactive: { status: 'default', label: 'inactive' },
  deactivating: { status: 'warning', label: 'deactivating' },
}

// systemState → 徽标样式（running 绿 / degraded 橙 / 其他默认）
const SYSTEM_META: Record<string, { status: 'success' | 'error' | 'default' | 'warning'; label: string }> = {
  running: { status: 'success', label: 'running' },
  degraded: { status: 'warning', label: 'degraded：有 unit 处于 failed 状态' },
  maintenance: { status: 'warning', label: 'maintenance' },
  stopping: { status: 'error', label: 'stopping' },
  offline: { status: 'default', label: 'offline' },
  unknown: { status: 'default', label: 'unknown' },
}

const Services = () => {
  const queryClient = useQueryClient()
  const [selectedAgent, setSelectedAgent] = useState<string>()
  const [search, setSearch] = useState('')
  const [actingUnit, setActingUnit] = useState<string>()
  const [actionError, setActionError] = useState<{ unit: string; msg: string }>()

  const { data: agents } = useQuery({ queryKey: ['agents'], queryFn: () => api.getAgents() })

  // 只有带 service capability（systemd / Windows SCM / macOS launchd）的
  // agent 可选；backend 决定后端特有处理——仅 systemd 有 reload 与全局
  // 系统状态概念（D9.6/D10.5）
  const agentOptions = useMemo(
    () =>
      (agents ?? []).map((a) => {
        const cap = (a.capabilities ?? []).find((c) => c.type === 'service')
        return {
          value: a.id,
          disabled: !cap || a.status === 'offline',
          label: `${a.hostname || a.id}${
            a.status === 'offline' ? '（离线）' : !cap ? '（未检测到服务管理）' : ''
          }`,
          backend: (cap?.metadata?.backend as string | undefined) ?? 'systemd',
        }
      }),
    [agents],
  )
  const selectedBackend = agentOptions.find((o) => o.value === selectedAgent)?.backend
  const isSystemd = selectedBackend === 'systemd'
  const isWindows = selectedBackend === 'windows-scm'

  const statusKey = ['service-status', selectedAgent]
  const listKey = ['service-list', selectedAgent]

  const { data: status } = useQuery({
    queryKey: statusKey,
    queryFn: () => api.getServiceStatus(selectedAgent!),
    enabled: !!selectedAgent,
  })
  const { data: listData, isLoading } = useQuery({
    queryKey: listKey,
    queryFn: () => api.getAgentServices(selectedAgent!),
    enabled: !!selectedAgent,
    refetchInterval: 30_000,
  })

  const refresh = () => {
    queryClient.invalidateQueries({ queryKey: ['service-list', selectedAgent] })
    queryClient.invalidateQueries({ queryKey: ['service-status', selectedAgent] })
  }

  const actionMut = useMutation({
    mutationFn: ({ unit, action }: { unit: string; action: ServiceActionName }) =>
      api.serviceAction(selectedAgent!, unit, action),
    onSuccess: (_res, { unit, action }) => {
      setActionError(undefined)
      message.success(`${unit} ${ACTION_LABELS[action]}成功`)
      refresh()
    },
    onError: (err, { unit }) => {
      setActionError({ unit, msg: getApiErrorMessage(err, '操作失败') })
    },
    onSettled: (_res, _err, { unit }) => {
      if (actingUnit === unit) setActingUnit(undefined)
    },
  })

  const runAction = (unit: string, action: ServiceActionName) => {
    if (!selectedAgent) return
    setActingUnit(unit)
    actionMut.mutate({ unit, action })
  }

  const columns: ColumnsType<ServiceUnit> = [
    {
      title: '服务',
      dataIndex: 'name',
      key: 'name',
      width: 280,
      render: (v: string, r) => (
        <Space size={8}>
          <Typography.Text code style={{ fontSize: 12 }}>
            {v.replace(/\.service$/, '')}
          </Typography.Text>
          {r.description && (
            <Typography.Text type="secondary" style={{ fontSize: 12 }} ellipsis={{ tooltip: r.description }}>
              {r.description}
            </Typography.Text>
          )}
        </Space>
      ),
    },
    {
      title: '状态',
      dataIndex: 'activeState',
      key: 'activeState',
      width: 140,
      render: (v: string, r) => {
        const meta = ACTIVE_META[v] ?? { status: 'default' as const, label: v }
        const badge = <Badge status={meta.status} text={`${meta.label}${r.subState ? ` (${r.subState})` : ''}`} />
        return v === 'failed' ? (
          <Tooltip title={`unit 处于失败状态，查看 journal 日志定位原因`}>{badge}</Tooltip>
        ) : (
          badge
        )
      },
    },
    {
      title: '自启',
      dataIndex: 'unitFileState',
      key: 'unitFileState',
      width: 110,
      render: (v: string) => {
        if (v === 'enabled') return <Tag color="green">自启</Tag>
        if (v === 'disabled') return <Tag>手动</Tag>
        if (v === 'static')
          return (
            <Tooltip title="static：被其他 unit 依赖或无 [Install] 段，不能也不需要单独设自启">
              <Tag color="blue">静态</Tag>
            </Tooltip>
          )
        if (v === 'alias') return <Tag color="blue">别名</Tag>
        if (v === 'linked') return <Tag color="blue">链接</Tag>
        if (v === 'masked') return <Tag color="red">屏蔽</Tag>
        return <Tag>{v || '—'}</Tag>
      },
    },
    {
      title: '操作',
      key: 'actions',
      width: 260,
      render: (_, r) => {
        const acting = actingUnit === r.name
        // 操作显隐按当前状态：active → 停止/重启/重载；inactive → 启动；
        // enabled ↔ disabled → 自启切换；static 等无 [Install] 段的不可设自启
        const canToggleEnable = r.unitFileState === 'enabled' || r.unitFileState === 'disabled'
        return (
          <Space size={4}>
            {r.activeState === 'active' ? (
              <>
                <Tooltip title="停止服务">
                  <Button
                    size="small"
                    type="text"
                    danger
                    icon={<PoweroffOutlined />}
                    loading={acting}
                    onClick={() => runAction(r.name, 'stop')}
                  />
                </Tooltip>
                <Button size="small" type="text" loading={acting} onClick={() => runAction(r.name, 'restart')}>
                  重启
                </Button>
                {/* launchd/SCM 无 reload 语义（仅 systemd 后端支持） */}
                {isSystemd && (
                  <Button size="small" type="text" loading={acting} onClick={() => runAction(r.name, 'reload')}>
                    重载
                  </Button>
                )}
              </>
            ) : (
              <Button
                size="small"
                type="text"
                icon={<PoweroffOutlined />}
                loading={acting}
                onClick={() => runAction(r.name, 'start')}
              >
                启动
              </Button>
            )}
            {canToggleEnable &&
              (r.unitFileState === 'enabled' ? (
                <Button size="small" type="text" loading={acting} onClick={() => runAction(r.name, 'disable')}>
                  停自启
                </Button>
              ) : (
                <Button size="small" type="text" loading={acting} onClick={() => runAction(r.name, 'enable')}>
                  设自启
                </Button>
              ))}
          </Space>
        )
      },
    },
  ]

  const services = listData?.services ?? []
  const filtered = search.trim()
    ? services.filter(
        (s) =>
          s.name.toLowerCase().includes(search.trim().toLowerCase()) ||
          s.description.toLowerCase().includes(search.trim().toLowerCase()),
      )
    : services
  // 活跃服务置顶：active → 其他 → failed 最前提示
  const sorted = [...filtered].sort((a, b) => {
    const rank = (u: ServiceUnit) => (u.activeState === 'failed' ? 0 : u.activeState === 'active' ? 1 : 2)
    return rank(a) - rank(b) || a.name.localeCompare(b.name)
  })

  const sysMeta = status ? (SYSTEM_META[status.systemState] ?? { status: 'default' as const, label: status.systemState }) : null

  return (
    <div style={{ padding: 24 }}>
      <Card
        title="服务管理"
        extra={
          <Space>
            <Input.Search
              allowClear
              placeholder="搜索服务名或描述"
              style={{ width: 220 }}
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
            <Select
              style={{ minWidth: 260 }}
              options={agentOptions}
              value={selectedAgent}
              onChange={(v) => {
                setSelectedAgent(v)
                setActionError(undefined)
              }}
              placeholder="选择主机"
              showSearch
              optionFilterProp="label"
            />
            <Button icon={<ReloadOutlined />} disabled={!selectedAgent} onClick={refresh} />
          </Space>
        }
      >
        {!selectedAgent ? (
          <Empty
            description="选择一台主机管理服务"
            image={Empty.PRESENTED_IMAGE_SIMPLE}
          >
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              仅列出检测到服务管理 capability 的主机：Linux 需 systemd（systemctl + /run/systemd/system），Windows 由 SCM 内置支持
            </Typography.Text>
          </Empty>
        ) : (
          <Space direction="vertical" size={16} style={{ width: '100%' }}>
            {status && sysMeta && (
              <Descriptions
                size="small"
                column={{ xs: 1, sm: 4 }}
                items={[
                  // launchd/SCM 无全局状态概念，非 systemd 主机不显示该项
                  ...(isSystemd
                    ? [
                        {
                          key: 'state',
                          label: '系统状态',
                          children: <Badge status={sysMeta.status} text={sysMeta.label} />,
                        },
                      ]
                    : []),
                  { key: 'total', label: '服务总数', children: status.total },
                  { key: 'active', label: '运行中', children: status.active },
                  {
                    key: 'failed',
                    label: '失败',
                    children: status.failed > 0 ? <Typography.Text type="danger">{status.failed}</Typography.Text> : 0,
                  },
                ]}
              />
            )}
            <Alert
              type="info"
              showIcon
              style={{ marginBottom: 0 }}
              message={
                isWindows
                  ? '所有启停/自启操作直接作用于 Windows 服务控制管理器并记入审计日志；不支持重载操作。'
                  : isSystemd
                    ? '所有启停/自启操作直接作用于 systemd 并记入审计日志；failed 服务失败状态会在系统状态中体现（degraded）。'
                    : '所有启停/自启操作直接作用于 launchd 并记入审计日志；停止为卸载服务（plist 保留，可再次启动），不支持重载操作。'
              }
            />
            {actionError && (
              <Alert
                type="error"
                showIcon
                closable
                message={`${actionError.unit} 操作失败`}
                description={<pre style={{ margin: 0, whiteSpace: 'pre-wrap', fontSize: 12 }}>{actionError.msg}</pre>}
                onClose={() => setActionError(undefined)}
              />
            )}
            <Table<ServiceUnit>
              rowKey="name"
              size="small"
              columns={columns}
              dataSource={sorted}
              loading={isLoading}
              pagination={{ pageSize: 20, showSizeChanger: false, showTotal: (n) => `共 ${n} 个服务` }}
              locale={{ emptyText: search ? '无匹配服务' : '未发现服务' }}
            />
          </Space>
        )}
      </Card>
    </div>
  )
}

export default Services
