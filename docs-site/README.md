# ClashForge Docs Site

This directory contains the VitePress documentation site published to GitHub Pages.

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

## GitHub Pages

The workflow at `../.github/workflows/pages.yml` builds `docs-site/docs` and deploys the VitePress output.

For project pages, the default base is `/clashforge/`. Override with `DOCS_BASE` when needed.
