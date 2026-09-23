import React, { useCallback, useEffect, useRef, useState } from 'react'
import { Button, Space, Slider, message } from 'antd'
import { PlayCircleOutlined, PauseCircleOutlined } from '@ant-design/icons'
import Guacamole from 'guacamole-common-js'

// GuacPlayer：.guac 会话录制回放（todo.md M3 D5）。
// Guacamole.SessionRecording 承担解析与回放（内部建 PlaybackTunnel + Client、
// 自动 keyframe 帧索引、play/pause/seek/getDisplay 全备），本组件只管
// 「挂载 Display + 播放控制 + 生命周期」。与 .cast 的 xterm 回放器互补：
// 终端用开放格式（脱离 Guacamole 可回放），桌面用 Guacamole 格式。

interface GuacPlayerProps {
  /** 录制内容（.guac 二进制流） */
  blob: Blob
  onError?: (message: string) => void
}

const GuacPlayer: React.FC<GuacPlayerProps> = ({ blob, onError }) => {
  const displayRef = useRef<HTMLDivElement>(null)
  const recRef = useRef<InstanceType<typeof Guacamole.SessionRecording> | null>(null)
  const [playing, setPlaying] = useState(false)
  const [duration, setDuration] = useState(0)
  const [position, setPosition] = useState(0)

  useEffect(() => {
    if (!displayRef.current) return
    let rec: InstanceType<typeof Guacamole.SessionRecording>
    try {
      rec = new Guacamole.SessionRecording(blob)
    } catch (err) {
      const msg = err instanceof Error ? err.message : '录制解析失败'
      onError?.(msg)
      message.error(msg)
      return
    }
    recRef.current = rec

    rec.onplay = () => setPlaying(true)
    rec.onpause = () => setPlaying(false)
    rec.onseek = (pos) => setPosition(pos)
    rec.onerror = (status) => {
      setPlaying(false)
      const msg = status?.message || '录制回放失败'
      onError?.(msg)
      message.error(msg)
    }

    displayRef.current.appendChild(rec.getDisplay().getElement())
    // duration 不在 effect 里同步 setState（set-state-in-effect）：
    // SessionRecording 的 getDuration 依赖异步解析的 frames，首次播放时
    // 才可靠；在 toggle 事件处理器里读，顺带更新 Slider 上限
    return () => {
      rec.pause()
      recRef.current = null
    }
  }, [blob, onError])

  const toggle = useCallback(() => {
    const rec = recRef.current
    if (!rec) return
    if (rec.isPlaying()) {
      rec.pause()
    } else {
      rec.play()
      setDuration(rec.getDuration())
    }
  }, [])

  const seek = useCallback((v: number) => {
    recRef.current?.seek(v, () => setPosition(v))
  }, [])

  return (
    <div>
      <div ref={displayRef} style={{ background: '#000', minHeight: 240 }} />
      <Space style={{ marginTop: 8, width: '100%' }} direction="vertical">
        <Slider
          min={0}
          max={Math.max(duration, 1)}
          value={position}
          onChange={seek}
          tooltip={{ formatter: (v) => `${Math.round((v ?? 0) / 1000)}s` }}
        />
        <Space>
          <Button
            type="primary"
            icon={playing ? <PauseCircleOutlined /> : <PlayCircleOutlined />}
            onClick={toggle}
          >
            {playing ? '暂停' : '播放'}
          </Button>
          <span style={{ color: '#888' }}>
            {Math.round(position / 1000)}s / {Math.round(duration / 1000)}s
          </span>
        </Space>
      </Space>
    </div>
  )
}

export default GuacPlayer
