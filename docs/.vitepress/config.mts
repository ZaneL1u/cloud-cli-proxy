import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'Cloud CLI Proxy',
  description: '一条命令获取预装 Claude Code 的云主机，所有流量走指定出口 IP，零泄漏',

  base: '/cloud-cli-proxy/',

  head: [
    ['meta', { name: 'keywords', content: 'cloud cli proxy, ssh, docker, sing-box, claude code, egress ip, proxy, containerized' }],
    ['link', { rel: 'icon', href: '/cloud-cli-proxy/logo.svg', type: 'image/svg+xml' }],
  ],

  locales: {
    zh: {
      label: '中文',
      lang: 'zh-CN',
      link: '/zh/',
      themeConfig: {
        nav: [
          { text: '指南', link: '/zh/guide/quickstart' },
          { text: '参考', link: '/zh/reference/api' },
        ],
        sidebar: {
          '/zh/guide/': [
            {
              text: '指南',
              items: [
                { text: '快速开始', link: '/zh/guide/quickstart' },
                { text: '部署指南', link: '/zh/guide/deployment' },
                { text: '配置参考', link: '/zh/guide/configuration' },
                { text: '架构说明', link: '/zh/guide/architecture' },
              ],
            },
          ],
          '/zh/reference/': [
            {
              text: '参考',
              items: [
                { text: 'API 参考', link: '/zh/reference/api' },
                { text: '故障排查', link: '/zh/reference/faq' },
              ],
            },
          ],
        },
        outline: { label: '目录' },
        docFooter: { prev: '上一页', next: '下一页' },
      },
    },
    en: {
      label: 'English',
      lang: 'en-US',
      link: '/en/',
      description: 'One-command cloud host with Claude Code pre-installed. All traffic through your exit IP. Zero leaks.',
      themeConfig: {
        nav: [
          { text: 'Guide', link: '/en/guide/quickstart' },
          { text: 'Reference', link: '/en/reference/api' },
        ],
        sidebar: {
          '/en/guide/': [
            {
              text: 'Guide',
              items: [
                { text: 'Quick Start', link: '/en/guide/quickstart' },
                { text: 'Deployment', link: '/en/guide/deployment' },
                { text: 'Configuration', link: '/en/guide/configuration' },
                { text: 'Architecture', link: '/en/guide/architecture' },
              ],
            },
          ],
          '/en/reference/': [
            {
              text: 'Reference',
              items: [
                { text: 'API Reference', link: '/en/reference/api' },
                { text: 'FAQ & Recovery', link: '/en/reference/faq' },
              ],
            },
          ],
        },
      },
    },
  },

  themeConfig: {
    logo: '/logo.svg',
    socialLinks: [
      { icon: 'github', link: 'https://github.com/ZaneL1u/cloud-cli-proxy' },
    ],
    search: {
      provider: 'local',
    },
  },
})
