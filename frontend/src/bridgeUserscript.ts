import { captureVisible, clickKnownPublish, fillVisibleWithMedia, type BridgeRequest } from './bridgeAdapters'
import { platformForBridgeHost } from './platformRegistry'

declare function GM_xmlhttpRequest(options: Record<string, unknown>): void
declare function GM_getValue<T>(key: string, fallback: T): T
declare function GM_setValue<T>(key: string, value: T): void

type PairingResponse = { id: string; token: string }
type ScriptRequest = BridgeRequest & { id: string; kind: 'fill' | 'capture' }
type Config = { base: string; token: string; pairing: string }

const host = location.hostname.toLowerCase()
const platform = platformForBridgeHost(host) || 'qzone'
const configKey = `bpm-bridge-config-v1-${platform}`
const saved = GM_getValue<Partial<Config>>(configKey, {})
const injectedOrigin = '__BPM_ORIGIN__'
const defaultBase = injectedOrigin.startsWith('http://') || injectedOrigin.startsWith('https://') ? injectedOrigin : 'http://127.0.0.1:8787'
const config: Config = { base: saved.base || defaultBase, token: '', pairing: '' }
if (saved.token && saved.pairing) { config.token = saved.token; config.pairing = saved.pairing }
let current: ScriptRequest | null = null
let polling = false

function gmRequest(options: Record<string, unknown>): Promise<{ status: number; responseText: string; response: unknown }> {
  return new Promise((resolve, reject) => GM_xmlhttpRequest({ ...options, onload: resolve, onerror: reject, ontimeout: reject }))
}
function headers() {
  return { Authorization: `Bearer ${config.token}`, 'X-BPM-Page-URL': location.href, 'X-BPM-Pairing-ID': config.pairing, 'X-BPM-Platform': platform, 'Content-Type': 'application/json' }
}
async function call<T>(path: string, init: { method?: string; body?: string } = {}): Promise<T> {
  const response = await gmRequest({ method: init.method || 'GET', url: config.base.replace(/\/$/, '') + path, headers: headers(), data: init.body, timeout: 10_000 })
  let body: any
  try { body = JSON.parse(response.responseText) } catch { throw new Error(`本地桥接返回了无效响应（HTTP ${response.status}）`) }
  if (response.status < 200 || response.status >= 300) throw new Error(body.error || `HTTP ${response.status}`)
  return body as T
}
async function localMediaFetcher(path: string) {
  const response = await gmRequest({ method: 'GET', url: config.base.replace(/\/$/, '') + path, responseType: 'blob', timeout: 30_000 })
  return { ok: response.status >= 200 && response.status < 300, status: response.status, blob: async () => response.response as Blob } as Response
}

