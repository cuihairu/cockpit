// 配置文本轻量着色（drift diff 视图用，见 docs/guide/drift-design.md M5）。
// 单行无状态词法，覆盖 cockpit 下发的四类内容：nginx conf / Traefik 与
// compose YAML / cron JSON / .env。按 注释行 → 行首键（yaml `key:`、env
// `KEY=`、json `"key":`）→ nginx 首词指令 → 引号串 → 数字 → `{}`; 标点
// 依次上色，未命中保持默认色。刻意保守：不做跨行解析状态、不猜行内 #
// 注释（nginx/URL 值可能合法含 #），宁缺毋错。

export interface ConfigToken {
  text: string
  color?: string
}

const COLORS = {
  comment: '#8c8c8c',
  key: '#722ed1',
  string: '#08979c',
  number: '#d46b08',
  punct: '#8c8c8c',
} as const

// 行首键：缩进/列表项前缀 + 裸词或带引号 JSON 键 + `:` 或 `=`（env）
const KEY_RE = /^(\s*(?:-\s+)?)("[^"]+"|[A-Za-z_][\w.-]*)(:| =|=)/
// nginx 指令/块首词：仅当整行形如 `word ...;` 或 `word ... {` 才认
const NGINX_RE = /^(\s*)([a-z][\w]*)(\s+)(.*[;{]\s*)$/

// 独立数字：两侧不能贴词字符或点（v2、hunter2、IP 片段不上色，防拆碎）
const stickyStr = /"[^"]*"?|'[^']*'?/y
const stickyNum = /(?<![\w.])\d+(?:\.\d+)?(?![\w.])/y
const splitNumPunct =
  /((?<![\w.])\d+(?:\.\d+)?(?![\w.]))|([{};])/g

export function highlightConfigLine(line: string): ConfigToken[] {
  if (/^\s*#/.test(line)) return [{ text: line, color: COLORS.comment }]

  const tokens: ConfigToken[] = []
  const push = (text: string, color?: string) => {
    if (text) tokens.push({ text, color })
  }

  let rest = line
  const key = KEY_RE.exec(rest)
  const nginx = key ? null : NGINX_RE.exec(rest)
  if (key) {
    push(key[1])
    push(key[2], COLORS.key) // 引号一并算键色（JSON 键）
    push(key[3]) // 键后分隔符（: / = / 空格=）保持默认色——slice 会连它一起截掉，须显式补回
    rest = rest.slice(key[0].length)
  } else if (nginx) {
    push(nginx[1])
    push(nginx[2], COLORS.key)
    push(nginx[3])
    rest = nginx[4]
  }

  let plain = ''
  const flushPlain = () => {
    // 非字符串段拆出独立数字与标点
    for (const seg of plain.split(splitNumPunct)) {
      if (!seg) continue
      if (/^\d/.test(seg)) push(seg, COLORS.number)
      else if (/^[{};]$/.test(seg)) push(seg, COLORS.punct)
      else push(seg)
    }
    plain = ''
  }

  for (let i = 0; i < rest.length; ) {
    stickyStr.lastIndex = i
    const str = stickyStr.exec(rest)
    if (str && str.index === i) {
      flushPlain()
      push(str[0], COLORS.string)
      i += str[0].length
      continue
    }
    stickyNum.lastIndex = i
    const num = stickyNum.exec(rest)
    if (num && num.index === i) {
      flushPlain()
      push(num[0], COLORS.number)
      i += num[0].length
      continue
    }
    plain += rest[i]
    i++
  }
  flushPlain()
  return tokens
}
