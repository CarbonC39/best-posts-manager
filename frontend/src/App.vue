<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { api } from './api'
import PublicationList from './PublicationList.vue'

type Profile = { id: string; name: string }
type Account = { id: string; profile_id: string; platform: string; name: string; remote_id: string }
type Topic = { id: string; profile_id: string; title: string; status: string }
type Draft = { topic_id: string; text: string; format: string; revision: number }
type Variant = { id: string; topic_id: string; account_id: string; text: string; format: string; status: string; relation: string; visibility: string; spoiler_text: string; local_only: boolean; outdated: boolean }
type Media = { id: string; variant_id: string; filename: string; alt_text: string }
type ExternalPost = { id: string; platform: string; url: string; text_snapshot: string; incomplete: boolean; incomplete_reason: string }
type Emoji = { shortcode: string; url: string; static_url: string }
type BridgePairing = { id: string; platform: string; code?: string; expires_at: string; code_used: boolean; revoked: boolean }
type BridgeRequest = { id: string; kind: string; status: string; error?: string; external_post_id?: string }
type Capability = { platform: string; format: string; text_max_bytes: number; max_media: number; max_media_bytes: number; visibilities: string[]; supports_cw: boolean; supports_local_only: boolean; custom_emoji: boolean; visibility_note?: string }
type TranslationConfig = { id: string; provider: string; base_url: string; project_id: string; location: string; token_env: string; key_env: string; model: string }

const section = ref('topics')
const profiles = ref<Profile[]>([])
const accounts = ref<Account[]>([])
const topics = ref<Topic[]>([])
const variants = ref<Variant[]>([])
const media = ref<Media[]>([])
const externalPosts = ref<ExternalPost[]>([])
const emojis = ref<Emoji[]>([])
const capabilities = ref<Record<string, Capability>>({})
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
const remoteID = ref('')
const topicTitle = ref('')
const text = ref('')
const altText = ref('')
const externalURL = ref('')
const externalReference = ref('')
const externalPlatform = ref('mastodon')
const externalAccountID = ref('')
const accountPlatform = ref('mastodon')
const bridgePairings = ref<BridgePairing[]>([])
const bridgePlatform = ref('x')
const bridgePageURL = ref('')
const bridgeMessage = ref('')
const bridgeRequest = ref<BridgeRequest | null>(null)
const translationConfigs = ref<TranslationConfig[]>([])
const translationSourceID = ref('')
const translationTargetAccountID = ref('')
const translationSourceLanguage = ref('')
const translationTargetLanguage = ref('zh')
const translationConfigID = ref('')
const translationProvider = ref('openai')
const translationCredentialEnv = ref('')
const translationBaseURL = ref('')
const translationProjectOrModel = ref('')
const translationMessage = ref('')
const baseURLPlaceholder = computed(() => accountPlatform.value === 'bluesky' ? 'Bluesky PDS URL' : '平台实例 URL')
const tokenEnvPlaceholder = computed(() => accountPlatform.value === 'bluesky' ? 'app password 环境变量名' : 'token 环境变量名')

const profileAccounts = computed(() => accounts.value.filter((account) => account.profile_id === selectedProfileID.value))
const profileTopics = computed(() => topics.value.filter((topic) => topic.profile_id === selectedProfileID.value && topic.status === 'active'))
const selectedCapability = computed(() => capabilities.value[selectedAccountID.value])
const selectedAccount = computed(() => accounts.value.find((account) => account.id === selectedAccountID.value))
const canImportOfficially = computed(() => selectedAccount.value?.platform === 'misskey' || selectedAccount.value?.platform === 'bluesky')

function isBridgePlatform(platform: string) {
  return ['x', 'weibo', 'xiaohongshu', 'qzone'].includes(platform)
}

function variantCapability(variant: Variant) {
  return capabilities.value[variant.account_id]
}

