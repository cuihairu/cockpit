import { CheckOutlined } from '@ant-design/icons'
import { useSettingsContext } from '@/contexts/useSettingsContext'
import { resolveThemePreset } from '@/theme/themePresets'
import {
  THEME_SKINS,
  chromeColor,
  resolveSkinPalette,
  resolveThemeSkin,
  type SkinMode,
} from '@/theme/themeSkins'

// 主题皮肤卡片：设置页「显示设置 → 主题皮肤」入口。
// 每张卡内嵌一张按该皮肤自身色板绘制的缩略图（不依赖当前生效的 CSS 变量，
// 否则未选中的皮肤无从预览），点选即换（写 UISettings → localStorage + <html> 变量，
// 无需刷新）。预览跟随当前明暗档位，所见即切换后的效果。
export const ThemeSkinCards = () => {
  const { settings, updateSettings, resolvedTheme } = useSettingsContext()
  const current = resolveThemeSkin(settings.themeSkin)
  const primary = resolveThemePreset(settings.themeColor).color

  return (
    <div
      role="radiogroup"
      aria-label="主题皮肤"
      style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(150px, 1fr))', gap: 12 }}
    >
      {THEME_SKINS.map((skin) => {
        const active = skin.key === current.key
        const p = resolveSkinPalette(skin, resolvedTheme as SkinMode)
        const chrome = chromeColor(p, resolvedTheme as SkinMode)
        return (
          <button
            key={skin.key}
            type="button"
            role="radio"
            aria-checked={active}
            aria-label={`主题皮肤 ${skin.name}`}
            title={skin.description}
            onClick={() => updateSettings({ themeSkin: skin.key })}
            style={{
              position: 'relative',
              padding: 8,
              textAlign: 'left',
              cursor: 'pointer',
              borderRadius: 6,
              border: `1px solid ${active ? p.text : p.border}`,
              background: p.bg,
              boxShadow: active ? `0 0 0 2px ${primary}` : 'none',
            }}
          >
            {/* 缩略图：顶栏 + 侧栏 + 一张卡 + 行分隔，用皮肤自身色板现画 */}
            <div
              aria-hidden="true"
              style={{
                display: 'flex',
                height: 56,
                borderRadius: 3,
                overflow: 'hidden',
                border: `1px solid ${p.border}`,
                background: p.bg,
              }}
            >
              <div style={{ width: 20, background: chrome, borderRight: `1px solid ${p.border}` }} />
              <div style={{ flex: 1, display: 'flex', flexDirection: 'column' }}>
                <div style={{ height: 10, background: chrome, borderBottom: `1px solid ${p.border}` }} />
                <div style={{ flex: 1, padding: 4 }}>
                  <div
                    style={{
                      height: '100%',
                      background: p.surface,
                      border: `1px solid ${p.border}`,
                      borderRadius: 2,
                      padding: 3,
                      display: 'flex',
                      flexDirection: 'column',
                      gap: 3,
                    }}
                  >
                    <div style={{ height: 3, width: '72%', background: p.text, opacity: 0.75 }} />
                    <div
                      style={{
                        height: 3,
                        width: '46%',
                        background: p.textMuted,
                      }}
                    />
                    <div
                      style={{
                        height: 3,
                        width: '88%',
                        background: p.borderSoft,
                        borderTop: `1px solid ${p.borderSoft}`,
                      }}
                    />
                    <div style={{ flex: 1, background: primary, opacity: 0.85, borderRadius: 1, width: 22 }} />
                  </div>
                </div>
              </div>
            </div>
            <div style={{ marginTop: 6, color: p.text, fontSize: 13, fontWeight: active ? 600 : 500 }}>
              {skin.name}
            </div>
            <div style={{ color: p.textMuted, fontSize: 11, marginTop: 1 }}>{skin.description}</div>
            {active && (
              <CheckOutlined
                aria-hidden="true"
                style={{ position: 'absolute', top: 10, right: 12, color: primary, fontSize: 14 }}
              />
            )}
          </button>
        )
      })}
    </div>
  )
}

export default ThemeSkinCards