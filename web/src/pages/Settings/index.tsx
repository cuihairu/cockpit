import { useState, useEffect } from 'react'
import { Card, Form, Input, InputNumber, Switch, Button, Space, Tabs, message, Select, Descriptions, Statistic, Alert, Modal, Popconfirm } from 'antd'
import { SaveOutlined, SafetyOutlined, CheckCircleOutlined, WarningOutlined } from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { api } from '@/services/api'
import type { UserInfo } from '@/types'
import { logger } from '@/utils/logger'

const Settings = () => {
  const [form] = Form.useForm()
  const [loading, setLoading] = useState(false)
  const navigate = useNavigate()
  const [userInfo, setUserInfo] = useState<UserInfo | null>(null)
  const [totpVerifyCode, setTotpVerifyCode] = useState('')
  const [disablingTOTP, setDisablingTOTP] = useState(false)
  const [showDisableModal, setShowDisableModal] = useState(false)

  // 获取用户信息（包括 TOTP 状态）
  useEffect(() => {
    const fetchUserInfo = async () => {
      try {
        const info = await api.getCurrentUser()
        setUserInfo(info)
      } catch (err) {
        logger.error('Failed to fetch user info:', err)
        // 降级到 localStorage
        const username = localStorage.getItem('username')
        if (username) {
          setUserInfo({
            id: localStorage.getItem('userId') || '',
            username,
            role: localStorage.getItem('role') || 'user',
            totp_enabled: false,
          })
        }
      }
    }
    fetchUserInfo()
  }, [])

  const handleSave = async (values: any) => {
    setLoading(true)
    try {
      await api.saveSettings({
        siteName: values.siteName,
        refreshInterval: values.refreshInterval,
        enableNotifications: values.enableNotifications,
        theme: values.theme,
        compactMode: values.compactMode,
        showResourceCount: values.showResourceCount,
      })
      message.success('设置已保存')
    } catch (err: any) {
      const errorMsg = err.response?.data?.error || '保存设置失败'
      message.error(errorMsg)
    } finally {
      setLoading(false)
    }
  }

  const handleDisableTOTP = async () => {
    if (!totpVerifyCode || totpVerifyCode.length !== 6) {
      message.warning('请输入 6 位验证码')
      return
    }

    setDisablingTOTP(true)
    try {
      await api.disableTOTP(totpVerifyCode)
      message.success('TOTP 已禁用')
      setShowDisableModal(false)
      setTotpVerifyCode('')
      // 刷新用户信息
      if (userInfo) {
        setUserInfo({ ...userInfo, totp_enabled: false, totp_setup_at: undefined })
      }
    } catch (err: any) {
      const errorMsg = err.response?.data?.error || '操作失败'
      message.error(errorMsg)
    } finally {
      setDisablingTOTP(false)
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

  const securityItems = [
    {
      key: '1',
      label: '二次验证 (TOTP)',
      children: (
        <Space direction="vertical" size="large" style={{ width: '100%' }}>
          <Alert
            message="关于二次验证"
            description="TOTP (Time-based One-Time Password) 为您的账户提供额外的安全保护。启用后，登录时需要输入认证器应用生成的 6 位验证码。"
            type="info"
            showIcon
          />

          <Card size="small" title="当前状态">
            {userInfo?.totp_enabled ? (
              <Space direction="vertical" style={{ width: '100%' }}>
                <Space>
                  <CheckCircleOutlined style={{ color: '#52c41a', fontSize: 18 }} />
                  <span style={{ fontSize: 14 }}>
                    <strong>TOTP 已启用</strong>
                  </span>
                </Space>
                {userInfo.totp_setup_at && (
                  <span style={{ fontSize: 12, color: '#999' }}>
                    启用时间: {new Date(userInfo.totp_setup_at).toLocaleString('zh-CN')}
                  </span>
                )}
                <Popconfirm
                  title="禁用二次验证"
                  description="禁用后账户安全性会降低，确定要继续吗？"
                  onConfirm={() => setShowDisableModal(true)}
                  okText="确定"
                  cancelText="取消"
                >
                  <Button danger>禁用 TOTP</Button>
                </Popconfirm>
              </Space>
            ) : (
              <Space direction="vertical" style={{ width: '100%' }}>
                <Space>
                  <WarningOutlined style={{ color: '#faad14', fontSize: 18 }} />
                  <span style={{ fontSize: 14 }}>
                    <strong>TOTP 未启用</strong>
                  </span>
                </Space>
                <p style={{ fontSize: 12, margin: 0, color: '#999' }}>
                  建议启用 TOTP 以保护账户安全
                </p>
                <Button
                  type="primary"
                  icon={<SafetyOutlined />}
                  onClick={() => navigate('/settings/setup-totp')}
                >
                  立即启用
                </Button>
              </Space>
            )}
          </Card>

          <Card size="small" title="支持的认证器应用">
            <Space wrap>
              <a
                href="https://apps.apple.com/app/google-authenticator/id388497605"
                target="_blank"
                rel="noopener noreferrer"
              >
                Google Authenticator (iOS)
              </a>
              <a
                href="https://play.google.com/store/apps/details?id=com.google.android.apps.authenticator2"
                target="_blank"
                rel="noopener noreferrer"
              >
                Google Authenticator (Android)
              </a>
              <a
                href="https://apps.apple.com/app/microsoft-authenticator/id983156458"
                target="_blank"
                rel="noopener noreferrer"
              >
                Microsoft Authenticator (iOS)
              </a>
              <a
                href="https://play.google.com/store/apps/details?id=com.azure.authenticator"
                target="_blank"
                rel="noopener noreferrer"
              >
                Microsoft Authenticator (Android)
              </a>
            </Space>
          </Card>
        </Space>
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
            key: 'security',
            label: '安全设置',
            children: <Tabs items={securityItems} tabBarStyle={{ marginBottom: 24 }} />,
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

    {/* 禁用 TOTP 验证模态框 */}
    <Modal
      title="禁用二次验证"
      open={showDisableModal}
      onCancel={() => {
        setShowDisableModal(false)
        setTotpVerifyCode('')
      }}
      onOk={handleDisableTOTP}
      confirmLoading={disablingTOTP}
      okText="确认禁用"
      cancelText="取消"
    >
      <Space direction="vertical" style={{ width: '100%' }}>
        <Alert
          message="安全警告"
          description="禁用 TOTP 后，您的账户将仅受密码保护。建议在启用 TOTP 的同时妥善保管备份码。"
          type="warning"
          showIcon
        />
        <div>
          <p>请输入当前 TOTP 验证码以确认禁用：</p>
          <Input
            size="large"
            value={totpVerifyCode}
            onChange={(e) => setTotpVerifyCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
            placeholder="123456"
            maxLength={6}
            style={{ textAlign: 'center', fontSize: 20, letterSpacing: 4 }}
            autoFocus
          />
        </div>
      </Space>
    </Modal>
    </div>
  )
}

export default Settings
