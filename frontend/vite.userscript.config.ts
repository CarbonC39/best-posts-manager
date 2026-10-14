import { defineConfig } from 'vite'
import { copyFileSync, mkdirSync } from 'node:fs'
import { resolve } from 'node:path'
import { userscriptMatches } from './src/platformRegistry'

const matchMetadata = userscriptMatches.map((match) => `// @match        ${match}`).join('\n')

const metadata = `// ==UserScript==
// @name         Best Post Manager local bridge
// @namespace    best-post-manager
// @version      1.6.1
// @description  Fill visible editors, safely publish X or Weibo on explicit request, and capture visible posts.
${matchMetadata}
// @grant        GM_xmlhttpRequest
// @grant        GM_getValue
// @grant        GM_setValue
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
