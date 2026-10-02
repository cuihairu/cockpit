import React, { useState } from 'react'
import { Modal, Form, Input, Button, Space, message } from 'antd'
import { verifyVault } from '@/services/remote'

// 凭据保险箱二次验证弹窗（阿里云密码箱模式）：保存/删除已存凭据前重验
// 身份——登录密码或 TOTP 动态码二选一。验证通过由 verifyVault 内部记录
// vault token（模块内存，10 分钟内连续管理不用反复输），本组件只报事件。

interface VaultVerifyModalProps {
  visible: boolean
  title?: string
  onVerified: () => void
  onCancel: () => void
}

const VaultVerifyModal: React.FC<VaultVerifyModalProps> = ({
  visible,
  title = '身份验证',
  onVerified,
  onCancel,
}) => {
  const [form] = Form.useForm()
  const [confirming, setConfirming] = useState(false)

  const handleVerify = async (values: { password?: string; totpCode?: string }) => {
    setConfirming(true)
    try {
      await verifyVault({ password: values.password, totpCode: values.totpCode })
      message.success('验证通过')
      form.resetFields()
      onVerified()
    } catch (err) {
      const msg =
        (err as { response?: { data?: string } })?.response?.data || '验证失败，请重试'
      message.error(typeof msg === 'string' ? msg : '验证失败，请重试')
    } finally {
      setConfirming(false)
    }
  }

  return (
    <Modal
      title={title}
      open={visible}
      onCancel={onCancel}
      footer={null}
      width={380}
      destroyOnClose
    >
      <Form form={form} layout="vertical" onFinish={(v) => void handleVerify(v)}>
        <Form.Item
          label="登录密码"
          name="password"
          extra="输入登录密码或下方 TOTP 动态码（二选一）"
        >
          <Input.Password placeholder="登录密码" autoFocus />
        </Form.Item>
        <Form.Item label="TOTP 动态码" name="totpCode">
          <Input placeholder="6 位动态码（未开启 TOTP 可留空）" />
        </Form.Item>
        <Space style={{ display: 'flex', justifyContent: 'flex-end' }}>
          <Button onClick={onCancel}>取消</Button>
          <Button type="primary" htmlType="submit" loading={confirming}>
            验证
          </Button>
        </Space>
      </Form>
    </Modal>
  )
}

export default VaultVerifyModal
