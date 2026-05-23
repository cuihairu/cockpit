import { Card, Col, Row, Statistic, Table, Tag, Typography } from 'antd'
import { PageContainer } from '@ant-design/pro-layout'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/services/api'
import type { ColumnsType } from 'antd/es/table'

const { Title } = Typography

interface DashboardData {
  services: { running: number; down: number; unknown: number }
  domains: { valid: number; expiring: number }
  certificates: { valid: number; expiring: number }
  infrastructure: { total: number; online: number }
}

const Dashboard = () => {
  // 获取系统状态
  const { data: status, isLoading } = useQuery({
    queryKey: ['status'],
    queryFn: () => api.getStatus(),
  })

  // 获取 Agent 列表
  const { data: agents } = useQuery({
    queryKey: ['agents'],
    queryFn: () => api.getAgents(),
  })

  // Agent 表格列
  const agentColumns: ColumnsType<any> = [
    {
      title: 'Agent ID',
      dataIndex: 'id',
      key: 'id',
      width: 200,
    },
    {
      title: '位置',
      key: 'location',
      render: (_, record) => `${record.location?.region || '-'} / ${record.location?.zone || '-'}`,
    },
    {
      title: '主机名',
      dataIndex: 'hostname',
      key: 'hostname',
    },
    {
      title: 'IP',
      dataIndex: 'ip',
      key: 'ip',
    },
    {
      title: '能力',
      key: 'capabilities',
      render: (_, record) => (
        <>
          {record.capabilities?.map((cap: any) => (
            <Tag key={cap.type} color="blue">
              {cap.type}
            </Tag>
          ))}
        </>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status: string) => (
        <Tag color={status === 'online' ? 'success' : 'error'}>{status}</Tag>
      ),
    },
  ]

  return (
    <PageContainer
      header={{
        title: '仪表盘',
      }}
    >
      <Row gutter={[16, 16]}>
        {/* 统计卡片 */}
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic
              title="服务状态"
              value={status?.services?.running || 0}
              suffix={`/ ${status?.services?.down || 0} 个异常`}
              loading={isLoading}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic
              title="域名"
              value={status?.domains?.valid || 0}
              suffix={`/ ${status?.domains?.expiring || 0} 个即将过期`}
              loading={isLoading}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic
              title="证书"
              value={status?.certificates?.valid || 0}
              suffix={`/ ${status?.certificates?.expiring || 0} 个即将过期`}
              loading={isLoading}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic
              title="基础设施"
              value={status?.infrastructure?.online || 0}
              suffix={`/ ${status?.infrastructure?.total || 0} 在线`}
              loading={isLoading}
            />
          </Card>
        </Col>

        {/* Agent 列表 */}
        <Col span={24}>
          <Card title="在线 Agents">
            <Table
              columns={agentColumns}
              dataSource={agents || []}
              rowKey="id"
              pagination={false}
              size="small"
            />
          </Card>
        </Col>

        {/* 快捷入口 */}
        <Col span={24}>
          <Card title="快捷入口">
            <Row gutter={16}>
              <Col span={6}>
                <a href="/resources/compute" target="_blank">
                  <Card type="inner" hoverable>
                    <Card.Meta title="计算实例" description="管理 PVE VM、LXC、VPS 等" />
                  </Card>
                </a>
              </Col>
              <Col span={6}>
                <a href="/resources/domains" target="_blank">
                  <Card type="inner" hoverable>
                    <Card.Meta title="域名管理" description="查看域名到期、DNS 配置" />
                  </Card>
                </a>
              </Col>
              <Col span={6}>
                <a href="/resources/certificates" target="_blank">
                  <Card type="inner" hoverable>
                    <Card.Meta title="SSL 证书" description="监控证书过期状态" />
                  </Card>
                </a>
              </Col>
              <Col span={6}>
                <a href="/resources/services" target="_blank">
                  <Card type="inner" hoverable>
                    <Card.Meta title="服务状态" description="查看服务健康检查" />
                  </Card>
                </a>
              </Col>
            </Row>
          </Card>
        </Col>
      </Row>
    </PageContainer>
  )
}

export default Dashboard
