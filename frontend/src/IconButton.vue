<script setup lang="ts">
import { Archive, ArrowRight, Check, Copy, Pencil, Power, RefreshCcw, Trash2 } from '@lucide/vue'
defineProps<{ label: string; icon: 'check' | 'edit' | 'trash' | 'refresh' | 'copy' | 'arrow' | 'archive' | 'power' }>()
const iconComponents = { check: Check, edit: Pencil, trash: Trash2, refresh: RefreshCcw, copy: Copy, arrow: ArrowRight, archive: Archive, power: Power }
</script>

<template>
  <button class="icon-button" type="button" :aria-label="label" :title="label">
    <component :is="iconComponents[icon]" aria-hidden="true" />
    <span class="sr-only">{{ label }}</span>
  </button>
</template>
