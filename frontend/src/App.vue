<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { api } from './api'
import PublicationList from './PublicationList.vue'

type Profile = { id: string; name: string }
type Account = { id: string; profile_id: string; platform: string; name: string; remote_id: string; credential_ref?: string; disabled?: boolean }
type Topic = { id: string; profile_id: string; title: string; status: string }
type Draft = { topic_id: string; text: string; format: string; revision: number }
type Variant = { id: string; topic_id: string; account_id: string; text: string; format: string; status: string; relation: string; visibility: string; spoiler_text: string; local_only: boolean; outdated: boolean }
type Media = { id: string; variant_id: string; filename: string; alt_text: string }
type ExternalPost = { id: string; platform: string; url: string; text_snapshot: string; incomplete: boolean; incomplete_reason: string }
type Emoji = { shortcode: string; url: string; static_url: string }
type BridgePairing = { id: string; platform: string; code?: string; expires_at: string; code_used: boolean; revoked: boolean; connected?: boolean; page_url?: string; last_seen_at?: string }
type BridgeRequest = { id: string; kind: string; status: string; warning?: string; error?: string; external_post_id?: string }
type Capability = { platform: string; format: string; text_max_bytes: number; max_media: number; max_media_bytes: number; visibilities: string[]; supports_cw: boolean; supports_local_only: boolean; custom_emoji: boolean; visibility_note?: string }
type TranslationConfig = { id: string; provider: string; base_url: string; project_id: string; location: string; token_env: string; key_env: string; model: string }

function listOrEmpty<T>(value: T[] | null | undefined): T[] { return Array.isArray(value) ? value : [] }

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
const connectionMessage = ref('')
const baseURLPlaceholder = computed(() => accountPlatform.value === 'bluesky' ? 'Bluesky PDS URL' : '平台实例 URL')
const tokenEnvPlaceholder = computed(() => accountPlatform.value === 'bluesky' ? 'app password 环境变量名' : 'token 环境变量名')
const platformDescription = computed(() => ({
  mastodon: 'Mastodon-compatible：推荐 OAuth 自动连接；也可手动填写实例 URL 与 token 环境变量。',
  misskey: 'Misskey：推荐 MiAuth 自动连接；也可手动填写实例 URL 与 token 环境变量。',
  bluesky: 'Bluesky：填写 handle/identifier、PDS URL 与 app password 环境变量。',
  x: 'X：仅浏览器桥接，用户脚本填充后必须在页面人工发布。',
  weibo: '微博：仅浏览器桥接，用户脚本填充后必须在页面人工发布。',
  xiaohongshu: '小红书：仅浏览器桥接，用户脚本填充后必须在页面人工发布。',
  qzone: 'QQ空间：仅浏览器桥接；请人工检查可见范围和图片顺序后发布。',
} as Record<string, string>)[accountPlatform.value] || '')
const translationProviderDescription = computed(() => ({
  openai: 'OpenAI-compatible Responses provider；只保存环境变量名。',
  deepl: 'DeepL v2；只保存 key 环境变量名。',
  google: 'Google Cloud Translation v3；需要 project id 与 token 环境变量名。',
  'google-gtx': '非官方 Google translate_a/single：最多 5000 Unicode 字符，可能失效且无需凭据。',
} as Record<string, string>)[translationProvider.value] || '')

const profileAccounts = computed(() => accounts.value.filter((account) => account.profile_id === selectedProfileID.value && !account.disabled))
const profileTopics = computed(() => topics.value.filter((topic) => topic.profile_id === selectedProfileID.value && topic.status === 'active'))
const selectedCapability = computed(() => capabilities.value[selectedAccountID.value])
const selectedAccount = computed(() => accounts.value.find((account) => account.id === selectedAccountID.value))
const canImportOfficially = computed(() => selectedAccount.value?.platform === 'misskey' || selectedAccount.value?.platform === 'bluesky')
function pairingState(pairing: BridgePairing) {
  if (pairing.revoked) return '已撤销'
  if (pairing.connected) return '已连接，心跳正常'
  if (pairing.code_used && pairing.expires_at && new Date(pairing.expires_at).getTime() <= Date.now()) return '连接过期'
  if (pairing.code_used) return '未检测到近期心跳（授权仍有效）'
  return pairing.code ? '待输入 code' : '未配对'
}

