# Cockpit UI Design Baseline

> Trigger: `docs/review/2026-10-31-ui-audit-impeccable.md` T7（无 DESIGN.md / PRODUCT.md，审核无设计基准可比对）。
> This document is the **baseline future UI audits are measured against**. It states the
> color *usage* and *contrast* rules that the palette inventory alone does not spell out.
> The authoritative palette values live in `docs/ui-theme-design.md` (skins + accents);
> this document does not duplicate that table, it references it and adds the rules.

## 1. Scope

- Surface: `web/` (React 19 + AntD 5 + ProComponents). Mobile follows the same skin data
  (`mobile/lib/theme/app_theme.dart`) but contrast rules here target the Web surface.
- Basis for all ratios: WCAG 2.x relative luminance; "AA" below means 4.5:1 (normal text),
  3:1 (large text ≥ 28px, or ≥ 18.66px bold).

## 2. 色板 (Palette)

- **Accent / theme color**: the 6 presets in `web/src/theme/themePresets.ts`
  (`rose #e11d8f`, `blue #1677ff`, `cyan #13c2c2`, `green #16a34a`, `purple #722ed1`,
  `orange #fa8c16`); default = rose `#e11d8f`. Drives buttons/tags/selected states/links
  via `--cockpit-primary*` and the antd `colorPrimary` token.
- **Neutral skins**: 4 skins × light/dark (`default`, `midnight`, `eyecare`, `graphite`);
  fields `bg / surface / surfaceAlt / border / borderSoft / text / textMuted / inputBg`.
  Full value tables: `docs/ui-theme-design.md` §2. Default = `default` (Mist White).
- **Semantic status colors** (success / warning / error): `web/src/App.less` declares
  `@success-color #22c55e`, `@warning-color #f59e0b`, `@error-color #ef4444`; many
  components instead use the antd defaults `#52c41a` / `#faad14` / `#ff4d4f`. Dashboard
  stat numbers use `#00b42a` (green), `#ff7d00` (orange), `#13c2c2` (cyan).

## 3. 文字色规范 (Text color rules)

| Role | Token / source | Rule |
| --- | --- | --- |
| 主要文字 | skin `text` (e.g. `#0f172a`) | ≥ 4.5:1 on its `surface`; never below |
| 次要文字 | skin `textMuted` (e.g. `#64748b`) | **≥ 4.5:1** on its `surface` — column headers, helper notes, stat labels, timestamps. The audit's F3/F4 fixes standardized several spots on `#667085` (4.97:1 on `#fff`); the token is the baseline |
| 占位色 | antd `colorTextPlaceholder` = `skin.textMuted` | **≥ 4.5:1** — a placeholder often *is* the only label of a field, so it is **not** WCAG-exempt (audit T1). AntD's default `rgba(0,0,0,0.25)` is only 1.84:1 and must not be used |

Neutral-text invariants (never relax):

- No text or placeholder may sit below 4.5:1 on its own surface (disabled controls are the
  sole WCAG exemption, and even then a field-identifying placeholder is not exempt).
- Secondary text is one step lighter than primary, but never below AA. `#86909c` (3.24:1)
  and `#999` (2.85:1) are retired.

## 4. 语义色规范 (Semantic color rules)

Status colors have a **dual role**, and the contrast requirement depends on the role:

1. **Fill / icon / tag / badge** (the semantic color as a filled chip, icon, or dot):
   exempt from text AA rules — these are graphical objects.
2. **Text on a light surface**: the raw semantic color is usually *below* AA, so use a
   **darker same-hue variant** that meets ≥ 4.5:1 (small) / ≥ 3:1 (large).

Raw on-`#fff` ratios of the colors in use (all fail 4.5:1 except brand pink, marginal):

| Color | Where | on `#fff` |
| --- | --- | --- |
| `#e11d8f` | brand pink (default accent) | 4.40:1 |
| `#1677ff` | brand blue | 4.10:1 |
| `#d48806` | gold tag 「内置」 | 2.87:1 |
| `#00b42a` | dashboard stat green | 2.78:1 |
| `#13c2c2` | dashboard stat cyan | 2.21:1 |
| `#ff7d00` | dashboard stat orange | 2.57:1 |
| `#ff4d4f` | error / 「清理离线 agent」 | 3.27:1 |
| `#52c41a` | antd success | 2.27:1 |
| `#faad14` | antd warning | 1.90:1 |
| `#ef4444` | `@error-color` | 3.76:1 |

**Rule for role 2**: pick a darker same-hue variant. Verified candidates that pass on `#fff`:
`#c41d7f` (pink, 5.48:1), `#0958d9` (blue, 6.16:1), `#cf1322` (red, 5.57:1),
`#d4380d` (orange, 4.80:1), `#ad6800` (gold, 4.41:1). Green/cyan need a deeper variant
than these; the exact green/cyan text values are left to the T3 design decision.

> **Open decision**: audit T3 (brand/semantic on-white text contrast) is 待拍板 — applying
> these darker text variants across the affected spots is a separate design change, not done
> here. This document only establishes the *rule*; the raw colors stay as the fill/icon set.

## 5. 触控目标 (Touch targets)

- WCAG 2.5.8 minimum **24 × 24px**. Audit T6 found several antd small controls below it
  (42×21 buttons, 1×15 separators, 26×21 tabs, 21px numeric inputs) — informational for
  now; main interactive controls should reach ≥ 24px height (`size` / `padding`), to be
  confirmed once touch usage is the primary target.

## 6. 布局 (Layout invariants)

- No page-level horizontal overflow at 390px viewport (desktop 1440px too). Dense tables
  must set the `scroll` prop with `x: 'max-content'` and page toolbars `wrap`
  (audit F1/F2/T5).
- Every interactive control needs an accessible name (audit F6: header search button).

## 7. Audit checklist (how to re-audit)

1. Contrast: every text/placeholder ≥ 4.5:1 (≥ 3:1 large) on its surface — scan at
   1440×900 and 390×844, light + dark.
2. Overflow: `scrollWidth <= viewport` for all 22 routes at 390px, including loading state.
3. Names: no unnamed interactive controls (buttons, switches, numeric inputs).
4. Touch: primary actions ≥ 24px tall.

## 8. Related files

| File | Responsibility |
| --- | --- |
| `docs/ui-theme-design.md` | Authoritative palette (skins, accents, CSS var / token wiring) |
| `web/src/theme/themePresets.ts` | Accent color presets |
| `web/src/theme/themeSkins.ts` | Skin data + token/CSS-var generation |
| `web/src/App.less` | Neutral var references + `@success/warning/error-color` |
| `web/src/App.tsx` | ConfigProvider tokens (`colorTextPlaceholder` etc.) |
| `docs/review/2026-10-31-ui-audit-impeccable.md` | The audit this baseline answers |
