<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { api } from './api'
import PublicationList from './PublicationList.vue'

type Profile = { id: string; name: string }
type Account = { id: string; profile_id: string; platform: string; name: string }
type Topic = { id: string; profile_id: string; title: string; status: string }
type Draft = { topic_id: string; text: string; format: string; revision: number }
type Variant = { id: string; topic_id: string; account_id: string; text: string; status: string; relation: string; visibility: string; spoiler_text: string; outdated: boolean }
type Media = { id: string; variant_id: string; filename: string; alt_text: string }
type ExternalPost = { id: string; platform: string; url: string }
type Emoji = { shortcode: string; url: string; static_url: string }

const section = ref('topics')
const profiles = ref<Profile[]>([])
const accounts = ref<Account[]>([])
const topics = ref<Topic[]>([])
const variants = ref<Variant[]>([])
const media = ref<Media[]>([])
const externalPosts = ref<ExternalPost[]>([])
const emojis = ref<Emoji[]>([])
const draft = ref<Draft | null>(null)
const selected = ref<Topic | null>(null)
const message = ref('')
const selectedProfileID = ref('')
const selectedAccountID = ref('')
const mediaVariantID = ref('')
const profileName = ref('')
const accountName = ref('')
const baseURL = ref('')
const tokenEnv = ref('')
const topicTitle = ref('')
const text = ref('')
const altText = ref('')
const externalURL = ref('')
const externalPlatform = ref('mastodon')
const externalAccountID = ref('')

const profileAccounts = computed(() => accounts.value.filter((account) => account.profile_id === selectedProfileID.value))
const profileTopics = computed(() => topics.value.filter((topic) => topic.profile_id === selectedProfileID.value && topic.status === 'active'))

watch(selectedProfileID, () => {
  if (!profileAccounts.value.some((account) => account.id === selectedAccountID.value)) {
    selectedAccountID.value = profileAccounts.value[0]?.id || ''
  }
})

function showError(error: unknown) {
  message.value = `操作失败：${error instanceof Error ? error.message : String(error)}`
}

async function load() {
  try {
    ;[profiles.value, accounts.value, topics.value] = await Promise.all([
      api<Profile[]>('profiles'), api<Account[]>('accounts'), api<Topic[]>('topics'),
    ])
    if (!selectedProfileID.value && profiles.value[0]) selectedProfileID.value = profiles.value[0].id
    if (!selectedAccountID.value && profileAccounts.value[0]) selectedAccountID.value = profileAccounts.value[0].id
  } catch (error) { showError(error) }
}

async function addProfile() {
  if (!profileName.value.trim()) return
  try {
    const created = await api<Profile>('profiles', { method: 'POST', body: JSON.stringify({ name: profileName.value }) })
    profileName.value = ''
    await load()
    selectedProfileID.value = created.id
  } catch (error) { showError(error) }
}

async function addAccount() {
  if (!accountName.value.trim() || !selectedProfileID.value) return
  try {
    await api('accounts', { method: 'POST', body: JSON.stringify({ profile_id: selectedProfileID.value, name: accountName.value, platform: 'mastodon', base_url: baseURL.value, token_env: tokenEnv.value }) })
    accountName.value = ''; baseURL.value = ''; tokenEnv.value = ''
    await load()
  } catch (error) { showError(error) }
}

async function addTopic() {
  if (!topicTitle.value.trim() || !selectedProfileID.value) return
  try {
    await api('topics', { method: 'POST', body: JSON.stringify({ profile_id: selectedProfileID.value, title: topicTitle.value }) })
    topicTitle.value = ''
    await load()
  } catch (error) { showError(error) }
}

async function openTopic(topic: Topic) {
  try {
    selected.value = topic
    selectedProfileID.value = topic.profile_id
    ;[draft.value, variants.value, media.value, externalPosts.value] = await Promise.all([
      api<Draft>(`topics/${topic.id}/draft`), api<Variant[]>(`variants?topic_id=${topic.id}`),
      api<Media[]>(`media?topic_id=${topic.id}`), api<ExternalPost[]>(`external-posts?topic_id=${topic.id}`),
    ])
    text.value = draft.value.text
    mediaVariantID.value = variants.value[0]?.id || ''
    section.value = 'editor'
  } catch (error) { showError(error) }
}

