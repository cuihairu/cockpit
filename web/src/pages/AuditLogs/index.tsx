import { useState, useEffect } from 'react'
import { Card, Table, Tag, Button, Space, Input, Select, DatePicker, Statistic, Row, Col } from 'antd'
import {
  ReloadOutlined,
  SearchOutlined,
  DownloadOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { RangePickerProps } from 'antd/es/date-picker'
import dayjs from 'dayjs'
import { logger } from '@/utils/logger'
import './index.less'

const { RangePicker } = DatePicker

interface AuditLog {
  id: number
  user_id: number
  username: string
  action: string
  resource: string
  resource_id: string
  details: string
  ip: string
  user_agent: string
  status: string
  created_at: string
}

interface AuditLogStats {
  total_logs: number
  today_logs: number
  failed_logs: number
  by_action: Record<string, number>
  by_resource: Record<string, number>
  by_user: Record<string, number>
}

const ACTION_MAP: Record<string, { text: string; color: string }> = {
  login: { text: '登录', color: 'blue' },
  logout: { text: '登出', color: 'default' },
  create: { text: '创建', color: 'green' },
  update: { text: '更新', color: 'orange' },
  delete: { text: '删除', color: 'red' },
  view: { text: '查看', color: 'default' },
  export: { text: '导出', color: 'purple' },
  import: { text: '导入', color: 'cyan' },
  start: { text: '启动', color: 'green' },
  stop: { text: '停止', color: 'orange' },
}

const RESOURCE_MAP: Record<string, string> = {
  user: '用户',
  agent: 'Agent',
  domain: '域名',
  certificate: '证书',
  service: '服务',
  compute: '计算实例',
  gateway: '网关',
  storage: '存储',
  settings: '设置',
}

const AuditLogs = () => {
  const [loading, setLoading] = useState(false)
  const [logs, setLogs] = useState<AuditLog[]>([])
  const [stats, setStats] = useState<AuditLogStats | null>(null)
  const [pagination, setPagination] = useState({ current: 1, pageSize: 20, total: 0 })
  const [filters, setFilters] = useState<{
    action?: string
    resource?: string
    username?: string
    status?: string
    start_time?: string
    end_time?: string
  }>({})

  const fetchLogs = async (page = pagination.current, pageSize = pagination.pageSize) => {
    setLoading(true)
    try {
      const params = new URLSearchParams({
        page: page.toString(),
        page_size: pageSize.toString(),
        ...filters,
      })
      const response = await fetch(`/api/admin/audit/logs?${params}`)
      const data = await response.json()
      setLogs(data.data || [])
      setPagination({
        current: page,
        pageSize: pageSize,
        total: data.pagination?.total || 0,
      })
    } finally {
      setLoading(false)
    }
  }

  const fetchStats = async () => {
    try {
      const response = await fetch('/api/admin/audit/stats')
      const data = await response.json()
      setStats(data)
    } catch (error) {
      logger.error('Failed to fetch stats:', error)
    }
  }

  useEffect(() => {
    fetchLogs()
    fetchStats()
  }, [filters])

  const handleDateRangeChange: RangePickerProps['onChange'] = (dates) => {
    if (dates && dates[0] && dates[1]) {
      setFilters({
        ...filters,
        start_time: dates[0].startOf('day').toISOString(),
        end_time: dates[1].endOf('day').toISOString(),
      })
    } else {
      const { start_time, end_time, ...rest } = filters
      setFilters(rest)
    }
  }

  const columns: ColumnsType<AuditLog> = [
    {
      title: 'ID',
      dataIndex: 'id',
      key: 'id',
      width: 80,
    },
    {
      title: '时间',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 180,
      render: (date: string) => {
        const d = new Date(date)
        return (
          <Space direction="vertical" size="small">
            <span>{dayjs(d).format('YYYY-MM-DD')}</span>
            <span style={{ color: '#86909C', fontSize: 12 }}>{dayjs(d).format('HH:mm:ss')}</span>
          </Space>
        )
      },
      sorter: true,
    },
    {
      title: '用户',
      dataIndex: 'username',
      key: 'username',
      width: 120,
    },
    {
      title: '操作',
      dataIndex: 'action',
      key: 'action',
      width: 100,
      render: (action: string) => {
        const config = ACTION_MAP[action] || { text: action, color: 'default' }
        return <Tag color={config.color}>{config.text}</Tag>
      },
    },
    {
      title: '资源',
      dataIndex: 'resource',
      key: 'resource',
      width: 100,
      render: (resource: string) => RESOURCE_MAP[resource] || resource,
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 100,
      render: (status: string) => (
        <Tag
          icon={status === 'success' ? <CheckCircleOutlined /> : <CloseCircleOutlined />}
          color={status === 'success' ? 'success' : 'error'}
        >
          {status === 'success' ? '成功' : '失败'}
        </Tag>
      ),
    },
    {
      title: 'IP地址',
      dataIndex: 'ip',
      key: 'ip',
      width: 140,
      ellipsis: true,
    },
    {
      title: '详情',
      dataIndex: 'details',
      key: 'details',
      ellipsis: true,
      render: (details: string) => {
        if (!details) return '-'
        try {
          const obj = JSON.parse(details)
          return <span style={{ fontSize: 12 }}>{JSON.stringify(obj, null, 2)}</span>
        } catch {
          return <span style={{ fontSize: 12 }}>{details}</span>
        }
      },
    },
  ]

  return (
    <div className="audit-logs-container">
      {/* 统计卡片 */}
      {stats && (
        <Row gutter={16} style={{ marginBottom: 24 }}>
          <Col span={6}>
            <Card>
              <Statistic title="总日志数" value={stats.total_logs} />
            </Card>
          </Col>
          <Col span={6}>
            <Card>
              <Statistic title="今日日志" value={stats.today_logs} valueStyle={{ color: '#1366EC' }} />
            </Card>
          </Col>
          <Col span={6}>
            <Card>
              <Statistic title="失败操作" value={stats.failed_logs} valueStyle={{ color: '#F53F3F' }} />
            </Card>
          </Col>
          <Col span={6}>
            <Card>
              <Statistic
                title="成功率"
                value={stats.total_logs > 0 ? ((stats.total_logs - stats.failed_logs) / stats.total_logs * 100).toFixed(1) : 100}
                suffix="%"
                valueStyle={{ color: '#00A870' }}
              />
            </Card>
          </Col>
        </Row>
      )}

      {/* 主表格 */}
      <Card
        title="审计日志"
        extra={
          <Space>
            <Input
              placeholder="搜索用户名"
              prefix={<SearchOutlined />}
              style={{ width: 200 }}
              value={filters.username}
              onChange={(e) => setFilters({ ...filters, username: e.target.value })}
              onPressEnter={() => fetchLogs(1)}
              allowClear
            />
            <Select
              placeholder="操作类型"
              style={{ width: 120 }}
              value={filters.action}
              onChange={(value) => setFilters({ ...filters, action: value })}
              allowClear
            >
              {Object.entries(ACTION_MAP).map(([key, { text }]) => (
                <Select.Option key={key} value={key}>{text}</Select.Option>
              ))}
            </Select>
            <Select
              placeholder="资源类型"
              style={{ width: 120 }}
              value={filters.resource}
              onChange={(value) => setFilters({ ...filters, resource: value })}
              allowClear
            >
              {Object.entries(RESOURCE_MAP).map(([key, value]) => (
                <Select.Option key={key} value={key}>{value}</Select.Option>
              ))}
            </Select>
            <RangePicker
              style={{ width: 280 }}
              onChange={handleDateRangeChange}
            />
            <Button icon={<ReloadOutlined />} onClick={() => fetchLogs()}>
              刷新
            </Button>
            <Button icon={<DownloadOutlined />}>
              导出
            </Button>
          </Space>
        }
      >
        <Table
          columns={columns}
          dataSource={logs}
          rowKey="id"
          loading={loading}
          pagination={{
            current: pagination.current,
            pageSize: pagination.pageSize,
            total: pagination.total,
            showSizeChanger: true,
            showQuickJumper: true,
            showTotal: (total) => `共 ${total} 条`,
            onChange: (page, pageSize) => fetchLogs(page, pageSize),
          }}
          size="small"
        />
      </Card>
    </div>
  )
}

export default AuditLogs
