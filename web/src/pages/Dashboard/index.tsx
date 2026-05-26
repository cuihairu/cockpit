import { Card, Row, Col, Statistic, Progress, Table, Tag, Space, Typography, Button } from 'antd'
import {
  CloudServerOutlined,
  SafetyOutlined,
  CheckCircleOutlined,
  WarningOutlined,
  ReloadOutlined,
} from '@ant-design/icons'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/services/api'
import type { ColumnsType } from 'antd/es/table'
import './index.less'

const { Title, Text } = Typography

const Dashboard = () => {
  const { data: status, refetch } = useQuery({
    queryKey: ['status'],
    queryFn: () => api.getStatus(),
    refetchInterval: 30000,
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

  const resourceCards = [
    {
      title: '运行中服务',
      value: stats.services.running,
      suffix: '个',
      icon: <CheckCircleOutlined />,
      color: 'stat-card-green',
    },
    {
      title: '异常服务',
      value: stats.services.down,
      suffix: '个',
      icon: <WarningOutlined />,
      color: 'stat-card-red',
    },
    {
      title: '在线 Agent',
      value: stats.infrastructure.online,
      suffix: `/${stats.infrastructure.total}`,
      icon: <CloudServerOutlined />,
      color: 'stat-card-blue',
    },
    {
      title: '有效域名',
      value: stats.domains.valid,
      suffix: '个',
      icon: <SafetyOutlined />,
      color: 'stat-card-purple',
    },
    {
      title: '有效证书',
      value: stats.certificates.valid,
      suffix: '个',
      icon: <SafetyOutlined />,
      color: 'stat-card-cyan',
    },
    {
      title: '即将过期',
      value: stats.certificates.expiring,
      suffix: '个',
      icon: <WarningOutlined />,
      color: 'stat-card-orange',
    },
  ]

  const agentColumns: ColumnsType<any> = [
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
      render: (_: any, record: any) => `${record.location?.region || '-'}/${record.location?.zone || '-'}`,
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
      render: (caps: string[]) => (
        <Space size={4}>
          {caps?.slice(0, 3).map((cap, i) => (
            <Tag key={i} color="processing" style={{ fontSize: 11 }}>
              {cap}
            </Tag>
          ))}
          {caps?.length > 3 && <Tag>+{caps.length - 3}</Tag>}
        </Space>
      ),
    },
  ]

  return (
    <div className="dashboard-container">
      <div style={{ marginBottom: 24, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <div>
          <Title level={3} style={{ margin: 0 }}>总览</Title>
          <Text type="secondary">实时监控您的混合基础设施状态</Text>
        </div>
        <Button icon={<ReloadOutlined />} onClick={() => refetch()}>
          刷新
        </Button>
      </div>

      <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
        {resourceCards.map((card, index) => (
          <Col xs={24} sm={12} md={8} lg={4} key={index}>
            <div className={`stat-card ${card.color}`}>
              <div style={{ fontSize: 20, marginBottom: 8 }}>{card.icon}</div>
              <Statistic
                title={card.title}
                value={card.value}
                suffix={card.suffix}
                valueStyle={{ fontSize: 24, fontWeight: 600 }}
              />
            </div>
          </Col>
        ))}
      </Row>

      <Card style={{ marginBottom: 24 }} variant="borderless">
        <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
          <Title level={5} style={{ margin: 0 }}>基础设施健康度</Title>
          <Text type="secondary">{onlineRate}% 在线</Text>
        </div>
        <Progress
          percent={onlineRate}
          strokeColor={{
            '0%': '#108ee9',
            '100%': '#87d068',
          }}
          status={onlineRate >= 80 ? 'success' : onlineRate >= 50 ? 'normal' : 'exception'}
        />
      </Card>

      <Card title="Agent 列表" variant="borderless" extra={<a href="/agents">查看全部</a>}>
        <Table
          dataSource={agents || []}
          columns={agentColumns}
          rowKey="id"
          pagination={{ pageSize: 5 }}
          size="small"
        />
      </Card>
    </div>
  )
}

export default Dashboard
