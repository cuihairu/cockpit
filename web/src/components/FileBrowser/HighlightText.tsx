// 命中行关键词高亮（大小写按搜索模式切分，纯展示不递归）
import type { ReactNode } from 'react'

export const HighlightText = ({ text, query, caseSensitive }: { text: string; query: string; caseSensitive: boolean }) => {
  if (!query) return <>{text}</>
  const needle = caseSensitive ? query : query.toLowerCase()
  const parts: ReactNode[] = []
  let rest = text
  let key = 0
  while (rest) {
    const h = caseSensitive ? rest : rest.toLowerCase()
    const idx = h.indexOf(needle)
    if (idx < 0) {
      parts.push(rest)
      break
    }
    if (idx > 0) parts.push(rest.slice(0, idx))
    parts.push(
      <mark key={key++} style={{ background: '#613400', color: '#ffd666', padding: '0 1px' }}>
        {rest.slice(idx, idx + needle.length)}
      </mark>,
    )
    rest = rest.slice(idx + needle.length)
  }
  return <>{parts}</>
}
