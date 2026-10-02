import { describe, expect, it } from 'vitest'
import {
  DEFAULT_THEME_SKIN,
  THEME_SKINS,
  buildSkinVars,
  chromeColor,
  darkenColor,
  lightenColor,
  parseHexColor,
  resolveSkinPalette,
  resolveSkinTokens,
  resolveThemeSkin,
  withAlpha,
} from './themeSkins'

// 主题皮肤数据层：key 解析、色板选择、颜色工具、CSS 变量与 antd token 生成

describe('resolveThemeSkin', () => {
  it('命中 / 非法回退 / 空值回退默认', () => {
    expect(resolveThemeSkin('eyecare').key).toBe('eyecare')
    expect(resolveThemeSkin('nope').key).toBe(DEFAULT_THEME_SKIN)
    expect(resolveThemeSkin(undefined).key).toBe(DEFAULT_THEME_SKIN)
    expect(resolveThemeSkin(null).key).toBe(DEFAULT_THEME_SKIN)
  })

  it('预设齐备：≥3 套、key 唯一、名称与说明非空、明暗档位都配齐', () => {
    expect(THEME_SKINS.length).toBeGreaterThanOrEqual(3)
    expect(new Set(THEME_SKINS.map((s) => s.key)).size).toBe(THEME_SKINS.length)
    for (const skin of THEME_SKINS) {
      expect(skin.name).toBeTruthy()
      expect(skin.description).toBeTruthy()
      for (const palette of [skin.light, skin.dark]) {
        expect(palette.bg).toBeTruthy()
        expect(palette.surface).toBeTruthy()
        expect(palette.surfaceAlt).toBeTruthy()
        expect(palette.border).toBeTruthy()
        expect(palette.borderSoft).toBeTruthy()
        expect(palette.text).toBeTruthy()
        expect(palette.textMuted).toBeTruthy()
        expect(palette.inputBg).toBeTruthy()
        expect(palette.hoverAlpha).toBeGreaterThan(0)
      }
    }
  })

  it('resolveSkinPalette：dark 取暗色板，其余取亮色板', () => {
    const skin = resolveThemeSkin('midnight')
    expect(resolveSkinPalette(skin, 'dark')).toBe(skin.dark)
    expect(resolveSkinPalette(skin, 'light')).toBe(skin.light)
  })

  it('chromeColor：浅色跟表面色，深色跟页面底色', () => {
    const skin = resolveThemeSkin('graphite')
    expect(chromeColor(skin.light, 'light')).toBe(skin.light.surface)
    expect(chromeColor(skin.dark, 'dark')).toBe(skin.dark.bg)
  })
})

describe('颜色工具', () => {
  it('parseHexColor：六位/三位展开，非 hex 返回 null', () => {
    expect(parseHexColor('#e11d8f')).toEqual([225, 29, 143])
    expect(parseHexColor('#abc')).toEqual([170, 187, 204])
    expect(parseHexColor('  #FFF  ')).toEqual([255, 255, 255])
    expect(parseHexColor('rgba(1, 2, 3, 0.4)')).toBeNull()
    expect(parseHexColor('nope')).toBeNull()
  })

  it('withAlpha：hex 转 rgba()，已是色值原样透传', () => {
    expect(withAlpha('#e11d8f', 0.1)).toBe('rgba(225, 29, 143, 0.1)')
    expect(withAlpha('rgba(1, 2, 3, 0.4)', 0.2)).toBe('rgba(1, 2, 3, 0.4)')
  })

  it('darkenColor / lightenColor：向黑白压暗提亮并做字节钳位，非 hex 透传', () => {
    expect(darkenColor('#808080', 0.5)).toBe('#404040')
    expect(lightenColor('#000000', 0.5)).toBe('#808080')
    // 钳位：提亮不超过 255，压暗不为负
    expect(lightenColor('#ffffff', 0.5)).toBe('#ffffff')
    expect(darkenColor('#102030', 2)).toBe('#000000')
    expect(darkenColor('rgb(1,2,3)', 0.2)).toBe('rgb(1,2,3)')
    expect(lightenColor('rgb(1,2,3)', 0.2)).toBe('rgb(1,2,3)')
  })
})

