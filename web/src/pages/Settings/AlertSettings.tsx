import { useEffect, useState } from 'react'
import axios from 'axios'
import { useQuery } from '@tanstack/react-query'
import {
  Alert,
  Button,
  Card,
  Descriptions,
  Form,
  InputNumber,
  Space,
  Table,
  Tag,
  Typography,
  message,
} from 'antd'
import { SaveOutlined, SendOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { api } from '@/services/api'
import type { NotificationChannelSummary, NotificationSendResult } from '@/types'
import { getApiErrorMessage } from '@/utils/apiError'

// 渠道类型 → 展示名与颜色
const channelMeta: Record<string, { label: string; color: string }> = {
  herald: { label: 'Herald', color: 'geekblue' },
  ntfy: { label: 'ntfy', color: 'green' },
  webhook: { label: 'Webhook', color: 'orange' },
  telegram: { label: 'Telegram', color: 'blue' },
}

const channelColumns: ColumnsType<NotificationChannelSummary> = [
  {
    title: '类型',
    dataIndex: 'channel',
    key: 'channel',
    width: 120,
    render: (ch: string) => (
      <Tag color={channelMeta[ch]?.color || 'default'}>{channelMeta[ch]?.label || ch}</Tag>
    ),
  },
  { title: '目标', dataIndex: 'target', key: 'target', ellipsis: true },
]

// 拨测与通知设置：探测间隔（服务端持久化、立即生效）+ 通知渠道状态 + 测试通知。
// 通知渠道（herald/ntfy/webhook/telegram）在服务端 config.yaml 配置，此处只读展示。
export const AlertSettings: React.FC = () => {
  const [form] = Form.useForm<{ intervalSeconds: number }>()
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const [testResults, setTestResults] = useState<NotificationSendResult[] | null>(null)

  const { data: probeConfig, isLoading: probeLoading } = useQuery({
    queryKey: ['probe-config'],
    queryFn: () => api.getProbeConfig(),
  })
  const { data: notifStatus, isLoading: notifLoading } = useQuery({
    queryKey: ['notification-status'],
    queryFn: () => api.getNotificationStatus(),
  })

  // 探测配置加载完成后回填表单
  useEffect(() => {
    if (probeConfig) {
      form.setFieldValue('intervalSeconds', probeConfig.interval_seconds)
    }
  }, [probeConfig, form])

  const saveInterval = async (values: { intervalSeconds: number }) => {
    setSaving(true)
    try {
      const updated = await api.saveProbeConfig(values.intervalSeconds)
      message.success(`探测间隔已更新为 ${updated.interval_seconds} 秒，下一轮探测生效`)
    } catch (err) {
      message.error(getApiErrorMessage(err, '保存探测间隔失败'))
    } finally {
      setSaving(false)
    }
  }

  const sendTest = async () => {
    setTesting(true)
    setTestResults(null)
    try {
      const res = await api.testNotification()
      setTestResults(res.results)
      if (res.results.every((r) => r.ok)) {
        message.success('测试通知已发送到全部渠道')
      } else {
        message.warning('部分渠道发送失败，详见下方结果')
      }
    } catch (err) {
      if (axios.isAxiosError(err) && err.response?.status === 503) {
        message.warning('通知未启用：请在服务端 config.yaml 的 notification 段配置渠道')
      } else {
        message.error(getApiErrorMessage(err, '发送测试通知失败'))
      }
    } finally {
      setTesting(false)
    }
  }

  const channels = notifStatus?.channels ?? []

  return (
    <Space direction="vertical" style={{ width: '100%' }} size={16}>
      <Card type="inner" title="拨测" loading={probeLoading}>
        <Form form={form} layout="inline" onFinish={saveInterval}>
          <Form.Item label="探测间隔" required style={{ marginBottom: 8 }}>
            <Form.Item
              name="intervalSeconds"
              noStyle
              rules={[
                { required: true, message: '请输入探测间隔' },
                {
                  type: 'number',
                  min: probeConfig?.min_interval_seconds ?? 30,
                  max: probeConfig?.max_interval_seconds ?? 3600,
                  message: `范围 ${probeConfig?.min_interval_seconds ?? 30} - ${
                    probeConfig?.max_interval_seconds ?? 3600
                  } 秒`,
                },
              ]}
            >
              <InputNumber
                min={probeConfig?.min_interval_seconds ?? 30}
                max={probeConfig?.max_interval_seconds ?? 3600}
                style={{ width: 140 }}
                addonAfter="秒"
              />
            </Form.Item>
          </Form.Item>
          <Form.Item style={{ marginBottom: 8 }}>
            <Button type="primary" htmlType="submit" icon={<SaveOutlined />} loading={saving}>
              保存
            </Button>
          </Form.Item>
        </Form>
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          服务/域名/证书的自动健康探测周期，保存后立即生效并持久化（默认 300 秒）。
        </Typography.Text>
      </Card>

      <Card
        type="inner"
        title="通知渠道"
        loading={notifLoading}
        extra={
          <Button
            icon={<SendOutlined />}
            onClick={sendTest}
            loading={testing}
            disabled={!notifStatus?.enabled || channels.length === 0}
          >
            发送测试通知
          </Button>
        }
      >
        {channels.length === 0 ? (
          <Alert
            type="info"
            showIcon
            message="尚未配置通知渠道"
            description={
              <Typography.Text style={{ fontSize: 12 }}>
                在服务端 config.yaml 的 <Typography.Text code>notification</Typography.Text> 段配置
                herald / ntfy / webhook / telegram 渠道（支持多目标），重启服务后此处显示渠道状态。
              </Typography.Text>
            }
          />
        ) : (
          <>
            <Table
              columns={channelColumns}
              dataSource={channels}
              rowKey={(r) => `${r.channel}-${r.target}`}
              pagination={false}
              size="small"
            />
            {notifStatus?.events && notifStatus.events.length > 0 && (
              <div style={{ marginTop: 12 }}>
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                  已配置的事件开关：
                </Typography.Text>
                <Space size={4} wrap style={{ marginLeft: 8 }}>
                  {notifStatus.events.map((e) => (
                    <Tag key={e.type} color={e.enabled ? 'success' : 'default'}>
                      {e.type} {e.enabled ? '开' : '关'}
                    </Tag>
                  ))}
                </Space>
                <Typography.Text
                  type="secondary"
                  style={{ fontSize: 12, display: 'block', marginTop: 4 }}
                >
                  未列出的事件类型默认不发送（白名单机制）；恢复通知 service.up 需显式启用。
                </Typography.Text>
              </div>
            )}
          </>
        )}
        {testResults && (
          <Descriptions
            column={1}
            size="small"
            bordered
            style={{ marginTop: 12 }}
            items={testResults.map((r) => ({
              key: `${r.channel}-${r.target}`,
              label: channelMeta[r.channel]?.label || r.channel,
              children: r.ok ? (
                <Typography.Text type="success">发送成功</Typography.Text>
              ) : (
                <Typography.Text type="danger" style={{ fontSize: 12 }}>
                  {r.error || '发送失败'}
                </Typography.Text>
              ),
            }))}
          />
        )}
      </Card>
    </Space>
  )
}
