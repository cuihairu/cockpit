import { useState, useEffect } from 'react'
import { Button, Input, Card, message, Space, Typography } from 'antd'
import { SafetyOutlined, KeyOutlined } from '@ant-design/icons'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { api } from '@/services/api'
import type { TOTPVerifyResponse } from '@/types'
import './index.less'

const { Title, Text, Link } = Typography

const TOTPVerify = () => {
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const [code, setCode] = useState('')
  const [loading, setLoading] = useState(false)
  const [isBackupMode, setIsBackupMode] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const tmpToken = searchParams.get('tmp_token')
  const username = searchParams.get('username') || '用户'

  useEffect(() => {
    if (!tmpToken) {
      message.error('无效的验证链接')
      navigate('/login')
    }
  }, [tmpToken, navigate])

  const handleSubmit = async () => {
    if (!code.trim()) {
      message.warning(isBackupMode ? '请输入备份码' : '请输入验证码')
      return
    }

    setLoading(true)
    setError(null)

    try {
      const response: TOTPVerifyResponse = await api.verifyTOTP(code.trim(), tmpToken!)

      // 保存 token
      localStorage.setItem('token', response.token)
      localStorage.setItem('username', response.username)

      message.success('验证成功')

      // 跳转到首页
      navigate('/')
    } catch (err: any) {
      const errorMsg = err.response?.data?.error || '验证失败，请重试'
      setError(errorMsg)
      message.error(errorMsg)
    } finally {
      setLoading(false)
    }
  }

  const handleResendLogin = () => {
    navigate('/login')
  }

  const handleCodeChange = (value: string) => {
    // 只允许数字和字母（备份码可能包含字母）
    const cleanValue = isBackupMode ? value.toUpperCase().replace(/[^A-Z0-9]/g, '') : value.replace(/\D/g, '')
    setCode(cleanValue.slice(0, isBackupMode ? 12 : 6))
  }

  return (
    <div className="totp-verify-page">
      <Card className="totp-verify-card">
        <div className="totp-verify-header">
          <div className="totp-icon-wrapper">
            <SafetyOutlined />
          </div>
          <Title level={3} className="totp-title">
            二次验证
          </Title>
          <Text type="secondary">你好，{username}</Text>
        </div>

        <div className="totp-verify-body">
          <Space direction="vertical" size="large" style={{ width: '100%' }}>
            <div>
              <Text className="totp-description">
                {isBackupMode
                  ? '请输入一个恢复码以恢复访问权限'
                  : '请打开您的认证器应用，输入 6 位验证码'}
              </Text>
            </div>

            <div>
              <Input
                size="large"
                value={code}
                onChange={(e) => handleCodeChange(e.target.value)}
                placeholder={isBackupMode ? 'xxxx-xxxx-xxxx' : '123456'}
                maxLength={isBackupMode ? 12 : 6}
                prefix={isBackupMode ? <KeyOutlined /> : <SafetyOutlined />}
                autoFocus
                onPressEnter={handleSubmit}
                status={error ? 'error' : undefined}
              />
              {error && <Text type="danger">{error}</Text>}
            </div>

            <Button
              type="link"
              onClick={() => {
                setIsBackupMode(!isBackupMode)
                setCode('')
                setError(null)
              }}
            >
              {isBackupMode ? '使用验证码登录' : '使用恢复码登录'}
            </Button>

            <Button
              type="primary"
              size="large"
              block
              loading={loading}
              onClick={handleSubmit}
              disabled={code.length === 0}
            >
              验证
            </Button>
          </Space>
        </div>

        <div className="totp-verify-footer">
          <Text type="secondary">
            丢失了设备？{' '}
            <Link onClick={handleResendLogin}>返回登录</Link>
          </Text>
        </div>
      </Card>
    </div>
  )
}

export default TOTPVerify
