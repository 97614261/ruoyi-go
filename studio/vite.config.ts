import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  // Public sample URLs stay rooted at the site, including in component tests.
  plugins: [vue({ template: { transformAssetUrls: { includeAbsolute: false } } })],
  server: {
    host: '127.0.0.1', port: 5177, strictPort: true,
    allowedHosts: true,
    proxy: { '/api': { target: 'http://127.0.0.1:18110', rewrite: path => path.replace(/^\/api/, '') } },
  },
  build: { sourcemap: false },
})
