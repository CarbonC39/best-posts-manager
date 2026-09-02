import { defineConfig } from 'vite'
import { copyFileSync, mkdirSync } from 'node:fs'
import { resolve } from 'node:path'

const metadata = `// ==UserScript==
// @name         Best Post Manager local bridge
// @namespace    best-post-manager
// @version      1.1.0
// @description  Fill visible editors and capture visible posts; final publishing always stays manual.
// @match        https://x.com/*
// @match        https://*.x.com/*
// @match        https://twitter.com/*
// @match        https://*.twitter.com/*
// @match        https://weibo.com/*
// @match        https://*.weibo.com/*
// @match        https://xiaohongshu.com/*
// @match        https://*.xiaohongshu.com/*
// @match        https://qzone.qq.com/*
// @match        https://*.qzone.qq.com/*
// @grant        GM_xmlhttpRequest
// @connect      127.0.0.1
// @connect      localhost
// @downloadURL  __BPM_ORIGIN__/userscript/bpm-bridge.user.js
// @updateURL    __BPM_ORIGIN__/userscript/bpm-bridge.user.js
// @noframes
// ==/UserScript==`

export default defineConfig({
  plugins: [{
    name: 'userscript-metadata',
    generateBundle(_options, bundle) {
      for (const output of Object.values(bundle)) {
        if (output.type === 'chunk') output.code = `${metadata}\n${output.code}`
      }
    },
    closeBundle() {
      const embeddedDir = resolve(__dirname, '../cmd/bpm/dist')
      mkdirSync(embeddedDir, { recursive: true })
      copyFileSync(resolve(__dirname, 'userscript/bpm-bridge.user.js'), resolve(embeddedDir, 'bpm-bridge.user.js'))
    },
  }],
  build: {
    outDir: resolve(__dirname, 'userscript'),
    emptyOutDir: false,
    minify: false,
    lib: { entry: resolve(__dirname, 'src/bridgeUserscript.ts'), name: 'BPMBridge', formats: ['iife'], fileName: () => 'bpm-bridge.user.js' },
  },
})
