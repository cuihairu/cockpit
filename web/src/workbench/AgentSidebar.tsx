import { Card, Input, List, Space, Tag, Typography } from 'antd'
import { ReloadOutlined, SearchOutlined } from '@ant-design/icons'
import type { Agent } from '@/types'

type Props = {
  agents: Agent[]
  loading: boolean
  query: string
  selectedAgentId: string
  onQueryChange: (value: string) => void
  onRefresh: () => void
  onSelect: (agentId: string) => void
}

const AgentSidebar = ({
  agents,
  loading,
  query,
  selectedAgentId,
  onQueryChange,
  onRefresh,
  onSelect,
}: Props) => {
  return (
    <Card
      title="服务器"
      extra={
        <ReloadOutlined onClick={onRefresh} style={{ cursor: 'pointer' }} />
      }
      style={{ width: '100%' }}
    >
      <Input
        allowClear
        prefix={<SearchOutlined />}
        placeholder="搜索主机名、IP、Agent ID"
        value={query}
        onChange={(e) => onQueryChange(e.target.value)}
        style={{ marginBottom: 12 }}
      />
      <List
        loading={loading}
        dataSource={agents}
        renderItem={(agent) => {
          const active = agent.id === selectedAgentId
          return (
            <List.Item
              onClick={() => onSelect(agent.id)}
              style={{
                cursor: 'pointer',
                padding: '12px 14px',
                borderRadius: 8,
                background: active ? 'rgba(22, 93, 255, 0.06)' : 'transparent',
              }}
            >
              <Space direction="vertical" size={4} style={{ width: '100%' }}>
                <Space style={{ justifyContent: 'space-between', width: '100%' }}>
                  <strong>{agent.hostname || agent.id}</strong>
                  <Tag color={agent.status === 'online' ? 'success' : 'default'}>
                    {agent.status === 'online' ? '在线' : '离线'}
                  </Tag>
                </Space>
                <Typography.Text type="secondary">{agent.ip || '-'}</Typography.Text>
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                  {agent.location?.region || '-'} / {agent.location?.zone || '-'}
                </Typography.Text>
              </Space>
            </List.Item>
          )
        }}
      />
    </Card>
  )
}

export default AgentSidebar
