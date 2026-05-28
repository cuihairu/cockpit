import { Button, Card, Descriptions, Space, Tag } from 'antd'
import type { RemoteService, WorkbenchTab } from './types'

const ConnectionPanel = ({
  protocol,
  service,
  onConnect,
}: {
  protocol: 'ssh' | 'rdp' | 'vnc'
  service?: RemoteService
  onConnect: () => void
}) => {
  return (
    <Card size="small" title={`${protocol.toUpperCase()} 连接`}>
      {service ? (
        <Space direction="vertical" size={12}>
          <Descriptions column={2} size="small">
            <Descriptions.Item label="主机">{service.host}</Descriptions.Item>
            <Descriptions.Item label="端口">{service.port}</Descriptions.Item>
            <Descriptions.Item label="服务">{service.name}</Descriptions.Item>
            <Descriptions.Item label="状态">
              <Tag color="success">可用</Tag>
            </Descriptions.Item>
          </Descriptions>
          <Button type="primary" onClick={onConnect}>
            打开 {protocol.toUpperCase()}
          </Button>
        </Space>
      ) : (
        <div style={{ padding: 24, color: '#86909C' }}>当前服务器未检测到 {protocol.toUpperCase()} 服务</div>
      )}
    </Card>
  )
}

export default ConnectionPanel
