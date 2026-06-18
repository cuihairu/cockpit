import { Space, Tag } from 'antd'

export const LabelsDisplay: React.FC<{ labels?: Record<string, unknown> }> = ({ labels }) => {
  if (!labels || Object.keys(labels).length === 0) {
    return <span style={{ color: '#999' }}>-</span>
  }

  return (
    <Space size="small" wrap>
      {Object.entries(labels).map(([key, value]) => {
        let displayValue: string
        let color = 'default'

        if (Array.isArray(value)) {
          displayValue = value.join(', ')
          color = 'geekblue'
        } else if (typeof value === 'boolean') {
          displayValue = value ? '是' : '否'
          color = value ? 'success' : 'default'
        } else {
          displayValue = String(value)
          color = 'blue'
        }

        return (
          <Tag key={key} color={color}>
            <strong>{key}</strong>: {displayValue}
          </Tag>
        )
      })}
    </Space>
  )
}
