import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'node:path'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: { '@': path.resolve(__dirname, './src') },
  },
  server: {
    // During `npm run dev` the SPA lives on http://localhost:5173 and the
    // Go backend on http://localhost:8080. The `--dev` flag on the Go
    // server adds :5173 to the origin allow-list; everything under /api
    // is proxied so the SPA always uses relative URLs and same-origin
    // behaves consistently between dev and the embedded production build.
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
})
