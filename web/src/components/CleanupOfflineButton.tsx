import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { Button, Input, Modal, Space, Typography, message } from 'antd'
import { ClearOutlined } from '@ant-design/icons'
import { api } from '@/services/api'
import { getApiErrorMessage } from '@/utils/apiError'
import { formatOfflineThreshold, parseOfflineThreshold } from '@/utils/format'

const { Text } = Typography

// 一键清理离线 agent：二次确认 + 阈值自由填（30m/2h/7d 或中文单位），
// 留空 = 默认全清。按钮旁注展示当前筛选口径。使用方按 RBAC 收口
// （inventory:write）决定是否渲染入口。
const CleanupOfflineButton = ({ onCleaned }: { onCleaned?: () => void }) => {
  const [open, setOpen] = useState(false)
  // 阈值原文（跨开关弹窗保留，按钮旁注与弹窗内口径始终一致）
  const [raw, setRaw] = useState('')
  const parsed = parseOfflineThreshold(raw)
  const valid = parsed.kind !== 'invalid'
  const minutes = parsed.kind === 'minutes' ? parsed.minutes : 0
  // 按钮旁注与弹窗提示共用的口径文案：全部离线 / 离线超过 2 小时
  const scopeText = minutes > 0 ? `离线超过 ${formatOfflineThreshold(minutes)}` : '全部离线'

  const cleanupMutation = useMutation({
    mutationFn: () =>
      api.cleanupAgents(minutes > 0 ? { thresholdMinutes: minutes } : {}),
    onSuccess: (res) => {
      message.success(`已清理 ${res.count} 台离线 Agent`)
      setOpen(false)
      onCleaned?.()
    },
    onError: (err) => {
      message.error(getApiErrorMessage(err, '清理失败'))
    },
  })

  return (
    <Space size={6}>
      <Button
        danger
        icon={<ClearOutlined />}
        loading={cleanupMutation.isPending}
        onClick={() => setOpen(true)}
      >
        清理离线 agent
      </Button>
      <Text type="secondary" style={{ fontSize: 12 }}>
        {valid ? scopeText : '口径无效'}
      </Text>
      <Modal
        title="一键清理离线 Agent"
        open={open}
        onCancel={() => setOpen(false)}
        onOk={() => cleanupMutation.mutate()}
        okText="确认清理"
        okButtonProps={{ danger: true, disabled: !valid }}
        confirmLoading={cleanupMutation.isPending}
        destroyOnClose
      >
        <Space direction="vertical" size={12} style={{ width: '100%' }}>
          <Text>将删除满足条件的离线 Agent 记录及其标签关联，操作不可恢复。</Text>
          <Input
            allowClear
            value={raw}
            placeholder="离线时长阈值，留空 = 全部离线（如 30m / 2h / 7d）"
            onChange={(e) => setRaw(e.target.value)}
            onPressEnter={() => valid && cleanupMutation.mutate()}
          />
          <Text type={parsed.kind === 'invalid' ? 'danger' : 'secondary'}>
            {parsed.kind === 'invalid'
              ? '格式无效：支持 30m / 2h / 7d 或 30 分钟 / 2 小时 / 7 天（需带单位）'
              : `当前口径：${scopeText}`}
          </Text>
        </Space>
      </Modal>
    </Space>
  )
}

export default CleanupOfflineButton
