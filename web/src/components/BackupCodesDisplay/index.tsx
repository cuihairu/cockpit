import { useState } from 'react'
import { Button, message } from 'antd'
import { CopyOutlined, CheckOutlined, EyeInvisibleOutlined, EyeTwoTone } from '@ant-design/icons'
import './index.less'

interface BackupCodesDisplayProps {
  codes: string[]
  title?: string
  warning?: string
}

const BackupCodesDisplay: React.FC<BackupCodesDisplayProps> = ({
  codes,
  title = '备份恢复码',
  warning = '请妥善保存这些恢复码。每个代码只能使用一次。启用 TOTP 后，如果丢失认证设备，可以使用这些代码登录。',
}) => {
  const [copied, setCopied] = useState<Set<number>>(new Set())
  const [blurred, setBlurred] = useState(true)

  const copyCode = (code: string, index: number) => {
    navigator.clipboard.writeText(code).then(() => {
      const newCopied = new Set(copied)
      newCopied.add(index)
      setCopied(newCopied)
      message.success('已复制到剪贴板')

      setTimeout(() => {
        const reset = new Set(newCopied)
        reset.delete(index)
        setCopied(reset)
      }, 2000)
    })
  }

  const copyAllCodes = () => {
    const allCodes = codes.join('\n')
    navigator.clipboard.writeText(allCodes).then(() => {
      message.success('所有备份码已复制到剪贴板')
    })
  }

  return (
    <div className="backup-codes-display">
      {title && <div className="backup-codes-title">{title}</div>}

      <div className="backup-codes-warning">
        <span className="warning-icon">⚠️</span>
        <span>{warning}</span>
      </div>

      <div className="backup-codes-actions">
        <Button
          type="link"
          icon={blurred ? <EyeInvisibleOutlined /> : <EyeTwoTone />}
          onClick={() => setBlurred(!blurred)}
        >
          {blurred ? '显示代码' : '隐藏代码'}
        </Button>
        <Button type="link" icon={<CopyOutlined />} onClick={copyAllCodes}>
          全部复制
        </Button>
      </div>

      <div className={`backup-codes-list ${blurred ? 'blurred' : ''}`}>
        {codes.map((code, index) => (
          <div key={index} className="backup-code-item">
            <span className="backup-code-number">{index + 1}.</span>
            <code className="backup-code-value">{code}</code>
            <Button
              type="text"
              size="small"
              icon={copied.has(index) ? <CheckOutlined /> : <CopyOutlined />}
              onClick={() => copyCode(code, index)}
              className={copied.has(index) ? 'copied' : ''}
            />
          </div>
        ))}
      </div>
    </div>
  )
}

export default BackupCodesDisplay
