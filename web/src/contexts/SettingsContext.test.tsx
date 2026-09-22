import { act, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { SettingsProvider } from './SettingsContext'
import { useSettingsContext } from './useSettingsContext'
import type { UISettings } from './settingsTypes'

// 消费者探针：读 context 值并渲染标记
const Probe = ({ patch }: { patch?: Partial<UISettings> }) => {
  const { settings, updateSettings, resolvedTheme } = useSettingsContext()
  return (
    <div>
      <span data-testid="site">{settings.siteName}</span>
      <span data-testid="interval">{settings.refreshInterval}</span>
      <span data-testid="theme">{resolvedTheme}</span>
      <button onClick={() => patch && updateSettings(patch)}>update</button>
    </div>
  )
}

describe('SettingsContext', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('无存量时用默认值（Cockpit / 30s / light）', () => {
    render(<SettingsProvider><Probe /></SettingsProvider>)
    expect(screen.getByTestId('site').textContent).toBe('Cockpit')
    expect(screen.getByTestId('interval').textContent).toBe('30')
    expect(screen.getByTestId('theme').textContent).toBe('light')
  })

  it('旧键 cockpit:settings 与新键都识别，坏 JSON 跳过', () => {
    localStorage.setItem('cockpit.ui.settings', '{broken')
    localStorage.setItem('cockpit:settings', JSON.stringify({ siteName: '  ', refreshInterval: 999 }))
    render(<SettingsProvider><Probe /></SettingsProvider>)
    // 坏 JSON 跳到旧键：空白站名回退默认、越界间隔回退 30
    expect(screen.getByTestId('site').textContent).toBe('Cockpit')
    expect(screen.getByTestId('interval').textContent).toBe('30')
  })

  it('合法存量加载（dark、间隔 60）', () => {
    localStorage.setItem('cockpit.ui.settings', JSON.stringify({ theme: 'dark', refreshInterval: 60 }))
    render(<SettingsProvider><Probe /></SettingsProvider>)
    expect(screen.getByTestId('theme').textContent).toBe('dark')
    expect(screen.getByTestId('interval').textContent).toBe('60')
  })

  it('auto 主题跟随 prefers-color-scheme（jsdom mock 为 false → light）', () => {
    localStorage.setItem('cockpit.ui.settings', JSON.stringify({ theme: 'auto' }))
    render(<SettingsProvider><Probe /></SettingsProvider>)
    expect(screen.getByTestId('theme').textContent).toBe('light')
  })

  it('非法 theme 值回退 light', () => {
    localStorage.setItem('cockpit.ui.settings', JSON.stringify({ theme: 'blue' }))
    render(<SettingsProvider><Probe /></SettingsProvider>)
    expect(screen.getByTestId('theme').textContent).toBe('light')
  })

  it('updateSettings 规范化并持久化双键 + 广播事件', () => {
    const listener = vi.fn()
    window.addEventListener('cockpit:settings-changed', listener)
    render(<SettingsProvider><Probe patch={{ siteName: 'Mine', refreshInterval: 120 }} /></SettingsProvider>)
    act(() => {
      screen.getByText('update').click()
    })
    expect(screen.getByTestId('site').textContent).toBe('Mine')
    expect(screen.getByTestId('interval').textContent).toBe('120')
    expect(localStorage.getItem('cockpit.ui.settings')).toContain('"siteName":"Mine"')
    expect(localStorage.getItem('cockpit:settings')).toContain('"siteName":"Mine"')
    expect(listener).toHaveBeenCalled()
    window.removeEventListener('cockpit:settings-changed', listener)
  })

  it('useSettingsContext 在 Provider 外抛错', () => {
    const Outside = () => {
      useSettingsContext()
      return null
    }
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {})
    expect(() => render(<Outside />)).toThrow('useSettingsContext must be used within a SettingsProvider')
    spy.mockRestore()
  })

  it('normalizeSettings：过小间隔回退、显式 false 开关保留、light 显式值', () => {
    localStorage.setItem('cockpit.ui.settings', JSON.stringify({
      siteName: 'Keep', refreshInterval: 3,
      enableNotifications: false, compactMode: false, showResourceCount: false, theme: 'light',
    }))
    render(<SettingsProvider><Probe /></SettingsProvider>)
    expect(screen.getByTestId('interval').textContent).toBe('30')
    expect(screen.getByTestId('site').textContent).toBe('Keep')
    expect(screen.getByTestId('theme').textContent).toBe('light')
  })

  it('首个键空串落到次键；双键皆空用默认', () => {
    localStorage.setItem('cockpit.ui.settings', '')
    localStorage.setItem('cockpit:settings', JSON.stringify({ siteName: 'Second' }))
    const { unmount } = render(<SettingsProvider><Probe /></SettingsProvider>)
    expect(screen.getByTestId('site').textContent).toBe('Second')
    unmount()
    localStorage.clear()
    render(<SettingsProvider><Probe /></SettingsProvider>)
    expect(screen.getByTestId('site').textContent).toBe('Cockpit')
  })

  it('auto 主题：prefers-color-scheme dark 解析为 dark，media 变更跟随', () => {
    const listeners: Array<() => void> = []
    const orig = window.matchMedia
    window.matchMedia = ((query: string) => ({
      matches: true,
      media: query,
      onchange: null,
      addListener: () => {},
      removeListener: () => {},
      addEventListener: (_: string, cb: () => void) => { listeners.push(cb) },
      removeEventListener: () => {},
      dispatchEvent: () => false,
    })) as unknown as typeof window.matchMedia
    localStorage.setItem('cockpit.ui.settings', JSON.stringify({ theme: 'auto' }))
    const { unmount } = render(<SettingsProvider><Probe /></SettingsProvider>)
    expect(screen.getByTestId('theme').textContent).toBe('dark')
    // media change → 重新解析（matches 仍为 true → 保持 dark，走 updateResolvedTheme）
    act(() => { listeners.forEach((cb) => cb()) })
    expect(screen.getByTestId('theme').textContent).toBe('dark')
    unmount()
    window.matchMedia = orig
  })

  it('无 matchMedia：resolveTheme 回退 light，主题 effect 提前返回', () => {
    const orig = window.matchMedia
    // @ts-expect-error 故意移除 matchMedia 模拟受限环境
    delete window.matchMedia
    localStorage.setItem('cockpit.ui.settings', JSON.stringify({ theme: 'auto' }))
    const { unmount } = render(<SettingsProvider><Probe /></SettingsProvider>)
    expect(screen.getByTestId('theme').textContent).toBe('light')
    unmount()
    window.matchMedia = orig
  })
})
