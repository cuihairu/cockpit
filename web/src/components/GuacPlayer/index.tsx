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

// parseGuacInstructions 解析 .guac 指令流（长度前缀 `len.val,...;` 序列，
// Guacamole 协议标准格式）。按 JS 字符口径切片——与官方 parser
// （instruction 解析）一致；畸形流抛错（构造 try/catch 收口）。
function parseGuacInstructions(text: string): Array<{ opcode: string; args: string[] }> {
  const out: Array<{ opcode: string; args: string[] }> = []
  let i = 0
  while (i < text.length) {
    if (text[i] === ';') {
      i++
      continue
    }
    const elements: string[] = []
    while (i < text.length && text[i] !== ';') {
      const dot = text.indexOf('.', i)
      if (dot < 0) throw new Error('malformed guac stream')
      const len = parseInt(text.slice(i, dot), 10)
      if (Number.isNaN(len) || dot + 1 + len > text.length) throw new Error('malformed guac stream')
      elements.push(text.slice(dot + 1, dot + 1 + len))
      i = dot + 1 + len + 1 // 值尾 + 分隔符（',' 或 ';'）
    }
    i++ // 指令结尾 ';'
    if (elements.length > 0) out.push({ opcode: elements[0], args: elements.slice(1) })
  }
  return out
}

// makeBlobTunnel 用 Blob 驱动 SessionRecording 的 tunnel 分支。
// 为什么不直接 SessionRecording(blob)：guacamole-common-js 1.5.0 dist
// （cjs/esm 双构建同缺）的 Blob 入参分支缺 `recordingBlob = source` 赋值，
// parseBlob(undefined) 恒 0 帧（getDuration()=0、play() 立即暂停）——真机
// 验收 2026-09-30 发现。duck tunnel 把 blob 解析成指令序列经基类
// receiveInstruction 注入（SessionRecording 构造时已挂 oninstruction 收帧，
// 帧索引/keyframe/seek/Display 全由官方实现承担），CLOSED 收尾对齐官方
// notifyLoaded 语义。
function makeBlobTunnel(blob: Blob): Guacamole.Tunnel {
  const tunnel = new Guacamole.Tunnel()
  tunnel.connect = () => {
    blob
      .text()
      .then((text) => {
        for (const ins of parseGuacInstructions(text)) {
          tunnel.receiveInstruction(ins.opcode, ins.args)
        }
        tunnel.setState(Guacamole.Tunnel.State.CLOSED)
      })
      .catch(() => tunnel.setState(Guacamole.Tunnel.State.CLOSED))
  }
  tunnel.disconnect = () => {}
  return tunnel
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
      rec = new Guacamole.SessionRecording(makeBlobTunnel(blob))
      rec.connect() // 触发 duck tunnel 注入指令流（Blob 分支官方即坏，见上）
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
      rec.disconnect()
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
