import { useState, useEffect } from 'react'
import { Form, Input, Button, message, Card, Typography, Space, Alert, Progress } from 'antd'
import { LockOutlined, KeyOutlined, CheckCircleOutlined, ArrowLeftOutlined } from '@ant-design/icons'
import { useNavigate, useSearchParams, Link } from 'react-router-dom'
import { api } from '@/services/api'
import './index.less'

const { Title, Text } = Typography

const ResetPassword = () => {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const [step, setStep] = useState<1 | 2 | 3>(1)
  const [loading, setLoading] = useState(false)
  const [token, setToken] = useState('')
  const [code, setCode] = useState('')
  const [passwordStrength, setPasswordStrength] = useState(0)

  // 从 URL 获取 token
  useEffect(() => {
    const urlToken = searchParams.get('token')
    if (!urlToken) {
      message.error('无效的重置链接')
      navigate('/login')
      return
    }
    setToken(urlToken)
  }, [searchParams, navigate])

  // 验证码验证
  const handleVerifyCode = async (values: { code: string }) => {
    setLoading(true)
    try {
      const response = await api.verifyResetCode(token, values.code)
      if (!response.valid) {
        message.error('验证码无效或已过期')
        return
      }
      setCode(values.code)
      setStep(2)
      message.success('验证码验证成功')
    } catch (err: any) {
      message.error(err.response?.data?.error || '验证失败')
    } finally {
      setLoading(false)
    }
  }

  // 密码强度计算
  const calculatePasswordStrength = (password: string) => {
    let strength = 0
    if (password.length >= 8) strength += 25
    if (password.length >= 12) strength += 15
    if (/[a-z]/.test(password)) strength += 20
    if (/[A-Z]/.test(password)) strength += 20
    if (/[0-9]/.test(password)) strength += 10
    if (/[^a-zA-Z0-9]/.test(password)) strength += 10
    return Math.min(strength, 100)
  }

  // 密码重置
  const handleResetPassword = async (values: { newPassword: string; confirmPassword: string }) => {
    setLoading(true)
    try {
      await api.resetPassword(token, code, values.newPassword)
      setStep(3)
      message.success('密码已成功重置')
    } catch (err: any) {
      message.error(err.response?.data?.error || '重置密码失败')
    } finally {
      setLoading(false)
    }
  }

  const getPasswordStrengthColor = () => {
    if (passwordStrength < 40) return '#ff4d4f'
    if (passwordStrength < 70) return '#faad14'
    return '#52c41a'
  }

  const getPasswordStrengthText = () => {
    if (passwordStrength < 40) return '弱'
    if (passwordStrength < 70) return '中'
    return '强'
  }

  return (
    <div className="reset-password-page">
      <Card className="reset-password-card">
        <div className="reset-password-header">
          <Link to="/login" className="back-link">
            <ArrowLeftOutlined /> 返回登录
          </Link>
          <Title level={3}>重置密码</Title>
        </div>

        <div className="reset-password-body">
          {/* 步骤1: 输入验证码 */}
          {step === 1 && (
            <Space direction="vertical" size="large" className="step-content">
              <Alert
                message="输入验证码"
                description="请输入邮件中发送的6位验证码以验证您的身份。"
                type="info"
                showIcon
              />

              <Form
                layout="vertical"
                onFinish={handleVerifyCode}
                autoComplete="off"
              >
                <Form.Item
                  label="验证码"
                  name="code"
                  rules={[
                    { required: true, message: '请输入验证码' },
                    { len: 6, message: '验证码为6位数字' },
                    { pattern: /^\d+$/, message: '验证码只能包含数字' }
                  ]}
                >
                  <Input
                    size="large"
                    placeholder="请输入6位验证码"
                    prefix={<KeyOutlined />}
                    maxLength={6}
                    style={{ letterSpacing: 4, fontSize: 18, textAlign: 'center' }}
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
                    验证
                  </Button>
                </Form.Item>

                <div className="reset-password-tips">
                  <Text type="secondary">
                    💡 提示：验证码有效期为30分钟，如未收到邮件请检查垃圾邮件文件夹。
                  </Text>
                </div>
              </Form>
            </Space>
          )}

          {/* 步骤2: 输入新密码 */}
          {step === 2 && (
            <Space direction="vertical" size="large" className="step-content">
              <Alert
                message="设置新密码"
                description="请输入您的新密码，建议使用包含大小写字母、数字和特殊字符的强密码。"
                type="info"
                showIcon
              />

              <Form
                layout="vertical"
                onFinish={handleResetPassword}
                autoComplete="off"
              >
                <Form.Item
                  label="新密码"
                  name="newPassword"
                  rules={[
                    { required: true, message: '请输入新密码' },
                    { min: 6, message: '密码长度至少6位' },
                  ]}
                >
                  <Input.Password
                    size="large"
                    placeholder="请输入新密码（至少6位）"
                    prefix={<LockOutlined />}
                    onChange={(e) => setPasswordStrength(calculatePasswordStrength(e.target.value))}
                  />
                </Form.Item>

                {passwordStrength > 0 && (
                  <div className="password-strength">
                    <Text type="secondary" style={{ fontSize: 12 }}>
                      密码强度：
                      <span style={{ color: getPasswordStrengthColor(), marginLeft: 4 }}>
                        {getPasswordStrengthText()}
                      </span>
                    </Text>
                    <Progress
                      percent={passwordStrength}
                      strokeColor={getPasswordStrengthColor()}
                      showInfo={false}
                      size="small"
                    />
                  </div>
                )}

                <Form.Item
                  label="确认密码"
                  name="confirmPassword"
                  dependencies={['newPassword']}
                  rules={[
                    { required: true, message: '请确认新密码' },
                    ({ getFieldValue }) => ({
                      validator(_, value) {
                        if (!value || getFieldValue('newPassword') === value) {
                          return Promise.resolve()
                        }
                        return Promise.reject(new Error('两次输入的密码不一致'))
                      },
                    }),
                  ]}
                >
                  <Input.Password
                    size="large"
                    placeholder="请再次输入新密码"
                    prefix={<LockOutlined />}
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
                    重置密码
                  </Button>
                </Form.Item>
              </Form>
            </Space>
          )}

          {/* 步骤3: 完成 */}
          {step === 3 && (
            <Space direction="vertical" size="large" className="step-content" align="center">
              <div className="success-icon">
                <CheckCircleOutlined />
              </div>

              <Title level={4}>密码重置成功</Title>

              <Text type="secondary">
                您的密码已成功重置，现在可以使用新密码登录。
              </Text>

              <Button type="primary" size="large" onClick={() => navigate('/login')}>
                前往登录
              </Button>
            </Space>
          )}
        </div>
      </Card>
    </div>
  )
}

export default ResetPassword
