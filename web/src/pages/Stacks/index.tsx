import { useMemo, useState } from 'react'
import type { CSSProperties } from 'react'
import axios from 'axios'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Alert,
  Button,
  Card,
  Empty,
  Form,
  Input,
  Modal,
  Select,
  Space,
  Table,
  Tag,
  Tooltip,
  Typography,
  message,
} from 'antd'
import { PlusOutlined, ReloadOutlined, RocketOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { api } from '@/services/api'
import type { StackView } from '@/types'
import { PermGuard } from '@/components/PermGuard'
import StackDetail from './StackDetail'
import TerminalBlock from './TerminalBlock'
import {
  DEFAULT_COMPOSE_TEMPLATE,
  STACK_NAME_PATTERN,
  STACK_TEMPLATES,
  extractApiError,
  formatTimestamp,
  lastStatusColor,
  serviceStatusColor,
} from './shared'

// 等宽字体
const MONO_FONT = "'Cascadia Code', 'Fira Code', 'Consolas', monospace"

const Stacks = () => {
  const queryClient = useQueryClient()

  // 详情抽屉
  const [detailStack, setDetailStack] = useState<StackView | null>(null)
  const [detailOpen, setDetailOpen] = useState(false)
  // 新建 Stack 弹窗
  const [createOpen, setCreateOpen] = useState(false)
  // 深色终端弹窗（新建时 YAML 校验错误输出）
  const [terminal, setTerminal] = useState<{ title: string; content: string } | null>(null)

  const [form] = Form.useForm<{ agentId: string; name: string; compose: string }>()

  // Stack 列表（聚合所有 agent）
  const {
    data: stacksData,
    isFetching: stacksLoading,
    refetch: refetchStacks,
  } = useQuery({
    queryKey: ['stacks'],
    queryFn: () => api.getStacks(),
  })
  const stacks = useMemo(() => stacksData?.stacks ?? [], [stacksData])
  // 各 agent 的 stacks 目录自检信息（M1.5）
  const agentInfo = useMemo(() => stacksData?.agentInfo ?? {}, [stacksData])
  // 目录不可写 / compose 缺失的 agent 告警
  const dirIssues = useMemo(
    () =>
      Object.entries(agentInfo).filter(
        ([, info]) => !info.dirWritable,
      ),
    [agentInfo],
  )

  // 在线且有 Docker 能力的 agent（新建 Stack 时可选）
  const { data: agents = [], isFetching: agentsLoading } = useQuery({
    queryKey: ['agents'],
    queryFn: () => api.getAgents(),
  })
  const stackAgents = useMemo(
    () =>
      agents.filter(
        (a) =>
          a.status === 'online' &&
          a.capabilities?.some((c) => c.type === 'docker-api' || c.type === 'docker'),
      ),
    [agents],
  )

  // 新建 Stack：保存初始 compose.yml（后端校验 YAML 通过后落盘）
  const createMutation = useMutation({
    mutationFn: (values: { agentId: string; name: string; compose: string }) =>
      api.saveStackCompose(values.agentId, values.name, { compose: values.compose }),
    onSuccess: (_res, vars) => {
      message.success(`Stack「${vars.name}」创建成功，可在详情中启动部署`)
      setCreateOpen(false)
      form.resetFields()
      void queryClient.invalidateQueries({ queryKey: ['stacks'] })
    },
    onError: (err) => {
      if (axios.isAxiosError(err) && err.response?.status === 502) {
        message.error('YAML 校验失败，请检查 compose.yml 内容')
        setTerminal({
          title: 'YAML 校验失败 — docker compose config 错误输出',
          content: extractApiError(err),
        })
        return
      }
      message.error(`创建失败: ${extractApiError(err)}`)
    },
  })

  const openDetail = (record: StackView) => {
    setDetailStack(record)
    setDetailOpen(true)
  }

  // 列表列定义
  const columns: ColumnsType<StackView> = [
    {
      title: 'Stack 名称',
      dataIndex: 'name',
      key: 'name',
      render: (name: string, record) => (
        <Space size={6}>
          <span>{name}</span>
          {!record.online && <Tag>离线</Tag>}
        </Space>
      ),
    },
    {
      title: '所在 Agent',
      dataIndex: 'agentName',
      key: 'agentName',
      ellipsis: true,
    },
    {
      title: '服务状态',
      key: 'serviceStatus',
      width: 130,
      render: (_, record) => (
        <Tag color={serviceStatusColor(record.running, record.total)}>
          {record.running}/{record.total}
        </Tag>
      ),
    },
    {
      title: '最近部署',
      key: 'lastDeploy',
      render: (_, record) =>
        record.lastAction ? (
          <Space size={6} wrap>
            <Tag color={lastStatusColor(record.lastStatus)}>{record.lastAction}</Tag>
            {record.lastDeployedAt > 0 && (
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                {formatTimestamp(record.lastDeployedAt)}
              </Typography.Text>
            )}
          </Space>
        ) : (
          <Typography.Text type="secondary">未部署</Typography.Text>
        ),
    },
    {
      title: '操作',
      key: 'actions',
      width: 90,
      render: (_, record) => (
        <Tooltip title={record.online ? undefined : 'Agent 离线，无法查看详情'}>
          <Button
            type="link"
            size="small"
            disabled={!record.online}
            onClick={() => openDetail(record)}
          >
            详情
          </Button>
        </Tooltip>
      ),
    },
  ]

  return (
    <div className="page-container">
      <Card
        title={
          <Space size={8}>
            <RocketOutlined />
            <span>应用部署（Compose Stack）</span>
          </Space>
        }
        extra={
          <Space>
            <PermGuard perm="stack:write">
              <Button
                icon={<PlusOutlined />}
                type="primary"
                onClick={() => setCreateOpen(true)}
                disabled={stackAgents.length === 0}
              >
                新建 Stack
              </Button>
            </PermGuard>
            <Button
              icon={<ReloadOutlined />}
              onClick={() => {
                void refetchStacks()
              }}
              loading={stacksLoading}
            >
              刷新
            </Button>
          </Space>
        }
      >
        {/* 目录自检提示：stacks 目录不可写的 agent 无法保存/部署 Stack */}
        {dirIssues.length > 0 && (
          <Alert
            type="warning"
            showIcon
            style={{ marginBottom: 16 }}
            message="部分 Agent 的 stacks 目录异常"
            description={dirIssues.map(([id, info]) => (
              <div key={id} style={{ fontSize: 12 }}>
                <Typography.Text code>{id}</Typography.Text>：目录 {info.dir} 不可写
                {info.dirError ? `（${info.dirError}）` : ''}
              </div>
            ))}
          />
        )}
        <Table
          columns={columns}
          dataSource={stacks}
          rowKey={(record) => `${record.agentId}-${record.name}`}
          loading={stacksLoading}
          pagination={{ pageSize: 20, showTotal: (t) => `共 ${t} 个 Stack` }}
          // 离线 agent 的 Stack 行置灰
          onRow={(record) =>
            record.online
              ? {}
              : { style: { opacity: 0.45 } as CSSProperties }
          }
          locale={{
            emptyText: (
              <Empty
                description={
                  stacksLoading ? '正在加载 Stack 列表...' : '暂无 Stack，点击右上角「新建 Stack」开始部署应用'
                }
              />
            ),
          }}
        />
      </Card>

      {/* 新建 Stack 弹窗 */}
      <Modal
        title="新建 Stack"
        open={createOpen}
        onCancel={() => setCreateOpen(false)}
        onOk={() => form.submit()}
        confirmLoading={createMutation.isPending}
        okText="创建"
        cancelText="取消"
        width={720}
        maskClosable={false}
      >
        <Form
          form={form}
          layout="vertical"
          initialValues={{ compose: DEFAULT_COMPOSE_TEMPLATE }}
          onFinish={(values) => createMutation.mutate(values)}
        >
          <Form.Item
            name="agentId"
            label="所在 Agent"
            rules={[{ required: true, message: '请选择 Agent' }]}
          >
            <Select
              placeholder={agentsLoading ? '正在加载 Agent 列表...' : '选择在线的 Docker Agent'}
              loading={agentsLoading}
              options={stackAgents.map((a) => ({
                value: a.id,
                label: `${a.hostname} (${a.id.slice(0, 12)})`,
              }))}
            />
          </Form.Item>
          <Form.Item
            name="name"
            label="Stack 名称"
            rules={[
              { required: true, message: '请输入 Stack 名称' },
              {
                pattern: STACK_NAME_PATTERN,
                message: '仅支持小写字母、数字、中划线和下划线，并以字母或数字开头',
              },
            ]}
          >
            <Input placeholder="如 my-blog" maxLength={64} showCount />
          </Form.Item>
          <Form.Item label="模板（选择后填充，可再编辑）">
            <Select
              placeholder="选择模板或粘贴已有 compose.yml"
              options={STACK_TEMPLATES.map((t) => ({ value: t.key, label: t.label }))}
              onChange={(key: string) => {
                const tpl = STACK_TEMPLATES.find((t) => t.key === key)
                if (tpl) form.setFieldValue('compose', tpl.compose)
              }}
              allowClear
            />
          </Form.Item>
          <Form.Item
            name="compose"
            label="compose.yml 初始内容"
            rules={[{ required: true, message: '请输入 compose.yml 内容' }]}
          >
            <Input.TextArea
              rows={12}
              styles={{ textarea: { background: '#1e1e1e', color: '#d4d4d4', fontFamily: MONO_FONT, fontSize: 12 } }}
            />
          </Form.Item>
        </Form>
      </Modal>

      {/* Stack 详情抽屉（切换 Stack 时通过 key 重挂载以重置内部状态） */}
      <StackDetail
        key={detailStack ? `${detailStack.agentId}-${detailStack.name}` : 'none'}
        open={detailOpen}
        stack={detailStack}
        onClose={() => setDetailOpen(false)}
      />

      {/* 深色终端弹窗：新建时 YAML 校验错误输出 */}
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
    </div>
  )
}

export default Stacks
