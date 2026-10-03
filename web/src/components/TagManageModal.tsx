import { useEffect, useState } from 'react'
import { Button, Input, Modal, Popconfirm, Space, Table, Tag as AntTag, message } from 'antd'
import { DeleteOutlined, EditOutlined, PlusOutlined } from '@ant-design/icons'
import { api } from '@/services/api'
import type { AgentTagRef } from '@/types'

type TagRow = AgentTagRef & { agentCount?: number }

/** 标签管理弹窗：新建/重命名/改色/删除（删除只摘关联，不动服务器） */
const TagManageModal = ({
  visible,
  onClose,
  onChanged,
}: {
  visible: boolean
  onClose: () => void
  onChanged?: () => void
}) => {
  const [tags, setTags] = useState<TagRow[]>([])
  const [loading, setLoading] = useState(false)
  const [draftName, setDraftName] = useState('')
  const [draftColor, setDraftColor] = useState('#1677ff')
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editName, setEditName] = useState('')
  const [editColor, setEditColor] = useState('')

  const load = async () => {
    setLoading(true)
    try {
      const list = await api.listAgentTags()
      setTags(list)
    } catch {
      message.error('加载标签失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    if (visible) void load()
  }, [visible])

  const create = async () => {
    const name = draftName.trim()
    if (!name) {
      message.warning('请输入标签名')
      return
    }
    try {
      await api.createAgentTag(name, draftColor)
      setDraftName('')
      await load()
      onChanged?.()
    } catch (e: unknown) {
      const err = e as { response?: { status?: number } }
      message.error(err?.response?.status === 409 ? '标签名已存在' : '创建失败')
    }
  }

  const saveEdit = async () => {
    if (!editingId) return
    const name = editName.trim()
    if (!name) {
      message.warning('标签名不能为空')
      return
    }
    try {
      await api.updateAgentTag(editingId, name, editColor)
      setEditingId(null)
      await load()
      onChanged?.()
    } catch {
      message.error('更新失败')
    }
  }

  const remove = async (id: string) => {
    try {
      await api.deleteAgentTag(id)
      await load()
      onChanged?.()
    } catch {
      message.error('删除失败')
    }
  }

  const startEdit = (tag: TagRow) => {
    setEditingId(tag.id)
    setEditName(tag.name)
    setEditColor(tag.color || '#1677ff')
  }

  return (
    <Modal title="标签管理" open={visible} onCancel={onClose} footer={null} width={620}>
      <Space style={{ marginBottom: 12 }} wrap>
        <Input
          placeholder="新标签名"
          value={draftName}
          onChange={(e) => setDraftName(e.target.value)}
          style={{ width: 200 }}
          onPressEnter={create}
        />
        <input
          type="color"
          value={draftColor}
          onChange={(e) => setDraftColor(e.target.value)}
          style={{ width: 40, height: 32, padding: 0, border: 'none', background: 'none' }}
        />
        <AntTag color={draftColor}>{draftName || '预览'}</AntTag>
        <Button type="primary" icon={<PlusOutlined />} onClick={create}>
          新建
        </Button>
      </Space>

      <Table<TagRow>
        size="small"
        loading={loading}
        rowKey="id"
        dataSource={tags}
        pagination={false}
        columns={[
          {
            title: '标签',
            dataIndex: 'name',
            render: (_, record) =>
              editingId === record.id ? (
                <Input size="small" value={editName} onChange={(e) => setEditName(e.target.value)} style={{ width: 140 }} />
              ) : (
                <AntTag color={record.color || 'default'}>{record.name}</AntTag>
              ),
          },
          {
            title: '颜色',
            dataIndex: 'color',
            render: (_, record) =>
              editingId === record.id ? (
                <input
                  type="color"
                  value={editColor}
                  onChange={(e) => setEditColor(e.target.value)}
                  style={{ width: 40, height: 24, padding: 0, border: 'none' }}
                />
              ) : (
                <span style={{ color: '#999', fontSize: 12 }}>{record.color || '-'}</span>
              ),
          },
          { title: '服务器数', dataIndex: 'agentCount', width: 90 },
          {
            title: '操作',
            width: 140,
            render: (_, record) =>
              editingId === record.id ? (
                <Space size="small">
                  <Button size="small" type="link" onClick={saveEdit}>保存</Button>
                  <Button size="small" type="link" onClick={() => setEditingId(null)}>取消</Button>
                </Space>
              ) : (
                <Space size="small">
                  <Button size="small" type="link" icon={<EditOutlined />} onClick={() => startEdit(record)} />
                  <Popconfirm title="删除后只摘除关联，不删服务器" onConfirm={() => remove(record.id)}>
                    <Button size="small" type="link" danger icon={<DeleteOutlined />} />
                  </Popconfirm>
                </Space>
              ),
          },
        ]}
      />
    </Modal>
  )
}

export default TagManageModal