function bridgeRequestState(request: BridgeRequest) {
  return ({
    pending: '等待平台脚本领取',
    filled: '已填入，等待你在平台页确认发布',
    published: '已确认发布并回传结果',
    needs_confirmation: '未收到确定结果，请到平台页人工核对',
    expired: '请求已过期',
    failed: '请求失败',
  } as Record<string, string>)[request.status] || request.status
}

function connectedPairing(platform?: string) {
  return bridgePairings.value
    .filter((pairing) => pairing.connected && !pairing.revoked && pairing.platform === platform)
    .sort((a, b) => new Date(b.last_seen_at || 0).getTime() - new Date(a.last_seen_at || 0).getTime())[0]
}

function isBridgePlatform(platform: string) {
  return ['x', 'weibo', 'xiaohongshu', 'qzone'].includes(platform)
}

function variantCapability(variant: Variant) {
  return capabilities.value[variant.account_id]
}

function variantAccount(variant: Variant) {
  return accounts.value.find((account) => account.id === variant.account_id)
}

function variantAccountLabel(variant: Variant) {
  const account = variantAccount(variant)
  return account ? `${account.name} · ${account.platform}` : variant.account_id
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
    profiles.value = listOrEmpty(loaded[0]); accounts.value = listOrEmpty(loaded[1]); topics.value = listOrEmpty(loaded[2])
    bridgePairings.value = listOrEmpty(loaded[3]).map((pairing) => ({ ...existingPairings.get(pairing.id), ...pairing }))
    translationConfigs.value = listOrEmpty(loaded[4])
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
  if (!selectedProfileID.value) { message.value = '请先选择 Profile'; return }
  if (!accountName.value.trim()) { message.value = '请填写账号名称'; return }
  try {
    const created = await api<Account>('accounts', { method: 'POST', body: JSON.stringify({ profile_id: selectedProfileID.value, name: accountName.value, platform: accountPlatform.value, remote_id: remoteID.value, base_url: baseURL.value, token_env: tokenEnv.value }) })
    accountName.value = ''; baseURL.value = ''; tokenEnv.value = ''; remoteID.value = ''
    await load()
    if (!accounts.value.some((account) => account.id === created.id)) accounts.value.push(created)
    message.value = '账号已添加'
  } catch (error) { showError(error) }
}
async function removeAccount(account: Account) {
  if (!window.confirm(`停用账号“${account.name}”？历史版本和发布记录会保留。`)) return
  try {
    await api(`accounts/${account.id}/deactivate`, { method: 'DELETE' })
    await load()
    message.value = '账号已停用，本地凭据已移除；历史记录仍保留'
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
    const loaded = await Promise.all([
      api<Draft>(`topics/${topic.id}/draft`), api<Variant[]>(`variants?topic_id=${topic.id}`),
      api<Media[]>(`media?topic_id=${topic.id}`), api<ExternalPost[]>(`external-posts?topic_id=${topic.id}`),
    ])
    draft.value = loaded[0]; variants.value = listOrEmpty(loaded[1]); media.value = listOrEmpty(loaded[2]); externalPosts.value = listOrEmpty(loaded[3])
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
    variants.value = listOrEmpty(await api<Variant[]>(`variants?topic_id=${selected.value.id}`))
    message.value = '基础稿已保存'
  } catch (error) { showError(error) }
}

