import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import { fireEvent } from '@testing-library/react'
import { SettingsProvider } from '@/contexts/SettingsContext'
import { DEFAULT_THEME_SKIN, THEME_SKINS, parseHexColor, resolveSkinPalette, resolveThemeSkin } from '@/theme/themeSkins'
import ThemeSkinCards from './index'

// 皮肤卡片：缩略图用该皮肤自身色板现画、点选即写设置、跟随明暗档位

const Probe = () => <span data-testid="probe" />

const renderCards = () =>
  render(
    <SettingsProvider>
      <ThemeSkinCards />
      <Probe />
    </SettingsProvider>,
  )

// jsdom 把内联 hex 归一为 rgb()，比对前先转
const asRgb = (hex: string): string => {
  const [r, g, b] = parseHexColor(hex) as [number, number, number]
  return `rgb(${r}, ${g}, ${b})`
}

const inlineStyles = () =>
  Array.from(document.querySelectorAll<HTMLElement>('[style]')).map((el) => el.getAttribute('style') ?? '')

describe('ThemeSkinCards', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('渲染全部皮肤卡，选中项 aria-checked 且打勾', () => {
    renderCards()
    for (const skin of THEME_SKINS) {
      expect(screen.getByRole('radio', { name: `主题皮肤 ${skin.name}` })).toBeTruthy()
    }
    const active = screen.getByRole('radio', {
      name: `主题皮肤 ${resolveThemeSkin(DEFAULT_THEME_SKIN).name}`,
    })
    expect(active.getAttribute('aria-checked')).toBe('true')
    expect(active.textContent).toContain('冷中性灰，历史默认观感')
  })

  it('点选卡片即换皮肤：更新 data-skin 标记并持久化 localStorage', () => {
    renderCards()
    fireEvent.click(screen.getByRole('radio', { name: `主题皮肤 ${THEME_SKINS[2].name}` }))
    expect(document.documentElement.getAttribute('data-skin')).toBe(THEME_SKINS[2].key)
    expect(screen.getByRole('radio', { name: `主题皮肤 ${THEME_SKINS[2].name}` }).getAttribute('aria-checked')).toBe('true')
    const stored = JSON.parse(localStorage.getItem('cockpit.ui.settings') ?? '{}')
    expect(stored.themeSkin).toBe(THEME_SKINS[2].key)
    // 中性色板变量随之改写（整套切换，而非只换一个强调色）
    expect(document.documentElement.style.getPropertyValue('--cockpit-bg')).toBe(
      resolveThemeSkin(THEME_SKINS[2].key).light.bg,
    )
  })

  it('缩略图按皮肤自身色板现画，且跟随明暗档位', () => {
    const { unmount } = renderCards()
    // 未选中的皮肤也能预览：缩略图用各自色板内联绘制，不依赖当前生效的 CSS 变量
    const midnight = resolveThemeSkin('midnight')
    const styles = inlineStyles()
    expect(styles.some((s) => s.includes(asRgb(midnight.light.border)))).toBe(true)
    // 当前为浅色档：深夜暗色板的主文字色不应出现在预览里
    expect(styles.some((s) => s.includes(asRgb(midnight.dark.text)))).toBe(false)
    unmount()

    localStorage.setItem('cockpit.ui.settings', JSON.stringify({ theme: 'dark', themeSkin: 'graphite' }))
    localStorage.setItem('cockpit:settings', JSON.stringify({ theme: 'dark', themeSkin: 'graphite' }))
    renderCards()
    const palette = resolveSkinPalette(resolveThemeSkin('graphite'), 'dark')
    expect(document.documentElement.style.getPropertyValue('--cockpit-bg')).toBe(palette.bg)
    expect(inlineStyles().some((s) => s.includes(asRgb(palette.text)))).toBe(true)
    expect(screen.getByRole('radio', { name: `主题皮肤 ${resolveThemeSkin('graphite').name}` }).getAttribute('aria-checked')).toBe('true')
  })
})