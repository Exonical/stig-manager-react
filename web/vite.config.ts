import path from 'node:path'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    sourcemap: true,
  },
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    port: 54000,
    proxy: {
      '/api': {
        target: process.env.VITE_API_PROXY_TARGET ?? 'http://localhost:54001',
        changeOrigin: false,
      },
      // The API renders /js/Env.js at runtime with OIDC settings; the
      // SPA's index.html loads it before the React bundle.
      '/js/Env.js': {
        target: process.env.VITE_API_PROXY_TARGET ?? 'http://localhost:54001',
        changeOrigin: false,
      },
    },
  },
})