async function saveDraft() {
  if (!selected.value) return
  try {
    draft.value = await api<Draft>(`topics/${selected.value.id}/draft`, { method: 'PUT', body: JSON.stringify({ text: text.value, format: 'plain', language: '' }) })
    variants.value = await api<Variant[]>(`variants?topic_id=${selected.value.id}`)
    message.value = '基础稿已保存'
  } catch (error) { showError(error) }
}

async function newVariant() {
  if (!selected.value || !selectedAccountID.value) return
  try {
    await api('variants', { method: 'POST', body: JSON.stringify({ topic_id: selected.value.id, account_id: selectedAccountID.value }) })
    variants.value = await api<Variant[]>(`variants?topic_id=${selected.value.id}`)
    mediaVariantID.value ||= variants.value[0]?.id || ''
  } catch (error) { showError(error) }
}

async function saveVariant(variant: Variant) {
  try {
    Object.assign(variant, await api<Variant>(`variants/${variant.id}`, { method: 'PUT', body: JSON.stringify(variant) }))
    message.value = '平台版本已保存并与基础稿分离'
  } catch (error) { showError(error) }
}

async function applyVariant(variant: Variant) {
  try {
    Object.assign(variant, await api<Variant>(`variants/${variant.id}/apply`, { method: 'POST' }))
    message.value = '已应用基础稿更新'
  } catch (error) { showError(error) }
}

async function publish(id: string) {
  try {
    await api('publications', { method: 'POST', body: JSON.stringify({ variant_id: id }) })
    message.value = '发布完成（请查看历史）'
  } catch (error) { message.value = `发布失败：${error instanceof Error ? error.message : String(error)}` }
}

async function markExternal() {
  if (!selected.value || !externalURL.value.trim()) return
  try {
    await api('external-posts', { method: 'POST', body: JSON.stringify({ topic_id: selected.value.id, account_id: externalAccountID.value, platform: externalPlatform.value, url: externalURL.value }) })
    externalURL.value = ''
    externalPosts.value = await api<ExternalPost[]>(`external-posts?topic_id=${selected.value.id}`)
    message.value = '已标记站外 URL'
  } catch (error) { showError(error) }
}

async function uploadMedia(event: Event) {
  if (!selected.value) return
  const input = event.target as HTMLInputElement
  if (!input.files?.[0]) return
  const form = new FormData()
  form.append('topic_id', selected.value.id)
  if (mediaVariantID.value) form.append('variant_id', mediaVariantID.value)
  form.append('alt_text', altText.value); form.append('file', input.files[0])
  try {
    const response = await fetch('/api/media', { method: 'POST', body: form })
    const body = await response.json()
    if (!response.ok) throw new Error(body.error || `HTTP ${response.status}`)
    media.value.push(body); altText.value = ''; input.value = ''
    message.value = '图片已保存'
  } catch (error) { showError(error) }
}

async function loadEmoji() {
  if (!selectedAccountID.value) return
  try {
    emojis.value = await api<Emoji[]>(`accounts/${selectedAccountID.value}/emojis`)
    message.value = `已加载 ${emojis.value.length} 个实例 emoji`
  } catch (error) { message.value = `emoji 加载失败：${error instanceof Error ? error.message : String(error)}` }
}

onMounted(load)
</script>

