import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Alert,
  Button,
  Card,
  Drawer,
  Empty,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Radio,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  TimePicker,
  Tooltip,
  Typography,
  message,
} from 'antd'
import {
  CaretRightOutlined,
  CloudUploadOutlined,
  DeleteOutlined,
  DownloadOutlined,
  EditOutlined,
  FolderOpenOutlined,
  HistoryOutlined,
  PlusOutlined,
  UndoOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import dayjs from 'dayjs'
import { api } from '@/services/api'
import type { BackupConfig, BackupConfigInput, BackupFile, BackupRun, BackupTask } from '@/types'
import { getApiErrorMessage } from '@/utils/apiError'
import { PermGuard } from '@/components/PermGuard'
import ServerBackupCard from './ServerBackupCard'

const NAME_PATTERN = /^[a-z0-9][a-z0-9_-]{0,63}$/

// schedule → 展示文本
const scheduleLabel = (s: string): string => {
  if (s === 'manual') return '手动'
  if (s.startsWith('daily@')) return `每日 ${s.slice(6)}`
  if (s.startsWith('every:')) return `每 ${parseInt(s.slice(6), 10)} 小时`
  return s
}

const statusTag = (status: string) => {
  switch (status) {
    case 'success':
      return <Tag color="success">成功</Tag>
    case 'failed':
    case 'timeout':
      return <Tag color="error">失败</Tag>
    case 'running':
      return <Tag color="processing">运行中</Tag>
    default:
      return <Tag>未运行</Tag>
  }
}

const formatBytes = (n: number): string => {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  if (n < 1024 * 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MB`
  return `${(n / 1024 / 1024 / 1024).toFixed(2)} GB`
}

const formatTime = (unix: number): string =>
  unix ? dayjs.unix(unix).format('YYYY-MM-DD HH:mm:ss') : '—'

// 备份管理：配置 CRUD + 立即运行 + 运行历史 + 备份文件浏览。
// 备份文件在 Agent 侧本地生成（tar.gz），server 只做调度与历史。
const Backups = () => {
  const queryClient = useQueryClient()
  const [editorOpen, setEditorOpen] = useState(false)
  const [editing, setEditing] = useState<BackupConfig | null>(null)
  const [saving, setSaving] = useState(false)
  const [historyFor, setHistoryFor] = useState<BackupConfig | null>(null)
  const [filesFor, setFilesFor] = useState<BackupConfig | null>(null)
  // 恢复（restore）：表单阶段 → 提交后切任务视图轮询
  const [restoreTarget, setRestoreTarget] = useState<{ cfg: BackupConfig; file: BackupFile } | null>(null)
  const [restoreForm] = Form.useForm<{ destDir: string; confirmName: string }>()
  const [restoreTaskId, setRestoreTaskId] = useState<string | null>(null)
  const [restoreDestDir, setRestoreDestDir] = useState('')
  const [restoring, setRestoring] = useState(false)
  const [form] = Form.useForm<{
    agentId: string
    name: string
    sources: string[]
    destDir: string
    remoteDest: string
    preHook: string
    scheduleType: 'manual' | 'daily' | 'every'
    dailyTime: dayjs.Dayjs | null
    everyHours: number
    retention: number
    enabled: boolean
  }>()

  const { data: configsData, isLoading } = useQuery({
    queryKey: ['backup-configs'],
    queryFn: () => api.getBackupConfigs(),
    // 有运行中的备份时轮询，终态自动刷新
    refetchInterval: (query) =>
      query.state.data?.configs.some((c) => c.last_status === 'running') ? 5000 : false,
  })
  const { data: agents } = useQuery({ queryKey: ['agents'], queryFn: () => api.getAgents() })

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['backup-configs'] })

  const runMutation = useMutation({
    mutationFn: (id: number) => api.runBackup(id),
    onSuccess: () => {
      message.success('备份任务已下发，稍后可查看运行历史')
      invalidate()
    },
    onError: (err) => message.error(getApiErrorMessage(err, '触发备份失败')),
  })

  const deleteMutation = useMutation({
    mutationFn: (id: number) => api.deleteBackupConfig(id),
    onSuccess: () => {
      message.success('配置已删除')
      invalidate()
    },
    onError: (err) => message.error(getApiErrorMessage(err, '删除配置失败')),
  })

  const configs = configsData?.configs ?? []
  const agentOptions = useMemo(
    () =>
      (agents ?? []).map((a) => ({
        value: a.id,
        label: `${a.hostname || a.id}${a.status === 'offline' ? '（离线）' : ''}`,
      })),
    [agents],
  )

  const openCreate = () => {
    setEditing(null)
    form.setFieldsValue({
      agentId: undefined,
      name: '',
      sources: [],
      destDir: '',
      remoteDest: '',
      preHook: '',
      scheduleType: 'daily',
      dailyTime: dayjs('03:00', 'HH:mm'),
      everyHours: 6,
      retention: 7,
      enabled: true,
    })
    setEditorOpen(true)
  }

  const openEdit = (cfg: BackupConfig) => {
    setEditing(cfg)
    const daily = cfg.schedule.startsWith('daily@')
    const every = cfg.schedule.startsWith('every:')
    form.setFieldsValue({
      agentId: cfg.agent_id,
      name: cfg.name,
      sources: cfg.sources,
      destDir: cfg.dest_dir,
      remoteDest: cfg.remote_dest ?? '',
      preHook: cfg.pre_hook ?? '',
      scheduleType: daily ? 'daily' : every ? 'every' : 'manual',
      dailyTime: daily ? dayjs(cfg.schedule.slice(6), 'HH:mm') : null,
      everyHours: every ? parseInt(cfg.schedule.slice(6), 10) : 6,
      retention: cfg.retention,
      enabled: cfg.enabled,
    })
    setEditorOpen(true)
  }

  const save = async () => {
    try {
      const values = await form.validateFields()
      let schedule = 'manual'
      if (values.scheduleType === 'daily' && values.dailyTime) {
        schedule = `daily@${values.dailyTime.format('HH:mm')}`
      } else if (values.scheduleType === 'every') {
        schedule = `every:${values.everyHours}h`
      }
      const input: BackupConfigInput = {
        agent_id: values.agentId,
        name: values.name,
        sources: values.sources,
        dest_dir: values.destDir,
        remote_dest: values.remoteDest ?? '',
        pre_hook: values.preHook ?? '',
        schedule,
        retention: values.retention ?? 0,
        enabled: values.enabled,
      }
      setSaving(true)
      if (editing) {
        await api.updateBackupConfig(editing.id, input)
        message.success('配置已更新')
      } else {
        await api.createBackupConfig(input)
        message.success('配置已创建')
      }
      setEditorOpen(false)
      invalidate()
    } catch (err) {
      if (err && typeof err === 'object' && 'errorFields' in err) return // 表单校验错误
      message.error(getApiErrorMessage(err, '保存配置失败'))
    } finally {
      setSaving(false)
    }
  }

  const formSelectedType = Form.useWatch('scheduleType', form)

  const configColumns: ColumnsType<BackupConfig> = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      render: (name: string, cfg) => (
        <Space>
          <Typography.Text strong>{name}</Typography.Text>
          {!cfg.enabled && <Tag>已停用</Tag>}
        </Space>
      ),
    },
    { title: 'Agent', dataIndex: 'agent_id', key: 'agent_id', width: 150, ellipsis: true },
    {
      title: '源路径',
      dataIndex: 'sources',
      key: 'sources',
      render: (sources: string[]) => (
        <Tooltip title={sources.join('\n')}>
          <Space size={4} wrap>
            {sources.slice(0, 2).map((s) => (
              <Tag key={s} style={{ maxWidth: 180, overflow: 'hidden', textOverflow: 'ellipsis' }}>
                {s}
              </Tag>
            ))}
            {sources.length > 2 && <Tag>+{sources.length - 2}</Tag>}
          </Space>
        </Tooltip>
      ),
    },
    { title: '目标目录', dataIndex: 'dest_dir', key: 'dest_dir', ellipsis: true },
    { title: '计划', dataIndex: 'schedule', key: 'schedule', width: 110, render: scheduleLabel },
    {
      title: '保留',
      dataIndex: 'retention',
      key: 'retention',
      width: 70,
      render: (n: number) => (n > 0 ? `${n} 份` : '不限'),
    },
    { title: '状态', dataIndex: 'last_status', key: 'last_status', width: 90, render: statusTag },
    {
      title: '上次运行',
      dataIndex: 'last_run_at',
      key: 'last_run_at',
      width: 160,
      render: formatTime,
    },
    {
      title: '操作',
      key: 'actions',
      width: 260,
      render: (_, cfg) => (
        <Space size={0}>
          <PermGuard perm="backup:write">
            <Button
              type="link"
              size="small"
              icon={<CaretRightOutlined />}
              disabled={cfg.last_status === 'running'}
              onClick={() => runMutation.mutate(cfg.id)}
            >
              运行
            </Button>
          </PermGuard>
          <Button
            type="link"
            size="small"
            icon={<HistoryOutlined />}
            onClick={() => setHistoryFor(cfg)}
          >
            历史
          </Button>
          <Button
            type="link"
            size="small"
            icon={<FolderOpenOutlined />}
            onClick={() => setFilesFor(cfg)}
          >
            文件
          </Button>
          <PermGuard perm="backup:write">
            <Button type="link" size="small" icon={<EditOutlined />} onClick={() => openEdit(cfg)} />
            <Popconfirm
              title="删除备份配置？"
              description="运行历史将一并删除，备份文件不受影响"
              onConfirm={() => deleteMutation.mutate(cfg.id)}
            >
              <Button type="link" size="small" danger icon={<DeleteOutlined />} />
            </Popconfirm>
          </PermGuard>
        </Space>
      ),
    },
  ]

  const historyQuery = useQuery({
    queryKey: ['backup-runs', historyFor?.id],
    queryFn: () => api.getBackupConfigRuns(historyFor!.id),
    enabled: !!historyFor,
    refetchInterval: (query) =>
      query.state.data?.runs.some((r) => r.status === 'running') ? 5000 : false,
  })

  const filesQuery = useQuery({
    queryKey: ['backup-files', filesFor?.id],
    queryFn: () => api.getBackupFiles(filesFor!.id),
    enabled: !!filesFor,
  })

  const deleteFileMutation = useMutation({
    mutationFn: ({ configId, name }: { configId: number; name: string }) =>
      api.deleteBackupFile(configId, name),
    onSuccess: () => {
      message.success('备份文件已删除')
      if (filesFor) queryClient.invalidateQueries({ queryKey: ['backup-files', filesFor.id] })
    },
    onError: (err) => message.error(getApiErrorMessage(err, '删除备份文件失败')),
  })

  // 手动补传（M2 D24）：自动推送失败通知后的面板内动作；rclone 幂等可重复点击
  const [syncingRemote, setSyncingRemote] = useState<string | null>(null)
  const syncRemoteMutation = useMutation({
    mutationFn: ({ configId, name }: { configId: number; name: string }) => {
      setSyncingRemote(name)
      return api.syncBackupFileRemote(configId, name)
    },
    onSuccess: () => message.success('已同步到远端'),
    onError: (err) => message.error(getApiErrorMessage(err, '同步到远端失败')),
    onSettled: () => setSyncingRemote(null),
  })

  // 下载：axios blob 带 JWT（window.open 带不上 Authorization），createObjectURL 触发保存
  const [downloading, setDownloading] = useState<string | null>(null)
  const downloadFile = async (configId: number, name: string) => {
    setDownloading(name)
    try {
      const blob = await api.downloadBackupFile(configId, name)
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = name
      a.click()
      URL.revokeObjectURL(url)
    } catch (err) {
      message.error(getApiErrorMessage(err, '下载备份文件失败'))
    } finally {
      setDownloading(null)
    }
  }

  const openRestore = (cfg: BackupConfig, file: BackupFile) => {
    setRestoreTarget({ cfg, file })
    setRestoreTaskId(null)
    restoreForm.setFieldsValue({ destDir: '', confirmName: '' })
  }

  const submitRestore = async () => {
    if (!restoreTarget) return
    try {
      const values = await restoreForm.validateFields()
      setRestoring(true)
      setRestoreDestDir(values.destDir)
      const started = await api.restoreBackup(restoreTarget.cfg.id, {
        file: restoreTarget.file.name,
        dest_dir: values.destDir,
        confirm_name: values.confirmName,
      })
      message.success('恢复任务已下发')
      setRestoreTaskId(started.taskId)
    } catch (err) {
      if (err && typeof err === 'object' && 'errorFields' in err) return
      message.error(getApiErrorMessage(err, '触发恢复失败'))
    } finally {
      setRestoring(false)
    }
  }

  // 恢复任务状态轮询（有 taskId 时启用，终态自动停）
  const restoreTaskQuery = useQuery({
    queryKey: ['backup-restore-task', restoreTarget?.cfg.id, restoreTaskId],
    queryFn: () => api.getBackupTask(restoreTarget!.cfg.id, restoreTaskId!),
    enabled: !!restoreTarget && !!restoreTaskId,
    refetchInterval: (query) =>
      query.state.data?.status === 'running' ? 2000 : false,
  })
  const restoreTask: BackupTask | undefined = restoreTaskQuery.data

  const runColumns: ColumnsType<BackupRun> = [
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 90,
      render: (st: string) =>
        st === 'success' ? (
          <Tag color="success">成功</Tag>
        ) : st === 'running' ? (
          <Tag color="processing">运行中</Tag>
        ) : (
          <Tag color="error">{st === 'timeout' ? '超时' : '失败'}</Tag>
        ),
    },
    { title: '文件', dataIndex: 'file', key: 'file', ellipsis: true, render: (f: string) => f || '—' },
    { title: '大小', dataIndex: 'size', key: 'size', width: 90, render: (n: number) => (n ? formatBytes(n) : '—') },
    {
      title: '异地',
      dataIndex: 'remoteStatus',
      key: 'remoteStatus',
      width: 90,
      // 空 = 该次运行未启用异地推送（D20/D21：失败不改本地终态，仅独立提示）
      render: (st: string | undefined, run) => {
        if (!st) return <Typography.Text type="secondary">—</Typography.Text>
        if (st === 'ok') return <Tag color="success">已推送</Tag>
        return (
          <Tooltip title={run.remoteError || 'rclone 推送失败'}>
            <Tag color="error">推送失败</Tag>
          </Tooltip>
        )
      },
    },
    { title: '开始', dataIndex: 'startedAt', key: 'startedAt', width: 160, render: formatTime },
    { title: '结束', dataIndex: 'finishedAt', key: 'finishedAt', width: 160, render: formatTime },
    {
      title: '错误',
      dataIndex: 'error',
      key: 'error',
      ellipsis: true,
      render: (e: string | undefined) =>
        e ? (
          <Tooltip title={e}>
            <Typography.Text type="danger" style={{ fontSize: 12 }}>
              {e}
            </Typography.Text>
          </Tooltip>
        ) : (
          '—'
        ),
    },
  ]

  const fileColumns: ColumnsType<BackupFile> = [
    { title: '文件', dataIndex: 'name', key: 'name', ellipsis: true },
    { title: '大小', dataIndex: 'size', key: 'size', width: 100, render: formatBytes },
    {
      title: '修改时间',
      dataIndex: 'mtime',
      key: 'mtime',
      width: 160,
      render: (t: number) => (t ? dayjs.unix(t).format('YYYY-MM-DD HH:mm') : '—'),
    },
    {
      title: '操作',
      key: 'actions',
      width: filesFor?.remote_dest ? 250 : 190,
      render: (_, f) => (
        <Space size={0}>
          <Button
            type="link"
            size="small"
            icon={<DownloadOutlined />}
            loading={downloading === f.name}
            onClick={() => filesFor && downloadFile(filesFor.id, f.name)}
          >
            下载
          </Button>
          <PermGuard perm="backup:write">
            <Button
              type="link"
              size="small"
              icon={<UndoOutlined />}
              onClick={() => filesFor && openRestore(filesFor, f)}
            >
              恢复
            </Button>
            {filesFor?.remote_dest && (
              <Tooltip title={`同步到远端：${filesFor.remote_dest}`}>
                <Button
                  type="link"
                  size="small"
                  icon={<CloudUploadOutlined />}
                  loading={syncingRemote === f.name}
                  onClick={() => filesFor && syncRemoteMutation.mutate({ configId: filesFor.id, name: f.name })}
                >
                  补传
                </Button>
              </Tooltip>
            )}
            <Popconfirm
              title="删除该备份文件？"
              description="此操作不可恢复"
              onConfirm={() => filesFor && deleteFileMutation.mutate({ configId: filesFor.id, name: f.name })}
            >
              <Button type="link" size="small" danger icon={<DeleteOutlined />} />
            </Popconfirm>
          </PermGuard>
        </Space>
      ),
    },
  ]

  return (
    <Space direction="vertical" style={{ width: '100%' }} size={16}>
      <Card
        title="备份配置"
        extra={
          <PermGuard perm="backup:write">
            <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
              新建备份
            </Button>
          </PermGuard>
        }
      >
        {configs.length === 0 && !isLoading ? (
          <Empty description="尚未配置备份任务，点击右上角创建第一条配置" />
        ) : (
          <Table columns={configColumns} dataSource={configs} rowKey="id" loading={isLoading} pagination={false} />
        )}
        <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginTop: 8 }}>
          备份在 Agent 主机本地打包为 tar.gz 落到目标目录（可直接指向 NAS 挂载点），控制台只记录调度与历史；
          失败时会通过已配置的通知渠道发送 backup.failed 事件。配置「异地目标」后备份完成即自动经 rclone
          推送到远端，推送失败单独通知（backup.remote-failed），可在备份文件里手动补传。
        </Typography.Text>
      </Card>

      <Modal
        title={editing ? `编辑备份：${editing.name}` : '新建备份'}
        open={editorOpen}
        onCancel={() => setEditorOpen(false)}
        onOk={save}
        confirmLoading={saving}
        width={560}
        destroyOnClose
      >
        <Form form={form} layout="vertical">
          <Form.Item name="agentId" label="Agent" rules={[{ required: true, message: '请选择 Agent' }]}>
            <Select options={agentOptions} placeholder="选择执行备份的主机" showSearch />
          </Form.Item>
          <Form.Item
            name="name"
            label="备份名"
            rules={[
              { required: true, message: '请输入备份名' },
              { pattern: NAME_PATTERN, message: '小写字母/数字开头，可用 - 和 _，最长 64 字符' },
            ]}
            extra="用作备份文件名前缀，如 etc → etc-20260914-030000.tar.gz"
          >
            <Input placeholder="etc" />
          </Form.Item>
          <Form.Item
            name="sources"
            label="源路径"
            rules={[{ required: true, message: '至少一个绝对路径' }]}
            extra="回车添加；目录（Docker 卷、配置目录）或单个文件（如 SQLite 数据库）"
          >
            <Select mode="tags" placeholder="/opt/app/data" open={false} suffixIcon={null} />
          </Form.Item>
          <Form.Item
            name="destDir"
            label="目标目录"
            rules={[
              { required: true, message: '请输入目标目录' },
              { pattern: /^\//, message: '必须是 Agent 主机上的绝对路径' },
            ]}
            extra="建议指向第二块盘或 NAS 挂载点"
          >
            <Input placeholder="/mnt/backup" />
          </Form.Item>
          <Form.Item
            name="remoteDest"
            label="异地目标（可选）"
            rules={[
              {
                pattern: /^[A-Za-z0-9][A-Za-z0-9._-]*:[^\s]+$/,
                message: '格式：rclone 远端名:路径，如 my-s3:cockpit/backups',
              },
            ]}
            extra="填写后每次备份自动经 Agent 侧 rclone 推送到对象存储/网盘；远端凭据在 Agent 主机的 rclone.conf 中配置，控制台不经手。留空不启用"
          >
            <Input placeholder="my-s3:cockpit/backups" />
          </Form.Item>
          <Form.Item
            name="preHook"
            label="前置命令（可选）"
            extra="打包前在 Agent 主机经 sh 执行，用于数据库一致性快照（失败即中止本次备份，超时 5 分钟）。源路径填快照产物路径"
          >
            <Input.TextArea
              rows={2}
              placeholder={'sqlite3 /data/app.db ".backup /tmp/app.db.bak"'}
            />
          </Form.Item>
          <Form.Item name="scheduleType" label="计划" initialValue="daily">
            <Radio.Group
              options={[
                { value: 'manual', label: '手动' },
                { value: 'daily', label: '每日' },
                { value: 'every', label: '间隔' },
              ]}
              optionType="button"
            />
          </Form.Item>
          {formSelectedType === 'daily' && (
            <Form.Item name="dailyTime" label="每天执行时间" rules={[{ required: true, message: '请选择时间' }]}>
              <TimePicker format="HH:mm" style={{ width: 140 }} />
            </Form.Item>
          )}
          {formSelectedType === 'every' && (
            <Form.Item name="everyHours" label="执行间隔（小时）" rules={[{ required: true }]}>
              <InputNumber min={1} max={168} style={{ width: 140 }} />
            </Form.Item>
          )}
          <Form.Item
            name="retention"
            label="保留份数"
            initialValue={7}
            extra="超过后自动清理最旧的备份包，0 表示不清理"
          >
            <InputNumber min={0} max={365} style={{ width: 140 }} addonAfter="份" />
          </Form.Item>
          <Form.Item name="enabled" label="启用" valuePropName="checked" initialValue={true}>
            <Switch />
          </Form.Item>
        </Form>
      </Modal>

      <Drawer
        title={`运行历史：${historyFor?.name ?? ''}`}
        width={720}
        open={!!historyFor}
        onClose={() => setHistoryFor(null)}
      >
        {historyQuery.data?.runs.length ? (
          <Table
            columns={runColumns}
            dataSource={historyQuery.data.runs}
            rowKey="id"
            pagination={{ pageSize: 10 }}
            size="small"
          />
        ) : (
          <Empty description="暂无运行记录" />
        )}
      </Drawer>

      <Drawer
        title={`备份文件：${filesFor?.dest_dir ?? ''}`}
        width={720}
        open={!!filesFor}
        onClose={() => setFilesFor(null)}
      >
        {filesQuery.isLoading ? null : filesQuery.data?.files.length ? (
          <>
            <Alert
              type="info"
              showIcon
              style={{ marginBottom: 12 }}
              message={`共 ${filesQuery.data.files.length} 个备份包（删除操作直接作用于 Agent 主机上的文件）`}
            />
            <Table
              columns={fileColumns}
              dataSource={filesQuery.data.files}
              rowKey="name"
              pagination={{ pageSize: 10 }}
              size="small"
            />
          </>
        ) : (
          <Empty description="目标目录暂无备份包（目录不存在或为空）" />
        )}
      </Drawer>

      <Modal
        title={restoreTaskId ? `恢复进度：${restoreTarget?.file.name ?? ''}` : `恢复备份：${restoreTarget?.file.name ?? ''}`}
        open={!!restoreTarget}
        onCancel={() => {
          if (restoreTask?.status === 'running') {
            message.warning('恢复任务仍在运行，关闭窗口不影响任务执行')
          }
          setRestoreTarget(null)
        }}
        footer={
          restoreTaskId
            ? restoreTask?.status === 'running'
              ? [<Button key="close" onClick={() => setRestoreTarget(null)}>后台运行，关闭</Button>]
              : [<Button key="done" type="primary" onClick={() => setRestoreTarget(null)}>完成</Button>]
            : [
                <Button key="cancel" onClick={() => setRestoreTarget(null)}>取消</Button>,
                <Button key="ok" type="primary" danger loading={restoring} onClick={submitRestore}>
                  开始恢复
                </Button>,
              ]
        }
        width={560}
        destroyOnClose
      >
        {restoreTarget && !restoreTaskId && (
          <>
            <Alert
              type="warning"
              showIcon
              style={{ marginBottom: 16 }}
              message="恢复到独立目录，不会覆盖任何现有数据"
              description={`备份包：${restoreTarget.file.name}（${formatBytes(restoreTarget.file.size)}，${formatTime(restoreTarget.file.mtime)}）。
                解包目标是 Agent 主机上一个不存在或为空的目录，恢复完成后需要手动将文件迁移回原位。`}
            />
            <Form form={restoreForm} layout="vertical">
              <Form.Item
                name="destDir"
                label="恢复目标目录"
                rules={[
                  { required: true, message: '请输入恢复目标目录' },
                  { pattern: /^\//, message: '必须是 Agent 主机上的绝对路径' },
                ]}
                extra="必须是不存在或为空的目录（如 /tmp/restore-nginx），已存在且非空会被拒绝"
              >
                <Input placeholder="/tmp/restore-20260914" />
              </Form.Item>
              <Form.Item
                name="confirmName"
                label={`输入备份文件名确认（${restoreTarget.file.name}）`}
                rules={[
                  { required: true, message: '请输入备份文件名' },
                  {
                    validator: (_, v: string) =>
                      v === restoreTarget.file.name
                        ? Promise.resolve()
                        : Promise.reject(new Error('输入与备份文件名不一致')),
                  },
                ]}
              >
                <Input placeholder={restoreTarget.file.name} allowClear />
              </Form.Item>
            </Form>
          </>
        )}
        {restoreTarget && restoreTaskId && (
          <>
            <Space style={{ marginBottom: 12 }}>
              {restoreTask?.status === 'success' ? (
                <Tag color="success">恢复成功</Tag>
              ) : restoreTask?.status === 'failed' ? (
                <Tag color="error">恢复失败</Tag>
              ) : (
                <Tag color="processing">解包中…</Tag>
              )}
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                目标目录：{restoreDestDir}
              </Typography.Text>
            </Space>
            {restoreTask?.status === 'failed' && restoreTask.error && (
              <Alert type="error" showIcon style={{ marginBottom: 12 }} message={restoreTask.error} />
            )}
            <pre
              style={{
                margin: 0,
                padding: 12,
                maxHeight: 280,
                overflow: 'auto',
                fontSize: 12,
                lineHeight: 1.6,
                background: 'rgba(0,0,0,0.04)',
                borderRadius: 6,
                whiteSpace: 'pre-wrap',
                wordBreak: 'break-all',
              }}
            >
              {restoreTask?.log || '等待任务输出…'}
            </pre>
          </>
        )}
      </Modal>
      <ServerBackupCard />
    </Space>
  )
}

export default Backups
