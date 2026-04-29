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
    ["link", { rel: "preconnect", href: "https://fonts.googleapis.com" }],
    ["link", { rel: "preconnect", href: "https://fonts.gstatic.com", crossorigin: "" }],
    ["link", { rel: "stylesheet", href: "https://fonts.googleapis.com/css2?family=IBM+Plex+Sans:wght@400;500;600;700&family=JetBrains+Mono:wght@400;500;600;700&display=swap" }],
    ["meta", { name: "theme-color", content: "#0f766e" }],
    ["meta", { property: "og:title", content: "ClashForge Docs" }],
    ["meta", { property: "og:description", content: "Install, configure, run, verify and troubleshoot ClashForge on OpenWrt." }]
  ],
  locales: {
    root: {
      label: "简体中文",
      lang: "zh-CN",
      title: "ClashForge 文档",
      description: "从首次上手到长期稳定使用的 OpenWrt 代理用户指南。",
      themeConfig: {
        nav: [
          { text: "开始使用", link: "/guide/quick-start" },
          { text: "验证与排障", link: "/guide/verify" },
          { text: "运维升级", link: "/guide/operations" },
          { text: "FAQ", link: "/guide/faq" },
          { text: "GitHub", link: repo }
        ],
        sidebar: [
          {
            text: "首次上手",
            items: [
              { text: "快速开始", link: "/guide/quick-start" },
              { text: "安装方式选择", link: "/guide/install" },
              { text: "首次配置", link: "/guide/config" },
              { text: "启动与接管", link: "/guide/run" }
            ]
          },
          {
            text: "验证与维护",
            items: [
              { text: "检查清单", link: "/guide/verify" },
              { text: "日常运维", link: "/guide/operations" },
              { text: "升级与回滚", link: "/guide/upgrade" },
              { text: "排障", link: "/guide/troubleshooting" },
              { text: "FAQ", link: "/guide/faq" }
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
    logo: "/logo.svg",
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
