import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Button,
  Card,
  Input,
  InputNumber,
  Popconfirm,
  Space,
  Switch,
  Table,
  Tooltip,
  Typography,
  message,
} from 'antd'
import {
  CloudUploadOutlined,
  DatabaseOutlined,
  DownloadOutlined,
  PlusOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { api } from '@/services/api'
import { formatBytes } from '@/utils/format'
import type { ServerBackupFile } from '@/types'
import { getApiErrorMessage } from '@/utils/apiError'
import { PermGuard } from '@/components/PermGuard'

// 面板数据库备份卡片（server 自身 SQLite 的 VACUUM INTO 定时备份，
// 见 docs/guide/server-backup-design.md）。恢复 = 下载产物停服替换。
// M2：remote_dest 非空时备份成功后 rclone copy 推送异地；补推按钮
// 对应 POST /{name}/sync-remote（D17）。

// 与后端 serverBackupRemoteDestRe 同源：remote:path，remote 名不以 - 开头
const remoteDestPattern = /^[A-Za-z0-9][A-Za-z0-9._-]*:[^\s]+$/

const ServerBackupCard = () => {
  const queryClient = useQueryClient()
  const [intervalHours, setIntervalHours] = useState<number | null>(null)
  const [retentionDays, setRetentionDays] = useState<number | null>(null)
  const [remoteDest, setRemoteDest] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [syncingRemote, setSyncingRemote] = useState<string | null>(null)

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
  const effRemoteDest = remoteDest ?? (cfg?.remote_dest || '')
  const rcloneReady = cfg?.rclone_available !== false

  const invalidate = () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: ['server-backups'] }),
      queryClient.invalidateQueries({ queryKey: ['server-backup-config'] }),
    ])

  const saveConfig = async () => {
    const dest = effRemoteDest.trim()
    if (dest && !remoteDestPattern.test(dest)) {
      message.error('异地目标需为 remote:path 形态（如 gdrive:cockpit）')
      return
    }
    setSaving(true)
    try {
      await api.putServerBackupConfig({
        interval_hours: enabled ? effInterval : 0,
        retention_days: effRetention,
        remote_dest: dest,
      })
      setIntervalHours(null)
      setRetentionDays(null)
      setRemoteDest(null)
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

  const syncRemote = async (name: string) => {
    setSyncingRemote(name)
    try {
      await api.syncServerBackupRemote(name)
      message.success('已推送到异地')
    } catch (err) {
      message.error(getApiErrorMessage(err, '推送失败'))
    } finally {
      setSyncingRemote(null)
    }
  }

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
    { title: '大小', dataIndex: 'size', width: 100, render: formatBytes },
    {
      title: '操作',
      width: effRemoteDest ? 190 : 130,
      render: (_, f) => (
        <Space size={0}>
          <Button type="link" size="small" icon={<DownloadOutlined />} onClick={() => void download(f.name)}>
            下载
          </Button>
          <PermGuard perm="backup:write">
            {effRemoteDest && (
              <Tooltip title={`补推到 ${effRemoteDest}`}>
                <Button
                  type="link"
                  size="small"
                  icon={<CloudUploadOutlined />}
                  loading={syncingRemote === f.name}
                  onClick={() => void syncRemote(f.name)}
                >
                  补推
                </Button>
              </Tooltip>
            )}
            <Popconfirm title="删除该备份？" onConfirm={() => deleteMutation.mutate(f.name)}>
              <Button type="link" size="small" danger>
                删除
              </Button>
            </Popconfirm>
          </PermGuard>
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
          <PermGuard perm="backup:write">
            <Button
              icon={<PlusOutlined />}
              loading={runMutation.isPending}
              onClick={() => runMutation.mutate()}
            >
              立即备份
            </Button>
          </PermGuard>
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
        <Typography.Text>异地目标</Typography.Text>
        <Tooltip title="rclone remote:path（如 gdrive:cockpit）；凭据取 server 主机 rclone.conf，留空 = 不推送">
          <Input
            value={effRemoteDest}
            onChange={(e) => setRemoteDest(e.target.value)}
            placeholder="gdrive:cockpit"
            style={{ width: 200 }}
            allowClear
            status={effRemoteDest.trim() && !remoteDestPattern.test(effRemoteDest.trim()) ? 'error' : undefined}
          />
        </Tooltip>
        {effRemoteDest.trim() && !rcloneReady && (
          <Typography.Text type="warning" style={{ fontSize: 12 }}>
            server 主机未检测到 rclone，推送将失败
          </Typography.Text>
        )}
        <PermGuard perm="backup:write">
          <Button size="small" loading={saving} onClick={() => void saveConfig()}>
            保存
          </Button>
        </PermGuard>
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
        配置异地目标后每次备份自动 rclone 推送（凭据取 server 主机 rclone.conf）。
        恢复方式：下载备份 → 停止 server → 替换 data/cockpit.db → 启动。
      </Typography.Text>
    </Card>
  )
}

export default ServerBackupCard