async function newVariant() {
  if (!selected.value || !selectedAccountID.value) return
  try {
    await api('variants', { method: 'POST', body: JSON.stringify({ topic_id: selected.value.id, account_id: selectedAccountID.value }) })
    variants.value = listOrEmpty(await api<Variant[]>(`variants?topic_id=${selected.value.id}`))
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
    externalPosts.value = listOrEmpty(await api<ExternalPost[]>(`external-posts?topic_id=${selected.value.id}`))
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
async function deleteMedia(item: Media) {
  if (!window.confirm(`删除图片“${item.filename}”？本地文件也会移除。`)) return
  try {
    await api(`media/${item.id}`, { method: 'DELETE' })
    media.value = media.value.filter((mediaItem) => mediaItem.id !== item.id)
    message.value = '图片已删除'
  } catch (error) { showError(error) }
}
async function deleteVariant(variant: Variant) {
  if (!window.confirm('删除这个未发布平台版本及其图片？有发布历史的版本不会被删除。')) return
  try {
    await api(`variants/${variant.id}`, { method: 'DELETE' })
    variants.value = variants.value.filter((item) => item.id !== variant.id)
    media.value = media.value.filter((item) => item.variant_id !== variant.id)
    message.value = '平台版本已删除'
  } catch (error) { showError(error) }
}

async function loadEmoji() {
  if (!selectedAccountID.value) return
  try {
    emojis.value = listOrEmpty(await api<Emoji[]>(`accounts/${selectedAccountID.value}/emojis`))
    message.value = `已加载 ${emojis.value.length} 个实例 emoji`
  } catch (error) { message.value = `emoji 加载失败：${error instanceof Error ? error.message : String(error)}` }
}
async function createBridgePairing() {
  try {
    const pairing = await api<BridgePairing>('bridge/pairings', { method: 'POST', body: JSON.stringify({ platform: bridgePlatform.value }) })
    bridgePairings.value.unshift(pairing)
    bridgeMessage.value = `配对码已生成：${pairing.code}。请复制到平台页脚本；10 分钟内有效。`
    if (pairing.code) {
      try {
        await copyText(pairing.code)
        bridgeMessage.value += ' 已自动复制。'
      } catch {
        bridgeMessage.value += ' 自动复制失败，请使用旁边的“复制 code”按钮。'
      }
    }
  } catch (error) { showError(error) }
}
async function copyText(value: string) {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(value)
    return
  }
  const input = document.createElement('textarea')
  input.value = value
  input.style.position = 'fixed'
  input.style.opacity = '0'
  document.body.appendChild(input)
  input.select()
  const copied = typeof document.execCommand === 'function' && document.execCommand('copy')
  input.remove()
  if (!copied) throw new Error('clipboard unavailable')
}
async function copyPairingCode(pairing: BridgePairing) {
  if (!pairing.code) return
  try { await copyText(pairing.code); bridgeMessage.value = '配对 code 已复制' }
  catch { bridgeMessage.value = '无法自动复制，请选中 code 后手动复制' }
}
async function revokeBridgePairing(pairing: BridgePairing) {
  try { await api(`bridge/pairings/${pairing.id}/revoke`, { method: 'POST' }); pairing.revoked = true; bridgeMessage.value = '配对已撤销' } catch (error) { showError(error) }
}
async function fillBrowser(variant: Variant, autoPublish = false) {
  const account = accounts.value.find((item) => item.id === variant.account_id)
  if (autoPublish && !window.confirm(`自动填入并点击 ${account?.name || account?.platform || '平台'} 当前页面的可见发布按钮？带图片时会为避免漏图而退回人工发布；页面结构变化时也可能失败，请随后核对结果。`)) return
  const pairing = connectedPairing(account?.platform)
  if (!pairing) {
    const authorized = bridgePairings.value.some((item) => item.code_used && !item.revoked && item.platform === account?.platform)
    bridgeMessage.value = authorized ? '已有授权，但没有检测到平台脚本近期心跳；请打开平台页并检查右下角 bridge 状态' : '请先完成匹配平台的配对；脚本会把当前页面安全回传给本地服务'
    return
  }
  try {
    bridgeRequest.value = await api<BridgeRequest>('bridge/requests/fill', { method: 'POST', body: JSON.stringify({ pairing_id: pairing.id, variant_id: variant.id, page_url: pairing.page_url || '', auto_publish: autoPublish }) })
    bridgeMessage.value = autoPublish ? '已发送自动发布请求；脚本只会点击已知、可见且启用的发布按钮，请随后核对平台结果' : '已发送，等待脚本领取；填入后请人工检查并点击平台发布'
  } catch (error) { showError(error) }
}
async function requestCapture() {
  if (!selected.value || !selectedAccountID.value) { bridgeMessage.value = '请选择话题和账号'; return }
  const account = accounts.value.find((item) => item.id === selectedAccountID.value)
  const pairing = connectedPairing(account?.platform)
  if (!pairing || !['x', 'weibo', 'xiaohongshu', 'qzone'].includes(account?.platform || '')) { bridgeMessage.value = '请先创建匹配平台的配对'; return }
  try { bridgeRequest.value = await api<BridgeRequest>('bridge/requests/capture', { method: 'POST', body: JSON.stringify({ pairing_id: pairing.id, topic_id: selected.value.id, account_id: selectedAccountID.value, platform: account?.platform, page_url: pairing.page_url || '' }) }); bridgeMessage.value = '已创建捕获请求：用户脚本只读取当前可见页面' } catch (error) { showError(error) }
}
async function addTranslationConfig() {
  if (translationProvider.value !== 'google-gtx' && !translationCredentialEnv.value.trim()) { translationMessage.value = '请填写凭据环境变量名'; return }
  try {
    const config = await api<TranslationConfig>('translation-configs', { method: 'POST', body: JSON.stringify({ provider: translationProvider.value, base_url: translationBaseURL.value, token_env: translationProvider.value === 'deepl' || translationProvider.value === 'google-gtx' ? '' : translationCredentialEnv.value, key_env: translationProvider.value === 'deepl' ? translationCredentialEnv.value : '', project_id: translationProvider.value === 'google' ? translationProjectOrModel.value : '', model: translationProvider.value === 'openai' ? translationProjectOrModel.value : '' }) })
    translationConfigs.value.push(config); translationConfigID.value = config.id; translationMessage.value = '翻译配置已保存'
  } catch (error) { translationMessage.value = `配置失败：${error instanceof Error ? error.message : String(error)}` }
}
async function startConnection(platform: 'mastodon' | 'misskey') {
  if (!selectedProfileID.value || !baseURL.value.trim()) { connectionMessage.value = '请选择 Profile 并填写实例 URL'; return }
  try {
    const result = await api<{ authorization_url: string }>(`connections/${platform}/start`, { method: 'POST', body: JSON.stringify({ profile_id: selectedProfileID.value, name: accountName.value, base_url: baseURL.value }) })
    connectionMessage.value = `${platform} 连接已准备，请在平台授权；回调只会保存本地凭据。`
    window.location.href = result.authorization_url
  } catch (error) { connectionMessage.value = `连接失败：${error instanceof Error ? error.message : String(error)}` }
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
    bridgePairings.value = listOrEmpty(await api<BridgePairing[]>('bridge/pairings')).map((pairing) => ({ ...existing.get(pairing.id), ...pairing }))
    if (bridgeRequest.value) bridgeRequest.value = await api<BridgeRequest>(`bridge/requests/${bridgeRequest.value.id}`)
    if (selected.value) externalPosts.value = listOrEmpty(await api<ExternalPost[]>(`external-posts?topic_id=${selected.value.id}`))
    bridgeMessage.value = '桥接状态与站外快照已刷新'
  } catch (error) { showError(error) }
}

