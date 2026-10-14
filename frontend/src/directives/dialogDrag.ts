import type { Directive } from 'vue'

const margin = 12
const interactiveSelector = 'button, input, select, textarea, a, summary, [contenteditable="true"], [role="button"], [role="link"]'
const keyboardLabel = '拖动对话框；聚焦标题栏后可使用方向键移动，按住 Shift 可快速移动'

type SavedStyle = { value: string; priority: string }

function saveStyle(element: HTMLElement, properties: string[]) {
  return Object.fromEntries(properties.map((property) => [property, {
    value: element.style.getPropertyValue(property),
    priority: element.style.getPropertyPriority(property),
  }])) as Record<string, SavedStyle>
}

function restoreStyle(element: HTMLElement, styles: Record<string, SavedStyle>) {
  for (const [property, saved] of Object.entries(styles)) {
    if (saved.value) element.style.setProperty(property, saved.value, saved.priority)
    else element.style.removeProperty(property)
  }
}

function saveAttribute(element: HTMLElement, name: string) {
  return element.hasAttribute(name) ? element.getAttribute(name) : null
}

function clamp(value: number, min: number, max: number) {
  return Math.min(Math.max(value, min), max)
}

export const vDialogDrag: Directive<HTMLElement, boolean | undefined> = {
  mounted(dialog, binding) {
    if (binding.value === false) return
    installDialogDrag(dialog)
  },
  updated(dialog, binding) {
    if (binding.value === false) cleanupByElement.get(dialog)?.()
    else installDialogDrag(dialog)
  },
  unmounted(dialog) {
    cleanupByElement.get(dialog)?.()
  },
}

const cleanupByElement = new WeakMap<HTMLElement, () => void>()

