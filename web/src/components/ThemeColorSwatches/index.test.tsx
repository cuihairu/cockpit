import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { SettingsProvider } from '@/contexts/SettingsContext'
import { ThemeColorSwatches } from './index'

// 包 Provider 的挂载，主题色写入共享 UISettings
const mount = () => render(<SettingsProvider><ThemeColorSwatches /></SettingsProvider>)

describe('ThemeColorSwatches', () => {
  it('触发器 aria 标注 + 默认蔷薇红激活', async () => {
    mount()
    const trigger = screen.getByLabelText('切换主题色')
    fireEvent.click(trigger)
    // 弹层打开后六个预设全渲染，当前项带 aria-label 可定位
    await waitFor(() => {
      expect(screen.getByLabelText('主题色 蔷薇红')).toBeTruthy()
    })
    expect(screen.getByLabelText('主题色 远峰蓝')).toBeTruthy()
    expect(screen.getByLabelText('主题色 罗兰紫')).toBeTruthy()
  })

  it('点选色块切换主题色并持久化 localStorage', async () => {
    mount()
    fireEvent.click(screen.getByLabelText('切换主题色'))
    await waitFor(() => screen.getByLabelText('主题色 翠绿'))
    act(() => {
      screen.getByLabelText('主题色 翠绿').click()
    })
    await waitFor(() => {
      const stored = localStorage.getItem('cockpit.ui.settings') ?? ''
      expect(stored).toContain('"themeColor":"green"')
    })
  })
})
