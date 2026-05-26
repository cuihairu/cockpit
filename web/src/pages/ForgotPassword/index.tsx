import { useState } from 'react'
import { Form, Input, Button, message, Card, Typography, Space, Alert, Steps } from 'antd'
import { MailOutlined, ArrowLeftOutlined, CheckCircleOutlined } from '@ant-design/icons'
import { useNavigate, Link } from 'react-router-dom'
import { api } from '@/services/api'
import './index.less'

const { Title, Text, Paragraph } = Typography

const ForgotPassword = () => {
  const navigate = useNavigate()
  const [step, setStep] = useState<1 | 2 | 3>(1)
  const [loading, setLoading] = useState(false)
  const [username, setUsername] = useState('')
  const [email, setEmail] = useState('')
  const [maskedEmail, setMaskedEmail] = useState('')
  const [countdown, setCountdown] = useState(0)
  const [resendLoading, setResendLoading] = useState(false)

  const handleSubmitUsername = async (values: { username: string }) => {
    setLoading(true)
    try {
      const response = await api.forgotPassword(values.username)
      setUsername(values.username)
      setMaskedEmail(response.masked_email)
      setEmail(response.email)
      setStep(2)
      // 开始60秒倒计时
      startCountdown()
    } catch (err: any) {
      // 为了安全，即使用户不存在也显示类似的消息
      message.info('如果该用户存在，重置邮件已发送')
    } finally {
      setLoading(false)
    }
  }

  const handleResendEmail = async () => {
    setResendLoading(true)
    try {
      const response = await api.forgotPassword(username)
      setMaskedEmail(response.masked_email)
      message.success('重置邮件已重新发送')
      startCountdown()
    } catch (err: any) {
      message.info('如果该用户存在，重置邮件已发送')
    } finally {
      setResendLoading(false)
    }
  }

  const startCountdown = () => {
    setCountdown(60)
    const timer = setInterval(() => {
      setCountdown((prev) => {
        if (prev <= 1) {
          clearInterval(timer)
          return 0
        }
        return prev - 1
      })
    }, 1000)
  }

  return (
    <div className="forgot-password-page">
      <Card className="forgot-password-card">
        <div className="forgot-password-header">
          <Link to="/login" className="back-link">
            <ArrowLeftOutlined /> 返回登录
          </Link>
          <Title level={3}>找回密码</Title>
        </div>

        <Steps current={step - 1} className="forgot-password-steps">
          <Steps.Step title="输入用户名" />
          <Steps.Step title="验证邮箱" />
          <Steps.Step title="完成" />
        </Steps>

        <div className="forgot-password-body">
          {/* 步骤1: 输入用户名 */}
          {step === 1 && (
            <Space direction="vertical" size="large" className="step-content">
              <Alert
                message="重置密码流程"
                description="请输入您的用户名，我们将向注册邮箱发送密码重置链接。"
                type="info"
                showIcon
              />

              <Form
                layout="vertical"
                onFinish={handleSubmitUsername}
                autoComplete="off"
              >
                <Form.Item
                  label="用户名"
                  name="username"
                  rules={[{ required: true, message: '请输入用户名' }]}
                >
                  <Input
                    size="large"
                    placeholder="请输入您的用户名"
                    prefix={<MailOutlined />}
                  />
                </Form.Item>

                <Form.Item>
                  <Button
                    type="primary"
                    size="large"
                    htmlType="submit"
                    block
                    loading={loading}
                  >
                    发送重置邮件
                  </Button>
                </Form.Item>
              </Form>

              <div className="forgot-password-tips">
                <Text type="secondary">
                  💡 提示：请确保您的账户已绑定邮箱，如未绑定请联系管理员。
                </Text>
              </div>
            </Space>
          )}

          {/* 步骤2: 邮件已发送 */}
          {step === 2 && (
            <Space direction="vertical" size="large" className="step-content" align="center">
              <div className="email-sent-icon">
                <MailOutlined />
              </div>

              <Title level={4}>重置邮件已发送</Title>

              <Space direction="vertical" size="small" align="center">
                <Text>我们已向以下邮箱发送了密码重置邮件：</Text>
                <Text strong style={{ fontSize: 16 }}>
                  {maskedEmail}
                </Text>
                <Text type="secondary">
                  请查收邮件并点击重置链接，或输入邮件中的验证码
                </Text>
              </Space>

              <Space style={{ marginTop: 16 }}>
                <Button onClick={() => navigate('/login')}>
                  返回登录
                </Button>
                <Button
                  type="primary"
                  onClick={handleResendEmail}
                  loading={resendLoading}
                  disabled={countdown > 0}
                >
                  {countdown > 0 ? `${countdown}秒后可重发` : '重新发送'}
                </Button>
              </Space>

              <div className="email-tips">
                <Text type="secondary" style={{ fontSize: 12 }}>
                  📧 没有收到邮件？请检查垃圾邮件文件夹，或确认邮箱地址是否正确
                </Text>
              </div>
            </Space>
          )}
        </div>
      </Card>
    </div>
  )
}

export default ForgotPassword
