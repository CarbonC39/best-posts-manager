import { bridgePlatformIDs, type BridgePlatformID } from './platformRegistry'

export type BridgeMedia = { id?: string; filename: string; mime: string; alt_text?: string; size?: number; url?: string }
export type BridgeRequest = { platform: string; page_url: string; text?: string; media?: BridgeMedia[]; auto_publish?: boolean }
export type Capture = { text: string; url: string; media: BridgeMedia[]; incomplete: boolean; incomplete_reason?: string }

type Adapter = { selectors: string[]; captureSelectors: string[]; mediaSelectors: string[]; publishSelectors: string[]; publishLabels?: string[] }
const adapters = {
  x: { selectors: ['[data-testid="tweetTextarea_0"]', 'textarea[aria-label*="Post"]'], captureSelectors: ['article [data-testid="tweetText"]'], mediaSelectors: ['input[type="file"]'], publishSelectors: ['[data-testid="tweetButtonInline"]', '[data-testid="tweetButton"]'] },
  weibo: { selectors: ['textarea[placeholder="有什么新鲜事想分享给大家？"]', 'textarea[placeholder*="分享"]', '[contenteditable="true"][node-type*="text"]'], captureSelectors: ['.WB_text', '[node-type="feed_list_content"]'], mediaSelectors: ['.wbpro-form input[type="file"]', 'input[type="file"]'], publishSelectors: ['.wbpro-form button', '.wbpro-form [role="button"]'], publishLabels: ['发送', '发布'] },
  xiaohongshu: { selectors: ['.ql-editor[contenteditable="true"]', '[contenteditable="true"][data-placeholder*="正文"]', 'textarea[placeholder*="正文"]', 'textarea[placeholder*="描述"]'], captureSelectors: ['.note-content', '[class*="note-content"]'], mediaSelectors: ['.image-upload-buttons input[type="file"]', 'input[type="file"]'], publishSelectors: [] },
  qzone: { selectors: ['.qz-poster-editor-cont [x\\:id="content_content"]', '.qz-poster-editor-cont [contenteditable="true"][class~="textinput"]', 'textarea[placeholder*="说说"]'], captureSelectors: ['.feed-content', '[class*="feed-content"]'], mediaSelectors: ['.qz-poster-editor-cont input[type="file"]', 'input[type="file"]'], publishSelectors: [] },
} satisfies Record<BridgePlatformID, Adapter>

function adapter(platform: string): Adapter {
  const value = adapters[platform as BridgePlatformID]
  if (!value) throw new Error(`unsupported bridge platform: ${platform}`)
  return value
}
function findComposer(platform: string, root: ParentNode = document): HTMLElement | HTMLTextAreaElement | null {
  for (const selector of adapter(platform).selectors) {
    const candidates = root.querySelectorAll<HTMLElement | HTMLTextAreaElement>(selector)
    for (const found of candidates) {
      if (!found.hidden && found.getAttribute('aria-hidden') !== 'true' && found.style.display !== 'none') return found
    }
  }
  return null
}
function composer(platform: string, root: ParentNode = document): HTMLElement | HTMLTextAreaElement {
  if (platform === 'qzone') {
    const container = root.querySelector('.qz-poster-editor-cont')
    const content = container?.querySelector<HTMLElement>('[x\\:id="content_content"]')
    if (content) {
      const substitutor = container?.querySelector<HTMLElement>('[x\\:id="substitutor_content"]')
      if (substitutor && !substitutor.hidden && substitutor.getAttribute('aria-hidden') !== 'true' && substitutor.style.display !== 'none') substitutor.click()
      content.focus()
      return content
    }
  }
  const found = findComposer(platform, root)
  if (found) return found
  throw new Error('当前页面未找到可见编辑框，请展开编辑器后重试')
}
function setText(element: HTMLElement | HTMLTextAreaElement, text: string) {
  if (element instanceof HTMLTextAreaElement || element instanceof HTMLInputElement) {
	const prototype = element instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype
	const setter = Object.getOwnPropertyDescriptor(prototype, 'value')?.set
    setter?.call(element, text)
  } else {
    element.textContent = text
  }
  element.dispatchEvent(new InputEvent('input', { bubbles: true, inputType: 'insertText', data: text }))
  element.dispatchEvent(new Event('change', { bubbles: true }))
}
function setQzoneText(element: HTMLElement, text: string) {
  element.focus()
  const selection = window.getSelection()
  const range = document.createRange()
  range.selectNodeContents(element)
  selection?.removeAllRanges()
  selection?.addRange(range)
  element.dispatchEvent(new InputEvent('beforeinput', { bubbles: true, cancelable: true, inputType: 'insertText', data: text }))

  const editableDocument = document as Document & { execCommand?: (command: string, showUI?: boolean, value?: string) => boolean }
  let inserted = false
  try { inserted = editableDocument.execCommand?.('insertText', false, text) === true } catch { /* fall back to a DOM range */ }
  if (!inserted) {
    range.deleteContents()
    const node = document.createTextNode(text)
    range.insertNode(node)
    range.setStartAfter(node)
    range.collapse(true)
    selection?.removeAllRanges()
    selection?.addRange(range)
  }
  element.dispatchEvent(new InputEvent('input', { bubbles: true, inputType: 'insertText', data: text }))
  element.dispatchEvent(new Event('change', { bubbles: true }))
}
export function fillVisible(platform: string, text: string): { mediaInput?: HTMLInputElement } {
  const element = composer(platform)
  if (platform === 'qzone' && !(element instanceof HTMLTextAreaElement)) setQzoneText(element, text)
  else setText(element, text)
  for (const selector of adapter(platform).mediaSelectors) {
    const mediaInput = document.querySelector(selector) as HTMLInputElement | null
    if (mediaInput) return { mediaInput }
  }
  return {}
}

