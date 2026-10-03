import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { Button, Modal, Radio, Space, Typography, message } from 'antd'
import { ClearOutlined } from '@ant-design/icons'
import { api } from '@/services/api'
import { getApiErrorMessage } from '@/utils/apiError'

const { Text } = Typography

// 三档阈值（小时）：与后端 handleAgentsCleanup 的 thresholdHours 白名单一致
const THRESHOLD_OPTIONS = [
  { label: '离线超过 24 小时', value: 24 },
  { label: '离线超过 3 天', value: 72 },
  { label: '离线超过 7 天', value: 168 },
]

// 一键清理离线 Agent：二次确认 + 阈值三档 + 清理数量回报。
// 使用方按 RBAC 收口（inventory:write）决定是否渲染入口。
const CleanupOfflineButton = ({ onCleaned }: { onCleaned?: () => void }) => {
  const [open, setOpen] = useState(false)
  const [hours, setHours] = useState(72)
  const cleanupMutation = useMutation({
    mutationFn: () => api.cleanupAgents({ thresholdHours: hours }),
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
    <>
      <Button
        danger
        icon={<ClearOutlined />}
        loading={cleanupMutation.isPending}
        onClick={() => setOpen(true)}
      >
        清理离线
      </Button>
      <Modal
        title="一键清理离线 Agent"
        open={open}
        onCancel={() => setOpen(false)}
        onOk={() => cleanupMutation.mutate()}
        okText="确认清理"
        okButtonProps={{ danger: true }}
        confirmLoading={cleanupMutation.isPending}
        destroyOnClose
      >
        <Space direction="vertical" style={{ width: '100%' }}>
          <Text>将删除满足条件的离线 Agent 记录及其标签关联，操作不可恢复。</Text>
          <Radio.Group
            options={THRESHOLD_OPTIONS}
            optionType="button"
            value={hours}
            onChange={(e) => setHours(e.target.value)}
          />
        </Space>
      </Modal>
    </>
  )
}

export default CleanupOfflineButton
