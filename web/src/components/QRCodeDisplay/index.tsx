import { useEffect, useRef, useState } from 'react'
import QRCode from 'qrcode'
import { Spin } from 'antd'
import { logger } from '@/utils/logger'
import './index.less'

interface QRCodeDisplayProps {
  value: string
  size?: number
  title?: string
}

const QRCodeDisplay: React.FC<QRCodeDisplayProps> = ({ value, size = 200, title }) => {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!value || !canvasRef.current) return

    setLoading(true)
    setError(null)

    QRCode.toCanvas(canvasRef.current, value, { width: size }, (err) => {
      setLoading(false)
      if (err) {
        setError('生成 QR 码失败')
        logger.error('QR Code generation error:', err)
      }
    })
  }, [value, size])

  return (
    <div className="qrcode-display">
      {title && <div className="qrcode-title">{title}</div>}
      <div className="qrcode-canvas-wrapper" style={{ width: size, height: size }}>
        {loading && (
          <div className="qrcode-loading">
            <Spin />
          </div>
        )}
        {error && <div className="qrcode-error">{error}</div>}
        <canvas ref={canvasRef} style={{ display: loading || error ? 'none' : 'block' }} />
      </div>
    </div>
  )
}

export default QRCodeDisplay
