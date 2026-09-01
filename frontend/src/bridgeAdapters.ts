export type BridgeMedia = { id?: string; filename: string; mime: string; alt_text?: string; size?: number; url?: string }
export type BridgeRequest = { platform: string; page_url: string; text?: string; media?: BridgeMedia[] }
export type Capture = { text: string; url: string; media: BridgeMedia[]; incomplete: boolean; incomplete_reason?: string }

type Adapter = { selectors: string[]; captureSelectors: string[]; mediaSelectors: string[] }
const adapters: Record<string, Adapter> = {
  x: { selectors: ['[data-testid="tweetTextarea_0"]', 'textarea[aria-label*="Post"]', 'textarea', '[contenteditable="true"]'], captureSelectors: ['article [data-testid="tweetText"]', 'article'], mediaSelectors: ['input[type="file"]'] },
  weibo: { selectors: ['textarea[placeholder*="分享"]', 'textarea', '[contenteditable="true"]'], captureSelectors: ['.WB_text', '[node-type="feed_list_content"]'], mediaSelectors: ['input[type="file"]'] },
  xiaohongshu: { selectors: ['textarea', '[contenteditable="true"]'], captureSelectors: ['.note-content', '[class*="note-content"]'], mediaSelectors: ['input[type="file"]'] },
  qzone: { selectors: ['textarea', '[contenteditable="true"]'], captureSelectors: ['.feed-content', '[class*="content"]'], mediaSelectors: ['input[type="file"]'] },
}

function adapter(platform: string): Adapter {
  const value = adapters[platform]
  if (!value) throw new Error(`unsupported bridge platform: ${platform}`)
  return value
}
function composer(platform: string, root: ParentNode = document): HTMLElement | HTMLTextAreaElement {
  for (const selector of adapter(platform).selectors) {
    const found = root.querySelector(selector) as HTMLElement | HTMLTextAreaElement | null
    if (found && !found.hidden && found.getAttribute('aria-hidden') !== 'true' && found.style.display !== 'none') return found
  }
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
export function fillVisible(platform: string, text: string): { mediaInput?: HTMLInputElement } {
  const element = composer(platform)
  setText(element, text)
  const mediaInput = document.querySelector(adapter(platform).mediaSelectors[0]) as HTMLInputElement | null
  return mediaInput ? { mediaInput } : {}
}
export async function fillVisibleWithMedia(platform: string, request: BridgeRequest, fetcher = fetch): Promise<string[]> {
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
export const supportedBridgePlatforms = Object.keys(adapters)
