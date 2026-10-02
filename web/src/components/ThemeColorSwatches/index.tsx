import { Button, Popover } from 'antd'
import { BgColorsOutlined, CheckOutlined } from '@ant-design/icons'
import { useSettingsContext } from '@/contexts/useSettingsContext'
import { THEME_PRESETS, resolveThemePreset } from '@/theme/themePresets'

// 顶栏主题色切换：色块九宫格，点选即全局换色（localStorage 持久化）。
// 与设置页「显示设置 → 主题色」写同一份 UISettings，两处入口等效。
export const ThemeColorSwatches = () => {
  const { settings, updateSettings } = useSettingsContext()
  const current = resolveThemePreset(settings.themeColor)

  const grid = (
    <div
      style={{
        display: 'grid',
        gridTemplateColumns: 'repeat(3, 30px)',
        gap: 10,
        padding: 4,
      }}
    >
      {THEME_PRESETS.map((preset) => {
        const active = preset.key === current.key
        return (
          <button
            key={preset.key}
            type="button"
            aria-label={`主题色 ${preset.name}`}
            title={preset.name}
            onClick={() => updateSettings({ themeColor: preset.key })}
            style={{
              position: 'relative',
              width: 30,
              height: 30,
              borderRadius: 6,
              border: 'none',
              cursor: 'pointer',
              background: preset.color,
              outline: active ? `2px solid ${preset.color}` : 'none',
              outlineOffset: 2,
            }}
          >
            {active && (
              <CheckOutlined style={{ color: '#fff', fontSize: 14, position: 'absolute', inset: 0, margin: 'auto' }} />
            )}
          </button>
        )
      })}
    </div>
  )

  return (
    <Popover content={grid} title="主题色" trigger="click" placement="bottomRight">
      <Button
        type="text"
        aria-label="切换主题色"
        icon={<BgColorsOutlined style={{ color: current.color }} />}
      />
    </Popover>
  )
}