function utf8Bytes(text: string) {
  return new TextEncoder().encode(text).length
}

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
    const existingPairings = new Map(bridgePairings.value.map((pairing) => [pairing.id, pairing]))
    const loaded = await Promise.all([
      api<Profile[]>('profiles'), api<Account[]>('accounts'), api<Topic[]>('topics'), api<BridgePairing[]>('bridge/pairings'), api<TranslationConfig[]>('translation-configs'),
    ] as const)
    profiles.value = loaded[0]; accounts.value = loaded[1]; topics.value = loaded[2]
    bridgePairings.value = loaded[3].map((pairing) => ({ ...existingPairings.get(pairing.id), ...pairing }))
    translationConfigs.value = loaded[4]
    if (!translationConfigID.value) translationConfigID.value = translationConfigs.value[0]?.id || ''
    capabilities.value = Object.fromEntries(await Promise.all(accounts.value.map(async (account) => [account.id, await api<Capability>(`accounts/${account.id}/capability`)])))
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
    await api('accounts', { method: 'POST', body: JSON.stringify({ profile_id: selectedProfileID.value, name: accountName.value, platform: accountPlatform.value, remote_id: remoteID.value, base_url: baseURL.value, token_env: tokenEnv.value }) })
    accountName.value = ''; baseURL.value = ''; tokenEnv.value = ''; remoteID.value = ''
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
    translationSourceID.value = variants.value[0]?.id || ''
    translationTargetAccountID.value = profileAccounts.value[0]?.id || ''
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
async function importExternal() { if (!selected.value || !selectedAccountID.value || !externalReference.value.trim()) return; try { const post=await api<ExternalPost>('external-posts/import',{method:'POST',body:JSON.stringify({topic_id:selected.value.id,account_id:selectedAccountID.value,reference:externalReference.value})});externalPosts.value.unshift(post);externalReference.value='';message.value='已通过官方 API 导入快照'} catch(error){showError(error)} }

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
async function createBridgePairing() {
  try { const pairing = await api<BridgePairing>('bridge/pairings', { method: 'POST', body: JSON.stringify({ platform: bridgePlatform.value }) }); bridgePairings.value.unshift(pairing); bridgeMessage.value = `请在 10 分钟内把一次性 code ${pairing.code} 输入用户脚本；bearer 将由配对交换并只保存在页面内存中` } catch (error) { showError(error) }
}
async function revokeBridgePairing(pairing: BridgePairing) {
  try { await api(`bridge/pairings/${pairing.id}/revoke`, { method: 'POST' }); pairing.revoked = true; bridgeMessage.value = '配对已撤销' } catch (error) { showError(error) }
}
async function fillBrowser(variant: Variant) {
  const account = accounts.value.find((item) => item.id === variant.account_id)
  const pairing = bridgePairings.value.find((item) => item.code_used && !item.revoked && item.platform === account?.platform)
  if (!pairing || !bridgePageURL.value.trim()) { bridgeMessage.value = '请先完成匹配平台的配对、刷新状态，并填写平台页面 URL'; return }
  try { bridgeRequest.value = await api<BridgeRequest>('bridge/requests/fill', { method: 'POST', body: JSON.stringify({ pairing_id: pairing.id, variant_id: variant.id, page_url: bridgePageURL.value }) }); bridgeMessage.value = '已发送填充请求：请在平台页检查后人工发布' } catch (error) { showError(error) }
}
async function requestCapture() {
  if (!selected.value || !selectedAccountID.value || !bridgePageURL.value.trim()) { bridgeMessage.value = '请选择账号并填写当前平台页面 URL'; return }
  const account = accounts.value.find((item) => item.id === selectedAccountID.value)
  const pairing = bridgePairings.value.find((item) => item.code_used && !item.revoked && item.platform === account?.platform)
  if (!pairing || !['x', 'weibo', 'xiaohongshu', 'qzone'].includes(account?.platform || '')) { bridgeMessage.value = '请先创建匹配平台的配对'; return }
  try { bridgeRequest.value = await api<BridgeRequest>('bridge/requests/capture', { method: 'POST', body: JSON.stringify({ pairing_id: pairing.id, topic_id: selected.value.id, account_id: selectedAccountID.value, platform: account?.platform, page_url: bridgePageURL.value }) }); bridgeMessage.value = '已创建捕获请求：用户脚本只读取当前可见页面' } catch (error) { showError(error) }
}
async function addTranslationConfig() {
  if (!translationCredentialEnv.value.trim()) { translationMessage.value = '请填写凭据环境变量名'; return }
  try {
    const config = await api<TranslationConfig>('translation-configs', { method: 'POST', body: JSON.stringify({ provider: translationProvider.value, base_url: translationBaseURL.value, token_env: translationProvider.value === 'deepl' ? '' : translationCredentialEnv.value, key_env: translationProvider.value === 'deepl' ? translationCredentialEnv.value : '', project_id: translationProvider.value === 'google' ? translationProjectOrModel.value : '', model: translationProvider.value === 'openai' ? translationProjectOrModel.value : '' }) })
    translationConfigs.value.push(config); translationConfigID.value = config.id; translationMessage.value = '翻译配置已保存（只保存环境变量名）'
  } catch (error) { translationMessage.value = `配置失败：${error instanceof Error ? error.message : String(error)}` }
}
async function translateVariant() {
  if (!translationSourceID.value || !translationTargetAccountID.value || !translationConfigID.value || !translationTargetLanguage.value.trim()) { translationMessage.value = '请选择源版本、目标账号、配置和目标语言'; return }
  try {
    const result = await api<{ variant: Variant; translation: unknown }>('translations', { method: 'POST', body: JSON.stringify({ source_variant_id: translationSourceID.value, target_account_id: translationTargetAccountID.value, source_language: translationSourceLanguage.value, target_language: translationTargetLanguage.value, config_id: translationConfigID.value }) })
    variants.value.push(result.variant); translationMessage.value = '已生成新的 detached 翻译版本，原文未修改'
  } catch (error) { translationMessage.value = `翻译失败：${error instanceof Error ? error.message : String(error)}；原文与目标版本均未改动` }
}
async function refreshBridgeState() {
  try {
    const existing = new Map(bridgePairings.value.map((pairing) => [pairing.id, pairing]))
    bridgePairings.value = (await api<BridgePairing[]>('bridge/pairings')).map((pairing) => ({ ...existing.get(pairing.id), ...pairing }))
    if (bridgeRequest.value) bridgeRequest.value = await api<BridgeRequest>(`bridge/requests/${bridgeRequest.value.id}`)
    if (selected.value) externalPosts.value = await api<ExternalPost[]>(`external-posts?topic_id=${selected.value.id}`)
    bridgeMessage.value = '桥接状态与站外快照已刷新'
  } catch (error) { showError(error) }
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
        <form @submit.prevent="addAccount"><input v-model="accountName" placeholder="账号名称"><select v-model="accountPlatform"><option value="mastodon">Mastodon-compatible</option><option value="misskey">Misskey</option><option value="bluesky">Bluesky</option><option value="x">X</option><option value="weibo">微博</option><option value="xiaohongshu">小红书</option><option value="qzone">QQ空间</option></select><input v-if="accountPlatform==='bluesky'" v-model="remoteID" placeholder="Bluesky handle/identifier"><input v-if="!isBridgePlatform(accountPlatform)" v-model="baseURL" :placeholder="baseURLPlaceholder"><input v-if="!isBridgePlatform(accountPlatform)" v-model="tokenEnv" :placeholder="tokenEnvPlaceholder"><button>添加账号</button></form>
        <form @submit.prevent="addTopic"><input v-model="topicTitle" placeholder="新话题标题"><button>添加话题</button></form>
        <p v-if="profileTopics.length===0">还没有话题；先添加 Profile、账号和第一个话题。</p><ul v-else class="rows"><li v-for="topic in profileTopics" :key="topic.id"><button class="link" @click="openTopic(topic)">{{topic.title}}</button><span>{{topic.status}}</span></li></ul>
      </section>
      <section v-else-if="section==='editor'">
        <button class="back" @click="section='topics'">← 返回话题</button><h2>{{selected?.title}}</h2><textarea v-model="text" placeholder="基础稿"></textarea>
        <div class="actions"><button @click="saveDraft">保存基础稿</button><select v-model="selectedAccountID"><option v-for="account in profileAccounts" :key="account.id" :value="account.id">{{account.name}} ({{account.platform}})</option></select><button @click="newVariant">从基础稿生成 Variant</button></div>
        <div class="actions"><select v-model="mediaVariantID"><option value="">通用媒体</option><option v-for="variant in variants" :key="variant.id" :value="variant.id">{{variant.account_id}}</option></select><input v-model="altText" placeholder="图片替代文本"><label class="upload">添加图片<input type="file" accept="image/*" @change="uploadMedia"></label></div>
        <ul v-if="media.length" class="media-list"><li v-for="item in media" :key="item.id"><img :src="`/media/${item.id}`" :alt="item.alt_text||item.filename"><span>{{item.filename}}</span></li></ul>
        <button v-if="selectedCapability?.custom_emoji" @click="loadEmoji">加载实例 emoji</button><div class="emoji-list"><span v-for="emoji in emojis" :key="emoji.shortcode" class="emoji"><img :src="emoji.static_url||emoji.url" alt="">:{{emoji.shortcode}}:</span></div>
        <h3>平台版本</h3><small>编辑控件由账号适配器能力决定；Misskey 保留原始 MFM，Bluesky 使用纯文本与链接/提及 facets。</small><small v-if="selectedCapability?.visibility_note">{{selectedCapability.visibility_note}}</small>
        <p v-if="variants.length===0">还没有平台版本；选择目标账号后从基础稿生成。</p><ul v-else class="rows variants"><li v-for="variant in variants" :key="variant.id"><div><span>{{variant.account_id}} · {{variant.format}} · {{variant.relation}} · {{variant.status}}<em v-if="variant.outdated">有基础稿更新</em></span><textarea v-model="variant.text" rows="3"></textarea><small v-if="variantCapability(variant)?.text_max_bytes">UTF-8 {{utf8Bytes(variant.text)}} / {{variantCapability(variant)?.text_max_bytes}} bytes</small><select v-model="variant.visibility"><option v-for="visibility in variantCapability(variant)?.visibilities || []" :key="visibility">{{visibility}}</option></select><input v-if="variantCapability(variant)?.supports_cw" v-model="variant.spoiler_text" placeholder="CW（可选）"><label v-if="variantCapability(variant)?.supports_local_only"><input type="checkbox" v-model="variant.local_only">仅本地</label><small v-if="variantCapability(variant)?.format==='mfm'">原始 MFM 预览（本地不渲染）</small><pre class="preview">{{variant.text}}</pre></div><div class="row-actions"><button @click="saveVariant(variant)">保存</button><button v-if="variant.outdated" @click="applyVariant(variant)">应用更新</button><button v-if="!isBridgePlatform(accounts.find(a=>a.id===variant.account_id)?.platform||'')" @click="publish(variant.id)">发布</button><button v-else @click="fillBrowser(variant)">填入浏览器（不发布）</button></div></li></ul>
        <h3>浏览器桥接 v1</h3><small>用户脚本只读取当前可见 DOM；填入后必须在平台页人工检查并发布，程序不会点击最终发布按钮。</small><div class="actions"><select v-model="bridgePlatform"><option>x</option><option>weibo</option><option>xiaohongshu</option><option>qzone</option></select><button @click="createBridgePairing">生成配对</button><input v-model="bridgePageURL" placeholder="当前平台页面 URL"><button @click="requestCapture">请求捕获当前可见帖子</button><button @click="refreshBridgeState">刷新状态/快照</button></div><p v-if="bridgeMessage" class="notice">{{bridgeMessage}}</p><p v-if="bridgeRequest">最近请求：{{bridgeRequest.kind}} · {{bridgeRequest.status}}<span v-if="bridgeRequest.error"> · {{bridgeRequest.error}}</span></p><ul class="rows"><li v-for="pairing in bridgePairings" :key="pairing.id"><span>{{pairing.platform}} · {{pairing.revoked?'已撤销':pairing.code_used?'已连接':'待连接'}}<code v-if="pairing.code">code={{pairing.code}}</code></span><button v-if="!pairing.revoked" @click="revokeBridgePairing(pairing)">撤销</button></li></ul>
        <h3>站外关联</h3>
        <form @submit.prevent="markExternal"><select v-model="externalPlatform"><option>mastodon</option><option>misskey</option><option>bluesky</option><option>x</option><option>weibo</option><option>xiaohongshu</option><option>qzone</option></select><select v-model="externalAccountID"><option value="">无账号</option><option v-for="account in profileAccounts" :key="account.id" :value="account.id">{{account.name}}</option></select><input v-model="externalURL" placeholder="仅标记 URL，不抓取"><button>标记 URL</button></form>
        <form @submit.prevent="importExternal"><input v-model="externalReference" placeholder="Misskey note ID/URL 或 Bluesky URL/at://"><button :disabled="!canImportOfficially">官方 API 导入快照</button><small v-if="!canImportOfficially">请选择 Misskey 或 Bluesky 账号</small></form>
        <ul class="rows"><li v-for="post in externalPosts" :key="post.id"><span>{{post.platform}}<em v-if="post.incomplete">不完整：{{post.incomplete_reason}}</em></span><a :href="post.url" target="_blank" rel="noreferrer">{{post.url}}</a><small v-if="post.text_snapshot">{{post.text_snapshot.slice(0,120)}}</small></li></ul>
        <h3>翻译</h3>
        <small>翻译只生成新的 detached Variant，不覆盖源版本；凭据只从环境变量读取。</small>
        <div class="actions"><select v-model="translationSourceID"><option v-for="variant in variants" :key="variant.id" :value="variant.id">源：{{variant.id}}</option></select><select v-model="translationTargetAccountID"><option v-for="account in profileAccounts" :key="account.id" :value="account.id">目标：{{account.name}}</option></select><input v-model="translationSourceLanguage" placeholder="源语言（可留空自动识别）"><input v-model="translationTargetLanguage" placeholder="目标语言，如 zh"><select v-model="translationConfigID"><option value="">选择翻译配置</option><option v-for="config in translationConfigs" :key="config.id" :value="config.id">{{config.provider}} · {{config.model || config.project_id || config.id}}</option></select><button @click="translateVariant" :disabled="!translationConfigID">生成翻译版本</button></div>
        <form class="actions" @submit.prevent="addTranslationConfig"><select v-model="translationProvider"><option value="openai">OpenAI-compatible</option><option value="deepl">DeepL</option><option value="google">Google Cloud</option></select><input v-model="translationCredentialEnv" placeholder="token/key 环境变量名"><input v-model="translationBaseURL" placeholder="可选 fake/provider URL"><input v-model="translationProjectOrModel" :placeholder="translationProvider==='google'?'Google project id':'可选模型名'"><button>保存翻译配置</button></form>
        <p v-if="translationMessage" class="notice">{{translationMessage}}</p>
        <h3>备份与凭据</h3><small>使用 CLI 执行 <code>bpm backup create backup.zip</code> 和 <code>bpm backup restore backup.zip 空目录</code>。备份包含 SQLite、本地图片和校验清单，不包含环境变量值；恢复拒绝覆盖非空目录。</small>
      </section>
      <section v-else><h2>发布历史</h2><PublicationList/></section>
    </main>
  </div>
</template>
