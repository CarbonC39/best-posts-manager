<script setup lang="ts">
defineProps<{ label: string; icon: 'check' | 'edit' | 'trash' | 'refresh' | 'copy' | 'arrow' | 'archive' }>()
</script>

<template>
  <button class="icon-button" type="button" :aria-label="label" :title="label">
    <svg aria-hidden="true" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">
      <path v-if="icon === 'check'" d="m4 10 3.5 3.5L16 5" />
      <path v-else-if="icon === 'edit'" d="m4 14.8-.7 2.9 2.9-.7L16.5 6.7a1.8 1.8 0 0 0-2.5-2.5L4 14.8Z" />
      <path v-else-if="icon === 'trash'" d="M4.5 6.5h11M8 6.5V4h4v2.5m-6.5 0 .7 10h7.6l.7-10M8.5 9v5m3-5v5" />
      <path v-else-if="icon === 'refresh'" d="M15.5 7.5V4.5h-3M15.2 4.8A6 6 0 1 0 16 12" />
      <path v-else-if="icon === 'copy'" d="M7 7V4.5h8.5V13H13M4.5 7h8.5v8.5H4.5V7Z" />
      <path v-else-if="icon === 'archive'" d="M4 7h12v9H4V7Zm-1-3h14v3H3V4Zm5 6h4" />
      <path v-else d="M4 10h11m-4-4 4 4-4 4" />
    </svg>
    <span class="sr-only">{{ label }}</span>
  </button>
</template>
