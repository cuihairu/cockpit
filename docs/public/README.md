# Public Directory

Static assets for VitePress documentation.

Place your files here:
- logo.png
- logo.svg
- favicon.ico

Files in this directory will be served at the root URL:
- /logo.png
- /logo.svg
- /favicon.ico

To use in VitePress config (`.vitepress/config.ts`):
```ts
export default defineConfig({
  themeConfig: {
    logo: '/logo.png',
    // or
    logo: {
      light: '/logo.svg',
      dark: '/logo-dark.svg',
    },
  },
})
```
