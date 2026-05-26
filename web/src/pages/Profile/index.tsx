import { useState } from 'react'
import { Card, Form, Input, Button, message, Descriptions, Avatar, Space, Divider } from 'antd'
import { UserOutlined, MailOutlined, LockOutlined, SaveOutlined } from '@ant-design/icons'
import { useUser } from '@/contexts/UserContext'
import { api } from '@/services/api'

const Profile = () => {
  const { user } = useUser()
  const [loading, setLoading] = useState(false)
  const [passwordForm] = Form.useForm()

  const handleProfileSave = async (values: any) => {
    setLoading(true)
    try {
      // TODO: 调用 API 保存用户信息
      console.log('Saving profile:', values)
      message.success('个人信息已更新')
    } finally {
      setLoading(false)
    }
  }

  const handlePasswordChange = async (values: any) => {
    setLoading(true)
    try {
      await api.changePassword(values.currentPassword, values.newPassword)
      message.success('密码已修改')
      passwordForm.resetFields()
    } catch (err: any) {
      const errorMsg = err.response?.data?.error || '修改密码失败'
      message.error(errorMsg)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="page-container">
      <Space direction="vertical" style={{ width: '100%' }} size="large">
        {/* 个人信息卡片 */}
        <Card title="个人信息">
          <Space direction="vertical" style={{ width: '100%' }} size="large">
            <Space size="large">
              <Avatar size={80} icon={<UserOutlined />} />
              <Descriptions column={1}>
                <Descriptions.Item label="用户名">{user?.username || 'Admin'}</Descriptions.Item>
                <Descriptions.Item label="角色">管理员</Descriptions.Item>
                <Descriptions.Item label="状态">在线</Descriptions.Item>
              </Descriptions>
            </Space>

            <Divider />

            <Form
              layout="vertical"
              initialValues={{
                email: user?.email || '',
                phone: '',
                department: '',
              }}
              onFinish={handleProfileSave}
            >
              <Form.Item
                label="邮箱"
                name="email"
                rules={[{ type: 'email', message: '请输入有效的邮箱地址' }]}
              >
                <Input prefix={<MailOutlined />} placeholder="请输入邮箱" />
              </Form.Item>

              <Form.Item label="手机号" name="phone">
                <Input placeholder="请输入手机号" />
              </Form.Item>

              <Form.Item label="部门" name="department">
                <Input placeholder="请输入部门" />
              </Form.Item>

              <Form.Item>
                <Button type="primary" htmlType="submit" icon={<SaveOutlined />} loading={loading}>
                  保存信息
                </Button>
              </Form.Item>
            </Form>
          </Space>
        </Card>

        {/* 修改密码卡片 */}
        <Card title="修改密码">
          <Form
            form={passwordForm}
            layout="vertical"
            onFinish={handlePasswordChange}
          >
            <Form.Item
              label="当前密码"
              name="currentPassword"
              rules={[{ required: true, message: '请输入当前密码' }]}
            >
              <Input.Password prefix={<LockOutlined />} placeholder="请输入当前密码" />
            </Form.Item>

            <Form.Item
              label="新密码"
              name="newPassword"
              rules={[
                { required: true, message: '请输入新密码' },
                { min: 6, message: '密码长度至少 6 位' },
              ]}
            >
              <Input.Password prefix={<LockOutlined />} placeholder="请输入新密码（至少 6 位）" />
            </Form.Item>

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
              <Input.Password prefix={<LockOutlined />} placeholder="请再次输入新密码" />
            </Form.Item>

            <Form.Item>
              <Button type="primary" htmlType="submit" icon={<SaveOutlined />} loading={loading}>
                修改密码
              </Button>
            </Form.Item>
          </Form>
        </Card>
      </Space>
    </div>
  )
}

export default Profile
