<script setup lang="ts">
import { computed } from 'vue'
import { siMastodon, siBluesky, siX, siSinaweibo, siXiaohongshu, siQzone, siMisskey } from 'simple-icons'
import type { SimpleIcon } from 'simple-icons'
import type { PlatformID } from './platformRegistry'

const props = defineProps<{ platform?: string }>()

const iconMap: Record<PlatformID, SimpleIcon> = {
  mastodon: siMastodon,
  'mastodon-compatible': siMastodon,
  bluesky: siBluesky,
  x: siX,
  weibo: siSinaweibo,
  xiaohongshu: siXiaohongshu,
  qzone: siQzone,
  misskey: siMisskey,
}

const icon = computed<SimpleIcon | null>(() => (props.platform ? (iconMap[props.platform as PlatformID] ?? null) : null))
</script>

<template>
  <svg v-if="icon" aria-hidden="true" viewBox="0 0 24 24">
    <title>{{ icon.title }}</title>
    <path :d="icon.path" :fill="'#' + icon.hex" />
  </svg>
  <svg v-else aria-hidden="true" viewBox="0 0 20 20">
    <circle cx="10" cy="10" r="6" fill="none" stroke="currentColor" stroke-width="1.5"/><path d="M4 10h12M10 4c2 2 2 10 0 12M10 4c-2 2-2 10 0 12" fill="none" stroke="currentColor" stroke-width="1.2"/>
  </svg>
</template>
