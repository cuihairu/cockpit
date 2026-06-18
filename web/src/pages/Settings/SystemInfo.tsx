import { Card, Descriptions, Space, Statistic, Tabs } from 'antd'

// 系统信息：版本/构建信息 + 资源统计
export const SystemInfo: React.FC = () => {
  const items = [
    {
      key: 'info',
      label: '系统信息',
      children: (
        <Descriptions bordered column={1}>
          <Descriptions.Item label="版本">v0.1.0</Descriptions.Item>
          <Descriptions.Item label="构建时间">2024-05-24</Descriptions.Item>
          <Descriptions.Item label="Go 版本">1.23</Descriptions.Item>
          <Descriptions.Item label="数据库">SQLite3</Descriptions.Item>
        </Descriptions>
      ),
    },
    {
      key: 'stats',
      label: '数据统计',
      children: (
        <Space direction="vertical" style={{ width: '100%' }}>
          <Card size="small" title="Agent">
            <Space>
              <Statistic title="在线" value={0} />
              <Statistic title="离线" value={0} />
            </Space>
          </Card>
          <Card size="small" title="资源">
            <Space>
              <Statistic title="计算实例" value={0} />
              <Statistic title="域名" value={0} />
              <Statistic title="证书" value={0} />
            </Space>
          </Card>
        </Space>
      ),
    },
  ]

  return <Tabs items={items} />
}
