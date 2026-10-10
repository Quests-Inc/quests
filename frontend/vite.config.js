import { svelte } from '@sveltejs/vite-plugin-svelte'
import { defineConfig } from 'vite'

export default defineConfig({
  plugins: [svelte()],
  server: {
    host: '127.0.0.1',
    port: 5173,
    strictPort: true,
    proxy: {
      // http target + ws:true — иначе после рестарта API сокет через Vite залипает.
      '/api': {
        target: process.env.QUESTS_DEV_API || 'http://127.0.0.1:8765',
        changeOrigin: true,
        secure: true,
      },
      '/ws': {
        target: process.env.QUESTS_DEV_API || 'http://127.0.0.1:8765',
        ws: true,
        changeOrigin: true,
        secure: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
})
