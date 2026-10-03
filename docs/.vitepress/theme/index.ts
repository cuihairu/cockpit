// cockpit docs theme — 默认主题 + 品牌配色（custom.css 见同目录）。
// VitePress 会自动发现本文件，无需在 config.mts 里引用。
// home-hero-after 插槽：首页 hero 下方挂「界面预览」走马灯（Screenshots.vue）。
import { h } from 'vue'
import type { Theme } from 'vitepress'
import DefaultTheme from 'vitepress/theme'
import Screenshots from './Screenshots.vue'
import './custom.css'

export default {
  extends: DefaultTheme,
  Layout: () =>
    h(DefaultTheme.Layout, null, {
      'home-hero-after': () => h(Screenshots),
    }),
} satisfies Theme
