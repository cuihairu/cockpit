// 深色终端风格文本块（对齐容器管理页日志弹窗样式）
const TerminalBlock = ({
  content,
  loading = false,
  maxHeight = 420,
  emptyText = '（无日志输出）',
}: {
  content: string
  loading?: boolean
  maxHeight?: number
  emptyText?: string
}) => (
  <div
    style={{
      maxHeight,
      overflow: 'auto',
      background: '#1e1e1e',
      color: '#d4d4d4',
      padding: 12,
      borderRadius: 6,
      fontSize: 12,
      fontFamily: "'Cascadia Code', 'Fira Code', 'Consolas', monospace",
      lineHeight: 1.6,
      whiteSpace: 'pre-wrap',
      wordBreak: 'break-all',
    }}
  >
    {loading ? (
      <span style={{ color: '#888' }}>加载中...</span>
    ) : content ? (
      content
    ) : (
      <span style={{ color: '#888' }}>{emptyText}</span>
    )}
  </div>
)

export default TerminalBlock
