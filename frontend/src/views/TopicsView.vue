<script setup lang="ts">
import IconButton from '../IconButton.vue'
import PlatformIcon from '../PlatformIcon.vue'
import { platformLabel } from '../platformRegistry'

type ProfileItem = { id: string; name: string }
type TopicItem = { id: string; profile_id: string; title: string; status: string; updated_at?: string; target_count?: number; published_count?: number; needs_action_count?: number; in_progress_count?: number; platforms?: string[] }

defineProps<{
  profiles: ProfileItem[]
  topics: TopicItem[]
  selectedProfileId: string
  topicSearch: string
  topicTitle: string
  showArchived: boolean
  progressLabel: (topic: TopicItem) => string
  timeLabel: (topic: TopicItem) => string
}>()

const emit = defineEmits<{
  'update:selectedProfileId': [value: string]
  'update:topicSearch': [value: string]
  'update:topicTitle': [value: string]
  'update:showArchived': [value: boolean]
  create: []
  'select-topic': [event: MouseEvent, topic: TopicItem]
  'toggle-archive': [topic: TopicItem, status: 'active' | 'archived']
}>()
</script>

<template>
  <section class="page-section topics-page">
    <header class="management-head"><div class="topics-title-row"><h1 id="topics-title">全部话题</h1><span class="topic-count" aria-label="话题数量">{{topics.length}} 个</span></div></header>
    <form class="topic-create mobile-topic-create" @submit.prevent="emit('create')"><input :value="topicTitle" placeholder="新话题标题" :disabled="!selectedProfileId" @input="emit('update:topicTitle', ($event.target as HTMLInputElement).value)"><button class="primary-action" :disabled="!selectedProfileId">＋ 新建话题</button></form>
    <section class="topic-browser" aria-label="话题筛选与列表"><header class="topic-browser-head"><div class="topic-management-tools">
      <label class="management-search"><svg viewBox="0 0 20 20" aria-hidden="true"><circle cx="8.5" cy="8.5" r="5" fill="none" stroke="currentColor" stroke-width="1.5"/><path d="m12.5 12.5 4 4" fill="none" stroke="currentColor" stroke-linecap="round" stroke-width="1.5"/></svg><input :value="topicSearch" placeholder="搜索标题" aria-label="搜索全部话题" @input="emit('update:topicSearch', ($event.target as HTMLInputElement).value)"></label>
      <label class="archived-filter"><input type="checkbox" :checked="showArchived" @change="emit('update:showArchived', ($event.target as HTMLInputElement).checked)">显示已归档</label>
      <select id="content-profile" class="profile-select" :value="selectedProfileId" aria-label="Profile" @change="emit('update:selectedProfileId', ($event.target as HTMLSelectElement).value)"><option v-for="profile in profiles" :key="profile.id" :value="profile.id">{{profile.name}}</option></select>
    </div></header>
    <p v-if="profiles.length===0" class="empty-state">还没有 Profile。前往账号管理创建后，就可以开始第一个话题。</p><p v-else-if="topics.length===0" class="empty-state">这个 Profile 还没有话题。</p><ul v-else class="rows management-list topic-list"><li v-for="topic in topics" :key="topic.id" class="management-row"><div class="row-main"><a class="link" :href="`/topics/${encodeURIComponent(topic.id)}`" @click="emit('select-topic', $event, topic)">{{topic.title}}</a><small v-if="timeLabel(topic)">{{timeLabel(topic)}} · {{topic.status==='archived'?'已归档':'进行中'}}</small></div><strong class="topic-progress" :class="{attention:Boolean(topic.needs_action_count)}">{{progressLabel(topic)}}</strong><div v-if="topic.platforms?.length" class="topic-platforms" aria-label="目标平台"><span v-for="platform in topic.platforms" :key="platform" :title="platformLabel(platform)"><PlatformIcon :platform="platform" /></span></div><IconButton :icon="topic.status==='archived' ? 'refresh' : 'archive'" :label="topic.status==='archived' ? '恢复话题' : '归档话题'" @click="emit('toggle-archive', topic, topic.status==='archived'?'active':'archived')" /></li></ul></section>
  </section>
</template>
