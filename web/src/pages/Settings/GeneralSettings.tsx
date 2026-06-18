import { Button, Form, Input, InputNumber, Select, Switch, Tabs } from 'antd'
import { SaveOutlined } from '@ant-design/icons'

interface GeneralSettingsProps {
  loading: boolean
  onSave: (values: GeneralSettingsValues) => void
}

interface GeneralSettingsValues {
  siteName?: string
  refreshInterval?: number | null
  enableNotifications?: boolean
  theme?: string
  compactMode?: boolean
  showResourceCount?: boolean
}

// 通用设置：基础设置 + 显示设置
export const GeneralSettings: React.FC<GeneralSettingsProps> = ({ loading, onSave }) => {
  const [generalForm] = Form.useForm()

  const items = [
    {
      key: 'basic',
      label: '基础设置',
      children: (
        <Form
          form={generalForm}
          layout="vertical"
          initialValues={{
            siteName: 'Cockpit',
            refreshInterval: 30,
            enableNotifications: true,
          }}
          onFinish={onSave}
        >
          <Form.Item label="站点名称" name="siteName">
            <Input />
          </Form.Item>
          <Form.Item label="数据刷新间隔 (秒)" name="refreshInterval">
            <InputNumber min={5} max={300} style={{ width: 200 }} />
          </Form.Item>
          <Form.Item label="启用通知" name="enableNotifications" valuePropName="checked">
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
      key: 'display',
      label: '显示设置',
      children: (
        <Form
          layout="vertical"
          initialValues={{ theme: 'light', compactMode: false, showResourceCount: true }}
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
          <Form.Item label="显示资源数量" name="showResourceCount" valuePropName="checked">
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

  return <Tabs items={items} tabBarStyle={{ marginBottom: 24 }} />
}
