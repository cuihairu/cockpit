import { useState } from 'react'
import axios from 'axios'
import { useMutation, useQueries, useQueryClient } from '@tanstack/react-query'
import { Alert, Button, Form, Input, Modal, Space, Typography, message } from 'antd'
import { BranchesOutlined } from '@ant-design/icons'
import { api } from '@/services/api'
import type { StackGitStatus } from '@/types'
import { PermGuard } from '@/components/PermGuard'
import { extractApiError } from './shared'

// GitBar（P2 ① Git-to-deploy）：按 agent 展示 stacks 仓库接入状态与 drift，
// 提供「接入仓库 / 拉取更新」入口。设计口径：
//  - 未接入（not-a-repo）是常态：只展示一条低调的接入入口，不告警；
//  - agent 无 git（git-unavailable）：warning 提示，不提供操作；
//  - drift（本地改动 vs 仓库）：warning 列出改动文件——stack.file.save 的
//    UI 编辑天然是 drift，提示而非阻止；
//  - behind>0（上次 fetch 时点远端有新提交）：info + 拉取按钮；
//  - pull 409（脏树冲突）：引导先处理 drift；
//  - 老 agent 不认识 git.status（502）：静默跳过，不制造噪音。

interface GitAgent {
  id: string
  hostname: string
}

// drift 列表最多展示的条数（超出折叠为「等 N 处改动」）
const DRIFT_PREVIEW_LIMIT = 5

const GitBar = ({ agents }: { agents: GitAgent[] }) => {
  const queryClient = useQueryClient()
  // 接入弹窗：null = 关闭；否则记录待接入的 agent
  const [attachAgent, setAttachAgent] = useState<GitAgent | null>(null)
  const [form] = Form.useForm<{ remote: string; branch: string }>()

  const statuses = useQueries({
    queries: agents.map((a) => ({
      queryKey: ['stack-git', a.id],
      queryFn: () => api.getStackGit(a.id),
      retry: false,
    })),
  })

  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: ['stack-git'] })
    void queryClient.invalidateQueries({ queryKey: ['stacks'] })
  }

  // pull / 接入共用一个 mutation：payload 为空 = 已接入仓库的拉取
  const pullMutation = useMutation({
    mutationFn: (vars: { agentId: string; payload?: { remote: string; branch: string } }) =>
      api.pullStackGit(vars.agentId, vars.payload),
    onSuccess: (res, vars) => {
      message.success(
        `「${vars.agentId}」已同步到 ${res.branch ?? '-'}@${res.head ?? '-'}（远端 ${res.remote ?? '-'}）`,
      )
      setAttachAgent(null)
      refresh()
    },
    onError: (err, vars) => {
      if (axios.isAxiosError(err) && err.response?.status === 409) {
        message.warning(`「${vars.agentId}」存在本地改动，与远端冲突：请先处理 drift（保留或还原后再拉取）`)
        return
      }
      message.error(`Git 拉取失败: ${extractApiError(err)}`)
    },
  })

  const openAttach = (agent: GitAgent) => {
    form.setFieldsValue({ remote: '', branch: 'main' })
    setAttachAgent(agent)
  }

  const submitAttach = () => {
    form
      .validateFields()
      .then((values) => {
        if (attachAgent) {
          pullMutation.mutate({ agentId: attachAgent.id, payload: values })
        }
      })
      .catch(() => {
        // 表单校验错误已就地展示
      })
  }

  // 组装每行的告警内容；未加载完成 / 老 agent 报错的查询跳过
  const rows: { agent: GitAgent; status: StackGitStatus }[] = []
  agents.forEach((agent, i) => {
    const st = statuses[i]?.data
    if (st) {
      rows.push({ agent, status: st })
    }
  })
  if (rows.length === 0 && !statuses.some((q) => q.isLoading)) {
    return null
  }

  return (
    <div style={{ marginBottom: 16 }}>
      {rows.map(({ agent, status }) => {
        if (!status.enabled && status.reason === 'git-unavailable') {
          return (
            <Alert
              key={agent.id}
              type="warning"
              showIcon
              style={{ marginBottom: 8 }}
              message={`${agent.hostname}（${agent.id.slice(0, 12)}）：agent 主机未安装 git，无法使用 Git 同步`}
            />
          )
        }
        if (!status.enabled) {
          // not-a-repo：低调接入入口
          return (
            <Alert
              key={agent.id}
              type="info"
              showIcon
              style={{ marginBottom: 8 }}
              message={
                <Space size={8} wrap>
                  <span>{`${agent.hostname}（${agent.id.slice(0, 12)}）：stacks 目录未接入 Git 仓库`}</span>
                  <PermGuard perm="stack:write">
                    <Button size="small" icon={<BranchesOutlined />} onClick={() => openAttach(agent)}>
                      接入 Git 仓库
                    </Button>
                  </PermGuard>
                </Space>
              }
            />
          )
        }
        const changes = status.changes ?? []
        const behind = status.behind > 0
        if (changes.length === 0 && !behind) {
          return null
        }
        return (
          <Alert
            key={agent.id}
            type={changes.length > 0 ? 'warning' : 'info'}
            showIcon
            style={{ marginBottom: 8 }}
            message={
              <Space size={8} wrap>
                <span>
                  {`${agent.hostname}：仓库 ${status.remote ?? '-'}（${status.branch ?? '-'}@${status.head ?? '-'}）`}
                  {changes.length > 0
                    ? `，本地改动 ${changes.length} 处`
                    : ''}
                  {behind
                    ? '，远端有新提交'
                    : ''}
                </span>
                {changes.length > 0 && (
                  <Typography.Text type="secondary" style={{ fontSize: 12 }} code>
                    {changes
                      .slice(0, DRIFT_PREVIEW_LIMIT)
                      .map((c) => `${c.code} ${c.path}`)
                      .join('；')}
                    {changes.length > DRIFT_PREVIEW_LIMIT
                      ? ` 等 ${changes.length} 处`
                      : ''}
                  </Typography.Text>
                )}
                <PermGuard perm="stack:write">
                  <Button
                    size="small"
                    loading={pullMutation.isPending && pullMutation.variables?.agentId === agent.id}
                    onClick={() => pullMutation.mutate({ agentId: agent.id })}
                  >
                    拉取更新
                  </Button>
                </PermGuard>
              </Space>
            }
          />
        )
      })}

      {/* 接入仓库弹窗：远端必填，分支默认 main（要求远端分支已存在） */}
      <Modal
        title={attachAgent ? `接入 Git 仓库 — ${attachAgent.hostname}` : '接入 Git 仓库'}
        open={!!attachAgent}
        onCancel={() => setAttachAgent(null)}
        onOk={submitAttach}
        confirmLoading={pullMutation.isPending}
        okText="接入并拉取"
        cancelText="取消"
        maskClosable={false}
      >
        <Form form={form} layout="vertical">
          <Form.Item
            name="remote"
            label="仓库地址"
            rules={[{ required: true, message: '请输入仓库地址' }]}
            extra="支持 https / ssh / 本地路径；不允许 ext:: 传输助手语法"
          >
            <Input placeholder="https://git.example.com/ops/stacks.git" />
          </Form.Item>
          <Form.Item
            name="branch"
            label="分支"
            rules={[{ required: true, message: '请输入分支名' }]}
            extra="要求远端分支已存在（空仓库请先推送首版）"
          >
            <Input placeholder="main" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

export default GitBar
