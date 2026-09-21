import { Button, Typography } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/services/api'
import { useSettingsContext } from '@/contexts/useSettingsContext'
import AgentTable from './AgentTable'
import HealthBar from './HealthBar'
import StatCards from './StatCards'
import './index.less'

const { Title, Text } = Typography

// 总览：资源六卡 + 基础设施健康度 + Agent 概览表（完整管理在 /agents）
const Dashboard = () => {
  const { settings } = useSettingsContext()
  const { data: status, refetch } = useQuery({
    queryKey: ['status'],
    queryFn: () => api.getStatus(),
    refetchInterval: settings.refreshInterval * 1000,
  })

  const { data: agents } = useQuery({
    queryKey: ['agents'],
    queryFn: () => api.getAgents(),
  })

  const stats = status || {
    services: { running: 0, down: 0, unknown: 0 },
    domains: { valid: 0, expiring: 0 },
    certificates: { valid: 0, expiring: 0 },
    infrastructure: { total: 0, online: 0 },
  }

  const onlineRate = stats.infrastructure.total > 0
    ? Math.round((stats.infrastructure.online / stats.infrastructure.total) * 100)
    : 0

  return (
    <div className="dashboard-container">
      <div style={{ marginBottom: 24, display: 'flex', flexWrap: 'wrap', gap: 8, justifyContent: 'space-between', alignItems: 'center' }}>
        <div>
          <Title level={3} style={{ margin: 0 }}>总览</Title>
          <Text type="secondary">实时监控您的混合基础设施状态</Text>
        </div>
        <Button icon={<ReloadOutlined />} onClick={() => refetch()}>
          刷新
        </Button>
      </div>

      <StatCards stats={stats} showAll={settings.showResourceCount} />
      <HealthBar onlineRate={onlineRate} />
      <AgentTable agents={agents || []} />
    </div>
  )
}

export default Dashboard