function visibleElement(selectors: string[]): HTMLElement | null {
  for (const selector of selectors) {
    for (const element of document.querySelectorAll<HTMLElement>(selector)) {
      if (!element.hidden && element.getAttribute('aria-hidden') !== 'true' && element.style.display !== 'none') return element
    }
  }
  return null
}

const wait = (milliseconds: number) => new Promise((resolve) => window.setTimeout(resolve, milliseconds))

async function waitForVisible(selectors: string[], timeout = 4_000): Promise<HTMLElement | null> {
  const deadline = Date.now() + timeout
  let found = visibleElement(selectors)
  while (!found && Date.now() < deadline) {
    await wait(100)
    found = visibleElement(selectors)
  }
  return found
}

async function prepareXiaohongshuComposer(hasMedia: boolean) {
  const creatorTab = visibleElement(['.creator-tab'])
  const modeSelectors = hasMedia
    ? ['.image-upload-buttons .upload-button']
    : ['.image-upload-buttons .text2image-button']
  const initialModeButton = visibleElement(modeSelectors)
  if (!creatorTab && !initialModeButton && findComposer('xiaohongshu')) return
  if (creatorTab) {
    creatorTab.click()
  }
  const modeButton = creatorTab ? await waitForVisible(modeSelectors) : initialModeButton
  if (modeButton) {
    modeButton.click()
    const deadline = Date.now() + 4_000
    while (!findComposer('xiaohongshu') && Date.now() < deadline) await wait(100)
  }
}
/** Click only an explicitly configured, visible, enabled publish control. */
export function clickKnownPublish(platform: string): string | null {
  const viable = new Set<HTMLElement>()
  const labels = adapter(platform).publishLabels
  for (const selector of adapter(platform).publishSelectors) {
    const candidates = document.querySelectorAll<HTMLElement>(selector)
    for (const element of candidates) {
      if (element.getAttribute('aria-disabled') === 'true' || (element as HTMLButtonElement).disabled) continue
      let current: HTMLElement | null = element
      let hidden = false
      while (current) {
        const style = getComputedStyle(current)
        if (current.hidden || current.getAttribute('aria-hidden') === 'true' || style.display === 'none' || style.visibility === 'hidden' || style.opacity === '0') { hidden = true; break }
        current = current.parentElement
      }
      if (hidden) continue
      if (labels && !labels.includes((element.innerText || element.textContent || '').trim())) continue
      viable.add(element)
    }
  }
  if (viable.size === 0) return '当前页面未找到明确的可见发布按钮，未自动点击；请人工发布'
  if (viable.size > 1) return '当前页面同时出现多个发布按钮，无法安全确定目标，未自动点击'
  viable.values().next().value?.click()
  return null
}
export async function fillVisibleWithMedia(platform: string, request: BridgeRequest, fetcher = fetch): Promise<string[]> {
  if (platform === 'xiaohongshu') await prepareXiaohongshuComposer(Boolean(request.media?.length))
  const { mediaInput } = fillVisible(platform, request.text || '')
  const failures: string[] = []
  if (!request.media?.length) return failures
  if (!mediaInput) return request.media.map((media) => `${media.filename}: 页面没有图片输入框`)
  const files: File[] = []
  for (const media of request.media) {
    if (!media.url) { failures.push(`${media.filename}: 本地媒体地址缺失`); continue }
    try {
      const response = await fetcher(media.url)
      if (!response.ok) throw new Error(`HTTP ${response.status}`)
      const blob = await response.blob()
      files.push(new File([blob], media.filename, { type: media.mime || blob.type }))
    } catch (error) { failures.push(`${media.filename}: ${error instanceof Error ? error.message : String(error)}`) }
  }
  if (files.length) {
    try {
      const transfer = new DataTransfer()
      files.forEach((file) => transfer.items.add(file))
      mediaInput.files = transfer.files
      mediaInput.dispatchEvent(new Event('input', { bubbles: true }))
      mediaInput.dispatchEvent(new Event('change', { bubbles: true }))
    } catch (error) { failures.push(`图片无法填入当前页面：${error instanceof Error ? error.message : String(error)}`) }
  }
  return failures
}
export function captureVisible(platform: string, pageURL = location.href, root: ParentNode = document): Capture {
  let element: HTMLElement | HTMLTextAreaElement | null = null
  for (const selector of adapter(platform).captureSelectors) { element = root.querySelector(selector) as HTMLElement | HTMLTextAreaElement | null; if (element) break }
  if (!element) {
    return { text: '', url: pageURL, media: [], incomplete: true, incomplete_reason: '当前页面没有可见正文' }
  }
	const text = (element instanceof HTMLTextAreaElement ? element.value : element.textContent || '').trim()
	const container = element.closest('article,.WB_feed_detail,.note-container,.feed-item') || element.parentElement || element
	const media = Array.from(container.querySelectorAll('img[src]')).slice(0, 16).map((image, index) => {
		const raw = (image as HTMLImageElement).currentSrc || (image as HTMLImageElement).src
		let filename = `image-${index + 1}`
		try { filename = new URL(raw, pageURL).pathname.split('/').filter(Boolean).pop() || filename } catch { /* retain generated name */ }
		return { filename, mime: '', url: raw, alt_text: (image as HTMLImageElement).alt || '' }
	})
	const incomplete = !text
	return { text, url: pageURL, media, incomplete, incomplete_reason: incomplete ? '当前页面没有可见正文' : undefined }
}
export const supportedBridgePlatforms = [...bridgePlatformIDs]
