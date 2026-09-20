import { useMemo, useRef, useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { Button, Card, Empty, Modal, Select, Space, Table, Tabs, Tag, message } from 'antd'
import { ReloadOutlined, EyeOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import dayjs from 'dayjs'
import { api } from '@/services/api'
import type { Agent, ContainerInfo, ImageInfo } from '@/types'
import { usePerm } from '@/hooks/usePerm'

const PAGE_SIZE = 20

// 容器状态 → Tag 颜色映射
const containerStateColor: Record<string, string> = {
  running: 'green',
  created: 'blue',
  paused: 'orange',
  exited: 'default',
  dead: 'red',
  restarting: 'gold',
}

// 字节 → 人类可读
const formatSize = (bytes: number): string => {
  if (!bytes || bytes <= 0) return '-'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let value = bytes
  let i = 0
  while (value >= 1024 && i < units.length - 1) {
    value /= 1024
    i++
  }
  return `${value.toFixed(value >= 10 || i === 0 ? 0 : 1)} ${units[i]}`
}

// Unix 秒级时间戳 → 可读时间
const formatTimestamp = (ts: number): string => {
  if (!ts) return '-'
  return dayjs.unix(ts).format('YYYY-MM-DD HH:mm:ss')
}

// 清理 Docker 日志：剥离流多路复用头（每帧前 8 字节控制头）
const cleanDockerLog = (raw: string): string => {
  if (!raw) return ''
  // 按行分割，每行去掉前导控制字符（Docker 流头 \x00-\x1f）
  return raw
    .split('\n')
    .map((line) => line.replace(/^[\x00-\x08\x0b\x0c\x0e-\x1f]+/, ''))
    .join('\n')
}
const trimContainerName = (name: string) => name?.replace(/^\//, '') || '-'

// 根据状态判断可用操作
const getAvailableActions = (state: string) => {
  const base: string[] = ['logs']
  switch (state) {
    case 'running':
      return [...base, 'stop', 'restart', 'pause'] as const
    case 'paused':
      return [...base, 'unpause', 'stop', 'restart'] as const
    case 'exited':
    case 'created':
      return [...base, 'start', 'remove'] as const
    case 'dead':
      return [...base, 'remove'] as const
    default:
      return base
  }
}

// 操作按钮中文标签
const actionLabel: Record<string, string> = {
  logs: '日志',
  start: '启动',
  stop: '停止',
  restart: '重启',
  pause: '暂停',
  unpause: '恢复',
  remove: '删除',
}

// 需要二次确认的操作
const destructiveActions = new Set(['stop', 'remove', 'restart'])

const tailOptions = [
  { value: '50', label: '50 行' },
  { value: '100', label: '100 行' },
  { value: '200', label: '200 行' },
  { value: '500', label: '500 行' },
  { value: '2000', label: '2000 行' },
]

const Docker = () => {
  const [selectedAgentId, setSelectedAgentId] = useState<string>('')
  const [activeKey, setActiveKey] = useState('containers')

  // 日志弹窗状态
  const [logContainer, setLogContainer] = useState<ContainerInfo | null>(null)
  const [logTail, setLogTail] = useState('100')
  const logContainerRef = useRef<HTMLDivElement>(null)

  // 写操作入口裁剪（RBAC 笔 8a）：无 docker:write 只留查看日志
  const canWrite = usePerm('docker:write')

  // 在线 agent 列表
  const { data: agents = [], isFetching: agentsLoading } = useQuery({
    queryKey: ['agents'],
    queryFn: () => api.getAgents(),
  })

  // 有 docker-api 能力的在线 agent
  const dockerAgents = useMemo(
    () =>
      agents.filter(
        (a) =>
          a.status === 'online' &&
          a.capabilities?.some((c) => c.type === 'docker-api' || c.type === 'docker'),
      ),
    [agents],
  )

  const effectiveAgentId = selectedAgentId || dockerAgents[0]?.id || ''
  const selectedAgent: Agent | undefined = agents.find((a) => a.id === effectiveAgentId)

  // 容器列表（显示全部，包括已停止的）
  const {
    data: containers = [],
    isFetching: containersLoading,
    refetch: refetchContainers,
  } = useQuery({
    queryKey: ['docker', 'containers', effectiveAgentId],
    queryFn: () => api.getContainers(effectiveAgentId, true),
    enabled: !!effectiveAgentId,
  })

  // 镜像列表
  const {
    data: images = [],
    isFetching: imagesLoading,
    refetch: refetchImages,
  } = useQuery({
    queryKey: ['docker', 'images', effectiveAgentId],
    queryFn: () => api.getImages(effectiveAgentId),
    enabled: !!effectiveAgentId,
  })

  // 日志内容
  const {
    data: logContent = '',
    isFetching: logLoading,
    refetch: refetchLog,
  } = useQuery({
    queryKey: ['docker', 'logs', effectiveAgentId, logContainer?.ID, logTail],
    queryFn: () =>
      api.getContainerLogs(effectiveAgentId, logContainer!.ID, {
        tail: logTail,
        timestamps: true,
      }),
    enabled: !!effectiveAgentId && !!logContainer,
  })

  // 容器操作通用 mutation
  const containerMutation = useMutation({
    mutationFn: async ({
      action,
      containerId,
    }: {
      action: string
      containerId: string
    }) => {
      switch (action) {
        case 'start':
          return api.startContainer(effectiveAgentId, containerId)
        case 'stop':
          return api.stopContainer(effectiveAgentId, containerId, 10)
        case 'restart':
          return api.restartContainer(effectiveAgentId, containerId, 10)
        case 'pause':
          return api.pauseContainer(effectiveAgentId, containerId)
        case 'unpause':
          return api.unpauseContainer(effectiveAgentId, containerId)
        case 'remove':
          return api.removeContainer(effectiveAgentId, containerId, { force: true })
        default:
          throw new Error(`Unknown action: ${action}`)
      }
    },
    onSuccess: () => {
      setTimeout(() => {
        void refetchContainers()
      }, 500)
    },
    onError: (err: Error) => {
      message.error(`操作失败: ${err.message}`)
    },
  })

  const handleAction = (containerId: string, containerName: string, action: string) => {
    if (action === 'logs') {
      const c = (containers as ContainerInfo[]).find((c) => c.ID === containerId)
      if (c) setLogContainer(c)
      return
    }

    const exec = () => {
      containerMutation.mutate({ action, containerId })
    }

    if (destructiveActions.has(action)) {
      Modal.confirm({
        title: `${actionLabel[action]}容器`,
        content: `确定要${actionLabel[action]}「${containerName}」吗？`,
        okText: actionLabel[action],
        okType: action === 'remove' ? 'danger' : 'primary',
        cancelText: '取消',
        onOk: exec,
      })
    } else {
      exec()
    }
  }

  const handleRefresh = () => {
    void refetchContainers()
    void refetchImages()
    message.success('已刷新')
  }

  // 滚动日志到底部
  const scrollLogToBottom = () => {
    requestAnimationFrame(() => {
      if (logContainerRef.current) {
        logContainerRef.current.scrollTop = logContainerRef.current.scrollHeight
      }
    })
  }

  // 容器列定义
  const containerColumns: ColumnsType<ContainerInfo> = [
    {
      title: '名称',
      dataIndex: 'Name',
      key: 'Name',
      ellipsis: true,
      render: (name: string) => trimContainerName(name),
    },
    {
      title: '镜像',
      dataIndex: 'Image',
      key: 'Image',
      ellipsis: true,
    },
    {
      title: '状态',
      key: 'State',
      width: 110,
      render: (_, record) => (
        <Tag color={containerStateColor[record.State] || 'default'}>{record.State}</Tag>
      ),
    },
    {
      title: '详情',
      dataIndex: 'Status',
      key: 'Status',
      ellipsis: true,
      render: (status: string) => status || '-',
    },
    {
      title: '创建时间',
      dataIndex: 'Created',
      key: 'Created',
      width: 180,
      render: (ts: number) => formatTimestamp(ts),
    },
    {
      title: '操作',
      key: 'actions',
      width: 300,
      render: (_, record) => {
        const actions = canWrite
          ? getAvailableActions(record.State)
          : getAvailableActions(record.State).filter((a) => a === 'logs')
        return (
          <Space size="small">
            {actions.map((action) => {
              const isPending =
                action !== 'logs' &&
                containerMutation.isPending &&
                containerMutation.variables?.containerId === record.ID &&
                containerMutation.variables?.action === action
              return (
                <Button
                  key={action}
                  type="link"
                  size="small"
                  icon={action === 'logs' ? <EyeOutlined /> : undefined}
                  loading={isPending}
                  onClick={() => handleAction(record.ID, trimContainerName(record.Name), action)}
                  disabled={!effectiveAgentId}
                >
                  {actionLabel[action]}
                </Button>
              )
            })}
          </Space>
        )
      },
    },
  ]

  const imageColumns: ColumnsType<ImageInfo> = [
    {
      title: '仓库标签',
      dataIndex: 'RepoTags',
      key: 'RepoTags',
      ellipsis: true,
      render: (tags: string[]) =>
        tags && tags.length > 0 ? (
          <Space size={4} wrap>
            {tags.map((tag) => (
              <Tag key={tag}>{tag}</Tag>
            ))}
          </Space>
        ) : (
          <Tag>&lt;none&gt;</Tag>
        ),
    },
    {
      title: 'ID',
      dataIndex: 'ID',
      key: 'ID',
      width: 180,
      ellipsis: true,
      render: (id: string) => (id ? id.replace(/^sha256:/, '').slice(0, 12) : '-'),
    },
    {
      title: '大小',
      dataIndex: 'Size',
      key: 'Size',
      width: 110,
      render: (size: number) => formatSize(size),
    },
    {
      title: '创建时间',
      dataIndex: 'Created',
      key: 'Created',
      width: 180,
      render: (ts: number) => formatTimestamp(ts),
    },
  ]

  // 无可用 agent 时的空状态提示
  if (!effectiveAgentId) {
    return (
      <div className="page-container">
        <Card title="Docker 容器管理">
          <Empty
            description={
              agentsLoading
                ? '正在加载 Agent 列表...'
                : '暂无在线的 Docker Agent。请在目标主机部署 Agent 并确保可访问 /var/run/docker.sock'
            }
          />
        </Card>
      </div>
    )
  }

  const tabItems = [
    {
      key: 'containers',
      label: `容器 (${containers.length})`,
      children: (
        <Table
          columns={containerColumns}
          dataSource={containers}
          rowKey="ID"
          loading={containersLoading}
          pagination={{ pageSize: PAGE_SIZE }}
        />
      ),
    },
    {
      key: 'images',
      label: `镜像 (${images.length})`,
      children: (
        <Table
          columns={imageColumns}
          dataSource={images}
          rowKey="ID"
          loading={imagesLoading}
          pagination={{ pageSize: PAGE_SIZE }}
        />
      ),
    },
  ]

  return (
    <div className="page-container">
      <Card
        title="Docker 容器管理"
        extra={
          <Space>
            <Select
              style={{ width: 240 }}
              placeholder="选择 Agent"
              value={effectiveAgentId}
              onChange={setSelectedAgentId}
              options={dockerAgents.map((a) => ({
                value: a.id,
                label: `${a.hostname} (${a.id.slice(0, 12)})`,
              }))}
            />
            <Button
              icon={<ReloadOutlined />}
              onClick={handleRefresh}
              loading={containersLoading || imagesLoading}
            >
              刷新
            </Button>
          </Space>
        }
      >
        <Tabs activeKey={activeKey} onChange={setActiveKey} items={tabItems} />
        <div style={{ marginTop: 8, color: 'rgba(0,0,0,0.45)', fontSize: 12 }}>
          Agent：{selectedAgent?.hostname} · IP：{selectedAgent?.ip || '-'} · 地区：
          {selectedAgent?.location?.region}/{selectedAgent?.location?.zone}
        </div>
      </Card>

      {/* 日志弹窗 */}
      <Modal
        title={`容器日志 — ${trimContainerName(logContainer?.Name || '')}`}
        open={!!logContainer}
        onCancel={() => setLogContainer(null)}
        footer={null}
        width={900}
        style={{ top: 50 }}
      >
        <Space direction="vertical" style={{ width: '100%' }}>
          <Space>
            <Select
              size="small"
              value={logTail}
              onChange={setLogTail}
              options={tailOptions}
            />
            <Button
              size="small"
              icon={<ReloadOutlined />}
              loading={logLoading}
              onClick={() => { void refetchLog(); scrollLogToBottom() }}
            >
              刷新
            </Button>
          </Space>
          <div
            ref={logContainerRef}
            style={{
              maxHeight: 500,
              overflow: 'auto',
              background: '#1e1e1e',
              color: '#d4d4d4',
              padding: 12,
              borderRadius: 6,
              fontSize: 12,
              fontFamily: "'Cascadia Code', 'Fira Code', 'Consolas', monospace",
              lineHeight: 1.6,
              whiteSpace: 'pre-wrap',
              wordBreak: 'break-all',
            }}
          >
            {logLoading ? (
              <span style={{ color: '#888' }}>加载中...</span>
            ) : logContent ? (
              cleanDockerLog(logContent)
            ) : (
              <span style={{ color: '#888' }}>（无日志输出）</span>
            )}
          </div>
        </Space>
      </Modal>
    </div>
  )
}

export default Docker
