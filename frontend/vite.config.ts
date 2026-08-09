import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// В dev-режиме запросы к /api проксируются на FastAPI (порт 8000),
// поэтому фронтенду не нужно знать полный адрес бэкенда и нет проблем с CORS.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8000',
        changeOrigin: true,
      },
    },
  },
})
