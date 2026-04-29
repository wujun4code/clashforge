# ClashForge Docs Site

This directory contains the VitePress user manual published to GitHub Pages.

The root repository `README.md` is the technical project overview. This docs site is the hands-on manual for installation, first configuration, routing strategy, operations and troubleshooting.

## Development

```sh
npm install
npm run docs:dev
```

## Build

```sh
npm run docs:build
npm run docs:preview
```

## Content Structure

| Path | Purpose |
| --- | --- |
| `docs/index.md` | Chinese landing page |
| `docs/guide/*.md` | Chinese user manual |
| `docs/en/index.md` | English landing page |
| `docs/en/guide/*.md` | English user manual |
| `docs/.vitepress/config.ts` | Navigation, sidebar, locale and theme configuration |

## GitHub Pages

The workflow at `../.github/workflows/pages.yml` builds `docs-site/docs` and deploys the VitePress output.

For project pages, the default base is `/clashforge/`. Override with `DOCS_BASE` when needed.
