import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Button, Card, Form, Input, Modal, Select, Space, Table, Tag, message } from 'antd'
import { PlusOutlined, ReloadOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { api } from '@/services/api'
import type { CloudAccount, CloudInstance, CloudInstanceSpecInput } from '@/types'
import { getApiErrorMessage } from '@/utils/apiError'
import { PermGuard } from '@/components/PermGuard'

const statusTag = (status: string) => {
  switch (status) {
    case 'running':
      return <Tag color="success">运行中</Tag>
    case 'stopped':
      return <Tag color="warning">已停止</Tag>
    case 'creating':
      return <Tag color="processing">创建中</Tag>
    case 'deleting':
      return <Tag color="warning">释放中</Tag>
    default:
      return <Tag>未知</Tag>
  }
}

const CloudInstancesPage = () => {
  const queryClient = useQueryClient()
  const [accountId, setAccountId] = useState<string | undefined>()
  const [createOpen, setCreateOpen] = useState(false)
  const [form] = Form.useForm<CloudInstanceSpecInput>()

  const accountsQuery = useQuery({ queryKey: ['cloud-accounts'], queryFn: api.getCloudAccounts })
  const instancesQuery = useQuery({
    queryKey: ['cloud-instances', accountId],
    queryFn: () => api.getCloudInstances(accountId!),
    enabled: !!accountId,
  })

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ['cloud-instances'] })
  }

  const actionMutation = useMutation({
    mutationFn: (v: { action: 'start' | 'stop' | 'delete'; instanceId: string }) =>
      api.cloudInstanceAction(accountId!, v.action, v.instanceId),
    onSuccess: () => {
      message.success('操作已受理')
      invalidate()
    },
    onError: (error) => message.error(getApiErrorMessage(error, '操作失败')),
  })

  const createMutation = useMutation({
    mutationFn: (spec: CloudInstanceSpecInput) => api.cloudInstanceCreate(accountId!, spec),
    onSuccess: () => {
      message.success('创建已受理')
      setCreateOpen(false)
      invalidate()
    },
    onError: (error) => message.error(getApiErrorMessage(error, '创建失败')),
  })

  const doAction = (action: 'start' | 'stop' | 'delete', record: CloudInstance) => {
    const label = action === 'start' ? '启动' : action === 'stop' ? '停止' : '释放'
    const value = window.prompt(`输入 ${record.id} 确认${label}`)
    if (value !== record.id) {
      if (value !== null) message.error('确认文本不一致')
      return
    }
    actionMutation.mutate({ action, instanceId: record.id })
  }

  const columns: ColumnsType<CloudInstance> = [
    { title: '名称', dataIndex: 'name', width: 140 },
    { title: '原生 ID', dataIndex: 'nativeId', width: 160 },
    { title: '地域', dataIndex: 'region', width: 120 },
    { title: '规格', dataIndex: 'spec', width: 120 },
    { title: '状态', dataIndex: 'status', width: 90, render: statusTag },
    { title: '计费', dataIndex: 'billingMode', width: 100 },
    {
      title: '计费延续',
      dataIndex: 'billingContinues',
      width: 90,
      render: (v: boolean) => (v ? <Tag color="orange">是</Tag> : <Tag>否</Tag>),
    },
    {
      title: '操作',
      key: 'actions',
      width: 190,
      render: (_, record) => (
        <Space>
          <PermGuard perm="cloud:write">
            <Button size="small" onClick={() => doAction('start', record)} disabled={record.status === 'running'}>
              启动
            </Button>
            <Button size="small" onClick={() => doAction('stop', record)} disabled={record.status === 'stopped'}>
              停止
            </Button>
            <Button danger size="small" onClick={() => doAction('delete', record)}>
              释放
            </Button>
          </PermGuard>
        </Space>
      ),
    },
  ]

  return (
    <div className="page-container">
      <Card
        title="云实例"
        extra={
          <Space>
            <Select
              placeholder="选择云账号"
              style={{ width: 200 }}
              value={accountId}
              onChange={setAccountId}
              options={(accountsQuery.data ?? []).map((a: CloudAccount) => ({ value: a.id, label: `${a.vendor}/${a.alias}` }))}
            />
            <Button size="small" icon={<ReloadOutlined />} onClick={() => void instancesQuery.refetch()} loading={instancesQuery.isFetching}>
              刷新
            </Button>
            <PermGuard perm="cloud:write">
              <Button type="primary" size="small" icon={<PlusOutlined />} disabled={!accountId} onClick={() => setCreateOpen(true)}>
                创建
              </Button>
            </PermGuard>
          </Space>
        }
      >
        <Table
          columns={columns}
          dataSource={instancesQuery.data ?? []}
          rowKey="id"
          loading={instancesQuery.isLoading}
          pagination={{ pageSize: 20 }}
          size="small"
        />
      </Card>

      <Modal
        title="创建云实例"
        open={createOpen}
        onCancel={() => setCreateOpen(false)}
        onOk={() => void form.submit()}
        confirmLoading={createMutation.isPending}
        destroyOnClose
      >
        <Form form={form} layout="vertical" onFinish={(v) => createMutation.mutate(v)}>
          <Form.Item label="名称" name="name" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item label="地域" name="region">
            <Input placeholder="留空用账号默认" />
          </Form.Item>
          <Form.Item label="可用区" name="zone">
            <Input />
          </Form.Item>
          <Form.Item label="规格" name="spec" rules={[{ required: true }]}>
            <Input placeholder="ecs.g6.large / S5.MEDIUM4" />
          </Form.Item>
          <Form.Item label="镜像 ID" name="image_id" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item label="安全组 ID" name="security_group_id">
            <Input />
          </Form.Item>
          <Form.Item label="计费模式" name="billing_mode">
            <Select
              options={[
                { value: 'on-demand', label: '按量' },
                { value: 'prepaid', label: '包年包月' },
                { value: 'spot', label: '竞价' },
              ]}
            />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

export default CloudInstancesPage
