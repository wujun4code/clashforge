import { defineConfig } from "vitepress";

declare const process: { env: Record<string, string | undefined> };

const repo = "https://github.com/wujun4code/clashforge";
const base = process.env.DOCS_BASE ?? (process.env.GITHUB_ACTIONS ? "/clashforge/" : "/");

export default defineConfig({
  title: "ClashForge Docs",
  description: "OpenWrt mihomo management console documentation for installation, configuration, routing and operations.",
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
    ["meta", { name: "theme-color", content: "#0f766e" }],
    ["meta", { property: "og:title", content: "ClashForge" }],
    ["meta", { property: "og:description", content: "OpenWrt 上的 mihomo 管理控制台：订阅、节点、设备分流、DNS 接管、诊断恢复和团队级出口管理。" }],
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
      description: "面向跨境电商、自媒体团队、AI 重度用户和多设备家庭的 OpenWrt mihomo 管理控制台手册。",
      themeConfig: {
        nav: [
          { text: "产品定位", link: "/guide/why" },
          { text: "功能模块", link: "/guide/features" },
          { text: "快速开始", link: "/guide/quick-start" },
          { text: "安装", link: "/guide/install" },
          { text: "排障", link: "/guide/troubleshooting" },
          { text: "FAQ", link: "/guide/faq" },
          { text: "GitHub", link: repo }
        ],
        sidebar: [
          {
            text: "先理解产品",
            items: [
              { text: "产品定位与适用人群", link: "/guide/why" },
              { text: "功能模块总览", link: "/guide/features" },
              { text: "快速开始", link: "/guide/quick-start" }
            ]
          },
          {
            text: "安装与首次配置",
            items: [
              { text: "安装到路由器", link: "/guide/install" },
              { text: "导入来源与配置", link: "/guide/config" },
              { text: "启动接管与设备生效", link: "/guide/run" },
              { text: "验证是否成功", link: "/guide/verify" }
            ]
          },
          {
            text: "长期运维",
            items: [
              { text: "日常维护", link: "/guide/operations" },
              { text: "更新软件", link: "/guide/upgrade" },
              { text: "故障排查", link: "/guide/troubleshooting" },
              { text: "常见问题", link: "/guide/faq" }
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
      description: "A technical and operational manual for the ClashForge OpenWrt mihomo management console.",
      themeConfig: {
        nav: [
          { text: "Overview", link: "/en/" },
          { text: "Features", link: "/en/guide/features" },
          { text: "Quick Start", link: "/en/guide/quick-start" },
          { text: "Install", link: "/en/guide/install" },
          { text: "Operations", link: "/en/guide/operations" },
          { text: "FAQ", link: "/en/guide/faq" },
          { text: "GitHub", link: repo }
        ],
        sidebar: [
          {
            text: "Product",
            items: [
              { text: "Overview", link: "/en/" },
              { text: "Feature Modules", link: "/en/guide/features" },
              { text: "Quick Start", link: "/en/guide/quick-start" }
            ]
          },
          {
            text: "Install and Configure",
            items: [
              { text: "Quick Start", link: "/en/guide/quick-start" },
              { text: "Install & Deploy", link: "/en/guide/install" },
              { text: "First Configuration", link: "/en/guide/config" },
              { text: "Run & Takeover", link: "/en/guide/run" },
              { text: "Verification", link: "/en/guide/verify" }
            ]
          },
          {
            text: "Operations",
            items: [
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