onMounted(async () => {
  await load()
  const params = new URLSearchParams(window.location.search)
  const connected = params.get('connection')
  if (connected === 'mastodon' || connected === 'misskey') {
    section.value = 'settings'
    connectionMessage.value = `${connected === 'mastodon' ? 'Mastodon' : 'Misskey'} 连接成功，本地凭据已保存。`
    params.delete('connection')
    const query = params.toString()
    window.history.replaceState({}, '', `${window.location.pathname}${query ? `?${query}` : ''}${window.location.hash}`)
  }
})
</script>

<template>
  <div class="shell">
    <aside class="sidebar">
      <div class="brand"><span class="brand-mark">↗</span><div><strong>Best Post</strong><small>Manager</small></div></div>
      <nav aria-label="主导航">
        <button :class="{active:section==='topics'}" @click="section='topics'"><span>⌂</span> 内容</button>
        <button :class="{active:section==='editor'}" @click="section='editor'"><span>✎</span> 当前编辑</button>
        <button :class="{active:section==='history'}" @click="section='history'"><span>↗</span> 发布记录</button>
        <button :class="{active:section==='settings'}" @click="section='settings'"><span>⚙</span> 设置</button>
      </nav>
      <div class="local-status"><span class="status-dot"></span><div><strong>本地单用户</strong><small>数据保存在本机</small></div></div>
    </aside>
    <main class="canvas">
      <div v-if="message" class="notice">{{message}}</div>
      <section v-if="section==='topics'" class="page-section">
        <div class="eyebrow">工作区</div><h1>内容</h1><p class="lede">把想法放进话题，基础稿会在这里保持清晰。</p>
        <div class="profile-picker"><label for="content-profile">当前 Profile</label><select id="content-profile" v-model="selectedProfileID"><option v-for="profile in profiles" :key="profile.id" :value="profile.id">{{profile.name}}</option></select><button class="text-button" @click="section='settings'">管理 Profile 与账号 →</button></div>
        <form class="topic-create" @submit.prevent="addTopic"><input v-model="topicTitle" placeholder="新话题标题" :disabled="!selectedProfileID"><button class="primary-action" :disabled="!selectedProfileID">创建话题</button></form>
        <div class="section-label"><span>话题</span><small>{{profileTopics.length}} 个</small></div>
        <p v-if="profiles.length===0" class="empty-state">还没有 Profile。前往设置创建后，就可以开始第一个话题。</p><p v-else-if="profileTopics.length===0" class="empty-state">这个 Profile 还没有话题，从上方写下第一个标题吧。</p><ul v-else class="rows topic-list"><li v-for="topic in profileTopics" :key="topic.id"><button class="link" @click="openTopic(topic)">{{topic.title}}</button><span>{{topic.status}}</span></li></ul>
      </section>
      <section v-else-if="section==='editor' && selected" class="page-section editor-page">
        <button class="back" @click="section='topics'">← 返回内容</button><div class="eyebrow">正在编辑</div><h1>{{selected.title}}</h1><p class="lede">基础稿是源头；平台版本只在你确认后更新。</p>
        <textarea class="draft-editor" v-model="text" placeholder="从这里开始写…"></textarea>
        <div class="actions editor-toolbar"><button class="primary-action" @click="saveDraft">保存基础稿</button><select v-model="selectedAccountID" aria-label="目标账号"><option v-for="account in profileAccounts" :key="account.id" :value="account.id">{{account.name}} ({{account.platform}})</option></select><button @click="newVariant">从基础稿生成 Variant</button></div>
        <div class="media-tools"><div class="section-label"><span>媒体</span><small>图片只存储在本机</small></div><div class="actions media-controls"><select v-model="mediaVariantID"><option value="">通用媒体</option><option v-for="variant in variants" :key="variant.id" :value="variant.id">{{variantAccountLabel(variant)}}</option></select><input v-model="altText" placeholder="图片替代文本"><label class="upload">添加图片<input type="file" accept="image/*" @change="uploadMedia"></label></div><ul v-if="media.length" class="media-list"><li v-for="item in media" :key="item.id"><img :src="`/media/${item.id}`" :alt="item.alt_text||item.filename"><span>{{item.filename}}</span><button class="text-button" @click="deleteMedia(item)">删除</button></li></ul></div>
        <div class="platform-heading"><div><div class="section-label"><span>平台版本</span><small>{{variants.length}} 个版本</small></div><small>编辑控件由账号能力决定；Misskey 保留 MFM，Bluesky 使用纯文本与 facets。</small><small v-if="selectedCapability?.visibility_note">{{selectedCapability.visibility_note}}</small></div><button v-if="selectedCapability?.custom_emoji" class="text-button" @click="loadEmoji">加载实例 emoji</button></div><div class="emoji-list"><span v-for="emoji in emojis" :key="emoji.shortcode" class="emoji"><img :src="emoji.static_url||emoji.url" alt="">:{{emoji.shortcode}}:</span></div>
        <p v-if="variants.length===0" class="empty-state">还没有平台版本；选择目标账号后从基础稿生成。</p><ul v-else class="variants"><li v-for="variant in variants" :key="variant.id" class="variant-card"><header class="variant-header"><div><strong>{{variantAccountLabel(variant)}}</strong><span>{{variant.format}} · {{variant.relation}} · {{variant.status}}</span></div><em v-if="variant.outdated">基础稿有更新</em></header><textarea class="variant-editor" v-model="variant.text" rows="5"></textarea><div class="variant-options"><small v-if="variantCapability(variant)?.text_max_bytes">UTF-8 {{utf8Bytes(variant.text)}} / {{variantCapability(variant)?.text_max_bytes}} bytes</small><label><span>可见范围</span><select v-model="variant.visibility"><option v-for="visibility in variantCapability(variant)?.visibilities || []" :key="visibility">{{visibility}}</option></select></label><input v-if="variantCapability(variant)?.supports_cw" v-model="variant.spoiler_text" placeholder="内容警告（可选）"><label v-if="variantCapability(variant)?.supports_local_only" class="checkbox-option"><input type="checkbox" v-model="variant.local_only">仅本地</label></div><div v-if="variantCapability(variant)?.format==='mfm'" class="mfm-preview"><small>原始 MFM 预览（本地不渲染）</small><pre class="preview">{{variant.text}}</pre></div><footer class="variant-actions"><button class="primary-action" @click="saveVariant(variant)">保存版本</button><button v-if="variant.outdated" @click="applyVariant(variant)">应用基础稿更新</button><button v-if="!variantAccount(variant)?.disabled && !isBridgePlatform(variantAccount(variant)?.platform||'')" @click="publish(variant.id)">直接发布</button><template v-else-if="!variantAccount(variant)?.disabled"><button @click="fillBrowser(variant)">仅填入</button><button @click="fillBrowser(variant, true)">填入并自动发布</button></template><small v-else class="disabled-account">账号已停用，历史版本仅供查看</small><button v-if="!variantAccount(variant)?.disabled" class="danger-text" @click="deleteVariant(variant)">删除版本</button></footer></li></ul>
        <div class="bridge-status-panel" aria-label="浏览器桥接状态"><div class="section-label"><span>浏览器填入</span><small>发布入口保持可见</small></div><p class="bridge-steps">每个平台首次安装脚本并输入一次 code；此后配对会跨刷新和程序重启恢复。Variant 可以仅填入；纯文本也可以在每次明确确认后填入并点击已知发布按钮，带图时会退回人工发布。</p><p class="settings-note">先<a href="/userscript/bpm-bridge.user.js">安装或更新本地用户脚本</a>。脚本只处理当前可见页面；自动发布默认关闭。</p><div class="actions bridge-controls"><select v-model="bridgePlatform"><option>x</option><option>weibo</option><option>xiaohongshu</option><option>qzone</option></select><button class="primary-action" @click="createBridgePairing">生成/重置配对 code</button><button @click="refreshBridgeState">刷新连接状态</button><button @click="requestCapture">捕获当前可见帖子</button></div><p v-if="bridgeMessage" class="notice">{{bridgeMessage}}</p><div v-if="bridgePairings.length===0" class="bridge-state">未安装/未配对：安装脚本后生成配对。</div><ul v-else class="rows bridge-pairings"><li v-for="pairing in bridgePairings" :key="pairing.id"><div><strong>{{pairing.platform}}：{{pairingState(pairing)}}</strong><code v-if="pairing.code">{{pairing.code}}</code><small v-if="pairing.page_url">当前页：{{pairing.page_url}}</small><small v-if="pairing.last_seen_at">最后心跳：{{new Date(pairing.last_seen_at).toLocaleString()}}</small><small v-if="pairing.expires_at">有效期至：{{new Date(pairing.expires_at).toLocaleString()}}</small></div><div class="row-actions"><button v-if="pairing.code" @click="copyPairingCode(pairing)">复制 code</button><button v-if="!pairing.revoked" @click="revokeBridgePairing(pairing)">撤销</button></div></li></ul><p v-if="bridgeRequest" class="bridge-request-status">最近请求：{{bridgeRequest.kind}} · {{bridgeRequestState(bridgeRequest)}}<span v-if="bridgeRequest.warning"> · 提示：{{bridgeRequest.warning}}</span><span v-if="bridgeRequest.error"> · 错误：{{bridgeRequest.error}}</span></p></div>
        <details class="tool-section"><summary>发布与导入工具</summary><div class="tool-content"><h3>浏览器桥接 v1</h3><small>默认仅填入；只有你对某次请求选择“填入并自动发布”后，脚本才会点击明确匹配且可见、启用的发布按钮。页面结构不匹配时停止并提示人工处理。</small>
          <h3>站外关联</h3><p class="settings-note">用户脚本安装/更新：<a href="/userscript/bpm-bridge.user.js">打开当前本地脚本</a>（同源地址，不会触发 file:///）。安装到油猴后，在已登录的平台页面使用配对功能。</p><form @submit.prevent="markExternal"><select v-model="externalPlatform"><option>mastodon</option><option>misskey</option><option>bluesky</option><option>x</option><option>weibo</option><option>xiaohongshu</option><option>qzone</option></select><select v-model="externalAccountID"><option value="">无账号</option><option v-for="account in profileAccounts" :key="account.id" :value="account.id">{{account.name}}</option></select><input v-model="externalURL" placeholder="仅标记 URL，不抓取"><button>标记 URL</button></form><form @submit.prevent="importExternal"><input v-model="externalReference" placeholder="Misskey note ID/URL 或 Bluesky URL/at://"><button :disabled="!canImportOfficially">官方 API 导入快照</button><small v-if="!canImportOfficially">请选择 Misskey 或 Bluesky 账号</small></form><ul class="rows"><li v-for="post in externalPosts" :key="post.id"><span>{{post.platform}}<em v-if="post.incomplete">不完整：{{post.incomplete_reason}}</em></span><a :href="post.url" target="_blank" rel="noreferrer">{{post.url}}</a><small v-if="post.text_snapshot">{{post.text_snapshot.slice(0,120)}}</small></li></ul></div></details>
        <details class="tool-section"><summary>翻译工具</summary><div class="tool-content"><small>翻译只生成新的 detached Variant，不覆盖源版本；官方 provider 凭据从本地凭据或环境变量读取。</small><div class="actions"><select v-model="translationSourceID"><option v-for="variant in variants" :key="variant.id" :value="variant.id">源：{{variant.id}}</option></select><select v-model="translationTargetAccountID"><option v-for="account in profileAccounts" :key="account.id" :value="account.id">目标：{{account.name}}</option></select><input v-model="translationSourceLanguage" placeholder="源语言（可留空自动识别）"><input v-model="translationTargetLanguage" placeholder="目标语言，如 zh"><select v-model="translationConfigID"><option value="">选择翻译配置</option><option v-for="config in translationConfigs" :key="config.id" :value="config.id">{{config.provider}} · {{config.model || config.project_id || config.id}}</option></select><button @click="translateVariant" :disabled="!translationConfigID">生成翻译版本</button></div><p v-if="translationMessage" class="notice">{{translationMessage}}</p></div></details>
      </section>
      <section v-else-if="section==='editor'" class="page-section empty-editor">
        <div class="eyebrow">正在编辑</div><h1>先选择一个话题</h1><p class="lede">从内容列表打开话题后，基础稿和平台版本会显示在这里。</p><button class="primary-action" @click="section='topics'">返回内容</button>
      </section>
      <section v-else-if="section==='settings'" class="page-section settings-page">
        <div class="eyebrow">偏好与连接</div><h1>设置</h1><p class="lede">Profile、账号和本地工具配置。</p>
        <div class="section-label"><span>Profile</span><small>内容分组</small></div><form @submit.prevent="addProfile"><input v-model="profileName" placeholder="新 Profile"><button class="primary-action">添加 Profile</button></form>
        <div class="section-label"><span>平台账号</span><small>自动连接存本地凭据；手动模式引用环境变量</small></div><form @submit.prevent="addAccount"><select v-model="selectedProfileID"><option v-for="profile in profiles" :key="profile.id" :value="profile.id">{{profile.name}}</option></select><input v-model="accountName" placeholder="账号名称" :disabled="!selectedProfileID"><select v-model="accountPlatform" :disabled="!selectedProfileID"><option value="mastodon">Mastodon-compatible</option><option value="misskey">Misskey</option><option value="bluesky">Bluesky</option><option value="x">X</option><option value="weibo">微博</option><option value="xiaohongshu">小红书</option><option value="qzone">QQ空间</option></select><p class="settings-note platform-description">{{platformDescription}}</p><input v-if="accountPlatform==='bluesky'" v-model="remoteID" placeholder="Bluesky handle/identifier" :disabled="!selectedProfileID"><input v-if="!isBridgePlatform(accountPlatform)" v-model="baseURL" :placeholder="baseURLPlaceholder" :disabled="!selectedProfileID"><div v-if="accountPlatform==='mastodon' || accountPlatform==='misskey'" class="connection-inline"><button type="button" @click="startConnection(accountPlatform)" :disabled="!selectedProfileID || !baseURL.trim()">{{accountPlatform==='mastodon'?'OAuth 自动连接':'MiAuth 自动连接'}}</button><small>授权将在当前回环服务完成，成功后自动刷新本地账号列表。</small></div><p v-if="connectionMessage" class="notice">{{connectionMessage}}</p><input v-if="!isBridgePlatform(accountPlatform)" v-model="tokenEnv" :placeholder="tokenEnvPlaceholder" :disabled="!selectedProfileID"><button :disabled="!selectedProfileID">手动添加账号</button></form><ul class="rows account-list"><li v-for="account in profileAccounts" :key="account.id"><span>{{account.name}} · {{account.platform}}</span><button type="button" @click="removeAccount(account)">停用并移除凭据</button></li></ul>
        <div class="section-label"><span>翻译 provider</span><small>配置不包含 key/token</small></div><form class="actions" @submit.prevent="addTranslationConfig"><select v-model="translationProvider"><option value="openai">OpenAI-compatible</option><option value="deepl">DeepL</option><option value="google">Google Cloud</option><option value="google-gtx">Google translate_a（非官方）</option></select><p class="settings-note platform-description">{{translationProviderDescription}}</p><input v-if="translationProvider!=='google-gtx'" v-model="translationCredentialEnv" placeholder="token/key 环境变量名"><input v-model="translationBaseURL" placeholder="可选 fake/provider URL"><input v-if="translationProvider!=='google-gtx'" v-model="translationProjectOrModel" :placeholder="translationProvider==='google'?'Google project id':'可选模型名'"><button>保存翻译配置</button></form><p v-if="translationMessage" class="notice">{{translationMessage}}</p>
        <div class="section-label"><span>备份与凭据</span><small>离线可恢复</small></div><p class="settings-note">用户脚本安装/更新：<a href="/userscript/bpm-bridge.user.js">打开当前同源脚本</a>。每个平台首次输入一次 code，之后保存在油猴隔离存储；可在悬浮窗断开或从 WebUI 撤销。使用 CLI 执行 <code>bpm backup create backup.zip</code> 和 <code>bpm backup restore backup.zip 空目录</code>。备份包含 SQLite、本地图片和校验清单，不包含环境变量值或其他明文凭据。</p>
      </section>
      <section v-else class="page-section history-page"><div class="eyebrow">历史</div><h1>发布记录</h1><p class="lede">每次发布的结果和可恢复状态都在这里。</p><PublicationList/></section>
    </main>
  </div>
</template>
