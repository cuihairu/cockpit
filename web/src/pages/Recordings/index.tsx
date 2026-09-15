import { useCallback, useEffect, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Button,
  Card,
  Modal,
  Popconfirm,
  Segmented,
  Space,
  Table,
  Tag,
  Typography,
  message,
} from 'antd'
import { VideoCameraOutlined, DownloadOutlined, PlayCircleOutlined } from '@ant-design/icons'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import type { ColumnsType } from 'antd/es/table'
import { api } from '@/services/api'
import type { TerminalRecording } from '@/types'
import { getApiErrorMessage } from '@/utils/apiError'
import '@xterm/xterm/css/xterm.css'

// 会话录制：远控终端输出流的 asciinema v2 落盘（server 侧录制，见
// docs/guide/recording-design.md）。列表 + 轻量回放器（按事件时间差调度
// term.write）+ 下载/删除。只录输出不录输入（输入含密码）。

const PROTOCOL_COLOR: Record<string, string> = {
  ssh: 'green',
  telnet: 'orange',
}

const fmtDuration = (ms: number) => {
  if (!ms) return '进行中'
  const s = Math.round(ms / 1000)
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m}m${s % 60}s`
  return `${Math.floor(m / 60)}h${m % 60}m`
}

const fmtBytes = (b: number) => {
  if (!b) return '-'
  if (b < 1024) return `${b} B`
  if (b < 1024 * 1024) return `${(b / 1024).toFixed(1)} KB`
  return `${(b / 1024 / 1024).toFixed(1)} MB`
}

// asciinema v2 事件：[相对秒, 类型, 数据]
type CastEvent = [number, string, string]

interface CastFile {
  events: CastEvent[]
  totalSeconds: number
}

const parseCast = (text: string): CastFile => {
  const events: CastEvent[] = []
  for (const line of text.split('\n')) {
    if (!line.trim()) continue
    try {
      const parsed = JSON.parse(line)
      if (Array.isArray(parsed) && parsed.length >= 3 && typeof parsed[0] === 'number') {
        events.push([parsed[0], parsed[1], String(parsed[2])])
      }
    } catch {
      // header 行之外损坏的行跳过（截断的尾部事件）
    }
  }
  const totalSeconds = events.length ? events[events.length - 1][0] : 0
  return { events, totalSeconds }
}

// 回放器 props：录制元数据 + 打开状态
interface PlayerProps {
  recording: TerminalRecording | null
  onClose: () => void
}

// PlaybackModal 轻量 cast 回放：逐事件按时间差写 xterm，倍速切换即重启
const PlaybackModal = ({ recording, onClose }: PlayerProps) => {
  const termDivRef = useRef<HTMLDivElement>(null)
  const termRef = useRef<Terminal | null>(null)
  const timersRef = useRef<number[]>([])
  const [speed, setSpeed] = useState(1)
  const [loading, setLoading] = useState(false)
  const [progress, setProgress] = useState(0)
  const [totalSeconds, setTotalSeconds] = useState(0)

  const clearTimers = () => {
    timersRef.current.forEach((t) => window.clearTimeout(t))
    timersRef.current = []
  }

  const play = useCallback(
    async (rec: TerminalRecording, playSpeed: number) => {
      setLoading(true)
      let cast: CastFile
      try {
        cast = parseCast(await api.getRecordingCast(rec.sessionId))
      } catch (err) {
        message.error(getApiErrorMessage(err, '加载录制内容失败'))
        setLoading(false)
        return
      }
      const term = termRef.current
      if (!term) {
        setLoading(false)
        return
      }
      term.reset()
      clearTimers()
      setTotalSeconds(cast.totalSeconds)
      setProgress(0)
      const total = cast.totalSeconds
      for (const ev of cast.events) {
        const [dt, , data] = ev
        const at = (dt * 1000) / playSpeed
        const timer = window.setTimeout(() => {
          term.write(data)
          setProgress(total > 0 ? dt : 0)
        }, at)
        timersRef.current.push(timer)
      }
      setLoading(false)
    },
    [],
  )

  useEffect(() => {
    if (!recording || !termDivRef.current) return
    const term = new Terminal({
      convertEol: false,
      cursorBlink: false,
      disableStdin: true,
      scrollback: 10000,
      theme: { background: '#1e1e1e' },
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.open(termDivRef.current)
    fit.fit()
    termRef.current = term
    return () => {
      clearTimers()
      term.dispose()
      termRef.current = null
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [recording?.sessionId])

  useEffect(() => {
    if (recording && termRef.current) {
      void play(recording, speed)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [recording?.sessionId, speed])

  return (
    <Modal
      title={
        recording
          ? `回放：${recording.username}@${recording.host}（${recording.protocol}）`
          : '回放'
      }
      open={!!recording}
      onCancel={() => {
        clearTimers()
        onClose()
      }}
      footer={null}
      width={860}
      destroyOnHidden
    >
      <Space wrap style={{ marginBottom: 8 }}>
        <Segmented
          value={speed}
          onChange={(v) => setSpeed(v as number)}
          options={[
            { label: '1x', value: 1 },
            { label: '2x', value: 2 },
            { label: '4x', value: 4 },
            { label: '8x', value: 8 },
          ]}
        />
        <Typography.Text type="secondary">
          {loading ? '加载中…' : `进度 ${fmtDuration(progress * 1000)} / ${fmtDuration(totalSeconds * 1000)}`}
        </Typography.Text>
      </Space>
      <div
        ref={termDivRef}
        style={{ background: '#1e1e1e', padding: 8, borderRadius: 6, minHeight: 420 }}
      />
    </Modal>
  )
}

// Recordings 会话录制列表页
const Recordings = () => {
  const queryClient = useQueryClient()
  const [playing, setPlaying] = useState<TerminalRecording | null>(null)

  const { data: recordings = [], isLoading } = useQuery({
    queryKey: ['recordings'],
    queryFn: () => api.getRecordings(),
  })

  const handleDelete = async (rec: TerminalRecording) => {
    try {
      await api.deleteRecording(rec.sessionId)
      message.success('已删除')
      await queryClient.invalidateQueries({ queryKey: ['recordings'] })
    } catch (err) {
      message.error(getApiErrorMessage(err, '删除失败'))
    }
  }

  const handleDownload = async (rec: TerminalRecording) => {
    try {
      const blob = await api.downloadRecording(rec.sessionId)
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `${rec.sessionId}.cast`
      a.click()
      URL.revokeObjectURL(url)
    } catch (err) {
      message.error(getApiErrorMessage(err, '下载失败'))
    }
  }

  const columns: ColumnsType<TerminalRecording> = [
    { title: '时间', dataIndex: 'startedAt', width: 170, render: (v: string) => new Date(v).toLocaleString() },
    { title: '用户', dataIndex: 'username', width: 100 },
    { title: '主机', dataIndex: 'host', ellipsis: true, render: (_, r) => `${r.host}:${r.port}` },
    {
      title: '协议',
      dataIndex: 'protocol',
      width: 90,
      render: (p: string) => <Tag color={PROTOCOL_COLOR[p] ?? 'default'}>{p}</Tag>,
    },
    {
      title: '时长',
      dataIndex: 'durationMs',
      width: 100,
      render: (v: number) => (v ? fmtDuration(v) : <Tag color="processing">进行中</Tag>),
    },
    { title: '大小', dataIndex: 'bytes', width: 90, render: fmtBytes },
    {
      title: '操作',
      width: 200,
      render: (_, rec) => (
        <Space>
          <Button
            size="small"
            icon={<PlayCircleOutlined />}
            disabled={!rec.durationMs}
            onClick={() => setPlaying(rec)}
          >
            回放
          </Button>
          <Button size="small" icon={<DownloadOutlined />} onClick={() => void handleDownload(rec)} />
          <Popconfirm title="删除该录制？" onConfirm={() => void handleDelete(rec)}>
            <Button size="small" danger>
              删除
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <Card
      title={
        <Space>
          <VideoCameraOutlined />
          会话录制
        </Space>
      }
    >
      <Table<TerminalRecording>
        rowKey="sessionId"
        columns={columns}
        dataSource={recordings}
        loading={isLoading}
        size="small"
        pagination={{ pageSize: 20, showSizeChanger: false }}
        locale={{ emptyText: '暂无录制（开启终端远控后自动录制输出流）' }}
      />
      <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginTop: 8 }}>
        录制终端输出流（asciinema v2 格式，可用 asciinema play 离线回放）；
        出于安全不录制键盘输入（含密码）。默认保留 7 天，过期自动清理。
      </Typography.Text>
      <PlaybackModal recording={playing} onClose={() => setPlaying(null)} />
    </Card>
  )
}

export default Recordings
