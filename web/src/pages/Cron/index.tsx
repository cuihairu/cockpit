import { useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Alert,
  AutoComplete,
  Button,
  Card,
  Collapse,
  Descriptions,
  Empty,
  Form,
  Input,
  Modal,
  Popconfirm,
  Select,
  Space,
  Switch,
  Table,
  Typography,
  message,
} from 'antd'
import { DeleteOutlined, EditOutlined, PlusOutlined, ReloadOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { api } from '@/services/api'
import type { CronJob, SystemdTimer } from '@/types'
import { getApiErrorMessage } from '@/utils/apiError'
import { PermGuard } from '@/components/PermGuard'
import dayjs from 'dayjs'

const NAME_PATTERN = /^[a-z0-9][a-z0-9_-]{0,63}$/

// cron 表达式 5 字段校验（与 agent validateCronExpr 同规则，双端防御）
const CRON_FIELD_RANGES: [number, number][] = [
  [0, 59], [0, 23], [1, 31], [1, 12], [0, 7],
]
const CRON_AT_SET = new Set(['@reboot', '@hourly', '@daily', '@weekly', '@monthly', '@yearly', '@annually'])
const CRON_FIELD_RE = /^(\*|[0-9]+)(-[0-9]+)?(\/[0-9]+)?$/

const validateCronExpr = (expr: string): string | null => {
  const e = expr.trim()
  if (!e) return '请输入表达式'
  if (e.startsWith('@')) return CRON_AT_SET.has(e) ? null : `不支持的简写 ${e}`
  const fields = e.split(/\s+/)
  if (fields.length !== 5) return '需要 5 个字段（分 时 日 月 周）'
  for (let i = 0; i < 5; i++) {
    const [lo, hi] = CRON_FIELD_RANGES[i]
    for (const part of fields[i].split(',')) {
      const m = CRON_FIELD_RE.exec(part)
      if (!m) return `第 ${i + 1} 字段格式非法：${part}`
      // 步长 ≥1（*/0 非法）
      if (m[3]) {
        const step = parseInt(m[3].slice(1), 10)
        if (step < 1 || step > hi) return `第 ${i + 1} 字段步长非法：${m[3]}`
      }
      // 基值/范围数字在字段范围内
      const base = m[1] === '*' ? '' : m[1]
      const range = m[2] ? m[2].slice(1) : ''
      for (const tok of [base, range]) {
        if (!tok) continue
        const n = parseInt(tok, 10)
        if (n < lo || n > hi) return `第 ${i + 1} 字段值 ${tok} 超出范围 [${lo},${hi}]`
      }
    }
  }
  return null
}

// 常用表达式预设（选中即填入）
const SCHEDULE_PRESETS: { value: string; label: string }[] = [
  { value: '* * * * *', label: '每分钟' },
  { value: '*/5 * * * *', label: '每 5 分钟' },
  { value: '0 * * * *', label: '每小时整点' },
  { value: '0 3 * * *', label: '每天 03:00' },
  { value: '0 4 * * 1', label: '每周一 04:00' },
  { value: '0 5 1 * *', label: '每月 1 日 05:00' },
  { value: '@reboot', label: '开机时（@reboot）' },
]

// 定时任务管理：cockpit 只管理自己名下的任务对（meta 注释行 + 命令行），
// 用户手写的 crontab 条目逐行原样保留（见 docs/guide/cron-design.md）。
// crontab 是唯一事实源，server 纯转发不落库。
// M4：可选目标用户（-u），缺省 = agent 当前运行用户。
const Cron = () => {
  const queryClient = useQueryClient()
  const [selectedAgent, setSelectedAgent] = useState<string>()
  const [selectedUser, setSelectedUser] = useState<string>('') // '' = 当前用户
  const [editorOpen, setEditorOpen] = useState(false)
  const [editing, setEditing] = useState<CronJob | null>(null) // null = 新建
  const [saving, setSaving] = useState(false)
  const [applyError, setApplyError] = useState<string>()

  const { data: agents } = useQuery({ queryKey: ['agents'], queryFn: () => api.getAgents() })

  // 只有带 cron capability（crontab 命令存在）的 agent 可选
  const agentOptions = useMemo(
    () =>
      (agents ?? []).map((a) => {
        const hasCron = (a.capabilities ?? []).some((c) => c.type === 'cron')
        return {
          value: a.id,
          disabled: !hasCron || a.status === 'offline',
          label: `${a.hostname || a.id}${
            a.status === 'offline' ? '（离线）' : !hasCron ? '（未检测到 crontab）' : ''
          }`,
        }
      }),
    [agents],
  )

  // 系统用户枚举（M4 D22/D24）：失败静默——下拉缺席仍可手输任意合法名
  const { data: usersData } = useQuery({
    queryKey: ['cron-users', selectedAgent],
    queryFn: () => api.getCronUsers(selectedAgent!),
    enabled: !!selectedAgent,
    retry: false,
  })
  const userOptions = useMemo(
    () => [
      { value: '', label: '当前用户' },
      ...(usersData?.users ?? []).map((u) => ({
        value: u.name,
        label: u.shell ? `${u.name}（${u.shell}）` : u.name,
      })),
    ],
    [usersData],
  )

  const jobsKey = ['cron-jobs', selectedAgent, selectedUser]
  const statusKey = ['cron-status', selectedAgent, selectedUser]

  const { data: status } = useQuery({
    queryKey: statusKey,
    queryFn: () => api.getCronStatus(selectedAgent!, selectedUser || undefined),
    enabled: !!selectedAgent,
  })
  const { data: jobsData, isLoading: jobsLoading } = useQuery({
    queryKey: jobsKey,
    queryFn: () => api.getCronJobs(selectedAgent!, selectedUser || undefined),
    enabled: !!selectedAgent,
  })

  // systemd timer 只读列表（cron-design.md M3）：失败静默（如无 systemd 的
  // 主机），面板不渲染
  const { data: timersData } = useQuery({
    queryKey: ['cron-timers', selectedAgent],
    queryFn: () => api.getCronTimers(selectedAgent!),
    enabled: !!selectedAgent,
    retry: false,
  })

  const refresh = () => {
    queryClient.invalidateQueries({ queryKey: ['cron-jobs', selectedAgent, selectedUser] })
    queryClient.invalidateQueries({ queryKey: ['cron-status', selectedAgent, selectedUser] })
  }

  const [form] = Form.useForm()
  const formSchedule = Form.useWatch('schedule', form)

  const openCreate = () => {
    setEditing(null)
    setApplyError(undefined)
    setEditorOpen(true)
  }

  const openEdit = (job: CronJob) => {
    setEditing(job)
    setApplyError(undefined)
    setEditorOpen(true)
  }

  const saveJob = async () => {
    if (!selectedAgent) return
    const raw = await form.validateFields().catch(() => undefined)
    if (!raw) return
    const job: CronJob = {
      name: raw.name.trim(),
      schedule: raw.schedule.trim(),
      command: raw.command,
      enabled: !!raw.enabled,
    }
    setSaving(true)
    setApplyError(undefined)
    try {
      await api.applyCronJob(selectedAgent, job, selectedUser || undefined)
      message.success(
        selectedUser ? `任务 ${job.name} 已写入 ${selectedUser} 的 crontab` : `任务 ${job.name} 已写入 crontab`,
      )
      setEditorOpen(false)
      refresh()
    } catch (e) {
      setApplyError(getApiErrorMessage(e, '写入失败'))
    } finally {
      setSaving(false)
    }
  }

  // 启停 = 原样 apply，仅翻转 enabled（停用任务渲染为注释行，不丢失）
  const toggleEnabled = async (job: CronJob, enabled: boolean) => {
    if (!selectedAgent) return
    try {
      await api.applyCronJob(selectedAgent, { ...job, enabled }, selectedUser || undefined)
      message.success(enabled ? `任务 ${job.name} 已启用` : `任务 ${job.name} 已停用`)
      refresh()
    } catch (e) {
      message.error(getApiErrorMessage(e, '操作失败'))
    }
  }

  const deleteJob = async (name: string) => {
    if (!selectedAgent) return
    try {
      await api.deleteCronJob(selectedAgent, name, selectedUser || undefined)
      message.success(`任务 ${name} 已删除`)
      refresh()
    } catch (e) {
      message.error(getApiErrorMessage(e, '删除失败'))
    }
  }

  const columns: ColumnsType<CronJob> = [
    { title: '名称', dataIndex: 'name', key: 'name', width: 140 },
    {
      title: '表达式',
      dataIndex: 'schedule',
      key: 'schedule',
      width: 160,
      render: (v: string) => <Typography.Text code>{v}</Typography.Text>,
    },
    {
      title: '命令',
      dataIndex: 'command',
      key: 'command',
      ellipsis: true,
      render: (v: string) => (
        <Typography.Text code ellipsis={{ tooltip: v }}>
          {v}
        </Typography.Text>
      ),
    },
    {
      title: '下次触发',
      key: 'next_run',
      width: 170,
      render: (_, record) => {
        // 下次触发预览（cron-design.md M2）：agent 按服务器时区算好 unix 秒
        if (!record.enabled) return <Typography.Text type="secondary">已禁用</Typography.Text>
        if (record.schedule === '@reboot') return <Typography.Text type="secondary">开机时</Typography.Text>
        if (!record.next_run) return '-'
        return dayjs.unix(record.next_run).format('YYYY-MM-DD HH:mm')
      },
    },
    {
      title: '启用',
      dataIndex: 'enabled',
      key: 'enabled',
      width: 90,
      render: (v: boolean, record) => (
        <PermGuard perm="cron:write" fallback={<Switch size="small" checked={v} disabled />}>
          <Switch size="small" checked={v} onChange={(checked) => toggleEnabled(record, checked)} />
        </PermGuard>
      ),
    },
    {
      title: '操作',
      key: 'actions',
      width: 160,
      render: (_, record) => (
        <Space size={0}>
          <PermGuard perm="cron:write">
            <Button type="link" size="small" icon={<EditOutlined />} onClick={() => openEdit(record)}>
              编辑
            </Button>
            <Popconfirm
              title="删除任务"
              description={`将从 crontab 移除 ${record.name}，确认？`}
              okText="删除"
              okButtonProps={{ danger: true }}
              onConfirm={() => deleteJob(record.name)}
            >
              <Button type="link" size="small" danger icon={<DeleteOutlined />}>
                删除
              </Button>
            </Popconfirm>
          </PermGuard>
        </Space>
      ),
    },
  ]

  const jobs = jobsData?.jobs ?? []

  // timer 关键字过滤（unit 名 / 描述）
  const [timerKeyword, setTimerKeyword] = useState('')
  const timers = useMemo(() => {
    const all = timersData?.timers ?? []
    const kw = timerKeyword.trim().toLowerCase()
    if (!kw) return all
    return all.filter(
      (t) => t.unit.toLowerCase().includes(kw) || (t.description ?? '').toLowerCase().includes(kw),
    )
  }, [timersData, timerKeyword])

  const timerColumns: ColumnsType<SystemdTimer> = [
    {
      title: 'Unit',
      dataIndex: 'unit',
      key: 'unit',
      width: 240,
      render: (v: string) => <Typography.Text code>{v}</Typography.Text>,
    },
    {
      title: '描述',
      dataIndex: 'description',
      key: 'description',
      ellipsis: true,
      render: (v: string) => v || '-',
    },
    {
      title: '日程',
      dataIndex: 'schedule',
      key: 'schedule',
      ellipsis: true,
      render: (v: string) =>
        v ? (
          <Typography.Text code ellipsis={{ tooltip: v }}>
            {v}
          </Typography.Text>
        ) : (
          '-'
        ),
    },
    { title: '状态', dataIndex: 'unitFileState', key: 'unitFileState', width: 90, render: (v: string) => v || '-' },
    {
      title: '上次触发',
      dataIndex: 'last_trigger',
      key: 'last_trigger',
      width: 150,
      render: (v?: number) => (v ? dayjs.unix(v).format('YYYY-MM-DD HH:mm') : '-'),
    },
    {
      title: '下次触发',
      dataIndex: 'next_run',
      key: 'next_run',
      width: 150,
      render: (v?: number) => (v ? dayjs.unix(v).format('YYYY-MM-DD HH:mm') : '-'),
    },
  ]

  return (
    <div style={{ padding: 24 }}>
      <Card
        title="定时任务"
        extra={
          <Space>
            <Select
              style={{ minWidth: 260 }}
              options={agentOptions}
              value={selectedAgent}
              onChange={(v) => {
                setSelectedAgent(v)
                setSelectedUser('') // 换主机回到当前用户视角
              }}
              placeholder="选择主机"
              showSearch
              optionFilterProp="label"
            />
            {/* 目标用户（M4 D24）：枚举下拉 + 可手输，空值 = agent 当前运行用户 */}
            <AutoComplete
              style={{ minWidth: 200 }}
              options={userOptions}
              value={selectedUser}
              onChange={(v) => setSelectedUser((v ?? '').trim())}
              placeholder="当前用户"
              disabled={!selectedAgent}
              filterOption={(input, option) =>
                (option?.value as string ?? '').includes(input.toLowerCase())
              }
            />
            <Button icon={<ReloadOutlined />} disabled={!selectedAgent} onClick={refresh} />
            <PermGuard perm="cron:write">
              <Button type="primary" icon={<PlusOutlined />} disabled={!selectedAgent} onClick={openCreate}>
                新建任务
              </Button>
            </PermGuard>
          </Space>
        }
      >
        {!selectedAgent ? (
          <Empty description="选择一台主机开始管理定时任务" />
        ) : (
          <>
            {status && (
              <Descriptions
                size="small"
                column={{ xs: 1, sm: 3 }}
                style={{ marginBottom: 16 }}
                items={[
                  {
                    key: 'user',
                    label: selectedUser ? '目标用户' : '运行用户',
                    children: status.user || '—',
                  },
                  { key: 'cockpit', label: 'Cockpit 任务', children: status.cockpitCount },
                  { key: 'external', label: '外部条目', children: status.externalCount },
                ]}
              />
            )}
            <Alert
              type="info"
              showIcon
              style={{ marginBottom: 16 }}
              message={
                selectedUser
                  ? `正在管理 ${selectedUser} 的 crontab（需要 agent 以 root 运行）：只管理 Cockpit 下发的任务（带标记注释），该用户手写的 crontab 条目逐行原样保留，写回前自动自检。`
                  : '只管理 Cockpit 下发的任务（带标记注释），用户手写的 crontab 条目逐行原样保留，写回前自动自检。可在右上角输入用户名管理其他用户的 crontab。'
              }
            />
            <Table<CronJob>
              rowKey="name"
              columns={columns}
              dataSource={jobs}
              loading={jobsLoading}
              pagination={false}
              locale={{ emptyText: '暂无任务，点击「新建任务」添加第一个定时任务' }}
            />
            {(jobsData?.external ?? '').trim() !== '' && (
              <Collapse
                style={{ marginTop: 16 }}
                items={[
                  {
                    key: 'external',
                    label: `外部条目（只读，共 ${status?.externalCount ?? 0} 行非空条目）`,
                    children: (
                      <pre
                        style={{
                          margin: 0,
                          maxHeight: 320,
                          overflow: 'auto',
                          fontSize: 12,
                          lineHeight: 1.6,
                          background: 'rgba(128,128,128,0.08)',
                          padding: 12,
                          borderRadius: 6,
                        }}
                      >
                        {jobsData?.external}
                      </pre>
                    ),
                  },
                ]}
              />
            )}
            {timersData && timersData.timers.length > 0 && (
              <Collapse
                style={{ marginTop: 16 }}
                items={[
                  {
                    key: 'timers',
                    label: `systemd 定时器（只读，共 ${timersData.timers.length} 个）`,
                    children: (
                      <>
                        <Input
                          placeholder="按 unit 或描述过滤"
                          allowClear
                          style={{ maxWidth: 280, marginBottom: 12 }}
                          value={timerKeyword}
                          onChange={(e) => setTimerKeyword(e.target.value)}
                        />
                        <Table<SystemdTimer>
                          rowKey="unit"
                          size="small"
                          columns={timerColumns}
                          dataSource={timers}
                          pagination={false}
                        />
                      </>
                    ),
                  },
                ]}
              />
            )}
          </>
        )}
      </Card>

      {/* 新建/编辑 Modal */}
      <Modal
        title={editing ? `编辑任务 ${editing.name}` : '新建任务'}
        open={editorOpen}
        onCancel={() => setEditorOpen(false)}
        onOk={saveJob}
        confirmLoading={saving}
        width={600}
        destroyOnClose
      >
        {applyError && (
          <Alert
            type="error"
            showIcon
            style={{ marginBottom: 16 }}
            message="写入失败"
            description={<pre style={{ margin: 0, whiteSpace: 'pre-wrap', fontSize: 12 }}>{applyError}</pre>}
          />
        )}
        <Form
          form={form}
          layout="vertical"
          initialValues={{
            enabled: true,
            schedule: '* * * * *',
            ...(editing
              ? { name: editing.name, schedule: editing.schedule, command: editing.command, enabled: editing.enabled }
              : {}),
          }}
        >
          <Form.Item
            name="name"
            label="任务名称"
            rules={[
              { required: true, message: '请输入任务名称' },
              { pattern: NAME_PATTERN, message: '小写字母/数字开头，可用 - 和 _，最长 64 字符' },
            ]}
            tooltip="用于任务标记注释，创建后不可修改"
          >
            <Input disabled={!!editing} placeholder="如 nightly-backup" />
          </Form.Item>
          <Form.Item label="常用预设">
            <Select
              placeholder="选择常用表达式自动填入"
              value={SCHEDULE_PRESETS.some((p) => p.value === formSchedule) ? formSchedule : undefined}
              onChange={(v) => form.setFieldValue('schedule', v)}
              options={SCHEDULE_PRESETS}
              allowClear
            />
          </Form.Item>
          <Form.Item
            name="schedule"
            label="表达式"
            rules={[
              { required: true, message: '请输入表达式' },
              {
                validator: (_, v: string) => {
                  const err = validateCronExpr(v ?? '')
                  return err ? Promise.reject(new Error(err)) : Promise.resolve()
                },
              },
            ]}
            tooltip="5 字段（分 时 日 月 周）或 @daily 等简写"
          >
            <Input placeholder="0 3 * * *" styles={{ input: { fontFamily: 'monospace' } }} />
          </Form.Item>
          <Form.Item
            name="command"
            label="命令"
            rules={[
              { required: true, message: '请输入命令' },
              {
                validator: (_, v: string) =>
                  v && /[\r\n]/.test(v)
                    ? Promise.reject(new Error('命令不能包含换行（crontab 单行语义）'))
                    : Promise.resolve(),
              },
            ]}
          >
            <Input.TextArea rows={3} styles={{ textarea: { fontFamily: 'monospace' } }} placeholder="/opt/scripts/backup.sh" />
          </Form.Item>
          <Form.Item name="enabled" label="启用" valuePropName="checked" extra="停用任务渲染为注释行，随时可重新启用">
            <Switch />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

export default Cron
