import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Alert, Button, Modal, Space, Tag, Typography, Upload, message } from 'antd'
import { UploadOutlined } from '@ant-design/icons'
import type { UploadProps } from 'antd'
import { api } from '@/services/api'
import type { DNSImportResult, DNSRecordInput } from '@/types'
import { getApiErrorMessage } from '@/utils/apiError'

// 批量导入 Modal（dns-design M4）：本地读取 .json（beforeUpload 返回
// false 不发起上传，栈 compose 上传先例）→ 解析预览 → 确认导入 →
// created/updated/skipped/failed 四分类结果摘要；成功后失效记录查询。
// 导入只增改不删（不做 replace 模式，M4「不做」清单）。

// 结构子集：避依赖 jsdom File.text() 支持度（readComposeFile 同款手法）
interface ImportFileLike {
  name: string
  size: number
  text: () => Promise<string>
}

const IMPORT_MAX_BYTES = 1024 * 1024 // 1MB（server 上限 500 条，1MB 已远超）

// 解析导入文件：必须是 {records:[...]} 形态（导出文件直接可用，其余
// 字段忽略）；返回 null 表示格式不符（错误由调用方提示）
export const parseImportFile = async (file: ImportFileLike): Promise<DNSRecordInput[]> => {
  if (!/\.json$/i.test(file.name)) {
    throw new Error('仅支持 .json 文件')
  }
  if (file.size > IMPORT_MAX_BYTES) {
    throw new Error('文件超过 1MB 上限')
  }
  const text = await file.text()
  if (!text.trim()) {
    throw new Error('文件内容为空')
  }
  let parsed: unknown
  try {
    parsed = JSON.parse(text)
  } catch {
    throw new Error('不是合法的 JSON 文件')
  }
  const records = (parsed as { records?: unknown }).records
  if (!Array.isArray(records)) {
    throw new Error('文件缺少 records 数组（应使用「导出」产物或同构 JSON）')
  }
  return records.filter((r): r is DNSRecordInput => !!r && typeof r === 'object')
}

interface ImportModalProps {
  zoneId: string
  zoneName?: string
  open: boolean
  onClose: () => void
}

const ImportModal = ({ zoneId, zoneName, open, onClose }: ImportModalProps) => {
  const [records, setRecords] = useState<DNSRecordInput[] | null>(null)
  const [fileName, setFileName] = useState('')
  const [result, setResult] = useState<DNSImportResult | null>(null)
  const queryClient = useQueryClient()

  const reset = () => {
    setRecords(null)
    setFileName('')
    setResult(null)
  }

  const beforeUpload: UploadProps['beforeUpload'] = (file) => {
    setResult(null)
    parseImportFile(file as unknown as ImportFileLike)
      .then((rs) => {
        if (rs.length === 0) {
          setRecords(null)
          message.error('records 数组为空')
          return
        }
        setRecords(rs)
        setFileName(file.name)
      })
      .catch((err: Error) => {
        setRecords(null)
        message.error(err.message)
      })
    return false // 只本地读取，不发起上传
  }

  const importMutation = useMutation({
    mutationFn: () => api.importDNSRecords(zoneId, records as DNSRecordInput[]),
    onSuccess: (res) => {
      setResult(res)
      queryClient.invalidateQueries({ queryKey: ['dns-records', zoneId] })
    },
    // 回调须返回 void：antd message 返回的 thenable 被 react-query await，
    // 关闭动画未结束会卡住 mutation 错误态回落（按钮停在 loading）
    onError: (err) => {
      message.error(getApiErrorMessage(err, '导入失败'))
    },
  })

  return (
    <Modal
      title={`批量导入记录${zoneName ? ` · ${zoneName}` : ''}`}
      open={open}
      onCancel={() => {
        onClose()
        reset()
      }}
      destroyOnClose
      footer={[
        <Button key="cancel" onClick={() => { onClose(); reset() }}>
          关闭
        </Button>,
        <Button
          key="ok"
          type="primary"
          disabled={!records || importMutation.isPending}
          loading={importMutation.isPending}
          onClick={() => {
            if (result) {
              onClose()
              reset()
            } else {
              importMutation.mutate()
            }
          }}
        >
          {result ? '完成' : `导入 ${records ? records.length : 0} 条`}
        </Button>,
      ]}
    >
      {result ? (
        <Space direction="vertical" size={12} style={{ width: '100%' }}>
          <Space wrap>
            共 {result.total} 条：
            <Tag color="green">新建 {result.created}</Tag>
            <Tag color="orange">更新 {result.updated}</Tag>
            <Tag>跳过 {result.skipped}</Tag>
            <Tag color={result.failed.length ? 'red' : 'default'}>失败 {result.failed.length}</Tag>
          </Space>
          {result.failed.length > 0 && (
            <div style={{ maxHeight: 240, overflow: 'auto' }}>
              {result.failed.map((f) => (
                <Typography.Text key={f.index} type="danger" code style={{ display: 'block' }}>
                  #{f.index} {f.name}: {f.error}
                </Typography.Text>
              ))}
            </div>
          )}
        </Space>
      ) : (
        <Space direction="vertical" size={12} style={{ width: '100%' }}>
          <Upload accept=".json" showUploadList={false} beforeUpload={beforeUpload}>
            <Button icon={<UploadOutlined />}>选择 JSON 文件</Button>
          </Upload>
          {records && (
            <Alert
              type="info"
              showIcon
              message={`已解析 ${records.length} 条记录（${fileName}）`}
              description="匹配键为 名称+类型+内容：命中且无差异跳过、有差异更新、未命中新建；已有同名同内容不同 TTL/代理的记录会被更新。导入不删除任何现有记录。"
            />
          )}
        </Space>
      )}
    </Modal>
  )
}

export default ImportModal
