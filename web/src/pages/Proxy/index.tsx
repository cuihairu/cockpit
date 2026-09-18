import { useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Alert,
  Button,
  Card,
  Descriptions,
  Empty,
  Form,
  Input,
  Modal,
  Popconfirm,
  Radio,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  Typography,
  message,
} from 'antd'
import {
  DeleteOutlined,
  EditOutlined,
  FileTextOutlined,
  PlusOutlined,
  ReloadOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { api } from '@/services/api'
import type { ProxySite } from '@/types'
import { getApiErrorMessage } from '@/utils/apiError'

const NAME_PATTERN = /^[a-z0-9][a-z0-9_-]{0,63}$/
const DOMAIN_PATTERN = /^[A-Za-z0-9*.\-]{1,253}$/
const UPSTREAM_PATTERN = /^[A-Za-z0-9.:\-]{1,253}$/

const schemeTag = (scheme: string) =>
  scheme === 'https' ? <Tag color="green">HTTPS</Tag> : <Tag color="blue">HTTP</Tag>

// 反向代理管理：cockpit 只下发自己名下的 cockpit-site-*.conf 片段，
// 用户已有 Nginx 配置零接触（见 docs/guide/proxy-design.md）。
// 状态以 Agent 侧片段文件为唯一事实源，server 纯转发不落库。
const Proxy = () => {
  const queryClient = useQueryClient()
  const [selectedAgent, setSelectedAgent] = useState<string>()
  const [editorOpen, setEditorOpen] = useState(false)
  const [editing, setEditing] = useState<ProxySite | null>(null) // null = 新建
  const [saving, setSaving] = useState(false)
  const [applyError, setApplyError] = useState<string>() // nginx -t 失败摘要等
  const [configView, setConfigView] = useState<{ name: string; content: string } | null>(null)

  const { data: agents } = useQuery({ queryKey: ['agents'], queryFn: () => api.getAgents() })

  // 只有带 nginx-proxy capability 的 agent 可选（其余主机没有 nginx 可管理）
  const agentOptions = useMemo(
    () =>
      (agents ?? []).map((a) => {
        const hasNginx = (a.capabilities ?? []).some((c) => c.type === 'nginx-proxy')
        const version = hasNginx
          ? String((a.capabilities ?? []).find((c) => c.type === 'nginx-proxy')?.metadata?.version ?? '')
          : ''
        return {
          value: a.id,
          disabled: !hasNginx || a.status === 'offline',
          label: `${a.hostname || a.id}${version ? `（${version}）` : ''}${
            a.status === 'offline' ? '（离线）' : !hasNginx ? '（未检测到 Nginx）' : ''
          }`,
        }
      }),
    [agents],
  )

  const sitesKey = ['proxy-sites', selectedAgent]
  const statusKey = ['proxy-status', selectedAgent]

  const { data: status, isLoading: statusLoading } = useQuery({
    queryKey: statusKey,
    queryFn: () => api.getProxyStatus(selectedAgent!),
    enabled: !!selectedAgent,
  })

  const { data: sitesData, isLoading: sitesLoading } = useQuery({
    queryKey: sitesKey,
    queryFn: () => api.getProxySites(selectedAgent!),
    enabled: !!selectedAgent,
  })

  const refresh = () => {
    queryClient.invalidateQueries({ queryKey: ['proxy-sites', selectedAgent] })
    queryClient.invalidateQueries({ queryKey: ['proxy-status', selectedAgent] })
  }

  const [form] = Form.useForm()
  const formScheme = Form.useWatch('scheme', form)

  // ---- 编辑 Modal ----

  const openCreate = () => {
    setEditing(null)
    setApplyError(undefined)
    setEditorOpen(true)
  }

  const openEdit = (site: ProxySite) => {
    setEditing(site)
    setApplyError(undefined)
    setEditorOpen(true)
  }

  const saveSite = async () => {
    if (!selectedAgent) return
    const raw = await form.validateFields()
    const site: ProxySite = {
      name: raw.name.trim(),
      serverNames: (raw.serverNames as string[])
        .map((s: string) => s.trim())
        .filter(Boolean),
      upstream: raw.upstream.trim(),
      scheme: raw.scheme,
      tlsCert: raw.scheme === 'https' ? (raw.tlsCert ?? '').trim() : undefined,
      tlsKey: raw.scheme === 'https' ? (raw.tlsKey ?? '').trim() : undefined,
      websocket: !!raw.websocket,
      extra: (raw.extra ?? '').trim() || undefined,
    }
    if (site.serverNames.length === 0) {
      message.error('请至少填写一个域名')
      return
    }
    setSaving(true)
    setApplyError(undefined)
    try {
      await api.applyProxySite(selectedAgent, site)
      message.success(`站点 ${site.name} 已应用`)
      setEditorOpen(false)
      refresh()
    } catch (e) {
      // agent 侧 nginx -t / reload 失败摘要原样展示——这是修正输入的关键信息
      setApplyError(getApiErrorMessage(e, '应用失败'))
    } finally {
      setSaving(false)
    }
  }

  const deleteSite = async (name: string) => {
    if (!selectedAgent) return
    try {
      await api.deleteProxySite(selectedAgent, name)
      message.success(`站点 ${name} 已删除`)
      refresh()
    } catch (e) {
      message.error(getApiErrorMessage(e, '删除失败'))
    }
  }

  const viewConfig = async (name: string) => {
    if (!selectedAgent) return
    try {
      const detail = await api.getProxySite(selectedAgent, name)
      setConfigView({ name, content: detail.content })
    } catch (e) {
      message.error(getApiErrorMessage(e, '读取配置失败'))
    }
  }

  const columns: ColumnsType<ProxySite> = [
    { title: '名称', dataIndex: 'name', key: 'name', width: 140 },
    {
      title: '域名',
      dataIndex: 'serverNames',
      key: 'serverNames',
      render: (names: string[]) => (
        <Space size={4} wrap>
          {names.map((n) => (
            <Tag key={n} color="geekblue">
              {n}
            </Tag>
          ))}
        </Space>
      ),
    },
    { title: '上游', dataIndex: 'upstream', key: 'upstream', width: 180 },
    { title: '协议', dataIndex: 'scheme', key: 'scheme', width: 90, render: schemeTag },
    {
      title: 'WebSocket',
      dataIndex: 'websocket',
      key: 'websocket',
      width: 100,
      render: (v: boolean) => (v ? <Tag color="purple">支持</Tag> : <Tag>—</Tag>),
    },
    {
      title: '操作',
      key: 'actions',
      width: 220,
      render: (_, record) => (
        <Space size={0}>
          <Button type="link" size="small" icon={<FileTextOutlined />} onClick={() => viewConfig(record.name)}>
            配置
          </Button>
          <Button type="link" size="small" icon={<EditOutlined />} onClick={() => openEdit(record)}>
            编辑
          </Button>
          <Popconfirm
            title="删除站点"
            description={`将从 Nginx 移除 ${record.name} 并 reload，确认？`}
            okText="删除"
            okButtonProps={{ danger: true }}
            onConfirm={() => deleteSite(record.name)}
          >
            <Button type="link" size="small" danger icon={<DeleteOutlined />}>
              删除
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  const sites = sitesData?.sites ?? []

  return (
    <div style={{ padding: 24 }}>
      <Card
        title="反向代理"
        extra={
          <Space>
            <Select
              style={{ minWidth: 260 }}
              options={agentOptions}
              value={selectedAgent}
              onChange={(v) => setSelectedAgent(v)}
              placeholder="选择 Nginx 所在主机"
              showSearch
              optionFilterProp="label"
            />
            <Button
              icon={<ReloadOutlined />}
              disabled={!selectedAgent}
              onClick={refresh}
            />
            <Button type="primary" icon={<PlusOutlined />} disabled={!selectedAgent} onClick={openCreate}>
              新建站点
            </Button>
          </Space>
        }
      >
        {!selectedAgent ? (
          <Empty description="选择一台安装了 Nginx 的主机开始管理" />
        ) : (
          <>
            {status && (
              <Descriptions
                size="small"
                column={{ xs: 1, sm: 2, md: 4 }}
                style={{ marginBottom: 16 }}
                loading={statusLoading}
                items={[
                  {
                    key: 'version',
                    label: 'Nginx 版本',
                    children: status.installed ? status.version || '已安装' : '未检测到',
                  },
                  { key: 'confDir', label: '配置目录', children: <Typography.Text code>{status.confDir}</Typography.Text> },
                  { key: 'siteCount', label: '站点数', children: status.siteCount },
                  {
                    key: 'reloadMode',
                    label: 'reload 方式',
                    children: status.reloadMode === 'systemctl' ? 'systemctl' : 'nginx -s reload',
                  },
                ]}
              />
            )}
            <Alert
              type="info"
              showIcon
              style={{ marginBottom: 16 }}
              message={`只管理 ${status?.confDir ?? '配置目录'} 下的 cockpit-site-*.conf 片段，已有配置零接触；应用前先 nginx -t 校验，失败不落盘，reload 失败自动回滚。`}
            />
            <Table<ProxySite>
              rowKey="name"
              columns={columns}
              dataSource={sites}
              loading={sitesLoading}
              pagination={false}
              locale={{ emptyText: '暂无站点，点击「新建站点」下发第一个配置' }}
            />
          </>
        )}
      </Card>

      {/* 新建/编辑 Modal */}
      <Modal
        title={editing ? `编辑站点 ${editing.name}` : '新建站点'}
        open={editorOpen}
        onCancel={() => setEditorOpen(false)}
        onOk={saveSite}
        confirmLoading={saving}
        width={620}
        destroyOnClose
      >
        {applyError && (
          <Alert
            type="error"
            showIcon
            style={{ marginBottom: 16 }}
            message="应用失败"
            description={<pre style={{ margin: 0, whiteSpace: 'pre-wrap', fontSize: 12 }}>{applyError}</pre>}
          />
        )}
        <Form
          form={form}
          layout="vertical"
          initialValues={{
            scheme: 'http',
            serverNames: [],
            websocket: false,
            ...(editing
              ? {
                  name: editing.name,
                  serverNames: editing.serverNames,
                  upstream: editing.upstream,
                  scheme: editing.scheme,
                  tlsCert: editing.tlsCert,
                  tlsKey: editing.tlsKey,
                  websocket: editing.websocket,
                  extra: editing.extra,
                }
              : {}),
          }}
        >
          <Form.Item
            name="name"
            label="站点名称"
            rules={[
              { required: true, message: '请输入站点名称' },
              { pattern: NAME_PATTERN, message: '小写字母/数字开头，可用 - 和 _，最长 64 字符' },
            ]}
            tooltip="用于配置文件名 cockpit-site-<名称>.conf，创建后不可修改"
          >
            <Input disabled={!!editing} placeholder="如 blog" />
          </Form.Item>
          <Form.Item
            name="serverNames"
            label="域名"
            rules={[
              { required: true, message: '请输入至少一个域名' },
              {
                validator: (_, values: string[]) =>
                  !values || values.every((v) => DOMAIN_PATTERN.test(v.trim()))
                    ? Promise.resolve()
                    : Promise.reject(new Error('域名只能含字母/数字/点/连字符/通配符 *')),
              },
            ]}
            tooltip="支持通配符如 *.example.com，回车确认，最多 16 个"
          >
            <Select mode="tags" open={false} suffixIcon={null} placeholder="blog.example.com" />
          </Form.Item>
          <Form.Item
            name="upstream"
            label="上游地址"
            rules={[
              { required: true, message: '请输入上游地址' },
              { pattern: UPSTREAM_PATTERN, message: '形如 127.0.0.1:3000' },
            ]}
          >
            <Input placeholder="127.0.0.1:3000" />
          </Form.Item>
          <Form.Item name="scheme" label="协议">
            <Radio.Group
              options={[
                { value: 'http', label: 'HTTP' },
                { value: 'https', label: 'HTTPS' },
              ]}
              optionType="button"
            />
          </Form.Item>
          {formScheme === 'https' && (
            <>
              <Form.Item
                name="tlsCert"
                label="证书路径"
                rules={[{ required: true, message: 'https 需要证书绝对路径' }]}
                extra="可经工作台 - 文件上传，或在「证书签发」页配置自动部署（默认推送到 /etc/cockpit/certs/，续期自动更新）"
              >
                <Input placeholder="/etc/cockpit/certs/site.crt.pem" />
              </Form.Item>
              <Form.Item
                name="tlsKey"
                label="私钥路径"
                rules={[{ required: true, message: 'https 需要私钥绝对路径' }]}
              >
                <Input placeholder="/etc/ssl/private/site.key" />
              </Form.Item>
            </>
          )}
          <Form.Item name="websocket" label="WebSocket 支持" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item
            name="extra"
            label="高级指令"
            tooltip="原样插入 server 块内，如 client_max_body_size 50m；语法由 nginx -t 校验兜底"
          >
            <Input.TextArea rows={4} styles={{ input: { fontFamily: 'monospace' } }} placeholder="client_max_body_size 50m;" />
          </Form.Item>
        </Form>
      </Modal>

      {/* 配置查看 Modal */}
      <Modal
        title={`配置预览：${configView?.name ?? ''}`}
        open={!!configView}
        onCancel={() => setConfigView(null)}
        footer={
          <Button type="primary" onClick={() => setConfigView(null)}>
            关闭
          </Button>
        }
        width={720}
      >
        <pre
          style={{
            margin: 0,
            maxHeight: 480,
            overflow: 'auto',
            fontSize: 12,
            lineHeight: 1.6,
            background: 'rgba(128,128,128,0.08)',
            padding: 12,
            borderRadius: 6,
          }}
        >
          {configView?.content}
        </pre>
      </Modal>
    </div>
  )
}

export default Proxy
