import { Descriptions, Space, Tag } from 'antd'
import type { Agent } from '@/types'

const OverviewPanel = ({ agent }: { agent: Agent | null }) => {
  if (!agent) {
    return <div style={{ padding: 24, color: '#86909C' }}>请选择左侧服务器</div>
  }

  return (
    <Descriptions bordered column={2} size="small">
      <Descriptions.Item label="Agent ID" span={2}>
        <code>{agent.id}</code>
      </Descriptions.Item>
      <Descriptions.Item label="主机名">{agent.hostname || '-'}</Descriptions.Item>
      <Descriptions.Item label="IP">{agent.ip || '-'}</Descriptions.Item>
      <Descriptions.Item label="地域">{agent.location?.region || '-'}</Descriptions.Item>
      <Descriptions.Item label="可用区">{agent.location?.zone || '-'}</Descriptions.Item>
      <Descriptions.Item label="状态">
        <Tag color={agent.status === 'online' ? 'success' : 'default'}>
          {agent.status === 'online' ? '在线' : '离线'}
        </Tag>
      </Descriptions.Item>
      <Descriptions.Item label="能力" span={2}>
        <Space wrap>
          {(agent.capabilities || []).map((cap) => (
            <Tag key={cap.type}>{cap.type}</Tag>
          ))}
        </Space>
      </Descriptions.Item>
    </Descriptions>
  )
}

export default OverviewPanel
