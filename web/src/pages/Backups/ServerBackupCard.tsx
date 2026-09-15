import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Button,
  Card,
  InputNumber,
  Popconfirm,
  Space,
  Switch,
  Table,
  Typography,
  message,
} from 'antd'
import { DatabaseOutlined, DownloadOutlined, PlusOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { api } from '@/services/api'
import type { ServerBackupFile } from '@/types'
import { getApiErrorMessage } from '@/utils/apiError'

// 面板数据库备份卡片（server 自身 SQLite 的 VACUUM INTO 定时备份，
// 见 docs/guide/server-backup-design.md）。恢复 = 下载产物停服替换。

const fmtBytes = (n: number) => {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}

const ServerBackupCard = () => {
  const queryClient = useQueryClient()
  const [intervalHours, setIntervalHours] = useState<number | null>(null)
  const [retentionDays, setRetentionDays] = useState<number | null>(null)
  const [saving, setSaving] = useState(false)

  const { data: backups = [], isLoading } = useQuery({
    queryKey: ['server-backups'],
    queryFn: () => api.getServerBackups(),
  })

  const { data: cfg } = useQuery({
    queryKey: ['server-backup-config'],
    queryFn: () => api.getServerBackupConfig(),
  })

  // 编辑态：未改动时从服务端配置派生
  const enabled = intervalHours ?? (cfg ? cfg.interval_hours > 0 : true)
  const effInterval = intervalHours ?? (cfg?.interval_hours || 24)
  const effRetention = retentionDays ?? (cfg?.retention_days ?? 7)

  const invalidate = () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: ['server-backups'] }),
      queryClient.invalidateQueries({ queryKey: ['server-backup-config'] }),
    ])

  const saveConfig = async () => {
    setSaving(true)
    try {
      await api.putServerBackupConfig({
        interval_hours: enabled ? effInterval : 0,
        retention_days: effRetention,
      })
      setIntervalHours(null)
      setRetentionDays(null)
      await invalidate()
      message.success('备份配置已保存')
    } catch (err) {
      message.error(getApiErrorMessage(err, '保存失败'))
    } finally {
      setSaving(false)
    }
  }

  const runMutation = useMutation({
    mutationFn: () => api.runServerBackup(),
    onSuccess: async () => {
      message.success('备份完成')
      await invalidate()
    },
    onError: (err) => message.error(getApiErrorMessage(err, '备份失败')),
  })

  const deleteMutation = useMutation({
    mutationFn: (name: string) => api.deleteServerBackup(name),
    onSuccess: async () => {
      message.success('已删除')
      await invalidate()
    },
    onError: (err) => message.error(getApiErrorMessage(err, '删除失败')),
  })

  const download = async (name: string) => {
    try {
      const blob = await api.downloadServerBackup(name)
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = name
      a.click()
      URL.revokeObjectURL(url)
    } catch (err) {
      message.error(getApiErrorMessage(err, '下载失败'))
    }
  }

  const columns: ColumnsType<ServerBackupFile> = [
    { title: '文件', dataIndex: 'name', ellipsis: true },
    {
      title: '时间',
      dataIndex: 'modTime',
      width: 170,
      render: (v: string) => new Date(v).toLocaleString(),
    },
    { title: '大小', dataIndex: 'size', width: 100, render: fmtBytes },
    {
      title: '操作',
      width: 130,
      render: (_, f) => (
        <Space size={0}>
          <Button type="link" size="small" icon={<DownloadOutlined />} onClick={() => void download(f.name)}>
            下载
          </Button>
          <Popconfirm title="删除该备份？" onConfirm={() => deleteMutation.mutate(f.name)}>
            <Button type="link" size="small" danger>
              删除
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <Card
      title={
        <Space>
          <DatabaseOutlined />
          面板数据库
        </Space>
      }
      extra={
        <Space>
          <Button
            icon={<PlusOutlined />}
            loading={runMutation.isPending}
            onClick={() => runMutation.mutate()}
          >
            立即备份
          </Button>
        </Space>
      }
    >
      <Space wrap style={{ marginBottom: 16 }} align="center">
        <Typography.Text>定时备份</Typography.Text>
        <Switch checked={enabled} onChange={(v) => setIntervalHours(v ? 24 : 0)} />
        {enabled && (
          <InputNumber
            min={1}
            max={168}
            value={effInterval}
            onChange={(v) => setIntervalHours(v ?? 24)}
            addonAfter="小时"
            style={{ width: 120 }}
          />
        )}
        <Typography.Text>保留</Typography.Text>
        <InputNumber
          min={0}
          max={365}
          value={effRetention}
          onChange={(v) => setRetentionDays(v ?? 7)}
          addonAfter="天"
          style={{ width: 110 }}
        />
        <Typography.Text type="secondary">（0 = 永久）</Typography.Text>
        <Button size="small" loading={saving} onClick={() => void saveConfig()}>
          保存
        </Button>
      </Space>
      <Table<ServerBackupFile>
        rowKey="name"
        columns={columns}
        dataSource={backups}
        loading={isLoading}
        size="small"
        pagination={false}
        locale={{ emptyText: '暂无备份（开启定时或点击立即备份）' }}
      />
      <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginTop: 8 }}>
        面板自身数据库（资源/用户/审计/配置）的在线快照（SQLite VACUUM INTO）。
        恢复方式：下载备份 → 停止 server → 替换 data/cockpit.db → 启动。
      </Typography.Text>
    </Card>
  )
}

export default ServerBackupCard
