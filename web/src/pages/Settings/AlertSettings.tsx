import { Button, Form, InputNumber } from 'antd'
import { SaveOutlined } from '@ant-design/icons'

// 告警规则：证书/域名/服务过期提醒阈值
export const AlertSettings: React.FC = () => {
  return (
    <Form layout="vertical">
      <Form.Item label="证书过期提醒 (天)" name="certExpiryDays">
        <InputNumber min={1} max={90} style={{ width: 200 }} />
      </Form.Item>
      <Form.Item label="域名过期提醒 (天)" name="domainExpiryDays">
        <InputNumber min={1} max={90} style={{ width: 200 }} />
      </Form.Item>
      <Form.Item label="服务离线后提醒 (秒)" name="serviceDownSeconds">
        <InputNumber min={30} max={3600} style={{ width: 200 }} />
      </Form.Item>
      <Form.Item>
        <Button type="primary" htmlType="submit" icon={<SaveOutlined />}>
          保存规则
        </Button>
      </Form.Item>
    </Form>
  )
}
