import { Descriptions, Tag } from 'antd'
import type { ComputeInstance } from '@/types'

// 后端 storage.ComputeInstance 实际返回 labels/createdAt/updatedAt
// （json 标签在模型上），TS 类型尚未声明——此处局部收窄补齐。
type ComputeRecord = ComputeInstance & {
  labels?: Record<string, string>
  createdAt?: string
  updatedAt?: string
}

interface Props {
  record: ComputeRecord
}

const ComputeDetail = ({ record }: Props) => {
  const mem = record.memoryMb || 0
  const memDisplay = mem >= 1024 ? `${(mem / 1024).toFixed(0)} GB` : `${mem} MB`
  const labels = record.labels || {}

  return (
    <Descriptions column={2} size="small" style={{ margin: '8px 0' }}>
      <Descriptions.Item label="ID">{record.id}</Descriptions.Item>
      <Descriptions.Item label="名称">{record.name}</Descriptions.Item>
      <Descriptions.Item label="类型">
        <Tag color={record.type === 'vm' ? 'blue' : record.type === 'container' ? 'green' : 'orange'}>
          {record.type?.toUpperCase()}
        </Tag>
      </Descriptions.Item>
      <Descriptions.Item label="状态">
        <Tag color={record.status === 'running' ? 'success' : record.status === 'stopped' ? 'default' : 'error'}>
          {record.status || '—'}
        </Tag>
      </Descriptions.Item>
      <Descriptions.Item label="Agent">{record.agentId || '—'}</Descriptions.Item>
      <Descriptions.Item label="位置">{record.region || '—'}/{record.zone || '—'}</Descriptions.Item>
      <Descriptions.Item label="CPU">{record.cpuCores ? `${record.cpuCores} 核` : '—'}</Descriptions.Item>
      <Descriptions.Item label="内存">{mem > 0 ? memDisplay : '—'}</Descriptions.Item>
      <Descriptions.Item label="磁盘">{record.diskGb ? `${record.diskGb} GB` : '—'}</Descriptions.Item>
      <Descriptions.Item label="IP">{record.ipv4 || '—'}</Descriptions.Item>
      {Object.keys(labels).length > 0 && (
        <Descriptions.Item label="标签" span={2}>
          {Object.entries(labels).map(([k, v]) => (
            <Tag key={k} style={{ marginRight: 4 }}>
              {k}: {String(v)}
            </Tag>
          ))}
        </Descriptions.Item>
      )}
      {record.createdAt && (
        <Descriptions.Item label="创建时间">{new Date(record.createdAt).toLocaleString()}</Descriptions.Item>
      )}
      {record.updatedAt && (
        <Descriptions.Item label="更新时间">{new Date(record.updatedAt).toLocaleString()}</Descriptions.Item>
      )}
    </Descriptions>
  )
}

export default ComputeDetail