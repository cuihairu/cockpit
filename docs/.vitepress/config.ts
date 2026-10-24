import { withMermaid } from "vitepress-plugin-mermaid";
import { defineConfig } from 'vitepress'

export default withMermaid(defineConfig({
  title: 'Cockpit',
  description: 'Server + Agent console for a lightweight resource view, monitoring and remote access',

  // 审核报告与整改回执是内部追溯材料，不作为公开文档站页面构建。
  srcExclude: ['review/**'],

  // GitHub Pages 使用子路径部署
  base: '/cockpit/',

  // VitePress 官方多语言：英文在根目录，中文在 /zh/。
  locales: {
    root: { label: "English", lang: "en-US" },
    zh:   { label: "简体中文", lang: "zh-CN", link: "/zh/" }
  },

  themeConfig: {
    logo: '/logo.svg',

    locales: {
      root: {
        nav: [
          { text: 'Guide', link: '/guide/introduction' },
          { text: 'Architecture', link: '/guide/architecture' },
          { text: 'Research', link: '/research/ops-oss-survey' }
        ],

        sidebar: {
          '/guide/': [
            {
              text: 'Getting Started',
              items: [
                { text: 'Introduction', link: '/guide/introduction' },
                { text: 'Quick Start', link: '/guide/getting-started' },
                { text: 'Core Concepts', link: '/guide/concepts' }
              ]
            },
            {
              text: 'Architecture',
              items: [
                { text: 'Architecture and Boundaries', link: '/guide/architecture' },
                { text: 'Agent Egress and SD-WAN', link: '/guide/agent-egress-sdwan' },
                { text: 'Protocol', link: '/guide/protocol' },
                { text: 'WebSocket Token Auth', link: '/websocket-token-auth' },
                { text: 'Decision Log', link: '/guide/decision-log' }
              ]
            },
            {
              text: 'Planning',
              items: [
                { text: 'Reference Projects', link: '/guide/reference-projects' },
                { text: 'Acceptance Checklist', link: '/guide/acceptance-checklist' },
                { text: 'Mobile Plan: Flutter', link: '/guide/mobile-design' },
                { text: 'P1 Plan: Stack Deploy', link: '/guide/stack-deploy-design' },
                { text: 'OpenWrt Agent Packaging', link: '/guide/openwrt-agent-design' },
                { text: 'Windows Agent Packaging', link: '/guide/windows-agent-design' },
                { text: 'Service Health and Self-healing', link: '/guide/service-health-design' },
                { text: 'Job Model', link: '/guide/jobs-design' },
                { text: 'Workflow and Async Jobs', link: '/guide/workflow-design' }
              ]
            }
          ],
          '/design/': [
            {
              text: 'Design',
              items: [
                { text: 'agent-core 3-Layer Architecture', link: '/design/agent-core-architecture' },
                { text: 'External Session API (memex)', link: '/design/external-session-api' }
              ]
            }
          ],
          '/operations/': [
            {
              text: 'Operations',
              items: [
                { text: 'Docker Deployment', link: '/operations/deploy-docker' }
              ]
            }
          ],
          '/research/': [
            {
              text: 'Research',
              items: [
                { text: 'Ops OSS Survey', link: '/research/ops-oss-survey' }
              ]
            }
          ]
        }
      },

      zh: {
        nav: [
          { text: '指南', link: '/zh/guide/introduction' },
          { text: '架构', link: '/zh/guide/architecture' },
          { text: '调研', link: '/zh/research/ops-oss-survey' }
        ],

        sidebar: {
          '/zh/guide/': [
            {
              text: '入门',
              items: [
                { text: '介绍', link: '/zh/guide/introduction' },
                { text: '快速开始', link: '/zh/guide/getting-started' },
                { text: '核心概念', link: '/zh/guide/concepts' }
              ]
            },
            {
              text: '架构',
              items: [
                { text: '架构与边界', link: '/zh/guide/architecture' },
                { text: 'Agent 出口与 SD-WAN', link: '/zh/guide/agent-egress-sdwan' },
                { text: '协议定义', link: '/zh/guide/protocol' },
                { text: 'WebSocket 认证', link: '/zh/websocket-token-auth' },
                { text: '架构决策记录', link: '/zh/guide/decision-log' }
              ]
            },
            {
              text: '规划',
              items: [
                { text: '参考项目对比与借鉴', link: '/zh/guide/reference-projects' },
                { text: '真机验收清单', link: '/zh/guide/acceptance-checklist' },
                { text: '移动端方案：Flutter', link: '/zh/guide/mobile-design' },
                { text: 'P1 方案：应用部署', link: '/zh/guide/stack-deploy-design' },
                { text: 'OpenWrt Agent 打包设计', link: '/zh/guide/openwrt-agent-design' },
                { text: 'Windows Agent 打包设计', link: '/zh/guide/windows-agent-design' },
                { text: '服务健康探针与自愈设计', link: '/zh/guide/service-health-design' },
                { text: '执行 Job 模型设计', link: '/zh/guide/jobs-design' },
                { text: 'Workflow 与异步 Job 设计', link: '/zh/guide/workflow-design' }
              ]
            }
          ],
          '/zh/design/': [
            {
              text: '设计',
              items: [
                { text: 'agent-core 三层架构（core/平台层/插件）', link: '/zh/design/agent-core-architecture' },
                { text: '外部新建远程会话接口契约（memex）', link: '/zh/design/external-session-api' }
              ]
            }
          ],
          '/zh/operations/': [
            {
              text: '运维',
              items: [
                { text: 'Docker 部署', link: '/zh/operations/deploy-docker' }
              ]
            }
          ],
          '/zh/research/': [
            {
              text: '调研',
              items: [
                { text: '运维开源方案调研', link: '/zh/research/ops-oss-survey' }
              ]
            }
          ]
        }
      }
    },

    socialLinks: [
      { icon: 'github', link: 'https://github.com/cuihairu/cockpit' }
    ],

    search: {
      provider: 'local'
    }
  }
}));
