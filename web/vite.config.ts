import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'path'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:8000',
        changeOrigin: true,
        configure(proxy) {
          proxy.on('proxyReq', (proxyReq, req) => {
            if (req.headers.origin === 'http://localhost:5173' || req.headers.origin === 'http://127.0.0.1:5173') {
              proxyReq.setHeader('Origin', 'http://127.0.0.1:8000')
            }
          })
        },
      },
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
})
