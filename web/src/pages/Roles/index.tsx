import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Button,
  Card,
  Checkbox,
  Form,
  Input,
  Modal,
  Popconfirm,
  Popover,
  Space,
  Table,
  Tag,
  message,
} from 'antd'
import { EditOutlined, PlusOutlined, ReloadOutlined, SafetyOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { api } from '@/services/api'
import type { RoleInfo } from '@/types'
import { getApiErrorMessage } from '@/utils/apiError'

// 角色管理（rbac-design.md P1 笔 7，roles:admin）：列表 + 新建/编辑自定义
// 角色的权限矩阵 + 删除。内置角色只读展示（后端拒改删，D12）；合法权限点
// 全集从内置 admin 角色的 permissions 推导（admin 恒为全量，零后端改动）。

const RESOURCE_LABELS: Record<string, string> = {
  inventory: '资产清单',
  files: '文件管理',
  logs: '日志检索',
  terminal: '远程终端',
  docker: '容器管理',
  stack: '应用部署',
  cron: '定时任务',
  backup: '备份管理',
  acme: '证书签发',
  dns: 'DNS 管理',
  ddns: 'DDNS',
  proxy: '反向代理',
  overlay: '组网云',
  drift: '漂移检测',
  nas: '存储池',
  alerts: '告警',
  audit: '审计日志',
  users: '用户管理',
  roles: '角色管理',
  settings: '系统设置',
  services: '系统服务',
  smart: '磁盘健康',
  recordings: '会话录制',
}

const ACTIONS = ['read', 'write', 'admin'] as const

interface RoleFormValues {
  name: string
  permissions: string[]
}

const RolesPage = () => {
  const queryClient = useQueryClient()
  const [editing, setEditing] = useState<RoleInfo | null>(null)
  const [createOpen, setCreateOpen] = useState(false)
  const [form] = Form.useForm<RoleFormValues>()

  const { data: roles = [], isLoading } = useQuery({
    queryKey: ['roles'],
    queryFn: () => api.listRoles(),
  })

  // 合法权限点全集：admin 内置角色恒为全量（seed 以代码为准覆盖更新）
  const allResources = useMemo(() => {
    const adminRole = roles.find((r) => r.name === 'admin' && r.builtin)
    const set = new Set<string>()
    ;(adminRole?.permissions ?? []).forEach((p) => {
      const res = p.split(':')[0]
      if (res) set.add(res)
    })
    return [...set].sort()
  }, [roles])

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['roles'] })

  const saveRole = useMutation({
    mutationFn: (v: RoleFormValues) =>
      editing ? api.updateRole(editing.name, v.permissions) : api.createRole(v),
    onSuccess: (_d, v) => {
      message.success(editing ? `角色 ${v.name} 已更新` : `角色 ${v.name} 已创建`)
      setEditing(null)
      setCreateOpen(false)
      form.resetFields()
      invalidate()
    },
    onError: (e) => message.error(getApiErrorMessage(e, '操作失败')),
  })

  const deleteRole = useMutation({
    mutationFn: (r: RoleInfo) => api.deleteRole(r.name),
    onSuccess: (_d, r) => {
      message.success(`角色 ${r.name} 已删除`)
      invalidate()
    },
    onError: (e) => message.error(getApiErrorMessage(e, '操作失败')),
  })

  const openEditor = (role: RoleInfo | null) => {
    form.resetFields()
    if (role) {
      setEditing(role)
      form.setFieldsValue({ name: role.name, permissions: role.permissions })
    } else {
      setEditing(null)
      setCreateOpen(true)
    }
  }

  const columns: ColumnsType<RoleInfo> = [
    {
      title: '角色名',
      dataIndex: 'name',
      render: (name: string, r) => (
        <Space>
          <SafetyOutlined />
          {name}
          {r.builtin && <Tag color="gold">内置</Tag>}
        </Space>
      ),
    },
    {
      title: '权限点',
      dataIndex: 'permissions',
      render: (perms: string[]) => (
        <Popover
          content={
            <div style={{ maxWidth: 360, maxHeight: 300, overflowY: 'auto' }}>
              {perms.length === 0 ? (
                <span>无权限（只读库 fail-closed 语义下将无法访问任何模块）</span>
              ) : (
                perms.map((p) => (
                  <Tag key={p} style={{ margin: 2 }}>
                    {p}
                  </Tag>
                ))
              )}
            </div>
          }
        >
          <Button type="link" style={{ padding: 0 }}>
            {perms.length} 项
          </Button>
        </Popover>
      ),
    },
    {
      title: '操作',
      width: 160,
      render: (_v, r) =>
        r.builtin ? (
          <span style={{ color: '#999' }}>内置角色不可修改</span>
        ) : (
          <Space>
            <Button size="small" icon={<EditOutlined />} onClick={() => openEditor(r)}>
              编辑
            </Button>
            <Popconfirm
              title={`删除角色 ${r.name}?`}
              description="仍被用户引用时后端会拒绝"
              okButtonProps={{ danger: true }}
              onConfirm={() => deleteRole.mutate(r)}
            >
              <Button size="small" danger>
                删除
              </Button>
            </Popconfirm>
          </Space>
        ),
    },
  ]

  // 权限矩阵：每 resource 一行 read/write/admin 复选；高权限隐含低权限
  // （后端 roleCovers：write ⊇ read、admin ⊇ write），提交按勾选原样存储
  const permOptions = (res: string) =>
    ACTIONS.map((a) => ({ label: a, value: `${res}:${a}` }))

  return (
    <div className="page-container">
      <Card
        title="角色管理"
        extra={
          <Space>
            <Button icon={<ReloadOutlined />} onClick={() => invalidate()} />
            <Button type="primary" icon={<PlusOutlined />} onClick={() => openEditor(null)}>
              新建角色
            </Button>
          </Space>
        }
      >
        <Table<RoleInfo>
          rowKey="name"
          size="middle"
          loading={isLoading}
          columns={columns}
          dataSource={roles}
          pagination={false}
        />
      </Card>

      <Modal
        title={editing ? `编辑角色 ${editing.name}` : '新建角色'}
        open={createOpen || !!editing}
        onCancel={() => {
          setEditing(null)
          setCreateOpen(false)
        }}
        onOk={() => form.submit()}
        confirmLoading={saveRole.isPending}
        width={640}
        destroyOnHidden
      >
        <Form form={form} layout="vertical" onFinish={(v) => saveRole.mutate(v)}>
          <Form.Item
            name="name"
            label="角色名"
            rules={[{ required: true, message: '请输入角色名' }]}
            extra={editing ? '角色名不可修改' : undefined}
          >
            <Input disabled={!!editing} />
          </Form.Item>
          <Form.Item
            name="permissions"
            label="权限矩阵（勾选高权限即隐含低权限，如 admin ⊇ write ⊇ read）"
          >
            <Checkbox.Group style={{ width: '100%' }}>
              {allResources.map((res) => (
                <div
                  key={res}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 12,
                    padding: '4px 0',
                    borderBottom: '1px solid #f0f0f0',
                  }}
                >
                  <span style={{ width: 110, flexShrink: 0 }}>
                    {RESOURCE_LABELS[res] ?? res}
                  </span>
                  {permOptions(res).map((opt) => (
                    <Checkbox key={opt.value} value={opt.value}>
                      {opt.label}
                    </Checkbox>
                  ))}
                </div>
              ))}
            </Checkbox.Group>
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

export default RolesPage
