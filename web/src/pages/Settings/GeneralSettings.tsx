import { useEffect } from 'react'
import { Button, Form, Input, InputNumber, Radio, Select, Switch, Tabs } from 'antd'
import { SaveOutlined } from '@ant-design/icons'
import ThemeSkinCards from '@/components/ThemeSkinCards'
import type { UISettings } from '@/contexts/settingsTypes'
import { THEME_PRESETS } from '@/theme/themePresets'

interface GeneralSettingsProps {
  loading: boolean
  settings: UISettings
  onSave: (values: GeneralSettingsValues) => void
}

export interface GeneralSettingsValues {
  siteName?: string
  refreshInterval?: number | null
  enableNotifications?: boolean
  theme?: 'light' | 'dark' | 'auto'
  themeColor?: string
  themeSkin?: string
  compactMode?: boolean
  showResourceCount?: boolean
}

// 通用设置：基础设置 + 显示设置
export const GeneralSettings: React.FC<GeneralSettingsProps> = ({ loading, settings, onSave }) => {
  const [generalForm] = Form.useForm()
  const [displayForm] = Form.useForm()

  useEffect(() => {
    generalForm.setFieldsValue({
      siteName: settings.siteName,
      refreshInterval: settings.refreshInterval,
      enableNotifications: settings.enableNotifications,
    })
    displayForm.setFieldsValue({
      theme: settings.theme,
      themeColor: settings.themeColor,
      compactMode: settings.compactMode,
      showResourceCount: settings.showResourceCount,
    })
  }, [displayForm, generalForm, settings])

  const items = [
    {
      key: 'basic',
      label: '基础设置',
      children: (
        <Form
          form={generalForm}
          layout="vertical"
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
          form={displayForm}
          layout="vertical"
          onFinish={onSave}
        >
          <Form.Item label="主题" name="theme">
            <Select style={{ width: 200 }}>
              <Select.Option value="light">浅色</Select.Option>
              <Select.Option value="dark">深色</Select.Option>
              <Select.Option value="auto">跟随系统</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item
            label="主题皮肤"
            extra="整屏配色方案，点选即换；与主题色（强调色）相互独立，可任意组合"
          >
            <ThemeSkinCards />
          </Form.Item>
          <Form.Item label="主题色" name="themeColor">
            <Radio.Group>
              {THEME_PRESETS.map((preset) => (
                <Radio.Button key={preset.key} value={preset.key} style={{ paddingInline: 10 }}>
                  <span
                    aria-hidden="true"
                    style={{
                      display: 'inline-block',
                      width: 12,
                      height: 12,
                      borderRadius: '50%',
                      background: preset.color,
                      marginInlineEnd: 6,
                      verticalAlign: 'middle',
                    }}
                  />
                  {preset.name}
                </Radio.Button>
              ))}
            </Radio.Group>
          </Form.Item>
          <Form.Item label="紧凑模式" name="compactMode" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item label="显示资源数量" name="showResourceCount" valuePropName="checked">
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
  ]

  return <Tabs items={items} tabBarStyle={{ marginBottom: 24 }} />
}
