import { useEffect, useRef, useState } from 'react'
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
  // 单码 copied 重置定时器句柄：卸载时清理（W4 配套）。
  const resetTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  // 剪贴板可用且写入成功才提示成功；http/不支持时 clipboard 为 undefined，
  // 同步 TypeError 会留下未处理 promise（W4）。
  const copyText = (text: string, successMsg: string, onDone?: () => void) => {
    const clipboard = navigator.clipboard
    if (!clipboard || typeof clipboard.writeText !== 'function') {
      message.warning('当前环境不支持剪贴板')
      return
    }
    clipboard.writeText(text).then(
      () => {
        message.success(successMsg)
        onDone?.()
      },
      () => {
        message.warning('复制失败，请手动复制')
      },
    )
  }

  const copyCode = (code: string, index: number) => {
    copyText(code, '已复制到剪贴板', () => {
      const newCopied = new Set(copied)
      newCopied.add(index)
      setCopied(newCopied)

      if (resetTimerRef.current) {
        clearTimeout(resetTimerRef.current)
      }
      resetTimerRef.current = setTimeout(() => {
        const reset = new Set(newCopied)
        reset.delete(index)
        setCopied(reset)
        resetTimerRef.current = null
      }, 2000)
    })
  }

  const copyAllCodes = () => {
    copyText(codes.join('\n'), '所有备份码已复制到剪贴板')
  }

  useEffect(() => {
    return () => {
      // 卸载清理 copied 重置定时器（W4 配套）。
      if (resetTimerRef.current) {
        clearTimeout(resetTimerRef.current)
        resetTimerRef.current = null
      }
    }
  }, [])

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
