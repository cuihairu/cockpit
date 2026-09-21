import { Card, Progress, Typography } from 'antd'

const { Title, Text } = Typography

// 基础设施在线率：>=80 success、>=50 normal、否则 exception
const HealthBar = ({ onlineRate }: { onlineRate: number }) => (
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
)

export default HealthBar