const panel = document.createElement('div')
panel.style.cssText = 'position:fixed;z-index:2147483647;right:16px;bottom:16px;width:304px;color:#20241f;font:13px Inter,"Segoe UI",system-ui,sans-serif;line-height:1.45'
panel.innerHTML = `<style>
  *{box-sizing:border-box}button,input{font:inherit}button{cursor:pointer}
  .bpm-shell{overflow:hidden;border:1px solid #d2d8d1;border-radius:14px;background:#fff;box-shadow:0 16px 44px rgb(24 38 29 / 18%)}
  .bpm-head{display:flex;align-items:center;gap:10px;min-height:48px;padding:8px 9px 8px 11px;background:#f7f8f6;cursor:grab;user-select:none;touch-action:none}
  .bpm-head:active{cursor:grabbing}.bpm-mark{display:grid;place-items:center;flex:0 0 30px;width:30px;height:30px;border-radius:9px;color:#fff;background:#264e36}.bpm-mark svg{width:17px;height:17px}
  .bpm-title{display:grid;min-width:0;line-height:1.15}.bpm-title strong{font-size:13px}.bpm-title small{margin-top:3px;color:#737c75;font-size:10px;text-transform:uppercase;letter-spacing:.08em}
  .bpm-icon{display:grid;place-items:center;width:30px;height:30px;margin-left:auto;padding:0;border:0;border-radius:7px;color:#607068;background:transparent;font-size:20px;line-height:1}.bpm-icon:hover{color:#173625;background:#e8f1eb}
  .bpm-content{display:grid;gap:11px;max-height:min(520px,calc(100vh - 92px));padding:12px;overflow:auto}
  .bpm-status{display:flex;align-items:flex-start;gap:8px;color:#496452;font-size:12px;overflow-wrap:anywhere}.bpm-dot{flex:0 0 7px;width:7px;height:7px;margin-top:5px;border-radius:50%;background:#c68a48}.bpm-shell[data-connected="true"] .bpm-dot{background:#5da777}
  label{display:grid;gap:5px;color:#3f4741;font-size:11px;font-weight:650}input{width:100%;height:36px;padding:0 9px;border:1px solid #c8d0c9;border-radius:7px;color:#20241f;background:#fff;font-weight:400;outline:0}input::placeholder{color:#707970}input:focus{border-color:#76a088;box-shadow:0 0 0 3px rgb(93 167 119 / 15%)}
  .bpm-actions{display:flex;align-items:center;gap:7px}.bpm-button{min-height:34px;padding:6px 10px;border:1px solid #c8d0c9;border-radius:7px;color:#405047;background:#fff}.bpm-button:hover{background:#f3f6f3}.bpm-button.primary{border-color:#264e36;color:#fff;background:#264e36}.bpm-button.primary:hover{background:#1e3f2c}
  [data-bpm=request]{display:grid;gap:8px;padding-top:10px;border-top:1px solid #e2e6e0;color:#3f4741;font-size:12px}[data-bpm=request]:empty{display:none}[data-bpm=request] button{justify-self:start;min-height:34px;padding:6px 10px;border:1px solid #c8d0c9;border-radius:7px;color:#315840;background:#fff}[data-bpm=request] input{margin-top:2px}
  .bpm-shell.minimized{width:52px;height:52px;border-radius:50%;border-color:#b9c9be;box-shadow:0 10px 26px rgb(24 38 29 / 22%)}.bpm-shell.minimized .bpm-head{width:52px;height:52px;min-height:52px;padding:0;justify-content:center;background:#264e36;cursor:grab}.bpm-shell.minimized .bpm-mark{width:52px;height:52px;border-radius:50%;background:transparent}.bpm-shell.minimized .bpm-mark svg{width:20px;height:20px}.bpm-shell.minimized .bpm-title,.bpm-shell.minimized .bpm-icon,.bpm-shell.minimized .bpm-content{display:none}
</style><section class="bpm-shell" data-bpm="shell"><header class="bpm-head" data-bpm="drag"><span class="bpm-mark" aria-hidden="true"><svg viewBox="0 0 20 20"><path d="M4 16 16 4M7 4h9v9" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="1.8"/></svg></span><span class="bpm-title"><strong>Best Post Manager</strong><small>Bridge · ${platform}</small></span><button type="button" class="bpm-icon" data-bpm="minimize" aria-label="最小化桥接面板" title="最小化">−</button></header><div class="bpm-content" data-bpm="content"><div class="bpm-status"><span class="bpm-dot" aria-hidden="true"></span><span data-bpm="status">未连接</span></div><label>本地服务<input data-bpm="base" placeholder="http://127.0.0.1:8787"></label><label>配对 code<input data-bpm="code" placeholder="一次性 code"></label><div class="bpm-actions"><button type="button" class="bpm-button primary" data-bpm="connect">连接</button><button type="button" class="bpm-button" data-bpm="disconnect">断开</button></div><div data-bpm="request"></div></div></section>`
const panelHost = document.createElement('div')
panelHost.attachShadow({ mode: 'closed' }).appendChild(panel)
document.body.appendChild(panelHost)
const element = <T extends HTMLElement>(name: string) => panel.querySelector(`[data-bpm="${name}"]`) as T
element<HTMLInputElement>('base').value = config.base
const shell = element('shell')
const status = (text: string) => { element('status').textContent = text; shell.dataset.connected = String(Boolean(config.token && config.pairing)) }
const requestBox = element('request')
let minimized = GM_getValue<boolean>(`${configKey}-minimized`, false)
type PanelPosition = { left: number; top: number }
const positionKey = `${configKey}-position`
const savedPosition = GM_getValue<Partial<PanelPosition>>(positionKey, {})
if (Number.isFinite(savedPosition.left) && Number.isFinite(savedPosition.top)) {
  panel.style.right = 'auto'; panel.style.bottom = 'auto'
  panel.style.left = `${savedPosition.left}px`; panel.style.top = `${savedPosition.top}px`
}
function clampPanel(left: number, top: number) {
  const rect = panel.getBoundingClientRect()
  return { left: Math.max(8, Math.min(left, window.innerWidth - rect.width - 8)), top: Math.max(8, Math.min(top, window.innerHeight - rect.height - 8)) }
}
function setPanelPosition(left: number, top: number, save = false) {
  const position = clampPanel(left, top)
  panel.style.right = 'auto'; panel.style.bottom = 'auto'
  panel.style.left = `${position.left}px`; panel.style.top = `${position.top}px`
  if (save) GM_setValue(positionKey, position)
}
function setMinimized(value: boolean) {
  const before = panel.getBoundingClientRect()
  minimized = value
  GM_setValue(`${configKey}-minimized`, value)
  shell.classList.toggle('minimized', value)
  panel.style.width = value ? '52px' : '304px'
  const dragHandle = element('drag')
  if (value) {
    dragHandle.setAttribute('role', 'button'); dragHandle.setAttribute('tabindex', '0'); dragHandle.setAttribute('aria-label', '展开桥接面板')
  } else {
    dragHandle.removeAttribute('role'); dragHandle.removeAttribute('tabindex'); dragHandle.removeAttribute('aria-label')
  }
  const after = panel.getBoundingClientRect()
  if (panel.style.left) setPanelPosition(before.right - after.width, before.bottom - after.height, true)
}
let dragged = false
element('drag').addEventListener('pointerdown', (event: PointerEvent) => {
  if ((event.target as HTMLElement).closest('[data-bpm="minimize"]')) return
  const handle = event.currentTarget as HTMLElement
  const start = panel.getBoundingClientRect(); const originX = event.clientX; const originY = event.clientY
  dragged = false; handle.setPointerCapture(event.pointerId)
  const move = (next: PointerEvent) => { const dx = next.clientX - originX; const dy = next.clientY - originY; if (Math.abs(dx) + Math.abs(dy) > 3) dragged = true; setPanelPosition(start.left + dx, start.top + dy) }
  const up = (next: PointerEvent) => { handle.removeEventListener('pointermove', move); handle.removeEventListener('pointerup', up); const rect = panel.getBoundingClientRect(); if (dragged) { next.preventDefault(); GM_setValue(positionKey, { left: rect.left, top: rect.top }) } }
  handle.addEventListener('pointermove', move); handle.addEventListener('pointerup', up)
})
element('drag').addEventListener('click', (event) => { if ((event.target as HTMLElement).closest('[data-bpm="minimize"]')) return; if (minimized && !dragged) setMinimized(false); dragged = false })
element('drag').addEventListener('keydown', (event: KeyboardEvent) => { if (minimized && (event.key === 'Enter' || event.key === ' ')) { event.preventDefault(); setMinimized(false) } })
window.addEventListener('resize', () => { if (panel.style.left) { const rect = panel.getBoundingClientRect(); setPanelPosition(rect.left, rect.top, true) } })
const addButton = (label: string, action: () => void) => { const button = document.createElement('button'); button.textContent = label; button.addEventListener('click', action); requestBox.appendChild(button); return button }

