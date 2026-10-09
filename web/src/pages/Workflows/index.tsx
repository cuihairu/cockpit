import { useMemo, useState } from 'react'
import { useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Button,
  Card,
  Descriptions,
  Drawer,
  Empty,
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
  Typography,
  message,
} from 'antd'
import {
  ArrowDownOutlined,
  ArrowUpOutlined,
  CaretRightOutlined,
  DeleteOutlined,
  EditOutlined,
  PlusOutlined,
  ReloadOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { api } from '@/services/api'
import { getJob } from '@/services/jobs'
import type { Job } from '@/services/jobs'
import {
  cancelWorkflowRun,
  createWorkflow,
  deleteWorkflow,
  getWorkflowRun,
  isRunTerminal,
  listWorkflowRuns,
  listWorkflows,
  runWorkflow,
  updateWorkflow,
} from '@/services/workflows'
import type { WorkflowDef, WorkflowRun, WorkflowRunStep, WorkflowStep } from '@/services/workflows'
import { getApiErrorMessage } from '@/utils/apiError'
import { PermGuard } from '@/components/PermGuard'
import dayjs from 'dayjs'

// Workflow 编排（docs/guide/workflow-design.md）：线性步骤链，步骤即真实
// Job。定义 CRUD + 触发 run；run 详情为步骤时间线（进行中 5s 轮询），
// running 可取消（W3：停止推进后续步骤）。

const RUN_STATUS_META: Record<string, { color: string; label: string }> = {
  running: { color: 'processing', label: '运行中' },
  success: { color: 'success', label: '成功' },
  failed: { color: 'error', label: '失败' },
  cancelled: { color: 'warning', label: '已取消' },
}

const StatusTag = ({ status }: { status: string }) => {
  const meta = RUN_STATUS_META[status] ?? { color: 'default', label: status }
  return <Tag color={meta.color}>{meta.label}</Tag>
}

const MAX_STEPS = 20

/** 编辑态步骤（表单字段平铺，提交时折回 API 形态） */
interface StepFormValue {
  name: string
  target: string
  command: string
  timeout_s?: number
  continue_on_error?: boolean
  retry?: number
}

const stepsToForm = (steps: WorkflowStep[]): StepFormValue[] =>
  steps.map((s) => ({
    name: s.parameters.name,
    target: s.target,
    command: s.parameters.command,
    timeout_s: s.parameters.timeout_s,
    continue_on_error: s.parameters.continue_on_error,
    retry: s.parameters.retry,
  }))

const formToSteps = (vals: StepFormValue[]): WorkflowStep[] =>
  vals.map((v) => ({
    type: 'agent.exec',
    target: v.target,
    parameters: {
      name: v.name,
      command: v.command,
      ...(v.timeout_s ? { timeout_s: v.timeout_s } : {}),
      ...(v.continue_on_error ? { continue_on_error: true } : {}),
      ...(v.retry ? { retry: v.retry } : {}),
    },
  }))

const Workflows = () => {
  const queryClient = useQueryClient()
  const [editorOpen, setEditorOpen] = useState(false)
  const [editing, setEditing] = useState<WorkflowDef | null>(null)
  const [saving, setSaving] = useState(false)
  const [runDetail, setRunDetail] = useState<string | null>(null)
  const [jobDetail, setJobDetail] = useState<Job | null>(null)
  const [steps, setSteps] = useState<StepFormValue[]>([])
  const [wfName, setWfName] = useState('')
  const [wfDesc, setWfDesc] = useState('')

  const { data: workflows, isLoading } = useQuery({
    queryKey: ['workflows'],
    queryFn: listWorkflows,
    refetchInterval: 15000,
  })

  const refresh = () => queryClient.invalidateQueries({ queryKey: ['workflows'] })

  const { data: agents } = useQuery({ queryKey: ['agents'], queryFn: () => api.getAgents() })

  // 目标下拉：仅在线 agent（离线 run 会被 server 503 拦截）
  const agentOptions = useMemo(
    () =>
      (agents ?? []).map((a) => ({
        value: a.id,
        disabled: a.status === 'offline',
        label: `${a.hostname || a.id}${a.status === 'offline' ? '（离线）' : ''}`,
      })),
    [agents],
  )

  // run 详情（进行中 5s 轮询、终态停轮询）
  const { data: run } = useQuery({
    queryKey: ['workflow-run', runDetail],
    queryFn: () => getWorkflowRun(runDetail as string),
    enabled: !!runDetail,
    refetchInterval: (q) => {
      const status = q.state.data?.status
      return status && isRunTerminal(status) ? false : 5000
    },
  })

  // 各定义最近一次 run 状态（列表列；个人规模定义少，逐个拉台账可接受）
  const runLists = useQueries({
    queries: (workflows ?? []).map((wf) => ({
      queryKey: ['workflow-runs', wf.id],
      queryFn: () => listWorkflowRuns(wf.id),
      refetchInterval: 15000,
    })),
  })
  const lastRunByWf = useMemo(() => {
    const m = new Map<string, WorkflowRun | undefined>()
    workflows?.forEach((wf, i) => {
      const runs = runLists[i]?.data
      m.set(wf.id, runs && runs.length > 0 ? runs[0] : undefined)
    })
    return m
  }, [workflows, runLists])

  const openCreate = () => {
    setEditing(null)
    setWfName('')
    setWfDesc('')
    setSteps([{ name: '', target: '', command: '' }])
    setEditorOpen(true)
  }

  const openEdit = (wf: WorkflowDef) => {
    setEditing(wf)
    setWfName(wf.name)
    setWfDesc(wf.description ?? '')
    setSteps(stepsToForm(wf.steps))
    setEditorOpen(true)
  }

  const submitSave = async () => {
    if (!wfName.trim()) {
      message.error('请输入 Workflow 名称')
      return
    }
    if (steps.length === 0 || steps.length > MAX_STEPS) {
      message.error(`步骤数须在 1-${MAX_STEPS} 之间`)
      return
    }
    const names = new Set<string>()
    for (const [i, s] of steps.entries()) {
      const where = `步骤 ${i + 1}`
      if (!s.name.trim()) {
        message.error(`${where}：请输入步骤名`)
        return
      }
      if (!s.target) {
        message.error(`${where}：请选择目标主机`)
        return
      }
      if (!s.command) {
        message.error(`${where}：请输入命令`)
        return
      }
      if (names.has(s.name.trim())) {
        message.error(`${where}：步骤名「${s.name.trim()}」重复`)
        return
      }
      names.add(s.name.trim())
    }
    setSaving(true)
    try {
      const payload = { name: wfName.trim(), description: wfDesc.trim() || undefined, steps: formToSteps(steps) }
      if (editing) {
        await updateWorkflow(editing.id, payload)
        message.success('Workflow 已更新')
      } else {
        await createWorkflow(payload)
        message.success('Workflow 已创建')
      }
      setEditorOpen(false)
      refresh()
    } catch (e) {
      message.error(getApiErrorMessage(e, '保存失败'))
    } finally {
      setSaving(false)
    }
  }

  const handleRun = async (wf: WorkflowDef) => {
    try {
      const r = await runWorkflow(wf.id)
      message.success('已触发运行')
      setRunDetail(r.id)
      refresh()
    } catch (e) {
      message.error(getApiErrorMessage(e, '运行失败'))
    }
  }

  const handleDelete = async (wf: WorkflowDef) => {
    try {
      await deleteWorkflow(wf.id)
      message.success('已删除')
      refresh()
    } catch (e) {
      message.error(getApiErrorMessage(e, '删除失败'))
    }
  }

  const handleCancelRun = async (runId: string) => {
    try {
      await cancelWorkflowRun(runId)
      message.success('已请求取消（当前在途步骤将自然结束）')
      queryClient.invalidateQueries({ queryKey: ['workflow-run', runId] })
    } catch (e) {
      message.error(getApiErrorMessage(e, '取消失败'))
    }
  }

  const openJob = async (id: string) => {
    try {
      setJobDetail(await getJob(id))
    } catch (e) {
      message.error(getApiErrorMessage(e, 'Job 读取失败'))
    }
  }

  const formatTime = (v?: string) => (v ? dayjs(v).format('MM-DD HH:mm:ss') : '-')

  // 步骤编辑器行内变更
  const updateStep = (idx: number, patch: Partial<StepFormValue>) =>
    setSteps((prev) => prev.map((s, i) => (i === idx ? { ...s, ...patch } : s)))

  const moveStep = (idx: number, dir: -1 | 1) =>
    setSteps((prev) => {
      const j = idx + dir
      if (j < 0 || j >= prev.length) return prev
      const next = [...prev]
      ;[next[idx], next[j]] = [next[j], next[idx]]
      return next
    })

  const columns: ColumnsType<WorkflowDef> = [
    { title: '名称', dataIndex: 'name', key: 'name', ellipsis: true },
    { title: '描述', dataIndex: 'description', key: 'description', ellipsis: true, render: (v: string) => v || '-' },
    { title: '步数', key: 'steps', width: 70, render: (_, r) => r.steps.length },
    {
      title: '最近 run',
      key: 'lastRun',
      width: 90,
      render: (_, r) => {
        const last = lastRunByWf.get(r.id)
        return last ? <StatusTag status={last.status} /> : '-'
      },
    },
    { title: '步骤概览', key: 'chain', ellipsis: true, render: (_, r) => (
      <Typography.Text type="secondary" ellipsis={{ tooltip: r.steps.map((s) => s.parameters.name).join(' → ') }}>
        {r.steps.map((s) => s.parameters.name).join(' → ')}
      </Typography.Text>
    ) },
    { title: '创建人', dataIndex: 'createdBy', key: 'createdBy', width: 100, ellipsis: true },
    { title: '更新时间', dataIndex: 'updatedAt', key: 'updatedAt', width: 140, render: formatTime },
    {
      title: '操作',
      key: 'actions',
      width: 220,
      render: (_, r) => (
        <Space>
          <PermGuard perm="workflows:write">
            <Button type="link" size="small" icon={<CaretRightOutlined />} onClick={() => handleRun(r)}>
              运行
            </Button>
          </PermGuard>
          <Button type="link" size="small" icon={<EditOutlined />} onClick={() => openEdit(r)}>
            编辑
          </Button>
          <PermGuard perm="workflows:write">
            <Popconfirm title={`删除「${r.name}」？（历史 run 台账保留）`} onConfirm={() => handleDelete(r)}>
              <Button type="link" size="small" danger icon={<DeleteOutlined />}>
                删除
              </Button>
            </Popconfirm>
          </PermGuard>
        </Space>
      ),
    },
  ]

  return (
    <div style={{ padding: 24 }}>
      <Card
        title="Workflow 编排"
        extra={
          <Space>
            <Button icon={<ReloadOutlined />} onClick={refresh} />
            <PermGuard perm="workflows:write">
              <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
                新建 Workflow
              </Button>
            </PermGuard>
          </Space>
        }
      >
        {workflows && workflows.length === 0 && !isLoading ? (
          <Empty description="暂无 Workflow" />
        ) : (
          <Table<WorkflowDef>
            rowKey="id"
            columns={columns}
            dataSource={workflows}
            loading={isLoading}
            size="middle"
            pagination={{ pageSize: 20, showSizeChanger: false }}
          />
        )}
      </Card>

      {/* 定义编辑抽屉：步骤编辑器（逐行 name/target/命令/超时/继续开关/重试，上下移） */}
      <Drawer
        title={editing ? `编辑 Workflow：${editing.name}` : '新建 Workflow'}
        open={editorOpen}
        onClose={() => setEditorOpen(false)}
        width={680}
        destroyOnHidden
        footer={
          <Space style={{ float: 'right' }}>
            <Button onClick={() => setEditorOpen(false)}>取消</Button>
            <Button type="primary" loading={saving} onClick={submitSave}>
              保存
            </Button>
          </Space>
        }
      >
        <Form layout="vertical">
          <Form.Item label="名称" required>
            <Input
              value={wfName}
              maxLength={64}
              onChange={(e) => setWfName(e.target.value)}
              placeholder="例如：停机 → 备份 → 升级 → 起机"
            />
          </Form.Item>
          <Form.Item label="描述">
            <Input value={wfDesc} onChange={(e) => setWfDesc(e.target.value)} placeholder="可选" />
          </Form.Item>
        </Form>
        <Typography.Title level={5}>步骤（线性链，自上而下执行）</Typography.Title>
        {steps.map((s, i) => (
          <Card
            key={i}
            size="small"
            style={{ marginBottom: 12 }}
            title={`步骤 ${i + 1}`}
            extra={
              <Space>
                <Button
                  size="small"
                  icon={<ArrowUpOutlined />}
                  disabled={i === 0}
                  onClick={() => moveStep(i, -1)}
                />
                <Button
                  size="small"
                  icon={<ArrowDownOutlined />}
                  disabled={i === steps.length - 1}
                  onClick={() => moveStep(i, 1)}
                />
                <Button
                  size="small"
                  danger
                  icon={<DeleteOutlined />}
                  disabled={steps.length <= 1}
                  onClick={() => setSteps((prev) => prev.filter((_, j) => j !== i))}
                />
              </Space>
            }
          >
            <Form layout="vertical">
              <Space style={{ display: 'flex' }} align="start">
                <Form.Item label="步骤名" style={{ flex: 1 }} required>
                  <Input
                    value={s.name}
                    maxLength={64}
                    placeholder="同一 Workflow 内唯一"
                    onChange={(e) => updateStep(i, { name: e.target.value })}
                  />
                </Form.Item>
                <Form.Item label="目标主机" style={{ width: 220 }} required>
                  <Select
                    value={s.target || undefined}
                    options={agentOptions}
                    placeholder="选择在线主机"
                    showSearch
                    optionFilterProp="label"
                    onChange={(v) => updateStep(i, { target: v })}
                  />
                </Form.Item>
              </Space>
              <Form.Item label="命令" required>
                <Input.TextArea
                  rows={2}
                  value={s.command}
                  spellCheck={false}
                  placeholder="shell 命令"
                  onChange={(e) => updateStep(i, { command: e.target.value })}
                />
              </Form.Item>
              <Space size={24} wrap>
                <Form.Item label="超时（秒）" style={{ marginBottom: 0 }}>
                  <InputNumber
                    min={1}
                    max={300}
                    value={s.timeout_s ?? 60}
                    onChange={(v) => updateStep(i, { timeout_s: v ?? undefined })}
                  />
                </Form.Item>
                <Form.Item label="失败重试" style={{ marginBottom: 0 }} tooltip="失败后重试次数（0-3，间隔 5s）">
                  <Select
                    value={s.retry ?? 0}
                    style={{ width: 80 }}
                    options={[0, 1, 2, 3].map((n) => ({ value: n, label: `${n} 次` }))}
                    onChange={(v) => updateStep(i, { retry: v })}
                  />
                </Form.Item>
                <Form.Item
                  label="失败继续"
                  style={{ marginBottom: 0 }}
                  tooltip="开启后本步失败仍继续后续步骤（run 结果不受本步影响）"
                  valuePropName="checked"
                >
                  <Switch
                    checked={!!s.continue_on_error}
                    onChange={(v) => updateStep(i, { continue_on_error: v || undefined })}
                  />
                </Form.Item>
              </Space>
            </Form>
          </Card>
        ))}
        <Button
          block
          icon={<PlusOutlined />}
          disabled={steps.length >= MAX_STEPS}
          onClick={() => setSteps((prev) => [...prev, { name: '', target: '', command: '' }])}
        >
          添加步骤{steps.length >= MAX_STEPS ? '（已达上限 20）' : ''}
        </Button>
      </Drawer>

      {/* run 详情：步骤时间线（每步状态 + 尝试数 + Job 输出入口） */}
      <Modal
        title={run ? `Run：${run.workflowName}` : 'Run'}
        open={!!runDetail}
        onCancel={() => setRunDetail(null)}
        width={720}
        footer={
          <Space>
            {run && run.status === 'running' && (
              <PermGuard perm="workflows:write">
                <Popconfirm title="取消后不再推进后续步骤，当前在途步骤将自然结束" onConfirm={() => handleCancelRun(run.id)}>
                  <Button danger>取消运行</Button>
                </Popconfirm>
              </PermGuard>
            )}
            <Button type="primary" onClick={() => setRunDetail(null)}>
              关闭
            </Button>
          </Space>
        }
      >
        {run && (
          <>
            <Descriptions
              size="small"
              column={3}
              items={[
                { key: 'status', label: '状态', children: <StatusTag status={run.status} /> },
                { key: 'actor', label: '触发人', children: run.actor || '-' },
                { key: 'created', label: '开始', children: formatTime(run.createdAt) },
                { key: 'finished', label: '结束', children: formatTime(run.finishedAt) },
              ]}
            />
            <Table<WorkflowRunStep>
              style={{ marginTop: 16 }}
              rowKey={(r) => r.name}
              dataSource={run.steps}
              pagination={false}
              size="small"
              columns={[
                { title: '步骤', dataIndex: 'name', key: 'name', ellipsis: true },
                { title: '目标', dataIndex: 'target', key: 'target', width: 110, ellipsis: true,
                  render: (v: string) => <Typography.Text code>{v}</Typography.Text> },
                { title: '状态', dataIndex: 'status', key: 'status', width: 90, render: (v: string) => <StatusTag status={v} /> },
                { title: '尝试', dataIndex: 'attempts', key: 'attempts', width: 60 },
                {
                  title: '输出',
                  key: 'job',
                  width: 80,
                  render: (_, r) =>
                    r.jobId ? (
                      <Button type="link" size="small" onClick={() => openJob(r.jobId as string)}>
                        查看
                      </Button>
                    ) : (
                      '-'
                    ),
                },
              ]}
            />
          </>
        )}
      </Modal>

      {/* 步骤 Job 详情：复用台账 Job 视角（状态/退出码/输出） */}
      <Modal
        title={jobDetail ? `Job ${jobDetail.id}` : ''}
        open={!!jobDetail}
        onCancel={() => setJobDetail(null)}
        width={720}
        footer={
          <Button type="primary" onClick={() => setJobDetail(null)}>
            关闭
          </Button>
        }
      >
        {jobDetail && (
          <>
            <Descriptions
              size="small"
              column={2}
              items={[
                { key: 'status', label: '状态', children: <StatusTag status={jobDetail.status} /> },
                { key: 'target', label: '目标', children: jobDetail.target },
                { key: 'actor', label: '执行人', children: jobDetail.actor || '-' },
                {
                  key: 'exit',
                  label: '退出码',
                  children: jobDetail.exitCode !== undefined ? jobDetail.exitCode : '-',
                },
                { key: 'created', label: '创建', children: formatTime(jobDetail.createdAt) },
                { key: 'finished', label: '完成', children: formatTime(jobDetail.finishedAt) },
              ]}
            />
            {jobDetail.error && (
              <Typography.Paragraph type="danger" style={{ marginTop: 12, marginBottom: 0 }}>
                {jobDetail.error}
              </Typography.Paragraph>
            )}
            <pre
              style={{
                background: 'rgba(128,128,128,0.12)',
                padding: 8,
                borderRadius: 4,
                whiteSpace: 'pre-wrap',
                wordBreak: 'break-all',
                maxHeight: 320,
                overflow: 'auto',
                margin: 0,
              }}
            >
              {jobDetail.output || '（无输出）'}
            </pre>
          </>
        )}
      </Modal>
    </div>
  )
}

export default Workflows
