import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'Cockpit',
  description: '个人混合基础设施控制台',
  lang: 'zh-CN',

  // GitHub Pages 使用子路径部署
  base: '/cockpit/',

  // 忽略死链检查（部分文档尚未完成）
  ignoreDeadLinks: true,

  themeConfig: {
    logo: '/logo.svg',

    nav: [
      { text: '指南', link: '/guide/introduction' },
      { text: '参考', link: '/reference/schema' },
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
          text: '部署',
          items: [
            { text: 'Server 部署', link: '/guide/deploy-server' },
            { text: 'Agent 部署', link: '/guide/deploy-agent' },
            { text: '网络配置', link: '/guide/networking' }
          ]
        },
        {
          text: '配置',
          items: [
            { text: '资产定义', link: '/guide/inventory' },
            { text: '监控配置', link: '/guide/monitoring' },
            { text: '第三方集成', link: '/guide/integrations' }
          ]
        },
        {
          text: '开发',
          items: [
            { text: '架构设计', link: '/guide/architecture' },
            { text: '协议定义', link: '/guide/protocol' },
            { text: '贡献指南', link: '/guide/contributing' }
          ]
        }
      ],
      '/reference/': [
        {
          text: '参考',
          items: [
            { text: 'Schema 定义', link: '/reference/schema' },
            { text: '配置选项', link: '/reference/config' },
            { text: 'CLI 命令', link: '/reference/cli' },
            { text: 'API 文档', link: '/reference/api' }
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