function installDialogDrag(dialog: HTMLElement) {
  if (cleanupByElement.has(dialog)) return
  const candidate = dialog.querySelector<HTMLElement>(':scope > header, :scope > .modal-titlebar')
  if (!candidate) return
  const header: HTMLElement = candidate

  const dialogStyles = saveStyle(dialog, ['position', 'left', 'top', 'right', 'bottom', 'transform', 'margin'])
  const headerStyles = saveStyle(header, ['cursor', 'touch-action', 'user-select'])
  const previousTabIndex = saveAttribute(header, 'tabindex')
  const previousAriaLabel = saveAttribute(header, 'aria-label')
  const alreadyHadHandleClass = header.classList.contains('dialog-drag-handle')
  let pointerID: number | null = null
  let pointerStartX = 0
  let pointerStartY = 0
  let dialogStartX = 0
  let dialogStartY = 0
  let moved = false

  header.style.setProperty('cursor', 'grab')
  header.style.setProperty('touch-action', 'none')
  header.style.setProperty('user-select', 'none')
  header.setAttribute('tabindex', '0')
  header.setAttribute('aria-label', keyboardLabel)
  header.classList.add('dialog-drag-handle')

  function anchorRect(dialogRect: DOMRect) {
    if (dialogRect.width <= window.innerWidth - margin * 2 && dialogRect.height <= window.innerHeight - margin * 2) return dialogRect
    const headerRect = header.getBoundingClientRect()
    if (headerRect.width <= window.innerWidth - margin * 2 && headerRect.height <= window.innerHeight - margin * 2) return headerRect
    const close = header.querySelector<HTMLElement>('button[aria-label*="关闭"], button[title*="关闭"], button[aria-label*="close" i]')
    return close?.getBoundingClientRect() || headerRect
  }

  function constrain(x: number, y: number) {
    const dialogRect = dialog.getBoundingClientRect()
    const anchor = anchorRect(dialogRect)
    const offsetX = anchor.left - dialogRect.left
    const offsetY = anchor.top - dialogRect.top
    const maxX = window.innerWidth - margin - anchor.width - offsetX
    const maxY = window.innerHeight - margin - anchor.height - offsetY
    const minX = margin - offsetX
    const minY = margin - offsetY
    return {
      x: clamp(x, Math.min(minX, maxX), Math.max(minX, maxX)),
      y: clamp(y, Math.min(minY, maxY), Math.max(minY, maxY)),
    }
  }

  function setPosition(x: number, y: number) {
    if (!moved) {
      dialog.style.setProperty('position', 'fixed')
      dialog.style.setProperty('right', 'auto')
      dialog.style.setProperty('bottom', 'auto')
      dialog.style.setProperty('transform', 'none')
      dialog.style.setProperty('margin', '0')
      moved = true
    }
    const position = constrain(x, y)
    dialog.style.setProperty('left', `${position.x}px`)
    dialog.style.setProperty('top', `${position.y}px`)
  }

  function moveIntoView() {
    const rect = dialog.getBoundingClientRect()
    const position = constrain(rect.left, rect.top)
    if (position.x !== rect.left || position.y !== rect.top) setPosition(position.x, position.y)
  }

  function finishDrag(releaseCapture = false) {
    const id = pointerID
    pointerID = null
    header.style.setProperty('cursor', 'grab')
    if (releaseCapture && id !== null && header.hasPointerCapture(id)) {
      try { header.releasePointerCapture(id) } catch { /* Capture may already have ended. */ }
    }
  }

  function onPointerDown(event: PointerEvent) {
    if (!event.isPrimary || event.button !== 0 || (event.target as Element | null)?.closest(interactiveSelector)) return
    pointerID = event.pointerId
    pointerStartX = event.clientX
    pointerStartY = event.clientY
    const rect = dialog.getBoundingClientRect()
    dialogStartX = rect.left
    dialogStartY = rect.top
    setPosition(rect.left, rect.top)
    header.style.setProperty('cursor', 'grabbing')
    try {
      header.setPointerCapture(event.pointerId)
      event.preventDefault()
    } catch {
      finishDrag()
    }
  }

  function onPointerMove(event: PointerEvent) {
    if (event.pointerId !== pointerID) return
    setPosition(dialogStartX + event.clientX - pointerStartX, dialogStartY + event.clientY - pointerStartY)
  }

  function onPointerUp(event: PointerEvent) {
    if (event.pointerId === pointerID) finishDrag(true)
  }

  function onKeyDown(event: KeyboardEvent) {
    if (event.target !== header) return
    const offsets: Record<string, [number, number]> = {
      ArrowLeft: [-1, 0], ArrowRight: [1, 0], ArrowUp: [0, -1], ArrowDown: [0, 1],
    }
    const offset = offsets[event.key]
    if (!offset) return
    event.preventDefault()
    const rect = dialog.getBoundingClientRect()
    const step = event.shiftKey ? 40 : 10
    setPosition(rect.left + offset[0] * step, rect.top + offset[1] * step)
  }

  header.addEventListener('pointerdown', onPointerDown)
  header.addEventListener('pointermove', onPointerMove)
  header.addEventListener('pointerup', onPointerUp)
  header.addEventListener('pointercancel', onPointerUp)
  header.addEventListener('lostpointercapture', onPointerUp)
  header.addEventListener('keydown', onKeyDown)
  window.addEventListener('resize', moveIntoView)
  const observer = typeof ResizeObserver === 'undefined' ? undefined : new ResizeObserver(moveIntoView)
  observer?.observe(dialog)
  observer?.observe(header)

  const cleanup = () => {
    if (!cleanupByElement.has(dialog)) return
    finishDrag(true)
    header.removeEventListener('pointerdown', onPointerDown)
    header.removeEventListener('pointermove', onPointerMove)
    header.removeEventListener('pointerup', onPointerUp)
    header.removeEventListener('pointercancel', onPointerUp)
    header.removeEventListener('lostpointercapture', onPointerUp)
    header.removeEventListener('keydown', onKeyDown)
    window.removeEventListener('resize', moveIntoView)
    observer?.disconnect()
    restoreStyle(dialog, dialogStyles)
    restoreStyle(header, headerStyles)
    if (previousTabIndex === null) header.removeAttribute('tabindex')
    else header.setAttribute('tabindex', previousTabIndex)
    if (previousAriaLabel === null) header.removeAttribute('aria-label')
    else header.setAttribute('aria-label', previousAriaLabel)
    if (!alreadyHadHandleClass) header.classList.remove('dialog-drag-handle')
    cleanupByElement.delete(dialog)
  }
  cleanupByElement.set(dialog, cleanup)
}

export default vDialogDrag
