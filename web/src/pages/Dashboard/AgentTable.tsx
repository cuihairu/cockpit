import { useMemo } from 'react'
import { Card, Space, Table, Tag } from 'antd'
import { CloudServerOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { Agent } from '@/types'
import { formatOfflineDuration } from '@/utils/format'
import { PermGuard } from '@/components/PermGuard'
import CleanupOfflineButton from '@/components/CleanupOfflineButton'

const columns: ColumnsType<Agent> = [
  {
    title: '主机名',
    dataIndex: 'hostname',
    key: 'hostname',
    render: (text: string) => (
      <Space>
        <CloudServerOutlined />
        <span>{text}</span>
      </Space>
    ),
  },
  {
    title: 'IP 地址',
    dataIndex: 'ip',
    key: 'ip',
  },
  {
    title: '区域',
    key: 'location',
    render: (_value, record) => `${record.region || '-'}/${record.zone || '-'}`,
  },
  {
    title: '状态',
    dataIndex: 'status',
    key: 'status',
    render: (_status, record) => {
      if (record.status === 'online') return <Tag color="success">在线</Tag>
      const duration = formatOfflineDuration(record.lastSeen)
      return <Tag>{duration ? `离线 ${duration}` : '离线'}</Tag>
    },
  },
  {
    title: '能力',
    dataIndex: 'capabilities',
    key: 'capabilities',
    render: (caps: Agent['capabilities']) => (
      <Space size={4}>
        {caps?.slice(0, 3).map((cap) => (
          <Tag key={cap.type} color="processing" style={{ fontSize: 11 }}>
            {cap.type}
          </Tag>
        ))}
        {caps?.length > 3 && <Tag>+{caps.length - 3}</Tag>}
      </Space>
    ),
  },
]

// Agent 概览表（完整管理在 /agents）。在线优先置底离线（弱化），离线行
// 标注离线时长；清理入口按 RBAC inventory:write 裁剪。
const AgentTable = ({ agents, onCleaned }: { agents: Agent[]; onCleaned?: () => void }) => {
  const sorted = useMemo(
    () =>
      [...agents].sort((a, b) => {
        const rank = (x: Agent) => (x.status === 'online' ? 0 : 1)
        return rank(a) - rank(b) || (b.lastSeen || 0) - (a.lastSeen || 0)
      }),
    [agents],
  )

  return (
    <Card
      title="Agent 列表"
      variant="borderless"
      extra={
        <Space>
          <PermGuard perm="inventory:write">
            <CleanupOfflineButton onCleaned={onCleaned} />
          </PermGuard>
          <a href="/agents">查看全部</a>
        </Space>
      }
    >
      <Table
        dataSource={sorted}
        columns={columns}
        rowKey="id"
        pagination={{ pageSize: 5 }}
        size="small"
        scroll={{ x: 640 }}
        rowClassName={(record) => (record.status === 'online' ? '' : 'agent-row-offline')}
      />
    </Card>
  )
}

export default AgentTable
