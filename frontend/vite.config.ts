import { defineConfig } from 'vite'
import { resolve } from 'node:path'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'
const backendTarget = process.env.BPM_API_TARGET || 'http://127.0.0.1:8787'

export default defineConfig({
  plugins: [vue(), tailwindcss()],
  server: {
    proxy: Object.fromEntries(['/api', '/media', '/userscript', '/bridge'].map((path) => [path, { target: backendTarget }])),
  },
  build: { outDir: resolve(__dirname, '../cmd/bpm/dist'), emptyOutDir: true },
  test: { environment: 'jsdom' },
})