<template>
  <div class="shell">
    <aside><h1>Best Post Manager</h1><nav><button :class="{active:section==='topics'}" @click="section='topics'">话题</button><button :class="{active:section==='editor'}" @click="section='editor'">编辑器</button><button :class="{active:section==='history'}" @click="section='history'">发布历史</button></nav><small>本地单用户</small></aside>
    <main>
      <div v-if="message" class="notice">{{message}}</div>
      <section v-if="section==='topics'">
        <h2>话题</h2><label>Profile <select v-model="selectedProfileID"><option v-for="profile in profiles" :key="profile.id" :value="profile.id">{{profile.name}}</option></select></label>
        <form @submit.prevent="addProfile"><input v-model="profileName" placeholder="新 Profile"><button>添加 Profile</button></form>
        <form @submit.prevent="addAccount"><input v-model="accountName" placeholder="账号名称"><input v-model="baseURL" placeholder="Mastodon 实例 URL"><input v-model="tokenEnv" placeholder="token 环境变量名"><button>添加账号</button></form>
        <form @submit.prevent="addTopic"><input v-model="topicTitle" placeholder="新话题标题"><button>添加话题</button></form>
        <ul class="rows"><li v-for="topic in profileTopics" :key="topic.id"><button class="link" @click="openTopic(topic)">{{topic.title}}</button><span>{{topic.status}}</span></li></ul>
      </section>
      <section v-else-if="section==='editor'">
        <button class="back" @click="section='topics'">← 返回话题</button><h2>{{selected?.title}}</h2><textarea v-model="text" placeholder="基础稿"></textarea>
        <div class="actions"><button @click="saveDraft">保存基础稿</button><select v-model="selectedAccountID"><option v-for="account in profileAccounts" :key="account.id" :value="account.id">{{account.name}} ({{account.platform}})</option></select><button @click="newVariant">从基础稿生成 Variant</button></div>
        <div class="actions"><select v-model="mediaVariantID"><option value="">通用媒体</option><option v-for="variant in variants" :key="variant.id" :value="variant.id">{{variant.account_id}}</option></select><input v-model="altText" placeholder="图片替代文本"><label class="upload">添加图片<input type="file" accept="image/*" @change="uploadMedia"></label></div>
        <ul v-if="media.length" class="media-list"><li v-for="item in media" :key="item.id"><img :src="`/media/${item.id}`" :alt="item.alt_text||item.filename"><span>{{item.filename}}</span></li></ul>
        <button @click="loadEmoji">加载实例 emoji</button><div class="emoji-list"><span v-for="emoji in emojis" :key="emoji.shortcode" class="emoji"><img :src="emoji.static_url||emoji.url" alt="">:{{emoji.shortcode}}:</span></div>
        <h3>平台版本</h3><small>节点 1 默认按 500 字符、最多 4 张图片校验；实例差异会在发布错误中显示。</small>
        <ul class="rows variants"><li v-for="variant in variants" :key="variant.id"><div><span>{{variant.account_id}} · {{variant.relation}} · {{variant.status}}<em v-if="variant.outdated">有基础稿更新</em></span><textarea v-model="variant.text" rows="3"></textarea><select v-model="variant.visibility"><option>public</option><option>unlisted</option><option>private</option><option>direct</option></select><input v-model="variant.spoiler_text" placeholder="CW（可选）"><pre class="preview">{{variant.text}}</pre></div><div class="row-actions"><button @click="saveVariant(variant)">保存</button><button v-if="variant.outdated" @click="applyVariant(variant)">应用更新</button><button @click="publish(variant.id)">发布</button></div></li></ul>
        <h3>站外关联</h3>
        <form @submit.prevent="markExternal"><select v-model="externalPlatform"><option>mastodon</option><option>x</option><option>weibo</option><option>xiaohongshu</option><option>qzone</option></select><select v-model="externalAccountID"><option value="">无账号</option><option v-for="account in profileAccounts" :key="account.id" :value="account.id">{{account.name}}</option></select><input v-model="externalURL" placeholder="仅标记 URL，不抓取"><button>标记 URL</button></form>
        <ul class="rows"><li v-for="post in externalPosts" :key="post.id"><span>{{post.platform}}</span><a :href="post.url" target="_blank" rel="noreferrer">{{post.url}}</a></li></ul>
      </section>
      <section v-else><h2>发布历史</h2><PublicationList/></section>
    </main>
  </div>
</template>
