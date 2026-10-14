<script setup lang="ts">
import PublicationList from '../PublicationList.vue'

defineProps<{ profileId: string }>()
const emit = defineEmits<{ notify: [message: string]; changed: [] }>()
</script>

<template>
  <section class="page-section history-page">
    <PublicationList :profile-id="profileId" @notify="emit('notify', $event)" @changed="emit('changed')" />
  </section>
</template>
