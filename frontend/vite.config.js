import http from 'node:http'
import { fileURLToPath } from 'node:url'

import { svelte } from '@sveltejs/vite-plugin-svelte'
import { defineConfig } from 'vite'

// The API closes idle sockets after two minutes. A keep-alive agent then writes
// the next request into a dead socket, and the browser only sees a network error.
const apiAgent = new http.Agent({ keepAlive: false })

function proxyError(proxy) {
  proxy.on('error', (_err, _req, res) => {
    if (!res || typeof res.writeHead !== 'function' || res.headersSent) return
    res.writeHead(502, { 'content-type': 'application/json' })
    res.end(JSON.stringify({ detail: 'сервер недоступен' }))
  })
}

export default defineConfig({
  plugins: [svelte()],
  resolve: {
    alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
  },
  server: {
    host: '127.0.0.1',
    port: 5173,
    strictPort: true,
    proxy: {
      // http target + ws:true — иначе после рестарта API сокет через Vite залипает.
      '/git': {
        target: process.env.QUESTS_DEV_API || 'http://127.0.0.1:8765',
        changeOrigin: true,
        secure: true,
        agent: apiAgent,
        configure: proxyError,
      },
      '/api': {
        target: process.env.QUESTS_DEV_API || 'http://127.0.0.1:8765',
        changeOrigin: true,
        secure: true,
        agent: apiAgent,
        configure: proxyError,
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
