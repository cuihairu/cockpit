import { useState } from 'react'
import { Form, Input, Button, message, Checkbox } from 'antd'
import { UserOutlined, LockOutlined } from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { useUser } from '@/contexts/UserContext'
import logo from '@/assets/logo.svg'
import './index.less'

const Login = () => {
  const [loading, setLoading] = useState(false)
  const navigate = useNavigate()
  const { login } = useUser()
  const [form] = Form.useForm()

  const onFinish = async (values: { username: string; password: string; remember?: boolean }) => {
    setLoading(true)
    try {
      await login(values.username, values.password)
      message.success('登录成功')
      navigate('/')
    } catch (error) {
      message.error('用户名或密码错误')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="login-container">
      <div className="login-background">
        <div className="login-bg-circle circle-1"></div>
        <div className="login-bg-circle circle-2"></div>
        <div className="login-bg-circle circle-3"></div>
      </div>
      <div className="login-box">
        <div className="login-logo-section">
          <img src={logo} alt="Cockpit" className="login-logo" />
          <h1 className="login-title">Cockpit</h1>
          <p className="login-subtitle">个人混合基础设施控制台</p>
        </div>
        <Form
          form={form}
          name="login"
          onFinish={onFinish}
          autoComplete="off"
          size="large"
          initialValues={{
            username: 'admin',
            password: 'admin',
            remember: true
          }}
          className="login-form"
        >
          <Form.Item
            name="username"
            rules={[{ required: true, message: '请输入用户名' }]}
          >
            <Input
              prefix={<UserOutlined />}
              placeholder="用户名"
              variant="borderless"
            />
          </Form.Item>
          <Form.Item
            name="password"
            rules={[{ required: true, message: '请输入密码' }]}
          >
            <Input.Password
              prefix={<LockOutlined />}
              placeholder="密码"
              variant="borderless"
            />
          </Form.Item>
          <Form.Item>
            <Form.Item name="remember" valuePropName="checked" noStyle>
              <Checkbox>记住账号</Checkbox>
            </Form.Item>
          </Form.Item>
          <Form.Item>
            <Button type="primary" htmlType="submit" block loading={loading} className="login-button">
              登 录
            </Button>
          </Form.Item>
        </Form>
        <div className="login-footer">
          <p>默认账号: admin / admin</p>
        </div>
      </div>
    </div>
  )
}

export default Login
