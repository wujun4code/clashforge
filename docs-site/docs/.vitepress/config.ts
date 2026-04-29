import { defineConfig } from "vitepress";

declare const process: { env: Record<string, string | undefined> };

const repo = "https://github.com/wujun4code/clashforge";
const base = process.env.DOCS_BASE ?? (process.env.GITHUB_ACTIONS ? "/clashforge/" : "/");

export default defineConfig({
  title: "ClashForge Docs",
  description: "A complete OpenWrt mihomo control-plane user guide.",
  base,
  lastUpdated: true,
  cleanUrls: true,
  markdown: {
    theme: {
      light: "github-light",
      dark: "github-dark"
    },
    lineNumbers: true
  },
  head: [
    ["link", { rel: "icon", type: "image/x-icon", href: "/favicon.ico" }],
    ["link", { rel: "icon", type: "image/png", sizes: "32x32", href: "/favicon-32.png" }],
    ["link", { rel: "apple-touch-icon", sizes: "256x256", href: "/apple-touch-icon.png" }],
    ["link", { rel: "preconnect", href: "https://fonts.googleapis.com" }],
    ["link", { rel: "preconnect", href: "https://fonts.gstatic.com", crossorigin: "" }],
    ["link", { rel: "stylesheet", href: "https://fonts.googleapis.com/css2?family=IBM+Plex+Sans:wght@400;500;600;700&family=JetBrains+Mono:wght@400;500;600;700&display=swap" }],
    ["meta", { name: "theme-color", content: "#7c3aed" }],
    ["meta", { property: "og:title", content: "ClashForge" }],
    ["meta", { property: "og:description", content: "在路由器上按设备定义分流规则，统一管控家庭和小型团队的网络出口。" }],
    ["meta", { property: "og:image", content: "/og-image.png" }],
    ["meta", { property: "og:type", content: "website" }],
    ["meta", { name: "twitter:card", content: "summary_large_image" }],
    ["meta", { name: "twitter:image", content: "/og-image.png" }]
  ],
  locales: {
    root: {
      label: "简体中文",
      lang: "zh-CN",
      title: "ClashForge 文档",
      description: "在路由器上按设备定义分流规则，统一管控家庭和小型团队的网络出口。支持机场订阅、Cloudflare Worker 节点、VPS 多出口并行，适合跨境团队、外贸公司、独立开发者和家庭用户。",
      themeConfig: {
        nav: [
          { text: "为什么 IP 质量很重要", link: "/guide/why" },
          { text: "快速开始", link: "/guide/quick-start" },
          { text: "不好用怎么办", link: "/guide/troubleshooting" },
          { text: "FAQ", link: "/guide/faq" },
          { text: "GitHub", link: repo }
        ],
        sidebar: [
          {
            text: "先理解，再动手",
            items: [
              { text: "为什么 IP 质量很重要", link: "/guide/why" }
            ]
          },
          {
            text: "安装和配置",
            items: [
              { text: "快速开始", link: "/guide/quick-start" },
              { text: "安装到路由器", link: "/guide/install" },
              { text: "添加代理来源", link: "/guide/config" },
              { text: "让设备开始使用", link: "/guide/run" }
            ]
          },
          {
            text: "长期稳定使用",
            items: [
              { text: "怎么知道成功了", link: "/guide/verify" },
              { text: "日常维护", link: "/guide/operations" },
              { text: "更新软件", link: "/guide/upgrade" },
              { text: "不好用怎么办", link: "/guide/troubleshooting" },
              { text: "常见问题 FAQ", link: "/guide/faq" }
            ]
          }
        ],
        editLink: {
          pattern: `${repo}/edit/main/docs-site/docs/:path`,
          text: "在 GitHub 上编辑此页"
        },
        docFooter: { prev: "上一页", next: "下一页" },
        outline: { level: [2, 3], label: "页面导航" },
        lastUpdated: { text: "最后更新" },
        returnToTopLabel: "返回顶部",
        sidebarMenuLabel: "菜单",
        darkModeSwitchLabel: "外观",
        footer: {
          message: "Released under the MIT License.",
          copyright: "Copyright © 2026 ClashForge"
        }
      }
    },
    en: {
      label: "English",
      lang: "en-US",
      link: "/en/",
      title: "ClashForge Docs",
      description: "A complete install, configuration, run, verification and troubleshooting guide.",
      themeConfig: {
        nav: [
          { text: "Quick Start", link: "/en/guide/quick-start" },
          { text: "Deploy", link: "/en/guide/install" },
          { text: "Operations", link: "/en/guide/operations" },
          { text: "FAQ", link: "/en/guide/faq" },
          { text: "GitHub", link: repo }
        ],
        sidebar: [
          {
            text: "User Guide",
            items: [
              { text: "Quick Start", link: "/en/guide/quick-start" },
              { text: "Install & Deploy", link: "/en/guide/install" },
              { text: "First Configuration", link: "/en/guide/config" },
              { text: "Run & Takeover", link: "/en/guide/run" },
              { text: "Verification", link: "/en/guide/verify" },
              { text: "Operations", link: "/en/guide/operations" },
              { text: "Upgrade & Rollback", link: "/en/guide/upgrade" },
              { text: "Troubleshooting", link: "/en/guide/troubleshooting" },
              { text: "FAQ", link: "/en/guide/faq" }
            ]
          }
        ],
        editLink: {
          pattern: `${repo}/edit/main/docs-site/docs/:path`,
          text: "Edit this page on GitHub"
        },
        docFooter: { prev: "Previous", next: "Next" },
        outline: { level: [2, 3], label: "On this page" }
      }
    }
  },
  themeConfig: {
    logo: "/logo.png",
    search: {
      provider: "local",
      options: {
        locales: {
          root: {
            translations: {
              button: { buttonText: "搜索文档", buttonAriaLabel: "搜索文档" },
              modal: {
                displayDetails: "显示详情",
                resetButtonTitle: "清除搜索",
                backButtonTitle: "关闭搜索",
                noResultsText: "没有找到结果",
                footer: { selectText: "选择", navigateText: "切换", closeText: "关闭" }
              }
            }
          }
        }
      }
    },
    socialLinks: [{ icon: "github", link: repo }]
  }
});