async function connect() {
  config.base = element<HTMLInputElement>('base').value.trim().replace(/\/$/, '')
  const code = element<HTMLInputElement>('code').value.trim()
  if (!/^https?:\/\/(127\.0\.0\.1|localhost)(:\d+)?$/.test(config.base)) throw new Error('本地服务地址必须是 127.0.0.1 或 localhost')
  if (!code) throw new Error('请输入 WebUI 生成的一次性 pairing code')
  GM_setValue(configKey, { base: config.base })
  const hello = await call<PairingResponse>('/bridge/v1/hello', { method: 'POST', body: JSON.stringify({ code, platform, page_url: location.href }) })
  config.token = hello.token
  config.pairing = hello.id
  GM_setValue(configKey, { base: config.base, token: config.token, pairing: config.pairing })
  element<HTMLInputElement>('code').value = ''
  status('已连接')
  if (!polling) { polling = true; void poll() }
}

function showCapture(request: ScriptRequest) {
  const snapshot = captureVisible(platform, location.href)
  const summary = document.createElement('div')
  summary.textContent = `准备捕获：${snapshot.text.slice(0, 160) || '未读取到正文'}${snapshot.incomplete ? `（${snapshot.incomplete_reason}）` : ''}`
  requestBox.appendChild(summary)
  addButton('确认捕获当前可见内容', () => void (async () => {
    await call(`/bridge/v1/requests/${request.id}/capture`, { method: 'POST', body: JSON.stringify(snapshot) })
    status(snapshot.incomplete ? `已保存不完整快照：${snapshot.incomplete_reason}` : '已保存当前可见快照')
    current = null
    requestBox.replaceChildren()
  })().catch((error) => status(`捕获失败：${String(error)}`)))
}

