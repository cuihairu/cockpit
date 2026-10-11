import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Button,
  Card,
  Popconfirm,
  Select,
  Space,
  Table,
  Tag,
  Tooltip,
  Typography,
  message,
} from 'antd'
import { CheckOutlined, CloseOutlined, ReloadOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { approveApproval, listApprovals, rejectApproval } from '@/services/approvals'
import type { Approval, ApprovalStatus } from '@/services/approvals'
import { getApiErrorMessage } from '@/utils/apiError'
import { PermGuard } from '@/components/PermGuard'
import dayjs from 'dayjs'

// 审批门（docs/guide/workflow-design.md M2d）：高危 Job 提交先落审批，批准
// 后才派发。等待态在 server 库里（重启不丢待批）；过期未决只能重新提交。

const STATUS_META: Record<ApprovalStatus, { color: string; label: string }> = {
  pending: { color: 'gold', label: '待批' },
  approved: { color: 'green', label: '已批准' },
  rejected: { color: 'red', label: '已驳回' },
  expired: { color: 'default', label: '已过期' },
}

const ApprovalStatusTag = ({ status }: { status: ApprovalStatus }) => {
  const meta = STATUS_META[status] ?? { color: 'default', label: status }
  return <Tag color={meta.color}>{meta.label}</Tag>
}

const Approvals = () => {
  const queryClient = useQueryClient()
  const [statusFilter, setStatusFilter] = useState<ApprovalStatus | undefined>()
  const [acting, setActing] = useState(false)

  const { data: approvals, isLoading } = useQuery({
    queryKey: ['approvals', statusFilter],
    queryFn: () => listApprovals(statusFilter),
    refetchInterval: 5000,
  })
  const refresh = () => queryClient.invalidateQueries({ queryKey: ['approvals'] })

  const decide = async (id: string, action: 'approve' | 'reject') => {
    setActing(true)
    try {
      await (action === 'approve' ? approveApproval(id) : rejectApproval(id))
      message.success(action === 'approve' ? '已批准，任务开始执行' : '已驳回')
      refresh()
    } catch (e) {
      message.error(getApiErrorMessage(e, action === 'approve' ? '批准失败' : '驳回失败'))
    } finally {
      setActing(false)
    }
  }

  const formatTime = (v?: string) => (v ? dayjs(v).format('MM-DD HH:mm:ss') : '-')

  const columns: ColumnsType<Approval> = [
    {
      title: '提交时间',
      dataIndex: 'createdAt',
      key: 'createdAt',
      width: 140,
      render: formatTime,
    },
    {
      title: '目标',
      dataIndex: 'target',
      key: 'target',
      width: 140,
      ellipsis: true,
      render: (v: string) => <Typography.Text code>{v}</Typography.Text>,
    },
    {
      title: '命令',
      key: 'summary',
      ellipsis: true,
      render: (_, r) => (
        <Typography.Text code ellipsis={{ tooltip: r.summary }}>
          {r.summary || '-'}
        </Typography.Text>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 90,
      render: (s: ApprovalStatus) => <ApprovalStatusTag status={s} />,
    },
    {
      title: '提交人',
      dataIndex: 'actor',
      key: 'actor',
      width: 100,
    },
    {
      title: '决策',
      key: 'decision',
      width: 170,
      render: (_, r) =>
        r.status === 'pending' ? (
          <span>-</span>
        ) : (
          <span>
            {r.decidedBy || '-'}
            <br />
            <Typography.Text type="secondary">{formatTime(r.decidedAt)}</Typography.Text>
          </span>
        ),
    },
    {
      title: '到期',
      dataIndex: 'expiresAt',
      key: 'expiresAt',
      width: 130,
      render: formatTime,
    },
    {
      title: '操作',
      key: 'action',
      width: 170,
      render: (_, r) =>
        r.status !== 'pending' ? (
          r.jobId ? (
            <Typography.Text code copyable={{ text: r.jobId }}>
              {r.jobId.slice(0, 8)}
            </Typography.Text>
          ) : (
            <span>-</span>
          )
        ) : (
          <PermGuard perm="approvals:write">
            <Space>
              <Popconfirm
                title="批准执行？"
                description={<Typography.Text code>{r.summary}</Typography.Text>}
                onConfirm={() => decide(r.id, 'approve')}
              >
                <Button size="small" type="primary" icon={<CheckOutlined />} loading={acting}>
                  批准
                </Button>
              </Popconfirm>
              <Popconfirm title="驳回该申请？" onConfirm={() => decide(r.id, 'reject')}>
                <Button size="small" danger icon={<CloseOutlined />} loading={acting}>
                  驳回
                </Button>
              </Popconfirm>
            </Space>
          </PermGuard>
        ),
    },
  ]

  return (
    <Card
      title="审批门"
      extra={
        <Space>
          <Tooltip title="过期未决的申请只能重新提交">
            <Select
              allowClear
              placeholder="全部状态"
              style={{ width: 130 }}
              value={statusFilter}
              onChange={(v) => setStatusFilter(v)}
              options={[
                { value: 'pending', label: '待批' },
                { value: 'approved', label: '已批准' },
                { value: 'rejected', label: '已驳回' },
                { value: 'expired', label: '已过期' },
              ]}
            />
          </Tooltip>
          <Button icon={<ReloadOutlined />} onClick={refresh} />
        </Space>
      }
    >
      <Table
        rowKey="id"
        loading={isLoading}
        columns={columns}
        dataSource={approvals ?? []}
        pagination={false}
        size="middle"
      />
    </Card>
  )
}

export default Approvals
