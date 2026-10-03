import { CodeOutlined, EnvironmentOutlined } from '@ant-design/icons'
import { Button, Space, Tag, Tooltip } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import type { Agent } from '@/types'
import { getVirtDisplay, statusConfig } from './helpers'

interface ColumnOptions {
  onShowDetail: (agent: Agent) => void
}

const formatUptime = (startedAt: number | undefined): string => {
  if (!startedAt) return '-'
  const sec = Math.max(0, Math.floor(Date.now() / 1000 - startedAt))
  const d = Math.floor(sec / 86400)
  const h = Math.floor((sec % 86400) / 3600)
  const m = Math.floor((sec % 3600) / 60)
  if (d > 0) return `${d}天${h}小时`
  if (h > 0) return `${h}小时${m}分`
  if (m > 0) return `${m}分钟`
  return `${sec}秒`
}

// 构建 Agent 表格列定义
export const buildAgentColumns = ({ onShowDetail }: ColumnOptions): ColumnsType<Agent> => [
  {
    title: 'Agent ID',
    dataIndex: 'id',
    key: 'id',
    width: 200,
    ellipsis: true,
    render: (id: string) => (
      <Space>
        <CodeOutlined />
        <span style={{ fontFamily: 'monospace' }}>{id}</span>
      </Space>
    ),
  },
  {
    title: '主机名',
    dataIndex: 'hostname',
    key: 'hostname',
    sorter: (a, b) => (a.hostname || '').localeCompare(b.hostname || ''),
    render: (hostname: string, record) => (
      <Space direction="vertical" size="small">
        <span>{hostname || '-'}</span>
        <span style={{ fontSize: 12, color: '#999' }}>{record.ip || '-'}</span>
      </Space>
    ),
  },
  {
    title: '位置',
    key: 'location',
    width: 180,
    render: (_, record) => (
      <Space direction="vertical" size="small">
        <Space>
          <EnvironmentOutlined />
          <span>{record.region || 'unknown'}</span>
        </Space>
        <span style={{ fontSize: 12, color: '#999', marginLeft: 20 }}>
          {record.zone || '-'}
        </span>
      </Space>
    ),
  },
  {
    title: '系统 / 架构',
    key: 'system',
    width: 160,
    sorter: (a, b) => (a.osName || '').localeCompare(b.osName || ''),
    render: (_, record) => (
      <Space direction="vertical" size="small">
        <span>{record.osName || '-'}{record.osVersion ? ` ${record.osVersion}` : ''}</span>
        <span style={{ fontSize: 12, color: '#999' }}>{record.arch || '-'}</span>
      </Space>
    ),
  },
  {
    title: 'Agent 版本',
    dataIndex: 'version',
    key: 'version',
    width: 120,
    sorter: (a, b) => (a.version || '').localeCompare(b.version || ''),
    render: (version: string) => version || '-',
  },
  {
    title: '启动时间 / 在线时长',
    key: 'uptime',
    width: 170,
    sorter: (a, b) => (a.startedAt || 0) - (b.startedAt || 0),
    render: (_, record) => (
      <Space direction="vertical" size="small">
        <span>{record.startedAt ? formatUptime(record.startedAt) : '-'}</span>
        <span style={{ fontSize: 12, color: '#999' }}>
          {record.startedAt ? new Date(record.startedAt * 1000).toLocaleString() : '-'}
        </span>
      </Space>
    ),
  },
  {
    title: '开放服务',
    key: 'services',
    width: 150,
    render: (_, record) => {
      const services = (record.services || []).filter((s) => s.running)
      if (services.length === 0) return <span style={{ color: '#999' }}>无</span>
      return (
        <Space size="small" wrap>
          {[...new Set(services.map((s) => s.protocol))].map((p) => (
            <Tag key={p} color="blue">{p.toUpperCase()}</Tag>
          ))}
        </Space>
      )
    },
  },
  {
    title: '标签',
    key: 'tags',
    width: 160,
    render: (_, record) => {
      const tags = record.tags || []
      if (tags.length === 0) return <span style={{ color: '#999' }}>无</span>
      return (
        <Space size="small" wrap>
          {tags.map((t) => (
            <Tag key={t.id} color={t.color || 'default'}>{t.name}</Tag>
          ))}
        </Space>
      )
    },
  },
  {
    title: '类型',
    key: 'virtualization',
    width: 120,
    sorter: (a, b) => (a.virtType || '').localeCompare(b.virtType || ''),
    render: (_, record) => {
      const config = getVirtDisplay(record)
      return (
        <Tag icon={config.icon} color={config.color}>
          {config.label}
        </Tag>
      )
    },
  },
  {
    title: '能力',
    dataIndex: 'capabilities',
    key: 'capabilities',
    width: 200,
    render: (capabilities: Agent['capabilities']) => (
      <Space size="small" wrap>
        {(capabilities || []).slice(0, 3).map((cap) => (
          <Tag key={cap.type} color="blue">
            {cap.type}
          </Tag>
        ))}
        {(capabilities || []).length > 3 && (
          <Tooltip title={capabilities.slice(3).map((c) => c.type).join(', ')}>
            <Tag>+{(capabilities || []).length - 3}</Tag>
          </Tooltip>
        )}
      </Space>
    ),
  },
  {
    title: '状态',
    dataIndex: 'status',
    key: 'status',
    width: 100,
    sorter: (a, b) => a.status?.localeCompare(b.status || ''),
    render: (status: string) => {
      const config = statusConfig[status] || statusConfig.offline
      return (
        <Tag icon={config.icon} color={config.color}>
          {config.text}
        </Tag>
      )
    },
  },
  {
    title: '最后连接',
    dataIndex: 'lastSeen',
    key: 'lastSeen',
    width: 120,
    render: (timestamp: number) => {
      if (!timestamp) return '-'
      const date = new Date(timestamp * 1000)
      const now = new Date()
      const diff = Math.floor((now.getTime() - date.getTime()) / 1000 / 60)
      if (diff < 1) return '刚刚'
      if (diff < 60) return `${diff} 分钟前`
      if (diff < 1440) return `${Math.floor(diff / 60)} 小时前`
      return date.toLocaleDateString()
    },
  },
  {
    title: '操作',
    key: 'actions',
    width: 120,
    render: (_, record) => (
      <Button type="link" onClick={() => onShowDetail(record)}>
        详情
      </Button>
    ),
  },
]
