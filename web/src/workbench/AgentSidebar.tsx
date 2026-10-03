import { Card, Input, List, Space, Tag, Typography } from 'antd'
import { ReloadOutlined, SearchOutlined, TagsOutlined } from '@ant-design/icons'
import type { Agent } from '@/types'

type Props = {
  agents: Agent[]
  loading: boolean
  query: string
  selectedAgentId: string
  onQueryChange: (value: string) => void
  onRefresh: () => void
  onSelect: (agentId: string) => void
  /** 全部标签名（去重），作筛选 chips */
  availableTags?: string[]
  activeTag?: string | null
  onTagFilter?: (name: string | null) => void
  onManageTags?: () => void
}

const AgentSidebar = ({
  agents,
  loading,
  query,
  selectedAgentId,
  onQueryChange,
  onRefresh,
  onSelect,
  availableTags = [],
  activeTag,
  onTagFilter,
  onManageTags,
}: Props) => {
  return (
    <Card
      title="服务器"
      extra={
        <Space size="middle">
          {onManageTags && (
            <TagsOutlined onClick={onManageTags} style={{ cursor: 'pointer' }} title="标签管理" />
          )}
          <ReloadOutlined onClick={onRefresh} style={{ cursor: 'pointer' }} />
        </Space>
      }
      style={{ width: '100%' }}
    >
      <Input
        allowClear
        prefix={<SearchOutlined />}
        placeholder="搜索主机名、IP、Agent ID、标签"
        value={query}
        onChange={(e) => onQueryChange(e.target.value)}
        style={{ marginBottom: 12 }}
      />
      {availableTags.length > 0 && (
        <Space size={[4, 4]} wrap style={{ marginBottom: 12 }}>
          {availableTags.map((name) => (
            <Tag.CheckableTag
              key={name}
              checked={activeTag === name}
              onChange={() => onTagFilter?.(activeTag === name ? null : name)}
            >
              {name}
            </Tag.CheckableTag>
          ))}
        </Space>
      )}
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
                  {agent.region || '-'} / {agent.zone || '-'}
                </Typography.Text>
                {agent.tags && agent.tags.length > 0 && (
                  <Space size={[2, 2]} wrap>
                    {agent.tags.map((t) => (
                      <Tag key={t.id} color={t.color || 'default'}>
                        {t.name}
                      </Tag>
                    ))}
                  </Space>
                )}
              </Space>
            </List.Item>
          )
        }}
      />
    </Card>
  )
}

export default AgentSidebar
