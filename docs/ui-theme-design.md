# UI Theme and Skin


Web and mobile each have their own set of UI appearance settings. This document is the implementation baseline of the appearance system: the color tables here are authoritative; when a constant in the code disagrees with this document, this document wins and the code is updated to match.

## 1. Two orthogonal dimensions

Appearance is a combination of two **mutually independent** dimensions that users can pair freely:

| Dimension | Value source | What it determines | Field |
| --- | --- | --- | --- |
| **Theme color** (accent color) | Preset palettes | The hue of **accent elements** such as buttons, tags, selected states, and links | `themeColor` |
| **Theme skin** (neutral palette) | 4 skin presets | The **neutral colors** of page background, panel surface, border, and text, plus the overall warm/cool tone | `themeSkin` |

On top of that comes a light/dark mode (`light` / `dark` / `auto`, where `auto` follows the system's `prefers-color-scheme`).
The combination space = theme color × skin × mode; every combination is valid.

Why the split is designed this way: changing the accent color must not move the full-screen background color, and changing the background color must not drag the button hue along. Historically the Web side hard-coded many neutral colors in `App.less` and overrode them wholesale under
`html[data-theme='dark']`, so adding a third background color meant copying another override block;
now that palettes are data, light/dark and skin are just different values of the same set of CSS variables.

## 2. Skin inventory

Four skins, each containing a light / dark pair of palettes:

| key | Name | Positioning |
| --- | --- | --- |
| `default` | Default · Mist White | Cool neutral gray; the historical default look |
| `midnight` | Deep · Night Flight | Indigo night sky; ops-console character |
| `eyecare` | Eye Care · Rice Paper | Warm rice-paper tone; easy on the eyes over long viewing |
| `graphite` | Graphite · Obsidian | Zero-hue pure gray; the most conservative background |

`default` is exactly the pre-rework look and serves as the fallback baseline: whenever `themeSkin` resolves to an illegal value it falls back to
`default` (`resolveThemeSkin` on Web, `resolveAppSkin` on mobile).

### Color value tables

Palette field meanings (same names on both ends; exceptions noted):

- `bg` page background color
- `surface` card / panel surface
- `surfaceAlt` secondary surface (table headers, striped rows)
- `border` primary border
- `borderSoft` even weaker separator (table row dividers)
- `text` primary text
- `textMuted` secondary text (column headers, helper notes)

#### default · Mist White

| Field | light | dark |
| --- | --- | --- |
| bg | `#f8fafc` | `#0c0e14` |
| surface | `#ffffff` | `#141620` |
| surfaceAlt | `#ffffff` | `#141620` |
| border | `#e2e8f0` | `rgba(255, 255, 255, 0.06)` |
| borderSoft | `#e2e8f0` | `rgba(255, 255, 255, 0.04)` |
| text | `#0f172a` | `#e2e8f0` |
| textMuted | `#64748b` | `#94a3b8` |
| inputBg | `#ffffff` | `#141620` |

#### midnight · Night Flight

| Field | light | dark |
| --- | --- | --- |
| bg | `#f2f5fa` | `#070d1a` |
| surface | `#ffffff` | `#0e1729` |
| surfaceAlt | `#f7f9fc` | `#101b30` |
| border | `#dbe3ee` | `rgba(148, 180, 255, 0.14)` |
| borderSoft | `#e8edf5` | `rgba(148, 180, 255, 0.09)` |
| text | `#16233b` | `#dbe7ff` |
| textMuted | `#5f7192` | `#8296bd` |
| inputBg | `#ffffff` | `#0e1729` |

#### eyecare · Rice Paper

| Field | light | dark |
| --- | --- | --- |
| bg | `#f4efe4` | `#1a1712` |
| surface | `#fbf7ee` | `#241f19` |
| surfaceAlt | `#f7f2e7` | `#272119` |
| border | `#ded2ba` | `rgba(226, 205, 168, 0.18)` |
| borderSoft | `#e7ddc9` | `rgba(226, 205, 168, 0.11)` |
| text | `#3b342a` | `#e8dfcd` |
| textMuted | `#7b6f5b` | `#a3937a` |
| inputBg | `#fbf7ee` | `#241f19` |

#### graphite · Obsidian

| Field | light | dark |
| --- | --- | --- |
| bg | `#f4f4f5` | `#131313` |
| surface | `#ffffff` | `#1c1c1e` |
| surfaceAlt | `#fafafa` | `#1f1f21` |
| border | `#e4e4e7` | `rgba(255, 255, 255, 0.1)` |
| borderSoft | `#ededf0` | `rgba(255, 255, 255, 0.06)` |
| text | `#18181b` | `#ededf0` |
| textMuted | `#71717a` | `#9b9ba3` |
| inputBg | `#ffffff` | `#1c1c1e` |

### Top bar / side bar background

The top and side bar background is not given directly by the skin but derived from `bg` and
`surface` per mode
(`chromeColor`): the light mode takes `surface` (a white panel floating over a gray base, giving
depth), the dark mode takes `bg` (one color across the whole screen, avoiding the double-border
feel in dark mode).

### Hover tint strength

The hover-state background is the **theme color** overlaid at a skin-given opacity
(`hoverAlpha`; on Web every skin has its own light/dark value in the 0.03~0.08 range). Hue
belongs to the theme color, tint strength to the skin — so a hover on warm skins never looks
muddy because of a yellowish base.

## 3. Web implementation

### CSS variables

The effect in `web/src/contexts/SettingsContext.tsx` computes the full variable set from
`themeSkin` + light/dark mode +
`themeColor`, writes it to the inline style of `<html>`, and also syncs
`data-skin="<key>"` for debugging and selectors:

| Variable | Source |
| --- | --- |
| `--cockpit-bg` | palette `bg` |
| `--cockpit-surface` | palette `surface` |
| `--cockpit-surface-alt` | palette `surfaceAlt` |
| `--cockpit-border` | palette `border` |
| `--cockpit-border-soft` | palette `borderSoft` |
| `--cockpit-text` | palette `text` |
| `--cockpit-text-muted` | palette `textMuted` |
| `--cockpit-input-bg` | palette `inputBg` |
| `--cockpit-chrome` | `chromeColor` (top/side bars) |
| `--cockpit-hover` | theme color + `hoverAlpha` |
| `--cockpit-primary` | theme color, raw value |
| `--cockpit-primary-hover` | dark mode `lightenColor(primary, .18)`, light mode `darkenColor(primary, .18)` |
| `--cockpit-primary-strong` | dark mode `lightenColor(primary, .42)`, light mode `darkenColor(primary, .24)` |
| `--cockpit-primary-soft` | `withAlpha(primary, .10)` |
| `--cockpit-primary-faint` | `withAlpha(primary, .06)` |
| `--cockpit-primary-border` | `withAlpha(primary, .20)` |

Accent-derived values are computed from the theme color hex **at runtime**, so the hue always
follows the theme color preset — before the rework, menus,
tags, and buttons hard-coded rose red, which did not follow theme color switches.

The neutral colors formerly hard-coded in
`web/src/App.less` are now all `var(--cockpit-*, fallback)` references,
with the fallbacks being exactly the pre-rework hard-coded values, so the look does not change while the variables are not ready (first frame, SSR, test environments).
The wholesale `html[data-theme='dark']` override is deleted; one set of rules serves every skin × mode; only the semantic colors
(success / warning / error) keep a brightening correction block for dark mode.

### Component library tokens

The `ConfigProvider` tokens in `web/src/App.tsx` draw on the same data
(`resolveSkinTokens`), so antd components share the same color source as the hand-written CSS: `colorBgLayout`
takes `bg`, `colorBgContainer` takes `surface`, `colorFillAlter` takes `surfaceAlt`,
`colorBorder` / `colorBorderSecondary`, `colorText` / `colorTextSecondary`,
`Layout`'s `headerBg` / `siderBg` take `chrome`, and `bodyBg` takes `bg` (page background color).

### Settings page

Inside `web/src/pages/Settings/GeneralSettings.tsx`:

- The "Theme" Select picks the theme color (the label stays "Theme", unchanged)
- "Theme skin" is rendered by `web/src/components/ThemeSkinCards` as a card group: each card's thumbnail
  is **drawn inline with that skin's own palette** (four color blocks: base / panel / border / text) and follows the current light/dark mode;
  the group is `role="radiogroup"`, each card is `role="radio"`.

Clicking a skin card **takes effect immediately** (written straight to context, not through
form save), because a skin is a pure frontend concern with an instantly visible effect —
making users click "Save" just to see it would be pointless. To keep other settings saves from
rolling it back with a stale form value, `useSettings.saveSettings` uses `values.themeSkin ?? settings.themeSkin`.

### Persistence

Web settings as a whole live in localStorage (reusing the existing settings channel); `themeSkin` is
a first-class field normalized by `SettingsContext`; after a refresh the effect replays it, so no extra API is needed.

## 4. Mobile implementation

- `mobile/lib/theme/app_theme.dart`: `AppSkin` / `SkinPalette` match the four skins in both names and values;
  `buildAppTheme(skinKey, brightness)` produces the `ThemeData`, with dark background colors taken from the `dark` palette.
- `mobile/lib/main.dart`: `MaterialApp`'s `theme` / `darkTheme` / `themeMode` are all
  wired to settings state.
- `mobile/lib/pages/settings_page.dart`: the settings page's "Theme appearance" entry opens the
  `_AppearanceSheet`, split into "light/dark mode" and "skin" sections; skin rows draw the same kind of thumbnail with `_SkinPreview`.
- `mobile/lib/state/settings.dart`: `themeMode` (`system` / `light` / `dark`) /
  `themeSkin` fields + `setThemeMode` / `setThemeSkin`, with SharedPreferences keys
  `theme_mode` / `theme_skin`. Existing installs lack these two keys and default to "follow system + default skin".

Note two Flutter 3.47 pitfalls: the `primary` produced by `ColorScheme.fromSeed` is a tonal variant,
so you must explicitly `copyWith(primary: cockpitAccent, ...)` to pin the accent back; and `RadioListTile`'s
`groupValue` / `onChanged` are deprecated — a radio group must be wrapped in `RadioGroup<T>`.

## 5. Cross-end semantic consistency

Being "in sync" means **consistent semantics**, not sharing one store:

- Skin keys, names, descriptions, and every palette color value are fully identical on both ends (section 2 of this document is the shared baseline; the sole exception: the Web-side `inputBg` palette field does not exist in the mobile SkinPalette).
  Switch skins on either end, pick the same key on the other end, and you get the same set of colors.
- The server has no persistence channel for UI settings, so each end stores locally: localStorage on Web,
  SharedPreferences on mobile. There is no real-time push where changing one end updates both at once.
- **Known difference**: mobile does not yet support custom accent colors; the accent is fixed to the Web default theme color "rose red"
  `#e11d8f` (`cockpitAccent` in `mobile/lib/theme/app_theme.dart`). Switching the theme color on Web
  does not affect mobile. Once mobile gains theme color selection, this spot needs to be updated in sync.

## 6. Process for adding a new skin

1. Add an entry with the same name and values to `THEME_SKINS` in
   `web/src/theme/themeSkins.ts` and to `appSkins` in
   `mobile/lib/theme/app_theme.dart`.
2. Add the color value table in section 2 of this document.
3. Run both ends' unit tests (`web/src/theme/themeSkins.test.ts`,
   `mobile/test/app_theme_test.dart`) to confirm the fallback and parsing behavior is unchanged.

No `App.less` changes, no new `data-theme` override block, no component code changes — the
new skin automatically appears among the options on both ends' settings pages and automatically
gets a thumbnail preview.

## 7. Related files

| File | Responsibility |
| --- | --- |
| `web/src/theme/themeSkins.ts` | Skin data layer, color utilities, CSS variable and antd token generation |
| `web/src/contexts/SettingsContext.tsx` | Settings normalization, `data-skin`, CSS variable writing |
| `web/src/App.less` | Variable references (neutral colors no longer overridden per light/dark) |
| `web/src/App.tsx` | `ConfigProvider` tokens wired to skin data |
| `web/src/components/ThemeSkinCards/index.tsx` | Settings-page skin cards + thumbnail preview |
| `web/src/pages/Settings/GeneralSettings.tsx` | Entry to the appearance block of the settings page |
| `mobile/lib/theme/app_theme.dart` | Mobile skins and `ThemeData` construction |
| `mobile/lib/state/settings.dart` | Mobile settings state and persistence |
| `mobile/lib/pages/settings_page.dart` | Mobile appearance panel |
