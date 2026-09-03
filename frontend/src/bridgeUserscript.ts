import { captureVisible, clickKnownPublish, fillVisibleWithMedia, type BridgeRequest } from './bridgeAdapters'

declare function GM_xmlhttpRequest(options: Record<string, unknown>): void
declare function GM_getValue<T>(key: string, fallback: T): T
declare function GM_setValue<T>(key: string, value: T): void

type PairingResponse = { id: string; token: string }
type ScriptRequest = BridgeRequest & { id: string; kind: 'fill' | 'capture' }
type Config = { base: string; token: string; pairing: string }

const host = location.hostname.toLowerCase()
const platform = host === 'x.com' || host.endsWith('.x.com') || host === 'twitter.com' || host.endsWith('.twitter.com')
  ? 'x' : host === 'weibo.com' || host.endsWith('.weibo.com')
    ? 'weibo' : host === 'xiaohongshu.com' || host.endsWith('.xiaohongshu.com') ? 'xiaohongshu' : 'qzone'
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
panel.style.cssText = 'position:fixed;z-index:2147483647;right:12px;bottom:12px;background:#fff;color:#111;border:1px solid #888;padding:10px;font:13px sans-serif;width:280px;box-shadow:0 2px 8px #0003'
panel.innerHTML = '<style>div{line-height:1.45}input{box-sizing:border-box;width:100%;margin:5px 0;padding:6px;border:1px solid #ccc;border-radius:4px}button{margin:5px 5px 5px 0;padding:5px 8px;border:1px solid #ccc;border-radius:4px;background:#f7f7f5;cursor:pointer}[data-bpm=status]{margin:6px 0;color:#4b6f55;overflow-wrap:anywhere}[data-bpm=request]{margin-top:6px;padding-top:6px;border-top:1px solid #eee}</style><b>Best Post Manager bridge v1</b><button data-bpm="minimize">缩小</button><div data-bpm="platform"></div><div data-bpm="status">未连接</div><input data-bpm="base" placeholder="本地服务地址"><input data-bpm="code" placeholder="一次性 pairing code"><button data-bpm="connect">连接</button><button data-bpm="disconnect">断开并清除</button><div data-bpm="request"></div>'
const panelHost = document.createElement('div')
panelHost.attachShadow({ mode: 'closed' }).appendChild(panel)
document.body.appendChild(panelHost)
const element = <T extends HTMLElement>(name: string) => panel.querySelector(`[data-bpm="${name}"]`) as T
element('platform').textContent = `当前平台：${platform}`
element<HTMLInputElement>('base').value = config.base
const status = (text: string) => { element('status').textContent = text }
const requestBox = element('request')
let minimized = GM_getValue<boolean>(`${configKey}-minimized`, false)
function setMinimized(value: boolean) {
  minimized = value
  GM_setValue(`${configKey}-minimized`, value)
  panel.style.width = value ? '46px' : '280px'
  panel.style.height = value ? '46px' : 'auto'
  panel.style.padding = value ? '0' : '10px'
  for (const name of ['platform', 'status', 'base', 'code', 'connect', 'disconnect', 'request']) element(name).style.display = value ? 'none' : ''
  const button = element<HTMLButtonElement>('minimize')
  button.textContent = value ? 'BPM' : '最小化'
  button.title = value ? '展开 Best Post Manager bridge' : '最小化 bridge'
  button.style.margin = value ? '0' : '5px'
  button.style.width = value ? '46px' : 'auto'
  button.style.height = value ? '46px' : 'auto'
}
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
  status('已连接；token 仅保存在油猴隔离存储中（服务端 TTL 90 天）')
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
  GM_setValue(configKey, { base: config.base }); status('已断开；脚本存储中的 token 已清除')
})
element('minimize').addEventListener('click', () => setMinimized(!minimized))
setMinimized(minimized)
if (config.token && config.pairing) { status('已恢复连接，正在检查本地请求'); polling = true; void poll() }
