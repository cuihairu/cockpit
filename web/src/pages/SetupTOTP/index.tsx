import { useState, useEffect } from 'react'
import { Button, Steps, Card, Input, message, Space, Alert, Typography } from 'antd'
import {
  SafetyOutlined,
  QrcodeOutlined,
  KeyOutlined,
  CheckOutlined,
  ArrowRightOutlined,
} from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { api } from '@/services/api'
import type { TOTPGenerateResponse } from '@/types'
import QRCodeDisplay from '@/components/QRCodeDisplay'
import BackupCodesDisplay from '@/components/BackupCodesDisplay'
import './index.less'

const { Title, Text, Paragraph } = Typography
const { Step } = Steps

type SetupStep = 0 | 1 | 2 | 3

const SetupTOTP = () => {
  const navigate = useNavigate()
  const [currentStep, setCurrentStep] = useState<SetupStep>(0)
  const [loading, setLoading] = useState(false)
  const [totpData, setTotpData] = useState<TOTPGenerateResponse | null>(null)
  const [verifyCode, setVerifyCode] = useState('')
  const [backupSaved, setBackupSaved] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    generateTOTP()
  }, [])

  const generateTOTP = async () => {
    setLoading(true)
    try {
      const data = await api.generateTOTP()
      setTotpData(data)
      setCurrentStep(1)
    } catch (err: any) {
      const errorMsg = err.response?.data?.error || '生成 TOTP 失败'
      message.error(errorMsg)
      navigate('/settings')
    } finally {
      setLoading(false)
    }
  }

  const handleVerify = async () => {
    if (!verifyCode.trim() || verifyCode.length !== 6) {
      message.warning('请输入 6 位验证码')
      return
    }

    setLoading(true)
    setError(null)

    try {
      await api.enableTOTP(verifyCode.trim())
      message.success('TOTP 已启用')
      setCurrentStep(3)
    } catch (err: any) {
      const errorMsg = err.response?.data?.error || '验证失败'
      setError(errorMsg)
      message.error(errorMsg)
    } finally {
      setLoading(false)
    }
  }

  const handleConfirmBackup = () => {
    if (!backupSaved) {
      message.warning('请确认已保存备份码')
      return
    }
    setCurrentStep(2)
  }

  const handleFinish = () => {
    navigate('/settings')
  }

  const handleRetry = () => {
    setVerifyCode('')
    setError(null)
  }

  const handleCodeChange = (value: string) => {
    setVerifyCode(value.replace(/\D/g, '').slice(0, 6))
  }

  const steps = [
    {
      title: '开始',
      description: '了解 TOTP',
      icon: <SafetyOutlined />,
    },
    {
      title: '扫码',
      description: '扫描 QR 码',
      icon: <QrcodeOutlined />,
    },
    {
      title: '验证',
      description: '输入验证码',
      icon: <KeyOutlined />,
    },
    {
      title: '完成',
      description: '保存备份码',
      icon: <CheckOutlined />,
    },
  ]

  return (
    <div className="setup-totp-page">
      <Card className="setup-totp-card">
        <div className="setup-totp-header">
          <Title level={3}>启用二次验证</Title>
          <Steps current={currentStep} className="setup-steps">
            {steps.map((step, index) => (
              <Step
                key={index}
                title={step.title}
                description={step.description}
                icon={step.icon}
              />
            ))}
          </Steps>
        </div>

        <div className="setup-totp-body">
          {/* Step 0: Introduction */}
          {currentStep === 0 && (
            <Space direction="vertical" size="large" className="setup-intro">
              <Alert
                message="什么是二次验证？"
                description="二次验证（2FA）为您的账户提供额外的安全保护。即使有人获取了您的密码，他们仍然无法访问您的账户，除非他们拥有您的验证设备。"
                type="info"
                showIcon
              />

              <div>
                <Title level={5}>使用步骤：</Title>
                <ol className="setup-instructions">
                  <li>在手机上安装 Google Authenticator 或 Microsoft Authenticator</li>
                  <li>扫描下方 QR 码添加账户</li>
                  <li>输入验证码确认设置成功</li>
                  <li>保存备份码以备不时之需</li>
                </ol>
              </div>

              <Button
                type="primary"
                size="large"
                icon={<ArrowRightOutlined />}
                onClick={generateTOTP}
                loading={loading}
              >
                开始设置
              </Button>
            </Space>
          )}

          {/* Step 1: Scan QR Code */}
          {currentStep === 1 && totpData && (
            <Space direction="vertical" size="large" className="setup-qrcode" align="center">
              <div>
                <Title level={5}>1. 安装认证器应用</Title>
                <Paragraph type="secondary">
                  在手机上安装{' '}
                  <a
                    href="https://apps.apple.com/app/google-authenticator/id388497605"
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    Google Authenticator
                  </a>{' '}
                  或{' '}
                  <a
                    href="https://apps.apple.com/app/microsoft-authenticator/id983156458"
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    Microsoft Authenticator
                  </a>
                </Paragraph>
              </div>

              <div>
                <Title level={5}>2. 扫描 QR 码</Title>
                <QRCodeDisplay value={totpData.qr_code} size={220} title="使用认证器扫描此码" />
              </div>

              <div>
                <Title level={5}>或手动输入密钥</Title>
                <Input.Password
                  value={totpData.secret}
                  readOnly
                  style={{ maxWidth: 300, textAlign: 'center', fontFamily: 'monospace' }}
                />
              </div>

              <Space>
                <Button onClick={() => navigate('/settings')}>取消</Button>
                <Button type="primary" onClick={handleConfirmBackup}>
                  下一步
                </Button>
              </Space>
            </Space>
          )}

          {/* Step 2: Verify Code */}
          {currentStep === 2 && (
            <Space direction="vertical" size="large" className="setup-verify" align="center">
              <div>
                <Title level={5}>输入验证码</Title>
                <Paragraph type="secondary">
                  打开认证器应用，输入显示的 6 位验证码
                </Paragraph>
              </div>

              <Input
                size="large"
                value={verifyCode}
                onChange={(e) => handleCodeChange(e.target.value)}
                placeholder="123456"
                maxLength={6}
                style={{
                  maxWidth: 200,
                  textAlign: 'center',
                  fontSize: 24,
                  letterSpacing: 8,
                  fontFamily: 'monospace',
                }}
                autoFocus
                onPressEnter={handleVerify}
                status={error ? 'error' : undefined}
              />

              {error && <Text type="danger">{error}</Text>}

              <Space>
                <Button onClick={() => setCurrentStep(1)}>上一步</Button>
                <Button onClick={handleRetry} disabled={!verifyCode}>
                  重新输入
                </Button>
                <Button
                  type="primary"
                  onClick={handleVerify}
                  loading={loading}
                  disabled={verifyCode.length !== 6}
                >
                  验证
                </Button>
              </Space>
            </Space>
          )}

          {/* Step 3: Complete - Show Backup Codes */}
          {currentStep === 3 && totpData && (
            <Space direction="vertical" size="large" className="setup-complete" align="center">
              <Alert
                message="TOTP 已启用"
                description="您的账户现在受到二次验证保护。请务必保存以下备份码。"
                type="success"
                showIcon
              />

              <BackupCodesDisplay codes={totpData.backup_codes} />

              <div className="backup-confirm">
                <label>
                  <input
                    type="checkbox"
                    checked={backupSaved}
                    onChange={(e) => setBackupSaved(e.target.checked)}
                  />
                  <Text>我已安全保存所有备份码</Text>
                </label>
              </div>

              <Space>
                <Button type="primary" onClick={handleFinish} disabled={!backupSaved}>
                  完成
                </Button>
              </Space>
            </Space>
          )}
        </div>
      </Card>
    </div>
  )
}

export default SetupTOTP
