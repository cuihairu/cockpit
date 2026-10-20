import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Button, Card, Form, Input, Modal, Popconfirm, Select, Space, Switch, Table, Tag, message } from 'antd'
import { DeleteOutlined, EditOutlined, PlusOutlined, ReloadOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { api } from '@/services/api'
import type { CloudAccount, CloudAccountInput } from '@/types'
import { getApiErrorMessage } from '@/utils/apiError'
import { PermGuard } from '@/components/PermGuard'

interface CloudAccountFormValues extends CloudAccountInput {
  confirmKey?: string
}

const CloudAccountsPage = () => {
  const queryClient = useQueryClient()
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<CloudAccount | null>(null)
  const [form] = Form.useForm<CloudAccountFormValues>()
  const query = useQuery({ queryKey: ['cloud-accounts'], queryFn: api.getCloudAccounts })

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['cloud-accounts'] })

  const saveMutation = useMutation({
    mutationFn: api.saveCloudAccount,
    onSuccess: () => {
      message.success(editing ? '云账号已更新' : '云账号已创建')
      setModalOpen(false)
      void invalidate()
    },
    onError: (error) => message.error(getApiErrorMessage(error, '保存失败')),
  })

  const deleteMutation = useMutation({
    mutationFn: api.deleteCloudAccount,
    onSuccess: () => {
      message.success('云账号已删除')
      void invalidate()
    },
    onError: (error) => message.error(getApiErrorMessage(error, '删除失败')),
  })

  const openCreate = () => {
    setEditing(null)
    form.setFieldsValue({ vendor: 'aliyun', alias: '', secret: '', username: '', scopes: [], enabled: true })
    setModalOpen(true)
  }

  const openEdit = (account: CloudAccount) => {
    setEditing(account)
    form.setFieldsValue({
      vendor: account.vendor,
      alias: account.alias,
      secret: '',
      username: '',
      scopes: account.scopes,
      enabled: account.enabled,
      confirmKey: '',
    })
    setModalOpen(true)
  }

  const submit = async () => {
    const values = await form.validateFields().catch(() => undefined)
    if (!values) return
    await saveMutation.mutateAsync({
      ...values,
      enabled: values.enabled ?? true,
      confirmKey: values.confirmKey || '',
    })
    form.resetFields()
  }

  const confirmDelete = (account: CloudAccount) => {
    const value = window.prompt(`输入 ${account.id} 确认删除`)
    if (value !== account.id) {
      if (value !== null) message.error('确认文本不一致')
      return
    }
    deleteMutation.mutate(account.id)
  }

  const columns: ColumnsType<CloudAccount> = [
    { title: '厂商', dataIndex: 'vendor', width: 100 },
    { title: '别名', dataIndex: 'alias', width: 140 },
    {
      title: '凭据引用',
      dataIndex: 'credentialRef',
      render: (value: string) => <Tag color="blue">{value}</Tag>,
    },
    {
      title: '权限范围',
      dataIndex: 'scopes',
      render: (scopes: string[]) => (scopes.length ? scopes.join(', ') : '—'),
    },
    {
      title: '状态',
      dataIndex: 'enabled',
      width: 90,
      render: (enabled: boolean) => (enabled ? <Tag color="success">启用</Tag> : <Tag>停用</Tag>),
    },
    {
      title: '操作',
      key: 'actions',
      width: 130,
      render: (_, record) => (
        <Space>
          <Button size="small" icon={<EditOutlined />} onClick={() => openEdit(record)}>
            编辑
          </Button>
          <Popconfirm title="输入账号 ID 二次确认" okText="确认" cancelText="取消" onConfirm={() => confirmDelete(record)}>
            <Button danger size="small" icon={<DeleteOutlined />} loading={deleteMutation.isPending && deleteMutation.variables === record.id}>
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
        title="云账号"
        extra={
          <Space>
            <Button size="small" icon={<ReloadOutlined />} onClick={() => query.refetch()} loading={query.isFetching}>
              刷新
            </Button>
            <PermGuard perm="cloud:write">
              <Button type="primary" size="small" icon={<PlusOutlined />} onClick={openCreate}>
                添加
              </Button>
            </PermGuard>
          </Space>
        }
      >
        <Table
          columns={columns}
          dataSource={query.data ?? []}
          rowKey="id"
          loading={query.isLoading}
          pagination={{ pageSize: 20 }}
          size="small"
        />
      </Card>

      <Modal
        title={editing ? '编辑云账号' : '添加云账号'}
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={() => void submit()}
        confirmLoading={saveMutation.isPending}
        destroyOnClose
      >
        <Form form={form} layout="vertical">
          <Form.Item label="厂商" name="vendor" rules={[{ required: true }]}>
            <Select
              options={[
                { value: 'aliyun', label: '阿里云' },
                { value: 'tencent', label: '腾讯云' },
              ]}
              disabled={!!editing}
            />
          </Form.Item>
          <Form.Item label="别名" name="alias" rules={[{ required: true, pattern: /^[a-z0-9](?:[a-z0-9-]{0,30}[a-z0-9])?$/ }]}>
            <Input disabled={!!editing} placeholder="prod" />
          </Form.Item>
          <Form.Item
            label={editing ? '替换 Secret' : 'Secret'}
            name="secret"
            rules={[{ required: true }]}
            extra="仅上行一次，服务端存入 vault；台账只保留 ${VAR} 引用"
          >
            <Input.Password autoComplete="new-password" />
          </Form.Item>
          <Form.Item label="配套用户名（可选）" name="username">
            <Input autoComplete="off" />
          </Form.Item>
          <Form.Item label="权限范围" name="scopes">
            <Select mode="tags" open={false} placeholder="例如 ecs:read" />
          </Form.Item>
          <Form.Item label="启用" name="enabled" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item
            label={editing ? '输入 别名 确认' : '输入 vendor/alias 确认'}
            name="confirmKey"
            rules={[
              {
                validator: (_, value: string) =>
                  value === (editing ? editing.alias : `${form.getFieldValue('vendor')}/${form.getFieldValue('alias')}`)
                    ? Promise.resolve()
                    : Promise.reject(new Error('确认文本不一致')),
              },
            ]}
          >
            <Input placeholder={editing ? editing.alias : 'aliyun/prod'} autoComplete="off" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

export default CloudAccountsPage
