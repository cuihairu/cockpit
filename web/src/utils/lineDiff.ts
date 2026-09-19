// 行级统一 diff（drift 漂移 diff 视图用，见 docs/guide/drift-design.md M3/D22）。
// 自写 LCS 避免引依赖：行先映射为整数键（DP 内整型比较），全表 DP + 回溯
// 产出统一 diff（del=基线有当前无 / add=当前新增 / same=一致）。

export interface DiffLine {
  type: 'same' | 'del' | 'add'
  text: string
  oldLine?: number // del/same 行在基线侧的行号（1 起）
  newLine?: number // add/same 行在当前侧的行号（1 起）
}

export interface DiffResult {
  lines: DiffLine[]
  truncated: boolean // 任一侧超过行数上限被截断
}

// 两侧各截断到的行数上限（DP 全表 ≤1001×1001，内存与渲染都在预算内）
export const DIFF_MAX_LINES = 1000

export function lineDiff(expected: string, current: string): DiffResult {
  const a = expected.split('\n')
  const b = current.split('\n')
  let truncated = false
  if (a.length > DIFF_MAX_LINES) {
    a.length = DIFF_MAX_LINES
    truncated = true
  }
  if (b.length > DIFF_MAX_LINES) {
    b.length = DIFF_MAX_LINES
    truncated = true
  }

  // 行文本 → 紧凑整数键
  const keys = new Map<string, number>()
  const idOf = (line: string): number => {
    let id = keys.get(line)
    if (id === undefined) {
      id = keys.size
      keys.set(line, id)
    }
    return id
  }
  const x = a.map(idOf)
  const y = b.map(idOf)

  // LCS 长度全表（Uint32Array，默认 0 恰好覆盖 i==m / j==n 边界）
  const m = x.length
  const n = y.length
  const stride = n + 1
  const dp = new Uint32Array((m + 1) * stride)
  for (let i = m - 1; i >= 0; i--) {
    for (let j = n - 1; j >= 0; j--) {
      dp[i * stride + j] =
        x[i] === y[j]
          ? dp[(i + 1) * stride + j + 1] + 1
          : Math.max(dp[(i + 1) * stride + j], dp[i * stride + j + 1])
    }
  }

  // 回溯：相等走对角；否则优先删（基线侧先行），产出稳定的统一 diff
  const lines: DiffLine[] = []
  let i = 0
  let j = 0
  let oldLine = 1
  let newLine = 1
  while (i < m && j < n) {
    if (x[i] === y[j]) {
      lines.push({ type: 'same', text: a[i], oldLine: oldLine++, newLine: newLine++ })
      i++
      j++
    } else if (dp[(i + 1) * stride + j] >= dp[i * stride + j + 1]) {
      lines.push({ type: 'del', text: a[i], oldLine: oldLine++ })
      i++
    } else {
      lines.push({ type: 'add', text: b[j], newLine: newLine++ })
      j++
    }
  }
  while (i < m) {
    lines.push({ type: 'del', text: a[i], oldLine: oldLine++ })
    i++
  }
  while (j < n) {
    lines.push({ type: 'add', text: b[j], newLine: newLine++ })
    j++
  }

  return { lines, truncated }
}

// ============ 双栏对照（M5/D26）============

// 一行对照：left=基线侧（del/same），right=当前侧（add/same），
// change 行允许一侧缺位（纯增/纯删/增删数目不齐）
export interface DiffPair {
  type: 'same' | 'change'
  left?: DiffLine
  right?: DiffLine
}

// pairDiffLines 统一 diff → 双栏行对：same 直通；连续 del/add 块内按序
// 一一配对，多出的一侧独占行（留空侧由渲染层画底纹）
export function pairDiffLines(lines: DiffLine[]): DiffPair[] {
  const rows: DiffPair[] = []
  let i = 0
  while (i < lines.length) {
    if (lines[i].type === 'same') {
      rows.push({ type: 'same', left: lines[i], right: lines[i] })
      i++
      continue
    }
    const dels: DiffLine[] = []
    const adds: DiffLine[] = []
    while (i < lines.length && lines[i].type !== 'same') {
      ;(lines[i].type === 'del' ? dels : adds).push(lines[i])
      i++
    }
    for (let k = 0; k < Math.max(dels.length, adds.length); k++) {
      rows.push({ type: 'change', left: dels[k], right: adds[k] })
    }
  }
  return rows
}
