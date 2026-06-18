import { Button, Card, Popconfirm, Space, Tabs, Alert } from 'antd'
import { CheckCircleOutlined, SafetyOutlined, WarningOutlined } from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import type { UserInfo } from '@/types'
import { authenticatorLinks } from './authenticatorLinks'

interface SecuritySettingsProps {
  userInfo: UserInfo | null
  onRequestDisable: () => void
}

// 安全设置：TOTP 状态展示与启用/禁用入口
export const SecuritySettings: React.FC<SecuritySettingsProps> = ({
  userInfo,
  onRequestDisable,
}) => {
  const navigate = useNavigate()

  const totpPanel = (
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
              onConfirm={onRequestDisable}
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
          {authenticatorLinks.map((item) => (
            <a key={item.name} href={item.href} target="_blank" rel="noopener noreferrer">
              {item.name}
            </a>
          ))}
        </Space>
      </Card>
    </Space>
  )

  return (
    <Tabs items={[{ key: 'totp', label: '二次验证 (TOTP)', children: totpPanel }]} />
  )
}
