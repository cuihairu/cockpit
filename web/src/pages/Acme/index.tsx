import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Badge,
  Button,
  Card,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  Tooltip,
  Typography,
  message,
} from 'antd'
import {
  CloudUploadOutlined,
  DeleteOutlined,
  DownloadOutlined,
  EditOutlined,
  PlusOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { api } from '@/services/api'
import type { AcmeCertView } from '@/types'
import { getApiErrorMessage } from '@/utils/apiError'

// ACME 证书自动签发页（见 docs/guide/acme-design.md D11）：
// DNS-01 challenge + Cloudflare（token 与 DNS 管理共用），
// 默认 staging 目录（假证书不触生产限频），自动续期巡检临期重签。

const STATUS_META: Record<AcmeCertView['status'], { label: string; badge: 'success' | 'error' | 'default' }> = {
  issued: { label: '已签发', badge: 'success' },
  failed: { label: '失败', badge: 'error' },
  pending: { label: '待签发', badge: 'default' },
}

interface AcmeFormValues {
  domains: string[]
  caDirectory: 'staging' | 'production'
  autoRenew: boolean
  renewBeforeDays: number
}

const formatUnix = (unix: number) => (unix ? new Date(unix * 1000).toLocaleString() : '—')

const daysLeft = (iso: string) => {
  if (!iso) return null
  return Math.floor((new Date(iso).getTime() - Date.now()) / 86400000)
}

const Acme = () => {
  const queryClient = useQueryClient()
  const [editing, setEditing] = useState<AcmeCertView | null>(null)
  const [modalOpen, setModalOpen] = useState(false)
  const [accountOpen, setAccountOpen] = useState(false)
  const [emailForm] = Form.useForm<{ email: string }>()
  const [form] = Form.useForm<AcmeFormValues>()
  const [scanEdit, setScanEdit] = useState<{ on?: boolean; minutes?: number } | null>(null)
  const [savingScan, setSavingScan] = useState(false)
  const [issuingId, setIssuingId] = useState<number | null>(null)

  const { data: certs = [], isLoading } = useQuery({ queryKey: ['acme-certs'], queryFn: () => api.getAcmeCerts() })
  const { data: scanCfg } = useQuery({ queryKey: ['acme-scan-config'], queryFn: () => api.getAcmeScanConfig() })
  const { data: account } = useQuery({ queryKey: ['acme-account'], queryFn: () => api.getAcmeAccount() })
  const { data: dnsStatus } = useQuery({ queryKey: ['dns-status'], queryFn: () => api.getDNSStatus() })

  const savedInterval = scanCfg?.scan_interval_seconds ?? 0
  const scanOn = scanEdit?.on ?? savedInterval > 0
  const scanMinutes = scanEdit?.minutes ?? (savedInterval > 0 ? Math.round(savedInterval / 60) : 60)

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['acme-certs'] })

  const saveScanConfig = async () => {
    const seconds = scanOn ? scanMinutes * 60 : 0
    if (scanOn && (scanMinutes < 5 || scanMinutes > 1440)) {
      message.warning('间隔需在 5～1440 分钟之间')
      return
    }
    setSavingScan(true)
    try {
      await api.putAcmeScanConfig(seconds)
      setScanEdit(null)
      await queryClient.invalidateQueries({ queryKey: ['acme-scan-config'] })
      message.success(scanOn ? '已开启自动续期' : '已关闭自动续期')
    } catch (err) {
      message.error(getApiErrorMessage(err, '保存失败'))
    } finally {
      setSavingScan(false)
    }
  }

  const issueMut = useMutation({
    mutationFn: (id: number) => api.issueAcmeCert(id),
    onSuccess: (res) => {
      void invalidate()
      message.success(`签发成功：${res.primaryDomain}，有效期至 ${new Date(res.expiresAt).toLocaleDateString()}`)
    },
    onError: (err) => message.error(getApiErrorMessage(err, '签发失败')),
  })

  const deleteMut = useMutation({
    mutationFn: (id: number) => api.deleteAcmeCert(id),
    onSuccess: () => {
      void invalidate()
      message.success('已删除（不影响 CA 侧已签发的证书）')
    },
    onError: (err) => message.error(getApiErrorMessage(err, '删除失败')),
  })

  const download = async (cert: AcmeCertView, part: 'cert' | 'issuer' | 'key') => {
    try {
      const body = await api.downloadAcmeCert(cert.id, part)
      const blob = new Blob([body], { type: 'application/x-pem-file' })
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `${cert.primaryDomain}.${part === 'cert' ? 'crt' : part === 'issuer' ? 'issuer' : 'key'}.pem`
      a.click()
      URL.revokeObjectURL(url)
      if (part === 'key') message.info('私钥已下载（该操作已记入审计日志）')
    } catch (err) {
      message.error(getApiErrorMessage(err, '下载失败'))
    }
  }

  const openCreate = () => {
    setEditing(null)
    form.setFieldsValue({ domains: [], caDirectory: 'staging', autoRenew: true, renewBeforeDays: 30 })
    setModalOpen(true)
  }

  const openEdit = (cert: AcmeCertView) => {
    setEditing(cert)
    form.setFieldsValue({
      domains: cert.domains,
      caDirectory: cert.caDirectory,
      autoRenew: cert.autoRenew,
      renewBeforeDays: cert.renewBeforeDays,
    })
    setModalOpen(true)
  }

  const submit = async () => {
    const values = await form.validateFields()
    const domains = values.domains.map((d) => d.trim().toLowerCase()).filter(Boolean)
    if (domains.length === 0) {
      message.warning('请至少填写一个域名')
      return
    }
    const input = {
      domains,
      caDirectory: values.caDirectory,
      autoRenew: values.autoRenew,
      renewBeforeDays: values.renewBeforeDays,
    }
    try {
      if (editing) {
        await api.updateAcmeCert(editing.id, input)
        message.success('已保存（域名/CA 变更后需重新签发）')
      } else {
        await api.createAcmeCert(input)
        message.success('已创建，点「签发」获取证书')
      }
      setModalOpen(false)
      await invalidate()
    } catch (err) {
      message.error(getApiErrorMessage(err, '保存失败'))
    }
  }

  const saveEmail = async () => {
    const { email } = await emailForm.validateFields()
    try {
      await api.putAcmeAccount(email.trim())
      setAccountOpen(false)
      await queryClient.invalidateQueries({ queryKey: ['acme-account'] })
      message.success('已保存')
    } catch (err) {
      message.error(getApiErrorMessage(err, '保存失败'))
    }
  }

  const columns: ColumnsType<AcmeCertView> = [
    {
      title: '状态',
      dataIndex: 'status',
      width: 90,
      render: (v: AcmeCertView['status'], r) => {
        const meta = STATUS_META[v] ?? STATUS_META.pending
        const badge = <Badge status={meta.badge} text={meta.label} />
        return v === 'failed' && r.lastError ? <Tooltip title={r.lastError}>{badge}</Tooltip> : badge
      },
    },
    {
      title: '域名',
      dataIndex: 'primaryDomain',
      render: (v: string, r) => (
        <Space size={4} wrap>
          <Typography.Text code>{v}</Typography.Text>
          {r.domains.length > 1 && <Tag>共 {r.domains.length} 个（含泛域名）</Tag>}
        </Space>
      ),
    },
    {
      title: 'CA',
      dataIndex: 'caDirectory',
      width: 90,
      render: (v: AcmeCertView['caDirectory']) =>
        v === 'production' ? <Tag color="green">正式</Tag> : <Tooltip title="Let's Encrypt staging：测试用假证书，不触生产限频"><Tag color="orange">测试</Tag></Tooltip>,
    },
    {
      title: '到期',
      dataIndex: 'expiresAt',
      width: 200,
      render: (v: string) => {
        const left = daysLeft(v)
        if (left === null) return '—'
        const color = left < 15 ? '#cf1322' : left < 30 ? '#d46b08' : undefined
        return (
          <span>
            {new Date(v).toLocaleDateString()}{' '}
            <Typography.Text type={color ? undefined : 'secondary'} style={{ color, fontSize: 12 }}>
              （剩 {left} 天）
            </Typography.Text>
          </span>
        )
      },
    },
    {
      title: '自动续期',
      dataIndex: 'autoRenew',
      width: 90,
      render: (v: boolean, r) =>
        v ? (
          <Tooltip title={`临期 ${r.renewBeforeDays} 天时巡检自动重签`}>
            <Tag color="blue">开（{r.renewBeforeDays}天前）</Tag>
          </Tooltip>
        ) : (
          <Tag>关</Tag>
        ),
    },
    {
      title: '最后签发',
      dataIndex: 'lastRenewAt',
      width: 170,
      render: (v: number) => <span style={{ fontSize: 12 }}>{formatUnix(v)}</span>,
    },
    {
      title: '操作',
      key: 'actions',
      width: 200,
      render: (_, r) => (
        <Space size={4}>
          <Tooltip title={r.status === 'issued' ? '重新签发' : '立即签发'}>
            <Button
              size="small"
              type="text"
              icon={<CloudUploadOutlined />}
              loading={issuingId === r.id}
              onClick={async () => {
                setIssuingId(r.id)
                try {
                  await issueMut.mutateAsync(r.id)
                } finally {
                  setIssuingId(null)
                }
              }}
            />
          </Tooltip>
          {r.status === 'issued' && (
            <Space.Compact size="small">
              <Button size="small" type="text" icon={<DownloadOutlined />} onClick={() => void download(r, 'cert')}>
                证书
              </Button>
              <Button size="small" type="text" onClick={() => void download(r, 'key')}>
                私钥
              </Button>
            </Space.Compact>
          )}
          <Button size="small" type="text" icon={<EditOutlined />} onClick={() => openEdit(r)} />
          <Popconfirm title="删除该签发配置？（仅删本地记录，不吊销 CA 侧证书）" onConfirm={() => deleteMut.mutate(r.id)}>
            <Button size="small" type="text" danger icon={<DeleteOutlined />} />
          </Popconfirm>
        </Space>
      ),
    },
  ]

  // 未配置 Cloudflare token：引导（ACME DNS-01 与 DNS 管理共用同一 token）
  if (dnsStatus && !dnsStatus.configured) {
    return (
      <Card>
        <Typography.Paragraph>
          在 config.yaml 设置 <Typography.Text code>dns.cloudflare.api_token</Typography.Text>
          ，或用环境变量 <Typography.Text code>CLOUDFLARE_API_TOKEN</Typography.Text> 注入后重启
          server。证书签发（DNS-01 验证）与 DNS 管理共用这一份 token，只需 Zone.DNS 编辑权限。
        </Typography.Paragraph>
      </Card>
    )
  }

  return (
    <Space direction="vertical" size={16} style={{ width: '100%' }}>
      <Space wrap align="center">
        <Typography.Text strong>自动续期巡检</Typography.Text>
        <Tooltip title="开启后 server 定时检查证书有效期，临期自动重新签发；失败产生告警（同一证书未处理期间只提醒一次）">
          <Switch checked={scanOn} onChange={(on) => setScanEdit((e) => ({ ...e, on }))} loading={!scanCfg} />
        </Tooltip>
        {scanOn && (
          <InputNumber
            min={5}
            max={1440}
            value={scanMinutes}
            onChange={(v) => setScanEdit((e) => ({ ...e, minutes: v ?? 5 }))}
            addonAfter="分钟"
            style={{ width: 130 }}
          />
        )}
        <Button size="small" onClick={() => void saveScanConfig()} loading={savingScan}>
          保存
        </Button>
        <Button
          size="small"
          onClick={() => {
            emailForm.setFieldsValue({ email: account?.email ?? '' })
            setAccountOpen(true)
          }}
        >
          账户邮箱{account?.email ? `：${account.email}` : '未设置'}
        </Button>
        <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
          新建签发
        </Button>
      </Space>
      <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block' }}>
        通过 DNS-01 验证签发 Let&apos;s Encrypt 证书：无需公网可达端口，支持泛域名；签发全程在 server 侧完成，已签发证书会同步出现在「资源 → 证书」。
      </Typography.Text>

      <Card size="small">
        <Table<AcmeCertView>
          rowKey="id"
          loading={isLoading}
          columns={columns}
          dataSource={certs}
          pagination={false}
          size="small"
          locale={{ emptyText: '暂无签发配置——新建一条，为你的域名自动获取 HTTPS 证书' }}
        />
      </Card>

      {certs.some((c) => c.status === 'failed') && (
        <Typography.Text type="warning" style={{ fontSize: 12 }}>
          有配置最近一次签发失败：悬停状态徽标查看原因。签发失败后自动续期每小时最多重试一次，避免触发 CA 限频；也可以手动「重新签发」。
        </Typography.Text>
      )}

      <Modal
        title={editing ? '编辑签发配置' : '新建签发配置'}
        open={modalOpen}
        onOk={() => void submit()}
        onCancel={() => setModalOpen(false)}
        okText="保存"
        destroyOnClose
        width={520}
      >
        <Form form={form} layout="vertical">
          <Form.Item
            name="domains"
            label="域名（首个为主域名）"
            rules={[{ required: true, message: '请填写至少一个域名' }]}
            extra="回车添加多个；泛域名填 *.example.com（仅 DNS-01 支持）"
          >
            <Select mode="tags" open={false} tokenSeparators={[',', ' ']} placeholder="home.example.com" />
          </Form.Item>
          <Form.Item name="caDirectory" label="CA 目录" rules={[{ required: true }]}>
            <Select
              options={[
                { value: 'staging', label: 'staging（测试：假证书，不触生产限频）' },
                { value: 'production', label: 'production（正式：受浏览器信任）' },
              ]}
            />
          </Form.Item>
          <Form.Item name="autoRenew" label="自动续期" valuePropName="checked" extra="开启后由巡检在临期时自动重签">
            <Switch />
          </Form.Item>
          <Form.Item name="renewBeforeDays" label="提前续期（天）" rules={[{ required: true }]}>
            <InputNumber min={7} max={90} style={{ width: 200 }} />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title="ACME 账户邮箱"
        open={accountOpen}
        onOk={() => void saveEmail()}
        onCancel={() => setAccountOpen(false)}
        okText="保存"
        destroyOnClose
      >
        <Form form={emailForm} layout="vertical">
          <Form.Item
            name="email"
            label="邮箱"
            extra="用于 CA 账户注册与到期提醒邮件；留空表示不设置"
            rules={[{ type: 'email', message: '邮箱格式不正确' }]}
          >
            <Input placeholder="you@example.com" autoComplete="off" />
          </Form.Item>
        </Form>
      </Modal>
    </Space>
  )
}

export default Acme
