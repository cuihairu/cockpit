import { describe, expect, it } from 'vitest'
import { highlightConfigLine } from './configHighlight'

// 行首键/指令紫色、注释灰、字符串青、数字橙、{}; 标点灰（词法保守）。

describe('highlightConfigLine', () => {
  it('注释行整行灰', () => {
    expect(highlightConfigLine('# full comment')).toEqual([
      { text: '# full comment', color: '#8c8c8c' },
    ])
    expect(highlightConfigLine('   # indented')).toEqual([
      { text: '   # indented', color: '#8c8c8c' },
    ])
  })

  it('yaml 键上色，键后分隔符保留为独立 token', () => {
    const tokens = highlightConfigLine('image: nginx:1.25')
    expect(tokens[0]).toEqual({ text: 'image', color: '#722ed1' })
    expect(tokens[1]).toEqual({ text: ':', color: undefined })
    // 1.25 是独立数字上橙色；nginx:1.25 中贴冒号的 1.25 前无词界不拆——
    // 此处 1.25 前是 ':'，满足非词字符边界，应上色
    expect(tokens.some((t) => t.text === '1.25' && t.color === '#d46b08')).toBe(true)
  })

  it('env KEY= 与带空格的 = 都算行首键', () => {
    const env = highlightConfigLine('DOMAIN=example.com')
    expect(env[0]).toEqual({ text: 'DOMAIN', color: '#722ed1' })
    const spaced = highlightConfigLine('key = value')
    expect(spaced[0]).toEqual({ text: 'key', color: '#722ed1' })
  })

  it('JSON 引号键与字符串值分开上色', () => {
    const tokens = highlightConfigLine('"name": "web"')
    expect(tokens[0]).toEqual({ text: '"name"', color: '#722ed1' })
    expect(tokens.some((t) => t.text === '"web"' && t.color === '#08979c')).toBe(true)
  })

  it('nginx 指令行：首词按指令上色', () => {
    const tokens = highlightConfigLine('listen 80;')
    expect(tokens[0]).toEqual({ text: 'listen', color: '#722ed1' })
    expect(tokens.some((t) => t.text === '80' && t.color === '#d46b08')).toBe(true)
    expect(tokens.some((t) => t.text === ';' && t.color === '#8c8c8c')).toBe(true)
  })

  it('nginx 块首 { 上标点色', () => {
    const tokens = highlightConfigLine('server {')
    expect(tokens[0]).toEqual({ text: 'server', color: '#722ed1' })
    expect(tokens.some((t) => t.text === '{' && t.color === '#8c8c8c')).toBe(true)
  })

  it('引号串整体上串色（含内部空格）', () => {
    const tokens = highlightConfigLine('cmd: "echo hello world"')
    expect(tokens.some((t) => t.text === '"echo hello world"' && t.color === '#08979c')).toBe(true)
  })

  it('贴词数字不上色（v2/hunter2/IP 片段防拆碎）', () => {
    const tokens = highlightConfigLine('version: v2')
    expect(tokens.some((t) => /v2/.test(t.text) && t.color === '#d46b08')).toBe(false)
  })

  it('未命中任何规则保持默认色（无 color）', () => {
    const tokens = highlightConfigLine('plain text here')
    expect(tokens.length).toBeGreaterThan(0)
    expect(tokens.every((t) => t.color === undefined)).toBe(true)
  })

  it('回归：tokens 拼回与原行完全一致（丢分隔符 bug）', () => {
    const lines = [
      'image: nginx:1.25',
      'DOMAIN=example.com',
      '"name": "web"',
      'key = value',
      '  - port: 443',
      'listen 80;',
      'server {',
      'plain text here',
      '# comment',
    ]
    for (const line of lines) {
      expect(highlightConfigLine(line).map((t) => t.text).join('')).toBe(line)
    }
  })

  it('列表项前缀保持默认，键上色', () => {
    const tokens = highlightConfigLine('  - port: 443')
    expect(tokens[0]).toEqual({ text: '  - ' })
    expect(tokens[1]).toEqual({ text: 'port', color: '#722ed1' })
    expect(tokens.some((t) => t.text === '443' && t.color === '#d46b08')).toBe(true)
  })
})

describe('highlightConfigLine 段首数字', () => {
  it('非独立数字开头的纯文本段按数字色（flushPlain 拆分数分支）', () => {
    // '3rd place' 不满足独立数字边界，整段落入 plain；
    // split 后段首是数字 → 走数字上色分支（不拆碎语义与 v2 不同）
    expect(highlightConfigLine('3rd place')).toEqual([
      { text: '3rd place', color: '#d46b08' },
    ])
    expect(highlightConfigLine('2fa required')).toEqual([
      { text: '2fa required', color: '#d46b08' },
    ])
  })
})
