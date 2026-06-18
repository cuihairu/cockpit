import { CodeOutlined, EnvironmentOutlined } from '@ant-design/icons'
import { Button, Space, Tag, Tooltip } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import type { Agent } from '@/types'
import { getVirtDisplay, statusConfig } from './helpers'

interface ColumnOptions {
  onShowDetail: (agent: Agent) => void
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
          <span>{record.location?.region || 'unknown'}</span>
        </Space>
        <span style={{ fontSize: 12, color: '#999', marginLeft: 20 }}>
          {record.location?.zone || '-'}
        </span>
      </Space>
    ),
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
    render: (capabilities: string[]) => (
      <Space size="small" wrap>
        {(capabilities || []).slice(0, 3).map((cap) => (
          <Tag key={cap} color="blue">
            {cap}
          </Tag>
        ))}
        {(capabilities || []).length > 3 && (
          <Tooltip title={capabilities.slice(3).join(', ')}>
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
