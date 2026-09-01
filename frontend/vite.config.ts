import { defineConfig } from 'vite'
import { resolve } from 'node:path'
import vue from '@vitejs/plugin-vue'
export default defineConfig({ plugins: [vue()], build: { outDir: resolve(__dirname, '../cmd/bpm/dist'), emptyOutDir: true }, test: { environment: 'jsdom' } })