describe('buildSkinVars', () => {
  it('皮肤 + 档位 → CSS 变量全集，含中性色与强调色派生量', () => {
    const palette = resolveThemeSkin('eyecare').light
    const vars = buildSkinVars(resolveThemeSkin('eyecare'), 'light', '#1677ff')
    expect(vars['--cockpit-bg']).toBe(palette.bg)
    expect(vars['--cockpit-surface']).toBe(resolveThemeSkin('eyecare').light.surface)
    expect(vars['--cockpit-surface-alt']).toBe(resolveThemeSkin('eyecare').light.surfaceAlt)
    expect(vars['--cockpit-border']).toBe(resolveThemeSkin('eyecare').light.border)
    expect(vars['--cockpit-border-soft']).toBe(resolveThemeSkin('eyecare').light.borderSoft)
    expect(vars['--cockpit-text']).toBe(resolveThemeSkin('eyecare').light.text)
    expect(vars['--cockpit-text-muted']).toBe(resolveThemeSkin('eyecare').light.textMuted)
    expect(vars['--cockpit-input-bg']).toBe(resolveThemeSkin('eyecare').light.inputBg)
    // 浅色档：顶栏跟表面色；hover 强调色 tint 随皮肤透明度 × 主题色
    expect(vars['--cockpit-chrome']).toBe(resolveThemeSkin('eyecare').light.surface)
    expect(vars['--cockpit-hover']).toBe('rgba(22, 119, 255, 0.07)')
    expect(vars['--cockpit-primary']).toBe('#1677ff')
    expect(vars['--cockpit-primary-hover']).toBe(darkenColor('#1677ff', 0.18))
    expect(vars['--cockpit-primary-strong']).toBe(darkenColor('#1677ff', 0.24))
    expect(vars['--cockpit-primary-soft']).toBe('rgba(22, 119, 255, 0.1)')
    expect(vars['--cockpit-primary-faint']).toBe('rgba(22, 119, 255, 0.06)')
    expect(vars['--cockpit-primary-border']).toBe('rgba(22, 119, 255, 0.2)')
  })

  it('深色档：顶栏跟页面底色，主色派生量改提亮', () => {
    const vars = buildSkinVars(resolveThemeSkin('default'), 'dark', '#e11d8f')
    expect(vars['--cockpit-chrome']).toBe(resolveThemeSkin('default').dark.bg)
    expect(vars['--cockpit-primary-hover']).toBe(lightenColor('#e11d8f', 0.18))
    expect(vars['--cockpit-primary-strong']).toBe(lightenColor('#e11d8f', 0.42))
    expect(vars['--cockpit-hover']).toBe('rgba(225, 29, 143, 0.05)')
  })
})

describe('resolveSkinTokens', () => {
  it('与 CSS 变量同源：中性色取色板，hover 取强调色 tint', () => {
    const tokens = resolveSkinTokens('midnight', 'dark', '#13c2c2')
    const palette = resolveThemeSkin('midnight').dark
    expect(tokens.bg).toBe(palette.bg)
    expect(tokens.surface).toBe(palette.surface)
    expect(tokens.surfaceAlt).toBe(palette.surfaceAlt)
    expect(tokens.border).toBe(palette.border)
    expect(tokens.borderSoft).toBe(palette.borderSoft)
    expect(tokens.text).toBe(palette.text)
    expect(tokens.textMuted).toBe(palette.textMuted)
    expect(tokens.inputBg).toBe(palette.inputBg)
    expect(tokens.chrome).toBe(palette.bg)
    expect(tokens.hover).toBe('rgba(19, 194, 194, 0.08)')
  })

  it('非法皮肤 key 回退默认皮肤色板', () => {
    const tokens = resolveSkinTokens('unknown-skin', 'light', '#e11d8f')
    expect(tokens.bg).toBe(resolveThemeSkin(DEFAULT_THEME_SKIN).light.bg)
    expect(resolveSkinTokens(undefined, 'light', '#e11d8f').surface).toBe(resolveThemeSkin(DEFAULT_THEME_SKIN).light.surface)
  })
})