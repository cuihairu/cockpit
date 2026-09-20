import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Button,
  Card,
  Form,
  Input,
  Modal,
  Popconfirm,
  Select,
  Space,
  Table,
  Tag,
  Tooltip,
  message,
} from 'antd'
import { KeyOutlined, PlusOutlined, ReloadOutlined, UserOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import dayjs from 'dayjs'
import { api } from '@/services/api'
import type { ManagedUser, RoleInfo } from '@/types'
import { useUser } from '@/contexts/useUser'
import { getApiErrorMessage } from '@/utils/apiError'

// 用户管理（rbac-design.md P1 笔 7，users:admin）：列表 + 新建 + 改角色
// + 改密 + 删除。D13（不可改自己角色、不可删最后 admin）在后端拦截，
// 这里把错误信息透出；「改自己角色」直接禁用入口。

interface CreateUserValues {
  username: string
  password: string
  role: string
  email?: string
}

const UsersPage = () => {
  const queryClient = useQueryClient()
  const { user: currentUser } = useUser()
  const [createOpen, setCreateOpen] = useState(false)
  const [pwdFor, setPwdFor] = useState<ManagedUser | null>(null)
  const [createForm] = Form.useForm<CreateUserValues>()
  const [pwdForm] = Form.useForm<{ newPassword: string }>()

  const { data: users = [], isLoading } = useQuery({
    queryKey: ['users'],
    queryFn: () => api.listUsers(),
  })
  const { data: roles = [] } = useQuery({
    queryKey: ['roles'],
    queryFn: () => api.listRoles(),
  })

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['users'] })

  const changeRole = useMutation({
    mutationFn: (u: ManagedUser & { nextRole: string }) =>
      api.updateUser(u.id, { role: u.nextRole }),
    onSuccess: (_d, v) => {
      message.success(`已将 ${v.username} 的角色改为 ${v.nextRole}`)
      invalidate()
    },
    onError: (e) => message.error(getApiErrorMessage(e, '操作失败')),
  })

  const createUser = useMutation({
    mutationFn: (v: CreateUserValues) => api.createUser(v),
    onSuccess: (_d, v) => {
      message.success(`用户 ${v.username} 已创建`)
      setCreateOpen(false)
      createForm.resetFields()
      invalidate()
    },
    onError: (e) => message.error(getApiErrorMessage(e, '操作失败')),
  })

  const changePassword = useMutation({
    mutationFn: (v: { user: ManagedUser; newPassword: string }) =>
      api.changeUserPassword(v.user.id, v.newPassword),
    onSuccess: (_d, v) => {
      message.success(`已重置 ${v.user.username} 的密码`)
      setPwdFor(null)
      pwdForm.resetFields()
    },
    onError: (e) => message.error(getApiErrorMessage(e, '操作失败')),
  })

  const deleteUser = useMutation({
    mutationFn: (u: ManagedUser) => api.deleteUser(u.id),
    onSuccess: (_d, u) => {
      message.success(`用户 ${u.username} 已删除`)
      invalidate()
    },
    onError: (e) => message.error(getApiErrorMessage(e, '操作失败')),
  })

  const columns: ColumnsType<ManagedUser> = [
    {
      title: '用户名',
      dataIndex: 'username',
      render: (name: string, u) => (
        <Space>
          <UserOutlined />
          {name}
          {currentUser?.id === u.id && <Tag color="blue">自己</Tag>}
        </Space>
      ),
    },
    { title: '邮箱', dataIndex: 'email', render: (v?: string) => v || '-' },
    {
      title: '角色',
      dataIndex: 'role',
      width: 200,
      render: (role: string, u) => (
        <Select
          value={role}
          style={{ width: 160 }}
          disabled={currentUser?.id === u.id}
          onChange={(nextRole) => changeRole.mutate({ ...u, nextRole })}
          options={roles.map((r: RoleInfo) => ({ value: r.name, label: r.name }))}
        />
      ),
    },
    {
      title: 'TOTP',
      dataIndex: 'totp_enabled',
      width: 90,
      render: (v: boolean) =>
        v ? <Tag color="success">已启用</Tag> : <Tag>未启用</Tag>,
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      width: 170,
      render: (v?: string) => (v ? dayjs(v).format('YYYY-MM-DD HH:mm') : '-'),
    },
    {
      title: '操作',
      width: 200,
      render: (_v, u) => (
        <Space>
          <Tooltip title="重置密码">
            <Button size="small" icon={<KeyOutlined />} onClick={() => setPwdFor(u)} />
          </Tooltip>
          <Popconfirm
            title={`删除用户 ${u.username}?`}
            description="该操作不可恢复"
            okButtonProps={{ danger: true }}
            onConfirm={() => deleteUser.mutate(u)}
          >
            <Button size="small" danger disabled={currentUser?.id === u.id}>
              删除
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <div className="page-container">
      <Card
        title="用户管理"
        extra={
          <Space>
            <Button icon={<ReloadOutlined />} onClick={() => invalidate()} />
            <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>
              新建用户
            </Button>
          </Space>
        }
      >
        <Table<ManagedUser>
          rowKey="id"
          size="middle"
          loading={isLoading}
          columns={columns}
          dataSource={users}
          pagination={false}
        />
      </Card>

      <Modal
        title="新建用户"
        open={createOpen}
        onCancel={() => setCreateOpen(false)}
        onOk={() => createForm.submit()}
        confirmLoading={createUser.isPending}
        destroyOnHidden
      >
        <Form
          form={createForm}
          layout="vertical"
          initialValues={{ role: 'viewer' }}
          onFinish={(v) => createUser.mutate(v)}
        >
          <Form.Item name="username" label="用户名" rules={[{ required: true, message: '请输入用户名' }]}>
            <Input autoComplete="off" />
          </Form.Item>
          <Form.Item
            name="password"
            label="初始密码"
            rules={[
              { required: true, message: '请输入密码' },
              { min: 8, message: '至少 8 位' },
            ]}
          >
            <Input.Password autoComplete="new-password" />
          </Form.Item>
          <Form.Item name="role" label="角色">
            <Select options={roles.map((r: RoleInfo) => ({ value: r.name, label: r.name }))} />
          </Form.Item>
          <Form.Item name="email" label="邮箱（可选）">
            <Input type="email" />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={`重置 ${pwdFor?.username ?? ''} 的密码`}
        open={!!pwdFor}
        onCancel={() => setPwdFor(null)}
        onOk={() => pwdForm.submit()}
        confirmLoading={changePassword.isPending}
        destroyOnHidden
      >
        <Form
          form={pwdForm}
          layout="vertical"
          onFinish={(v) => pwdFor && changePassword.mutate({ user: pwdFor, newPassword: v.newPassword })}
        >
          <Form.Item
            name="newPassword"
            label="新密码"
            rules={[
              { required: true, message: '请输入新密码' },
              { min: 8, message: '至少 8 位' },
            ]}
          >
            <Input.Password autoComplete="new-password" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

export default UsersPage
