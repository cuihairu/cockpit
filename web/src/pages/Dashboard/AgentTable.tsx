import { Card, Space, Table, Tag } from 'antd'
import { CloudServerOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { Agent } from '@/types'

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
    render: (status: string) => (
      <Tag color={status === 'online' ? 'success' : 'default'}>
        {status === 'online' ? '在线' : '离线'}
      </Tag>
    ),
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

// Agent 概览表（前 5 条，完整管理在 /agents）
const AgentTable = ({ agents }: { agents: Agent[] }) => (
  <Card title="Agent 列表" variant="borderless" extra={<a href="/agents">查看全部</a>}>
    <Table
      dataSource={agents}
      columns={columns}
      rowKey="id"
      pagination={{ pageSize: 5 }}
      size="small"
      scroll={{ x: 640 }}
    />
  </Card>
)

export default AgentTable