function showFill(request: ScriptRequest) {
  const summary = document.createElement('div')
  summary.textContent = `请求 ${request.id.slice(0, 8)}：${(request.text || '').slice(0, 80)}；图片 ${request.media?.length || 0} 张`
  requestBox.appendChild(summary)
  const resultURL = document.createElement('input')
  resultURL.placeholder = '发布后的帖子永久链接（不是编辑页地址）'
  requestBox.appendChild(resultURL)
  const resultButton = addButton('确认已发布并回传链接', () => void (async () => {
    const url = resultURL.value.trim()
    if (!url) throw new Error('请先填写发布后的帖子永久链接')
    await call(`/bridge/v1/requests/${request.id}/result`, { method: 'POST', body: JSON.stringify({ url }) })
    status('已回传人工确认的发布结果')
    current = null
    requestBox.replaceChildren()
  })().catch((error) => status(`回传失败：${String(error)}`)))
  resultButton.disabled = true
  status(request.auto_publish ? '已收到本次自动发布请求，正在填入并检查发布条件' : '已收到填入请求，正在写入当前可见编辑器（不会发布）')
  void (async () => {
    try {
      const failures = await fillVisibleWithMedia(platform, request, localMediaFetcher)
      if (request.auto_publish && request.media?.length) {
        failures.push('页面媒体上传完成状态无法可靠确认，已取消自动点击；请检查图片后人工发布')
      } else if (request.auto_publish && failures.length === 0) {
        const publishWarning = clickKnownPublish(platform)
        if (publishWarning) failures.push(publishWarning)
      }
      await call(`/bridge/v1/requests/${request.id}/filled`, { method: 'POST', body: JSON.stringify({ warning: failures.join('; ') }) })
      resultButton.disabled = false
      status(failures.length ? `文字已填入；请人工处理：${failures.join('；')}` : request.auto_publish ? '已填入并点击可见发布按钮；请确认平台结果' : '已填入，请人工检查可见范围和图片顺序后发布')
    } catch (error) {
      await call(`/bridge/v1/requests/${request.id}/error`, { method: 'POST', body: JSON.stringify({ error: String(error) }) })
      status(`填入失败：${String(error)}`)
      current = null
      requestBox.replaceChildren()
    }
  })()
}

async function poll() {
  if (!config.token || !config.pairing) { polling = false; return }
  try {
    const requests = await call<ScriptRequest[]>('/bridge/v1/requests')
    const request = requests[0]
    if (!current && request) {
      current = request
      requestBox.replaceChildren()
      if (request.kind === 'capture') showCapture(request); else showFill(request)
    }
  } catch (error) { status(`连接不可用：${error instanceof Error ? error.message : String(error)}`) }
  if (config.token && config.pairing) window.setTimeout(() => void poll(), 3_000)
  else polling = false
}

element('connect').addEventListener('click', () => void connect().catch((error) => status(`连接失败：${error instanceof Error ? error.message : String(error)}`)))
element('disconnect').addEventListener('click', () => {
  config.token = ''; config.pairing = ''; current = null; requestBox.replaceChildren()
  GM_setValue(configKey, { base: config.base }); status('已断开')
})
element('minimize').addEventListener('click', () => setMinimized(!minimized))
setMinimized(minimized)
if (config.token && config.pairing) { status('已恢复连接，正在检查本地请求'); polling = true; void poll() }
