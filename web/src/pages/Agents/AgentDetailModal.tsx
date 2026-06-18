import { Descriptions, Modal, Space, Tag } from 'antd'
import type { Agent } from '@/types'
import RemoteServicesCard from '@/components/RemoteServices'
import type { RemoteProtocol } from '@/services/remote'
import { getVirtDisplay, getRemoteServices, type RemoteService } from './helpers'
import { LabelsDisplay } from './LabelsDisplay'

interface AgentDetailModalProps {
  open: boolean
  agent: Agent | null
  loading: boolean
  onClose: () => void
  onConnect: (protocol: RemoteProtocol, host: string, port: number) => void
}

// Agent 详情弹窗
export const AgentDetailModal: React.FC<AgentDetailModalProps> = ({
  open,
  agent,
  loading,
  onClose,
  onConnect,
}) => {
  if (!agent) return null

  return (
    <Modal
      title="Agent 详情"
      open={open}
      onCancel={onClose}
      footer={null}
      width={900}
    >
      <Descriptions bordered column={2} style={{ marginBottom: 16 }}>
        <Descriptions.Item label="Agent ID" span={2}>
          <code>{agent.id}</code>
        </Descriptions.Item>
        <Descriptions.Item label="主机名">{agent.hostname || '-'}</Descriptions.Item>
        <Descriptions.Item label="IP 地址">{agent.ip || '-'}</Descriptions.Item>
        <Descriptions.Item label="地域">{agent.location?.region || '-'}</Descriptions.Item>
        <Descriptions.Item label="可用区">{agent.location?.zone || '-'}</Descriptions.Item>
        <Descriptions.Item label="系统类型">
          {(() => {
            const config = getVirtDisplay(agent)
            return (
              <Tag icon={config.icon} color={config.color}>
                {config.label} ({agent.virtRole === 'guest' ? '虚拟机' : '宿主机'})
              </Tag>
            )
          })()}
        </Descriptions.Item>
        <Descriptions.Item label="状态">
          <Tag color={agent.status === 'online' ? 'success' : 'default'}>
            {agent.status === 'online' ? '在线' : '离线'}
          </Tag>
        </Descriptions.Item>
        <Descriptions.Item label="最后连接">
          {agent.lastSeen ? new Date(Number(agent.lastSeen) * 1000).toLocaleString() : '-'}
        </Descriptions.Item>
        <Descriptions.Item label="能力" span={2}>
          <Space size="small" wrap>
            {(agent.capabilities || []).map((cap) => (
              <Tag key={cap.type} color="blue">
                {cap.type}
              </Tag>
            ))}
          </Space>
        </Descriptions.Item>
        <Descriptions.Item label="标签" span={2}>
          <LabelsDisplay labels={agent.labels} />
        </Descriptions.Item>
      </Descriptions>

      <RemoteServicesCard
        agentId={agent.id}
        services={getRemoteServices(agent)}
        loading={loading}
        onConnect={onConnect}
      />
    </Modal>
  )
}

export type { RemoteService }
