import { Alert, Input, Modal, Space } from 'antd'

interface DisableTOTPModalProps {
  open: boolean
  verifyCode: string
  loading: boolean
  onCodeChange: (value: string) => void
  onCancel: () => void
  onConfirm: () => void
}

// 禁用 TOTP 二次验证模态框
export const DisableTOTPModal: React.FC<DisableTOTPModalProps> = ({
  open,
  verifyCode,
  loading,
  onCodeChange,
  onCancel,
  onConfirm,
}) => {
  return (
    <Modal
      title="禁用二次验证"
      open={open}
      onCancel={onCancel}
      onOk={onConfirm}
      confirmLoading={loading}
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
            value={verifyCode}
            onChange={(e) => onCodeChange(e.target.value.replace(/\D/g, '').slice(0, 6))}
            placeholder="123456"
            maxLength={6}
            style={{ textAlign: 'center', fontSize: 20, letterSpacing: 4 }}
            autoFocus
          />
        </div>
      </Space>
    </Modal>
  )
}
