<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { X } from '@lucide/vue'
import PlatformIcon from './PlatformIcon.vue'
import { vDialogDrag } from './directives/dialogDrag'

type PreviewMedia = { id: string; filename: string; alt_text: string }

const props = defineProps<{
  accountName: string
  platform: string
  platformName: string
  text: string
  spoilerText?: string
  visibility: string
  localOnly?: boolean
  media: PreviewMedia[]
}>()
const emit = defineEmits<{ close: [] }>()
const closeButton = ref<HTMLButtonElement | null>(null)
const dialog = ref<HTMLElement | null>(null)

function onKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') emit('close')
  if (event.key !== 'Tab' || !dialog.value) return
  const controls = Array.from(dialog.value.querySelectorAll<HTMLElement>('button:not([disabled]), [tabindex="0"]'))
  const first = controls[0], last = controls[controls.length - 1]
  if (!first || !last) return
  if (event.shiftKey && (document.activeElement === first || !dialog.value.contains(document.activeElement))) { event.preventDefault(); last.focus() }
  else if (!event.shiftKey && (document.activeElement === last || !dialog.value.contains(document.activeElement))) { event.preventDefault(); first.focus() }
}

onMounted(() => {
  window.addEventListener('keydown', onKeydown)
  closeButton.value?.focus()
})
onBeforeUnmount(() => window.removeEventListener('keydown', onKeydown))
</script>

<template>
  <div class="post-preview-backdrop" role="presentation" @click.self="emit('close')">
    <section ref="dialog" v-dialog-drag class="post-preview-dialog" role="dialog" aria-modal="true" aria-labelledby="post-preview-title">
      <header class="post-preview-header">
        <div>
          <span class="post-preview-eyebrow">{{ platformName }}</span>
          <h2 id="post-preview-title">帖子预览</h2>
        </div>
        <button ref="closeButton" type="button" class="post-preview-close" aria-label="关闭预览" title="关闭预览" @click="emit('close')"><X aria-hidden="true" /></button>
      </header>

      <div class="post-preview-body">
        <article class="post-preview-card" :class="`preview-${platform}`">
          <header class="post-preview-identity">
            <span class="post-preview-platform" aria-hidden="true"><PlatformIcon :platform="platform" /></span>
            <span><strong>{{ accountName }}</strong><small>{{ platformName }} · {{ visibility }}<template v-if="localOnly"> · 仅本地</template></small></span>
          </header>
          <section v-if="spoilerText" class="post-preview-spoiler" aria-label="内容警告"><strong>{{ spoilerText }}</strong></section>
          <p v-if="text" class="post-preview-text">{{ text }}</p>
          <div v-if="media.length" class="post-preview-media" :class="`media-count-${Math.min(media.length, 4)}`">
            <img v-for="item in media.slice(0, 4)" :key="item.id" :src="`/media/${item.id}`" :alt="item.alt_text || item.filename">
            <span v-if="media.length > 4" class="post-preview-more">+{{ media.length - 4 }}</span>
          </div>
        </article>
      </div>
    </section>
  </div>
</template>

<style scoped>
.post-preview-backdrop { position: fixed; z-index: 30; inset: 0; display: grid; place-items: center; padding: 20px; background: rgb(25 39 52 / 44%); }
.post-preview-dialog { display: flex; flex-direction: column; width: 540px; height: min(600px, calc(100dvh - 48px)); max-width: calc(100vw - 32px); max-height: calc(100dvh - 48px); box-sizing: border-box; padding: 20px; overflow: hidden; border: 1px solid #dce4ea; border-radius: 14px; background: #fff; box-shadow: 0 16px 50px rgb(25 39 52 / 18%); }
.post-preview-header { display: flex; flex: 0 0 auto; align-items: flex-start; justify-content: space-between; gap: 20px; margin-bottom: 16px; }
.post-preview-body { flex: 1 1 auto; min-height: 0; overflow-x: hidden; overflow-y: auto; }
.post-preview-eyebrow { display: block; color: #56687a; font-size: 12px; line-height: 1.4; }
.post-preview-header h2 { margin: 2px 0 0; color: #253548; font-size: 22px; line-height: 1.25; }
.post-preview-close { display: grid; place-items: center; width: 36px; height: 36px; padding: 0; border: 0; border-radius: 9px; color: #425b70; background: transparent; }
.post-preview-close svg { width: 20px; height: 20px; stroke-width: 1.9; }
.post-preview-close:hover { color: #28525a; background: #edf6f8; }
.post-preview-card { overflow: hidden; padding: 18px; border: 1px solid #dce4ea; border-radius: 12px; background: #fff; }
.post-preview-identity { display: flex; align-items: center; gap: 10px; }
.post-preview-platform { display: grid; place-items: center; width: 34px; height: 34px; border-radius: 50%; color: #fff; background: #264e36; }
.post-preview-platform :deep(svg) { width: 18px; height: 18px; }
.preview-x .post-preview-platform :deep(path) { fill: #fff !important; }
.post-preview-identity > span:last-child { display: grid; min-width: 0; gap: 2px; }
.post-preview-identity strong { overflow: hidden; color: #253548; font-size: 14px; text-overflow: ellipsis; white-space: nowrap; }
.post-preview-identity small { color: #56687a; font-size: 12px; }
.post-preview-spoiler { margin-top: 16px; padding: 10px 12px; border-radius: 7px; color: #60491f; background: #faf3e6; font-size: 13px; }
.post-preview-text { margin: 16px 0 0; color: #253548; font-size: 14px; line-height: 1.65; overflow-wrap: anywhere; white-space: pre-wrap; }
.post-preview-media { position: relative; display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 3px; margin-top: 16px; overflow: hidden; border-radius: 9px; background: #fff; }
.post-preview-media.media-count-3 { aspect-ratio: 16 / 10; grid-template-columns: 1.35fr 1fr; grid-template-rows: repeat(2, minmax(0, 1fr)); }
.post-preview-media.media-count-3 img:first-child { grid-column: 1; grid-row: 1 / 3; height: 100%; }
.post-preview-media.media-count-3 img:nth-child(2) { grid-column: 2; grid-row: 1; height: 100%; }
.post-preview-media.media-count-3 img:nth-child(3) { grid-column: 2; grid-row: 2; height: 100%; }
.post-preview-media.media-count-1 { grid-template-columns: 1fr; }
.post-preview-media img { display: block; width: 100%; height: 180px; object-fit: cover; }
.post-preview-media.media-count-1 img { height: auto; max-height: 430px; object-fit: contain; }
.post-preview-more { position: absolute; right: 8px; bottom: 8px; padding: 4px 7px; border-radius: 5px; color: #fff; background: rgb(24 29 25 / 76%); font-size: 11px; }
.preview-bluesky .post-preview-platform { background: #087ccf; }
.preview-mastodon .post-preview-platform, .preview-mastodon-compatible .post-preview-platform { background: #6364ff; }
.preview-misskey .post-preview-platform { background: #78b34b; }
.preview-x .post-preview-platform { background: #151515; }
.preview-weibo .post-preview-platform { background: #e84c3d; }
.preview-xiaohongshu .post-preview-platform { background: #ff2442; }
.preview-qzone .post-preview-platform { background: #e5a900; }

@media (max-width: 600px) {
  .post-preview-backdrop { align-items: end; padding: 0; }
  .post-preview-dialog { width: calc(100vw - 24px); height: min(600px, calc(100dvh - 48px)); max-width: calc(100vw - 24px); max-height: calc(100dvh - 48px); border-radius: 14px 14px 0 0; }
}
</style>
