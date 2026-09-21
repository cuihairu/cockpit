import { Col, Row, Statistic } from 'antd'
import {
  CheckCircleOutlined,
  CloudServerOutlined,
  SafetyOutlined,
  WarningOutlined,
} from '@ant-design/icons'
import type { StatusResponse } from '@/types'

interface StatCardsProps {
  stats: StatusResponse
  showAll: boolean
}

interface StatCardItem {
  title: string
  value: number
  suffix: string
  icon: React.ReactNode
  color: string
}

const buildCards = (stats: StatusResponse): StatCardItem[] => [
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

// 六枚资源概览卡（前四枚常驻，后两枚由设置 showResourceCount 控制）
const StatCards = ({ stats, showAll }: StatCardsProps) => {
  const cards = showAll ? buildCards(stats) : buildCards(stats).slice(0, 4)
  return (
    <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
      {cards.map((card, index) => (
        <Col xs={12} sm={12} md={8} lg={4} key={index}>
          <div className={`stat-card ${card.color}`}>
            <div className="stat-card-icon">{card.icon}</div>
            <Statistic
              title={card.title}
              value={card.value}
              suffix={card.suffix}
            />
          </div>
        </Col>
      ))}
    </Row>
  )
}

export default StatCards
