import { useEffect, useRef, useState } from 'react'
import axios from 'axios'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Button,
  Collapse,
  Drawer,
  Empty,
  Input,
  Modal,
  Popconfirm,
  Select,
  Space,
  Table,
  Tabs,
  Tag,
  Timeline,
  Typography,
  message,
} from 'antd'
import {
  CloudDownloadOutlined,
  DeleteOutlined,
  EyeOutlined,
  PlayCircleOutlined,
  PoweroffOutlined,
  ReloadOutlined,
  SaveOutlined,
  SearchOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { api } from '@/services/api'
import type { StackService, StackView } from '@/types'
import { PermGuard } from '@/components/PermGuard'
import TerminalBlock from './TerminalBlock'
import {
  extractApiError,
  formatTimestamp,
  serviceStateColor,
  serviceStatusColor,
  tailOptions,
  taskActionLabel,
} from './shared'

// 等宽字体（compose 编辑器与日志一致）
const MONO_FONT = "'Cascadia Code', 'Fira Code', 'Consolas', monospace"

// 服务表列定义
const serviceColumns = (
  onLogs: (service: string) => void,
): ColumnsType<StackService> => [
  {
    title: '服务名',
    dataIndex: 'name',
    key: 'name',
    ellipsis: true,
  },
  {
    title: '镜像',
    dataIndex: 'image',
    key: 'image',
    ellipsis: true,
  },
  {
    title: '状态',
    dataIndex: 'state',
    key: 'state',
    width: 110,
    render: (state: string) => <Tag color={serviceStateColor[state] || 'default'}>{state}</Tag>,
  },
  {
    title: '详情',
    dataIndex: 'status',
    key: 'status',
    ellipsis: true,
    render: (status: string) => status || '-',
  },
  {
    title: '操作',
    key: 'actions',
    width: 90,
    render: (_, record) => (
      <Button type="link" size="small" icon={<EyeOutlined />} onClick={() => onLogs(record.name)}>
        日志
      </Button>
    ),
  },
]

// Stack 详情抽屉：服务、compose.yml、部署日志三个 Tab
// 注意：切换 Stack 时由父组件通过 key 重挂载本组件来重置内部状态
const StackDetail = ({
  open,
  stack,
  onClose,
}: {
  open: boolean
  stack: StackView | null
  onClose: () => void
}) => {
  const queryClient = useQueryClient()

  const [activeKey, setActiveKey] = useState('services')
  // 当前关注的异步任务（up / down / delete），仅在 mutation 回调中设置
  const [taskMeta, setTaskMeta] = useState<{ taskId: string; action: string } | null>(null)
  // 已处理过的任务（id:status），防止 effect 重复触发提示
  const handledTaskRef = useRef('')
  // 任务失败日志弹窗是否已被用户手动关闭
  const [failLogDismissed, setFailLogDismissed] = useState(false)
  // 通用深色终端弹窗内容（YAML 校验错误输出等，仅在事件回调中设置）
  const [terminal, setTerminal] = useState<{ title: string; content: string } | null>(null)

  // compose.yml / .env 编辑草稿（null = 未编辑，展示服务器内容）
  const [composeDraft, setComposeDraft] = useState<string | null>(null)
  const [envDraft, setEnvDraft] = useState<string | null>(null)

  // 部署日志查询条件（点「查询」后生效，null = 未查询）
  const [logService, setLogService] = useState<string | undefined>(undefined)
  const [logTail, setLogTail] = useState(200)
  const [logQuery, setLogQuery] = useState<{ service?: string; tail: number } | null>(null)

  const agentId = stack?.agentId ?? ''
  const stackName = stack?.name ?? ''

  // 服务列表（详情接口）
  const {
    data: detail,
    isFetching: detailLoading,
    refetch: refetchDetail,
  } = useQuery({
    queryKey: ['stacks', 'detail', agentId, stackName],
    queryFn: () => api.getStackDetail(agentId, stackName),
    enabled: open && !!stack?.online && !!agentId && !!stackName,
  })

  // 异步任务轮询：running 时每 2 秒一次，结束或出错后停止
  const taskQuery = useQuery({
    queryKey: ['stacks', 'task', agentId, taskMeta?.taskId],
    queryFn: () => api.getStackTask(agentId, taskMeta!.taskId),
    enabled: open && !!agentId && !!taskMeta,
    refetchInterval: (query) => {
      if (query.state.error) return false
      const t = query.state.data
      if (t && t.status !== 'running') return false
      return 2000
    },
  })
  const taskData = taskQuery.data

  // 任务是否仍在执行（由查询数据派生，错误时视为结束）
  const taskPending =
    !!taskMeta && !taskQuery.isError && (!taskData || taskData.status === 'running')

  // compose.yml / .env 内容（进入 Tab 时加载）
  const { data: composeData, isFetching: composeLoading } = useQuery({
    queryKey: ['stacks', 'compose', agentId, stackName],
    queryFn: () => api.getStackCompose(agentId, stackName),
    enabled: open && activeKey === 'compose' && !!agentId && !!stackName,
  })

  // 编辑器显示值：草稿优先，未编辑时展示服务器内容
  const composeValue = composeDraft ?? composeData?.compose ?? ''
  const envValue = envDraft ?? composeData?.env ?? ''

  // 部署日志
  const { data: logsData, isFetching: logsLoading, refetch: refetchLogs } = useQuery({
    queryKey: ['stacks', 'logs', agentId, stackName, logQuery],
    queryFn: () =>
      api.getStackLogs(agentId, stackName, { service: logQuery?.service, tail: logQuery?.tail }),
    enabled: open && activeKey === 'logs' && !!logQuery && !!agentId && !!stackName,
  })

  // 任务查询失败（如任务不存在）：提示一次，不重试（轮询已由 refetchInterval 停止）
  useEffect(() => {
    if (taskQuery.isError) {
      message.error(`查询任务状态失败: ${extractApiError(taskQuery.error)}`)
    }
  }, [taskQuery.isError, taskQuery.error])

  // 任务结束：一次性提示结果并刷新服务与列表（仅外部系统副作用，不触发 setState）
  useEffect(() => {
    if (!taskData || !taskMeta || taskData.status === 'running') return
    const handleKey = `${taskData.id}:${taskData.status}`
    if (handledTaskRef.current === handleKey) return
    handledTaskRef.current = handleKey

    const label = taskActionLabel[taskMeta.action] || taskMeta.action
    if (taskData.status === 'success') {
      message.success(`${label}任务执行成功`)
    } else {
      message.error(`${label}任务执行失败`)
    }
    void refetchDetail()
    void queryClient.invalidateQueries({ queryKey: ['stacks'] })
    // 删除任务成功后自动关闭抽屉
    if (taskMeta.action === 'delete' && taskData.status === 'success') {
      onClose()
    }
  }, [taskData, taskMeta, refetchDetail, queryClient, onClose])

  // 任务失败日志弹窗：完全由查询数据派生，关闭后不再自动弹出
  const failedTaskLog = taskMeta && taskData?.status === 'failed' ? taskData.log : ''
  const failedTaskTitle = taskMeta
    ? `${taskActionLabel[taskMeta.action] || taskMeta.action}任务日志 — ${stackName}`
    : ''

  // 启动 / 停止 / 重启 / 拉取镜像
  const actionMutation = useMutation({
    mutationFn: ({ action }: { action: 'up' | 'down' | 'restart' | 'pull' }) =>
      api.stackAction(agentId, stackName, action),
    onSuccess: (res, vars) => {
      message.info(`${taskActionLabel[vars.action]}任务已下发，正在执行...`)
      setFailLogDismissed(false)
      setTaskMeta({ taskId: res.taskId, action: vars.action })
    },
    onError: (err) => {
      if (axios.isAxiosError(err) && err.response?.status === 409) {
        message.warning('该 Stack 已有任务在执行，请稍后再试')
        return
      }
      message.error(`操作失败: ${extractApiError(err)}`)
    },
  })

  // 保存 compose.yml / .env
  const saveMutation = useMutation({
    mutationFn: (data: { compose: string; env?: string }) =>
      api.saveStackCompose(agentId, stackName, data),
    onSuccess: (res) => {
      message.success(res.created ? 'Stack 创建成功' : '保存成功')
      // 回到展示服务器内容的状态，并刷新列表与服务
      setComposeDraft(null)
      setEnvDraft(null)
      void queryClient.invalidateQueries({ queryKey: ['stacks'] })
    },
    onError: (err) => {
      if (axios.isAxiosError(err) && err.response?.status === 502) {
        // YAML 校验失败：展示 docker compose config 的错误输出（可能多行）
        message.error('YAML 校验失败，请检查 compose.yml 内容')
        setTerminal({
          title: 'YAML 校验失败 — docker compose config 错误输出',
          content: extractApiError(err),
        })
        return
      }
      message.error(`保存失败: ${extractApiError(err)}`)
    },
  })

  // 删除 Stack（异步：先 down 再删目录）
  const deleteMutation = useMutation({
    mutationFn: () => api.deleteStack(agentId, stackName),
    onSuccess: (res) => {
      message.info('删除任务已下发，正在执行...')
      setFailLogDismissed(false)
      setTaskMeta({ taskId: res.taskId, action: 'delete' })
    },
    onError: (err) => {
      if (axios.isAxiosError(err) && err.response?.status === 409) {
        message.warning('该 Stack 已有任务在执行，请稍后再试')
        return
      }
      message.error(`删除失败: ${extractApiError(err)}`)
    },
  })

  const handleSaveCompose = () => {
    if (!composeValue.trim()) {
      message.warning('compose.yml 内容不能为空')
      return
    }
    saveMutation.mutate({ compose: composeValue, env: envValue })
  }

  // 从服务表跳转到部署日志并按该服务过滤
  const openServiceLog = (service: string) => {
    setLogService(service)
    setActiveKey('logs')
    setLogQuery({ service, tail: logTail })
  }

  const running = detail?.running ?? stack?.running ?? 0
  const total = detail?.total ?? stack?.total ?? 0

  // 部署历史（M1.5）：server 侧记录，进入 Tab 或任务结束后刷新
  const { data: historyData, isFetching: historyLoading, refetch: refetchHistory } = useQuery({
    queryKey: ['stacks', 'history', agentId, stackName],
    queryFn: () => api.getStackHistory(agentId, stackName),
    enabled: open && !!agentId && !!stackName && activeKey === 'history',
  })
  useEffect(() => {
    if (activeKey === 'history') void refetchHistory()
  }, [activeKey, taskData?.status, refetchHistory])

  const historyItems = (historyData?.deployments ?? []).map((d) => {
    const label = taskActionLabel[d.action] || d.action
    const dot =
      d.status === 'success'
        ? 'green'
        : d.status === 'failed'
          ? 'red'
          : 'blue'
    const duration =
      d.finishedAt > 0 ? `${d.finishedAt - d.startedAt}s` : '进行中'
    return {
      color: dot,
      children: (
        <Space size={8} wrap>
          <Tag color={dot === 'blue' ? 'processing' : dot}>{label}</Tag>
          <Typography.Text style={{ fontSize: 12 }}>{formatTimestamp(d.startedAt)}</Typography.Text>
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            {d.status === 'success' ? '成功' : d.status === 'failed' ? '失败' : '执行中'} · {duration}
          </Typography.Text>
        </Space>
      ),
    }
  })

  const serviceOptions = (detail?.services ?? stack?.services ?? []).map((s) => ({
    value: s.name,
    label: s.name,
  }))

  const tabItems = [
    {
      key: 'services',
      label: '服务',
      children: (
        <>
          <Space style={{ marginBottom: 16 }} wrap>
            <PermGuard perm="stack:write">
              <Button
                type="primary"
                icon={<PlayCircleOutlined />}
                disabled={taskPending}
                loading={actionMutation.isPending && actionMutation.variables?.action === 'up'}
                onClick={() => actionMutation.mutate({ action: 'up' })}
              >
                启动
              </Button>
              <Button
                icon={<PoweroffOutlined />}
                disabled={taskPending}
                loading={actionMutation.isPending && actionMutation.variables?.action === 'down'}
                onClick={() => actionMutation.mutate({ action: 'down' })}
              >
                停止
              </Button>
              <Button
                icon={<ReloadOutlined />}
                disabled={taskPending || running === 0}
                loading={actionMutation.isPending && actionMutation.variables?.action === 'restart'}
                onClick={() => actionMutation.mutate({ action: 'restart' })}
              >
                重启
              </Button>
              <Button
                icon={<CloudDownloadOutlined />}
                disabled={taskPending}
                loading={actionMutation.isPending && actionMutation.variables?.action === 'pull'}
                onClick={() => actionMutation.mutate({ action: 'pull' })}
              >
                拉取镜像
              </Button>
            </PermGuard>
            <Button
              icon={<ReloadOutlined />}
              loading={detailLoading}
              onClick={() => void refetchDetail()}
            >
              刷新
            </Button>
            {total > 0 && (
              <Tag color={serviceStatusColor(running, total)}>
                {running}/{total} 运行中
              </Tag>
            )}
            {taskPending && <Tag color="processing">任务执行中...</Tag>}
          </Space>
          <Table
            columns={serviceColumns(openServiceLog)}
            dataSource={detail?.services ?? []}
            rowKey="name"
            loading={detailLoading || taskPending}
            pagination={false}
            size="middle"
          />
        </>
      ),
    },
    {
      key: 'compose',
      label: 'compose.yml',
      children: (
        <Space direction="vertical" style={{ width: '100%' }} size={12}>
          <Space style={{ justifyContent: 'space-between', width: '100%' }} wrap>
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              文件：{composeData?.composeFile || '-'}
              {composeData?.modifiedAt ? ` · 修改于 ${formatTimestamp(composeData.modifiedAt)}` : ''}
            </Typography.Text>
            <PermGuard perm="stack:write">
              <Button
                type="primary"
                icon={<SaveOutlined />}
                loading={saveMutation.isPending}
                onClick={handleSaveCompose}
              >
                保存
              </Button>
            </PermGuard>
          </Space>
          <Input.TextArea
            value={composeValue}
            onChange={(e) => setComposeDraft(e.target.value)}
            placeholder={composeLoading ? '正在加载...' : '输入 compose.yml 内容'}
            rows={16}
            styles={{
              textarea: {
                background: '#1e1e1e',
                color: '#d4d4d4',
                fontFamily: MONO_FONT,
                fontSize: 12,
              },
            }}
          />
          <Collapse
            ghost
            items={[
              {
                key: 'env',
                label: '.env 环境变量',
                extra: (
                  <Typography.Text type="warning" style={{ fontSize: 12 }}>
                    敏感信息，保存后不会出现在审计日志
                  </Typography.Text>
                ),
                children: (
                  <Input.TextArea
                    value={envValue}
                    onChange={(e) => setEnvDraft(e.target.value)}
                    placeholder="KEY=value（每行一条）"
                    rows={6}
                    styles={{
                      textarea: {
                        background: '#1e1e1e',
                        color: '#d4d4d4',
                        fontFamily: MONO_FONT,
                        fontSize: 12,
                      },
                    }}
                  />
                ),
              },
            ]}
          />
        </Space>
      ),
    },
    {
      key: 'logs',
      label: '部署日志',
      children: (
        <Space direction="vertical" style={{ width: '100%' }} size={12}>
          <Space wrap>
            <Select
              style={{ width: 200 }}
              placeholder="全部服务"
              allowClear
              value={logService}
              onChange={setLogService}
              options={serviceOptions}
            />
            <Select style={{ width: 110 }} value={logTail} onChange={setLogTail} options={tailOptions} />
            <Button
              type="primary"
              icon={<SearchOutlined />}
              onClick={() => setLogQuery({ service: logService, tail: logTail })}
            >
              查询
            </Button>
            <Button
              icon={<ReloadOutlined />}
              loading={logsLoading}
              onClick={() => void refetchLogs()}
              disabled={!logQuery}
            >
              刷新
            </Button>
          </Space>
          <TerminalBlock content={logsData?.logs ?? ''} loading={logsLoading} maxHeight={520} />
        </Space>
      ),
    },
    {
      key: 'history',
      label: '历史',
      children: historyItems.length > 0 ? (
        <Timeline items={historyItems} />
      ) : (
        <Empty
          description={
            historyLoading ? '正在加载部署历史...' : '暂无部署记录，启动或停止 Stack 后这里会显示时间线'
          }
        />
      ),
    },
  ]

  return (
    <Drawer
      title={
        <Space size={8} wrap>
          <span>Stack 详情 — {stackName}</span>
          <Tag color={stack?.online ? 'success' : 'default'}>{stack?.online ? '在线' : '离线'}</Tag>
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            Agent：{stack?.agentName}
          </Typography.Text>
        </Space>
      }
      open={open}
      onClose={onClose}
      width={840}
      destroyOnHidden
      footer={
        <div style={{ display: 'flex', justifyContent: 'space-between' }}>
          <PermGuard perm="stack:write">
            <Popconfirm
              title="删除 Stack"
              description={`确定要删除「${stackName}」吗？将停止并移除全部服务，且删除 Stack 目录，不可恢复。`}
              okText="删除"
              okButtonProps={{ danger: true }}
              cancelText="取消"
              onConfirm={() => deleteMutation.mutate()}
              disabled={taskPending}
            >
              <Button danger icon={<DeleteOutlined />} disabled={taskPending}>
                删除 Stack
              </Button>
            </Popconfirm>
          </PermGuard>
          <Button onClick={onClose}>关闭</Button>
        </div>
      }
    >
      <Tabs activeKey={activeKey} onChange={setActiveKey} items={tabItems} />

      {/* 任务失败日志弹窗（深色终端风格） */}
      <Modal
        title={failedTaskTitle}
        open={!!failedTaskLog && !failLogDismissed}
        onCancel={() => setFailLogDismissed(true)}
        footer={null}
        width={900}
        style={{ top: 50 }}
      >
        <TerminalBlock content={failedTaskLog} maxHeight={480} />
      </Modal>

      {/* 深色终端弹窗：YAML 校验错误输出等 */}
      <Modal
        title={terminal?.title}
        open={!!terminal}
        onCancel={() => setTerminal(null)}
        footer={null}
        width={900}
        style={{ top: 50 }}
      >
        <TerminalBlock content={terminal?.content ?? ''} maxHeight={480} />
      </Modal>
    </Drawer>
  )
}

export default StackDetail
