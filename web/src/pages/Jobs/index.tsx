import { useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Button,
  Card,
  Descriptions,
  Empty,
  Form,
  Input,
  InputNumber,
  Modal,
  Select,
  Space,
  Table,
  Tag,
  Typography,
  message,
} from 'antd'
import { PlayCircleOutlined, ReloadOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { api } from '@/services/api'
import { createJob, listJobs } from '@/services/jobs'
import type { Job, JobStatus } from '@/services/jobs'
import { getApiErrorMessage } from '@/utils/apiError'
import { PermGuard } from '@/components/PermGuard'
import dayjs from 'dayjs'

const STATUS_META: Record<JobStatus, { color: string; label: string }> = {
  pending: { color: 'default', label: '排队中' },
  running: { color: 'processing', label: '执行中' },
  success: { color: 'success', label: '成功' },
  failed: { color: 'error', label: '失败' },
}

const JobStatusTag = ({ status }: { status: JobStatus }) => {
  const meta = STATUS_META[status] ?? { color: 'default', label: status }
  return <Tag color={meta.color}>{meta.label}</Tag>
}

// Job 执行（docs/guide/jobs-design.md）：统一执行台账。创建即执行
// （同步 RPC 下发，返回即终态）；列表为全机 Job 倒序台账。
const Jobs = () => {
  const queryClient = useQueryClient()
  const [createOpen, setCreateOpen] = useState(false)
  const [creating, setCreating] = useState(false)
  const [detail, setDetail] = useState<Job | null>(null)

  const { data: jobs, isLoading } = useQuery({
    queryKey: ['jobs'],
    queryFn: listJobs,
    refetchInterval: 15000,
  })

  const refresh = () => queryClient.invalidateQueries({ queryKey: ['jobs'] })

  const [form] = Form.useForm()

  const { data: agents } = useQuery({ queryKey: ['agents'], queryFn: () => api.getAgents() })

  // 目标下拉：仅在线 agent（离线创建会被 server 503 拦截）
  const agentOptions = useMemo(
    () =>
      (agents ?? []).map((a) => ({
        value: a.id,
        disabled: a.status === 'offline',
        label: `${a.hostname || a.id}${a.status === 'offline' ? '（离线）' : ''}`,
      })),
    [agents],
  )

  const openCreate = () => {
    form.resetFields()
    setCreateOpen(true)
  }

  const submitCreate = async () => {
    const raw = await form.validateFields().catch(() => undefined)
    if (!raw) return
    setCreating(true)
    try {
      const params: { command: string; timeout_s?: number } = { command: raw.command }
      if (raw.timeout_s) params.timeout_s = raw.timeout_s
      const job = await createJob({ type: 'agent.exec', target: raw.target, parameters: params })
      message.success(`Job 已执行（${STATUS_META[job.status]?.label ?? job.status}）`)
      setCreateOpen(false)
      refresh()
    } catch (e) {
      message.error(getApiErrorMessage(e, 'Job 创建失败'))
    } finally {
      setCreating(false)
    }
  }

  const formatTime = (v?: string) => (v ? dayjs(v).format('MM-DD HH:mm:ss') : '-')

  const columns: ColumnsType<Job> = [
    {
      title: '时间',
      dataIndex: 'createdAt',
      key: 'createdAt',
      width: 130,
      render: formatTime,
    },
    {
      title: '目标',
      dataIndex: 'target',
      key: 'target',
      width: 150,
      ellipsis: true,
      render: (v: string) => <Typography.Text code>{v}</Typography.Text>,
    },
    {
      title: '类型',
      dataIndex: 'type',
      key: 'type',
      width: 110,
      render: (v: string) => <Typography.Text code>{v}</Typography.Text>,
    },
    {
      title: '命令',
      key: 'command',
      ellipsis: true,
      render: (_, r) => (
        <Typography.Text code ellipsis={{ tooltip: r.parameters?.command }}>
          {r.parameters?.command ?? '-'}
        </Typography.Text>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 90,
      render: (v: JobStatus) => <JobStatusTag status={v} />,
    },
    {
      title: '结果',
      key: 'result',
      width: 90,
      render: (_, r) =>
        r.status === 'success' || r.status === 'failed' ? (
          <Button type="link" size="small" onClick={() => setDetail(r)}>
            查看
          </Button>
        ) : (
          '-'
        ),
    },
    { title: '执行人', dataIndex: 'actor', key: 'actor', width: 110, ellipsis: true },
    {
      title: '耗时',
      key: 'duration',
      width: 90,
      render: (_, r) => {
        if (!r.startedAt || !r.finishedAt) return '-'
        const ms = dayjs(r.finishedAt).diff(dayjs(r.startedAt))
        return ms >= 1000 ? `${(ms / 1000).toFixed(1)}s` : `${ms}ms`
      },
    },
  ]

  return (
    <div style={{ padding: 24 }}>
      <Card
        title="Job 执行"
        extra={
          <Space>
            <Button icon={<ReloadOutlined />} onClick={refresh} />
            <PermGuard perm="jobs:write">
              <Button type="primary" icon={<PlayCircleOutlined />} onClick={openCreate}>
                执行命令
              </Button>
            </PermGuard>
          </Space>
        }
      >
        {jobs && jobs.length === 0 && !isLoading ? (
          <Empty description="暂无执行记录" />
        ) : (
          <Table<Job>
            rowKey="id"
            columns={columns}
            dataSource={jobs}
            loading={isLoading}
            size="middle"
            pagination={{ pageSize: 20, showSizeChanger: false }}
          />
        )}
      </Card>

      <Modal
        title="执行命令"
        open={createOpen}
        onCancel={() => setCreateOpen(false)}
        onOk={submitCreate}
        confirmLoading={creating}
        okText="执行"
        destroyOnHidden
      >
        <Form form={form} layout="vertical" initialValues={{ timeout_s: 60 }}>
          <Form.Item
            name="target"
            label="目标主机"
            rules={[{ required: true, message: '请选择目标主机' }]}
          >
            <Select options={agentOptions} placeholder="选择在线主机" showSearch optionFilterProp="label" />
          </Form.Item>
          <Form.Item
            name="command"
            label="命令"
            rules={[
              { required: true, message: '请输入命令' },
              { max: 16 * 1024, message: '命令过长（上限 16KB）' },
            ]}
          >
            <Input.TextArea
              rows={4}
              placeholder="shell 命令，例如：uptime && df -h"
              spellCheck={false}
            />
          </Form.Item>
          <Form.Item
            name="timeout_s"
            label="超时（秒）"
            tooltip="超时未结束将被强制终止；1-300 秒，缺省 60"
            rules={[{ type: 'number', min: 1, max: 300, message: '范围 1-300 秒' }]}
          >
            <InputNumber min={1} max={300} style={{ width: 160 }} />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={detail ? `Job ${detail.id}` : ''}
        open={!!detail}
        onCancel={() => setDetail(null)}
        footer={
          <Button type="primary" onClick={() => setDetail(null)}>
            关闭
          </Button>
        }
        width={720}
      >
        {detail && (
          <>
            <Descriptions
              size="small"
              column={2}
              items={[
                { key: 'status', label: '状态', children: <JobStatusTag status={detail.status} /> },
                { key: 'target', label: '目标', children: detail.target },
                { key: 'actor', label: '执行人', children: detail.actor || '-' },
                {
                  key: 'exit',
                  label: '退出码',
                  children: detail.exitCode !== undefined ? detail.exitCode : '-',
                },
                { key: 'created', label: '创建', children: formatTime(detail.createdAt) },
                { key: 'finished', label: '完成', children: formatTime(detail.finishedAt) },
              ]}
            />
            {detail.error && (
              <Typography.Paragraph type="danger" style={{ marginTop: 12, marginBottom: 0 }}>
                {detail.error}
              </Typography.Paragraph>
            )}
            {detail.parameters?.command !== undefined && (
              <Typography.Paragraph>
                <pre
                  style={{
                    background: 'rgba(128,128,128,0.12)',
                    padding: 8,
                    borderRadius: 4,
                    whiteSpace: 'pre-wrap',
                    wordBreak: 'break-all',
                    maxHeight: 120,
                    overflow: 'auto',
                  }}
                >
                  {String(detail.parameters.command)}
                </pre>
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
              {detail.output || '（无输出）'}
            </pre>
          </>
        )}
      </Modal>
    </div>
  )
}

export default Jobs
