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
})
