import { describe, expect, it } from 'vitest'
import { DIFF_MAX_LINES, lineDiff, pairDiffLines } from './lineDiff'

describe('lineDiff', () => {
  it('完全一致：全 same 且行号对齐', () => {
    const r = lineDiff('a\nb\nc', 'a\nb\nc')
    expect(r.truncated).toBe(false)
    expect(r.lines).toEqual([
      { type: 'same', text: 'a', oldLine: 1, newLine: 1 },
      { type: 'same', text: 'b', oldLine: 2, newLine: 2 },
      { type: 'same', text: 'c', oldLine: 3, newLine: 3 },
    ])
  })

  it('一侧为空：空串 split 出一行空行，del/add 各自全量', () => {
    // ''.split('\n') === ['']：空侧算一行空文本
    expect(lineDiff('', 'x\ny').lines.map((l) => l.type)).toEqual(['del', 'add', 'add'])
    expect(lineDiff('x\ny', '').lines.map((l) => l.type)).toEqual(['del', 'del', 'add'])
  })

  it('中间修改：del 优先于 add（基线侧先行）', () => {
    const r = lineDiff('a\nold\nc', 'a\nnew\nc')
    expect(r.lines).toEqual([
      { type: 'same', text: 'a', oldLine: 1, newLine: 1 },
      { type: 'del', text: 'old', oldLine: 2 },
      { type: 'add', text: 'new', newLine: 2 },
      { type: 'same', text: 'c', oldLine: 3, newLine: 3 },
    ])
  })

  it('纯插入与纯删除', () => {
    const ins = lineDiff('a\nc', 'a\nb\nc')
    expect(ins.lines.map((l) => l.type)).toEqual(['same', 'add', 'same'])
    const del = lineDiff('a\nb\nc', 'a\nc')
    expect(del.lines.map((l) => l.type)).toEqual(['same', 'del', 'same'])
  })

  it('连续改动块内行号连续递增', () => {
    const r = lineDiff('a\nb\nc\nd', 'a\nX\nY\nd')
    const dels = r.lines.filter((l) => l.type === 'del')
    const adds = r.lines.filter((l) => l.type === 'add')
    expect(dels.map((l) => l.oldLine)).toEqual([2, 3])
    expect(adds.map((l) => l.newLine)).toEqual([2, 3])
  })

  it('超过上限截断并置 truncated', () => {
    const big = Array.from({ length: DIFF_MAX_LINES + 5 }, (_, i) => `l${i}`).join('\n')
    const r = lineDiff(big, 'x')
    expect(r.truncated).toBe(true)
    // 基线侧截到上限 1000 行且与 'x' 无公共行：del 1000 + add 1
    expect(r.lines.filter((l) => l.type === 'del')).toHaveLength(DIFF_MAX_LINES)
    expect(r.lines.filter((l) => l.type === 'add')).toHaveLength(1)
  })

  it('两侧都超上限同样 truncated', () => {
    const a = Array.from({ length: DIFF_MAX_LINES + 1 }, (_, i) => `a${i}`).join('\n')
    const b = Array.from({ length: DIFF_MAX_LINES + 1 }, (_, i) => `b${i}`).join('\n')
    const r = lineDiff(a, b)
    expect(r.truncated).toBe(true)
    expect(r.lines).toHaveLength(DIFF_MAX_LINES * 2)
  })
})

describe('pairDiffLines', () => {
  it('same 直通为同行对照', () => {
    const rows = pairDiffLines([
      { type: 'same', text: 'a', oldLine: 1, newLine: 1 },
    ])
    expect(rows).toEqual([
      { type: 'same', left: { type: 'same', text: 'a', oldLine: 1, newLine: 1 }, right: { type: 'same', text: 'a', oldLine: 1, newLine: 1 } },
    ])
  })

  it('连续 del/add 块按序一一配对为 change', () => {
    const rows = pairDiffLines([
      { type: 'del', text: 'old1', oldLine: 1 },
      { type: 'del', text: 'old2', oldLine: 2 },
      { type: 'add', text: 'new1', newLine: 1 },
    ])
    expect(rows).toHaveLength(2)
    expect(rows[0]).toEqual({ type: 'change', left: { type: 'del', text: 'old1', oldLine: 1 }, right: { type: 'add', text: 'new1', newLine: 1 } })
    expect(rows[1]).toEqual({ type: 'change', left: { type: 'del', text: 'old2', oldLine: 2 }, right: undefined })
  })

  it('多出的 add 侧独占行', () => {
    const rows = pairDiffLines([
      { type: 'del', text: 'old', oldLine: 1 },
      { type: 'add', text: 'n1', newLine: 1 },
      { type: 'add', text: 'n2', newLine: 2 },
    ])
    expect(rows[1]).toEqual({ type: 'change', left: undefined, right: { type: 'add', text: 'n2', newLine: 2 } })
  })

  it('混合序列：same 分隔多个 change 块', () => {
    const rows = pairDiffLines(lineDiff('a\nb\nc\nd', 'a\nB\nc\nD').lines)
    expect(rows.map((r) => r.type)).toEqual(['same', 'change', 'same', 'change'])
  })
})

describe('lineDiff 尾部剩余', () => {
  it('主循环因当前侧耗尽退出后，基线剩余行走 del 尾循环', () => {
    const r = lineDiff('a\nb', 'a')
    expect(r.lines).toEqual([
      { type: 'same', text: 'a', oldLine: 1, newLine: 1 },
      { type: 'del', text: 'b', oldLine: 2 },
    ])
    expect(r.truncated).toBe(false)
  })

  it('主循环因基线侧耗尽退出后，当前剩余行走 add 尾循环', () => {
    const r = lineDiff('a', 'a\nb')
    expect(r.lines).toEqual([
      { type: 'same', text: 'a', oldLine: 1, newLine: 1 },
      { type: 'add', text: 'b', newLine: 2 },
    ])
  })
})
