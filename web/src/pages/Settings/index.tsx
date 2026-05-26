import { useState } from 'react'
import { Card, Form, Input, InputNumber, Switch, Button, Space, Tabs, message, Select, Descriptions, Statistic } from 'antd'
import { SaveOutlined } from '@ant-design/icons'

const Settings = () => {
  const [form] = Form.useForm()
  const [loading, setLoading] = useState(false)

  const handleSave = async (values: any) => {
    setLoading(true)
    try {
      // TODO: 调用 API 保存设置
      console.log('Saving settings:', values)
      message.success('设置已保存')
    } finally {
      setLoading(false)
    }
  }

  const generalItems = [
    {
      key: '1',
      label: '基础设置',
      children: (
        <Form
          form={form}
          layout="vertical"
          initialValues={{
            siteName: 'Cockpit',
            refreshInterval: 30,
            enableNotifications: true,
          }}
          onFinish={handleSave}
        >
          <Form.Item label="站点名称" name="siteName">
            <Input />
          </Form.Item>

          <Form.Item label="数据刷新间隔 (秒)" name="refreshInterval">
            <InputNumber min={5} max={300} style={{ width: 200 }} />
          </Form.Item>

          <Form.Item
            label="启用通知"
            name="enableNotifications"
            valuePropName="checked"
          >
            <Switch />
          </Form.Item>

          <Form.Item>
            <Button type="primary" htmlType="submit" icon={<SaveOutlined />} loading={loading}>
              保存设置
            </Button>
          </Form.Item>
        </Form>
      ),
    },
    {
      key: '2',
      label: '显示设置',
      children: (
        <Form
          layout="vertical"
          initialValues={{
            theme: 'light',
            compactMode: false,
            showResourceCount: true,
          }}
        >
          <Form.Item label="主题" name="theme">
            <Select style={{ width: 200 }}>
              <Select.Option value="light">浅色</Select.Option>
              <Select.Option value="dark">深色</Select.Option>
              <Select.Option value="auto">跟随系统</Select.Option>
            </Select>
          </Form.Item>

          <Form.Item label="紧凑模式" name="compactMode" valuePropName="checked">
            <Switch />
          </Form.Item>

          <Form.Item
            label="显示资源数量"
            name="showResourceCount"
            valuePropName="checked"
          >
            <Switch />
          </Form.Item>

          <Form.Item>
            <Button type="primary" htmlType="submit" icon={<SaveOutlined />}>
              保存设置
            </Button>
          </Form.Item>
        </Form>
      ),
    },
  ]

  const alertItems = [
    {
      key: '1',
      label: '告警规则',
      children: (
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
      ),
    },
  ]

  const systemItems = [
    {
      key: '1',
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
      key: '2',
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

  return (
    <div className="page-container">
      <Card title="系统设置">
      <Tabs
        items={[
          {
            key: 'general',
            label: '通用设置',
            children: (
              <Tabs items={generalItems} tabBarStyle={{ marginBottom: 24 }} />
            ),
          },
          {
            key: 'alerts',
            label: '告警设置',
            children: <Tabs items={alertItems} tabBarStyle={{ marginBottom: 24 }} />,
          },
          {
            key: 'system',
            label: '系统信息',
            children: <Tabs items={systemItems} tabBarStyle={{ marginBottom: 24 }} />,
          },
        ]}
      />
    </Card>
    </div>
  )
}

export default Settings
