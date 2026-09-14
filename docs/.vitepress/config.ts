import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'Cockpit',
  description: '个人混合基础设施控制台',
  lang: 'zh-CN',

  // GitHub Pages 使用子路径部署
  base: '/cockpit/',

  themeConfig: {
    logo: '/logo.svg',

    nav: [
      { text: '指南', link: '/guide/introduction' },
      { text: '架构', link: '/guide/architecture' },
      { text: 'GitHub', link: 'https://github.com/cuihairu/cockpit' }
    ],

    sidebar: {
      '/guide/': [
        {
          text: '入门',
          items: [
            { text: '介绍', link: '/guide/introduction' },
            { text: '快速开始', link: '/guide/getting-started' },
            { text: '核心概念', link: '/guide/concepts' }
          ]
        },
        {
          text: '架构',
          items: [
            { text: '架构与边界', link: '/guide/architecture' },
            { text: 'Agent 出口与 SD-WAN', link: '/guide/agent-egress-sdwan' },
            { text: '协议定义', link: '/guide/protocol' },
            { text: 'WebSocket 认证', link: '/websocket-token-auth' }
          ]
        },
        {
          text: '规划',
          items: [
            { text: '参考项目对比与借鉴', link: '/guide/reference-projects' },
            { text: 'P1 方案：应用部署', link: '/guide/stack-deploy-design' }
          ]
        }
      ]
    },

    socialLinks: [
      { icon: 'github', link: 'https://github.com/cuihairu/cockpit' }
    ],

    search: {
      provider: 'local'
    }
  }
})
