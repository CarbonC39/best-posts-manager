<script setup lang="ts">
import { computed, customRef, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { api } from './api'
import { canAutoPublish } from './bridgeAdapters'
import PublicationList from './PublicationList.vue'
import IconButton from './IconButton.vue'
import PlatformIcon from './PlatformIcon.vue'

type Profile = { id: string; name: string }
type Account = { id: string; profile_id: string; platform: string; name: string; remote_id: string; base_url?: string; token_env?: string; disabled?: boolean }
type Topic = { id: string; profile_id: string; title: string; status: string }
type Draft = { topic_id: string; text: string; format: string; revision: number }
type Variant = { id: string; topic_id: string; account_id: string; text: string; format: string; status: string; relation: string; source_revision?: string; visibility: string; spoiler_text: string; local_only: boolean; outdated: boolean; published?: boolean }
type Media = { id: string; variant_id: string; filename: string; alt_text: string }
type ExternalPost = { id: string; platform: string; url: string; text_snapshot: string; incomplete: boolean; incomplete_reason: string }
type Emoji = { shortcode: string; url: string; static_url: string }
type BridgePairing = { id: string; platform: string; code?: string; expires_at: string; code_used: boolean; revoked: boolean; connected?: boolean; page_url?: string; last_seen_at?: string }
type BridgeRequest = { id: string; kind: string; status: string; warning?: string; error?: string; external_post_id?: string }
type Capability = { platform: string; format: string; text_max_bytes: number; max_media: number; max_media_bytes: number; visibilities: string[]; supports_cw: boolean; supports_local_only: boolean; custom_emoji: boolean; visibility_note?: string }
type TranslationConfig = { id: string; provider: string; base_url: string; project_id: string; location: string; token_env: string; key_env: string; model: string }
type BlueskyDiagnostic = { ok: boolean; stage: string; category?: string; message: string; hint?: string; handle?: string; did?: string; elapsed_ms: number }
type PermissionSignal = { endpoint: string; status: string; message: string }
type MastodonDiagnostic = { ok: boolean; stage: string; category?: string; message: string; hint?: string; username?: string; account_id?: string; status_write: PermissionSignal; media_write: PermissionSignal; elapsed_ms: number }

function listOrEmpty<T>(value: T[] | null | undefined): T[] { return Array.isArray(value) ? value : [] }
function notificationRef() {
  let value = ''
  return customRef<string>((track, trigger) => ({
    get() { track(); return value },
    set(next) { value = next; trigger(); if (next) showToast(next) },
  }))
}

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
const message = notificationRef()
const toast = ref<{ text: string; kind: 'success' | 'error' | 'warning' }>({ text: '', kind: 'success' })
let toastTimer: number | undefined
const selectedProfileID = ref('')
const selectedAccountID = ref('')
const mediaVariantID = ref('')
const profileName = ref('')
const accountName = ref('')
const baseURL = ref('')
const tokenEnv = ref('')
const accountSecret = ref('')
const remoteID = ref('')
const editingAccountID = ref('')
const editAccountName = ref('')
const editBaseURL = ref('')
const editRemoteID = ref('')
const editTokenEnv = ref('')
const editAccountSecret = ref('')
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
const bridgeMessage = notificationRef()
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
const translationMessage = notificationRef()
const connectionMessage = notificationRef()
const blueskyDiagnostic = ref<Record<string, BlueskyDiagnostic>>({})
const mastodonDiagnostic = ref<Record<string, MastodonDiagnostic>>({})
const diagnosticBusy = ref<Set<string>>(new Set())
const emojiFilter = ref('')
const emojiLimit = ref(50)
const emojiLoadedAccountID = ref('')
const emojiLoading = ref(false)
const emojiTargetAccountID = ref('')
const variantSearch = ref('')
const activeVariantID = ref('')
const variantEditor = ref<HTMLTextAreaElement | null>(null)
const draftPublishBusy = ref(false)
const showArchivedTopics = ref(false)
const topicTitleEdit = ref('')
const externalImportAccountID = ref('')
const variantBaselines = ref<Record<string, string>>({})
const busyVariantIDs = ref<Set<string>>(new Set())
const topicSearch = ref('')
const sidebarTool = ref<'create' | 'profile' | 'search'>('create')
const showTargetAdd = ref(false)
const editorTool = ref<'' | 'emoji' | 'translation' | 'external'>('')
const mediaDialogOpen = ref(false)
const publishDialogOpen = ref(false)
const publishDialogBusy = ref(false)
const bridgeAutoPublish = ref(false)
const publishDialogEl = ref<HTMLElement | null>(null)
let publishDialogReturnFocus: HTMLElement | null = null
const saveState = ref<'idle' | 'saving' | 'error'>('idle')
const editorReady = ref(false)
const suppressAutosave = ref(false)
let saveTimer: number | undefined
const baseURLPlaceholder = computed(() => accountPlatform.value === 'bluesky' ? 'Bluesky PDS URL' : '平台实例 URL')
const tokenEnvPlaceholder = computed(() => accountPlatform.value === 'bluesky' ? 'app password 环境变量名' : 'token 环境变量名')
const platformDescription = computed(() => ({
  mastodon: 'Mastodon：推荐 OAuth 自动连接；也可手动保存 access token，或使用环境变量。',
  misskey: 'Misskey：推荐 MiAuth 自动连接；也可手动保存 access token，或使用环境变量。',
  bluesky: 'Bluesky：填写 handle/identifier、PDS（默认 https://bsky.social）与 app password。app password 可直接保存在本机凭据文件，也可使用环境变量。',
  x: 'X：使用浏览器桥接；默认只填入，纯文本可对单次请求确认自动发布。',
  weibo: '微博：使用浏览器桥接填入；发布按钮选择器尚未安全验收，因此最终发布保持人工操作。',
  xiaohongshu: '小红书：使用浏览器桥接填入；发布按钮选择器尚未安全验收，因此最终发布保持人工操作。',
  qzone: 'QQ空间：使用浏览器桥接填入；可见范围、发布和图片顺序仍需人工检查。',
} as Record<string, string>)[accountPlatform.value] || '')
const translationProviderDescription = computed(() => ({
  openai: 'OpenAI-compatible Responses provider；只保存环境变量名。',
  deepl: 'DeepL v2；只保存 key 环境变量名。',
  google: 'Google Cloud Translation v3；需要 project id 与 token 环境变量名。',
  'google-gtx': '非官方 Google translate_a/single：最多 5000 Unicode 字符，可能失效且无需凭据。',
} as Record<string, string>)[translationProvider.value] || '')

const profileAccounts = computed(() => accounts.value.filter((account) => account.profile_id === selectedProfileID.value && !account.disabled))
const disabledProfileAccounts = computed(() => accounts.value.filter((account) => account.profile_id === selectedProfileID.value && account.disabled))
const profileTopics = computed(() => topics.value.filter((topic) => topic.profile_id === selectedProfileID.value && (showArchivedTopics.value || topic.status === 'active')))
const visibleTopics = computed(() => {
  const query = topicSearch.value.trim().toLowerCase()
  return query ? profileTopics.value.filter((topic) => topic.title.toLowerCase().includes(query)) : profileTopics.value
})
const editableVariants = computed(() => variants.value.filter((variant) => !variant.published))
const selectedAccount = computed(() => accounts.value.find((account) => account.id === selectedAccountID.value))
const filteredVariants = computed(() => {
  const query = variantSearch.value.trim().toLowerCase()
  if (!query) return variants.value
  return variants.value.filter((variant) => variantAccountLabel(variant).toLowerCase().includes(query) || variantFormatLabel(variant).toLowerCase().includes(query) || variantStatusLabel(variant).toLowerCase().includes(query) || variant.text.toLowerCase().includes(query))
})
const activeVariant = computed(() => {
  if (activeVariantID.value === 'draft') return undefined
  const query = variantSearch.value.trim()
  return query ? filteredVariants.value.find((variant) => variant.id === activeVariantID.value) || filteredVariants.value[0] : variants.value.find((variant) => variant.id === activeVariantID.value) || variants.value[0]
})
const externalImportAccount = computed(() => accounts.value.find((account) => account.id === externalImportAccountID.value))
const canImportOfficially = computed(() => externalImportAccount.value?.platform === 'misskey' || externalImportAccount.value?.platform === 'bluesky')
const externalPlatformAccounts = computed(() => profileAccounts.value.filter((account) => account.platform === externalPlatform.value || (externalPlatform.value === 'mastodon' && account.platform === 'mastodon-compatible')))
const matchingEmojis = computed(() => {
  const query = emojiFilter.value.trim().toLowerCase()
  return query ? emojis.value.filter((emoji) => emoji.shortcode.toLowerCase().includes(query)) : emojis.value
})
const visibleEmojis = computed(() => matchingEmojis.value.slice(0, emojiLimit.value))
const hasMoreEmojis = computed(() => visibleEmojis.value.length < matchingEmojis.value.length)

function variantSignature(variant: Variant) {
  return JSON.stringify([variant.text, variant.visibility, variant.spoiler_text, variant.local_only])
}
function rememberVariants(items: Variant[]) {
  variantBaselines.value = Object.fromEntries(items.map((variant) => [variant.id, variantSignature(variant)]))
}
function variantDirty(variant: Variant) {
  return variantBaselines.value[variant.id] !== variantSignature(variant)
}
const draftDirty = computed(() => Boolean(draft.value && text.value !== draft.value.text))
const topicTitleDirty = computed(() => Boolean(selected.value && topicTitleEdit.value !== selected.value.title))
const hasUnsavedChanges = computed(() => topicTitleDirty.value || draftDirty.value || variants.value.some(variantDirty))
const editorText = computed({
  get: () => activeVariant.value ? (activeVariant.value.text || '') : (text.value || ''),
  set: (value: string) => { if (activeVariant.value) activeVariant.value.text = value; else text.value = value },
})
const currentMedia = computed(() => media.value.filter((item) => !item.variant_id || item.variant_id === activeVariant.value?.id))
const preflightChecks = computed(() => {
  const variant = activeVariant.value
  const account = variant ? variantAccount(variant) : selectedAccount.value
  const capability = variant ? variantCapability(variant) : account ? capabilities.value[account.id] : undefined
  const content = variant?.text || editorText.value || ''
  const checks: Array<{ tone: string; title: string; detail: string; result: string }> = []
  if (!account) return checks
  const contentBytes = utf8Bytes(content)
  if (capability?.text_max_bytes && contentBytes > capability.text_max_bytes) checks.push({ tone: 'error', title: '正文与格式', detail: `超过 ${capability.text_max_bytes} bytes 上限 ${contentBytes - capability.text_max_bytes} bytes`, result: '需修改' })
  else checks.push({ tone: 'ok', title: '正文与格式', detail: capability?.text_max_bytes ? `${content.length} 字符 · ${contentBytes}/${capability.text_max_bytes} bytes` : `${content.length} 字符`, result: '通过' })
  if (capability?.max_media && currentMedia.value.length > capability.max_media) checks.push({ tone: 'error', title: '媒体', detail: `最多 ${capability.max_media} 张，当前 ${currentMedia.value.length} 张`, result: '需修改' })
  else if (currentMedia.value.some((item) => !String(item.alt_text || '').trim())) checks.push({ tone: 'warning', title: '图片替代文本', detail: '有媒体尚未填写替代文本', result: '建议补充' })
  else checks.push({ tone: 'ok', title: '媒体', detail: currentMedia.value.length ? `${currentMedia.value.length} 张图片` : '未添加图片', result: '通过' })
  if (account.disabled) checks.push({ tone: 'error', title: '发布账号', detail: `${platformLabel(account.platform)} · ${account.name}`, result: '已停用' })
  else checks.push({ tone: 'ok', title: '发布账号', detail: `${platformLabel(account.platform)} · ${account.name}`, result: '可用' })
  return checks
})
const preflightBlocking = computed(() => preflightChecks.value.some((check) => check.tone === 'error'))
const preflightSummary = computed(() => {
  const issues = preflightChecks.value.filter((check) => check.tone !== 'ok').length
  return issues ? `${issues} 项${preflightBlocking.value ? '需处理' : '建议'}` : '全部通过'
})
function variantBusy(variant: Variant) { return busyVariantIDs.value.has(variant.id) }
function diagnosticIsBusy(account: Account) { return diagnosticBusy.value.has(account.id) }
function setDiagnosticBusy(account: Account, busy: boolean) {
  const next = new Set(diagnosticBusy.value)
  if (busy) next.add(account.id); else next.delete(account.id)
  diagnosticBusy.value = next
}
function isMastodon(account?: Account) { return account?.platform === 'mastodon' || account?.platform === 'mastodon-compatible' }
function accountPlatformChanged() {
  if (accountPlatform.value === 'bluesky' && !baseURL.value.trim()) baseURL.value = 'https://bsky.social'
}
function emojiOwner(variant: Variant) { return emojiLoadedAccountID.value === variant.account_id && variants.value.find((item) => item.account_id === variant.account_id)?.id === variant.id }
function setVariantBusy(variant: Variant, busy: boolean) {
  const next = new Set(busyVariantIDs.value)
  if (busy) next.add(variant.id); else next.delete(variant.id)
  busyVariantIDs.value = next
}
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
    captured: '当前页面快照已保存',
    error: '脚本执行失败',
    expired: '请求已过期',
    failed: '请求失败',
  } as Record<string, string>)[request.status] || request.status
}
function displayableTime(value?: string) { return Boolean(value && !value.startsWith('0001-')) }

function connectedPairing(platform?: string) {
  return bridgePairings.value
    .filter((pairing) => pairing.connected && !pairing.revoked && pairing.platform === platform)
    .sort((a, b) => new Date(b.last_seen_at || 0).getTime() - new Date(a.last_seen_at || 0).getTime())[0]
}

function isBridgePlatform(platform: string) {
  return ['x', 'weibo', 'xiaohongshu', 'qzone'].includes(platform)
}

function variantCapability(variant?: Variant) {
  return variant ? capabilities.value[variant.account_id] : undefined
}

function variantAccount(variant: Variant) {
  return accounts.value.find((account) => account.id === variant.account_id)
}

function variantAccountLabel(variant: Variant) {
  const account = variantAccount(variant)
  return account ? `${account.name} · ${platformLabel(account.platform)}` : variant.account_id
}
function selectVariant(variant: Variant) {
  activeVariantID.value = variant.id
  emojiTargetAccountID.value = variant.account_id
  translationSourceID.value = variant.id
}
async function selectDocument(id: string) {
  suppressAutosave.value = true
  try {
    await flushAutoSave()
    activeVariantID.value = id
    editorTool.value = ''
    if (id !== 'draft') {
      const variant = variants.value.find((item) => item.id === id)
      if (variant) { emojiTargetAccountID.value = variant.account_id; translationSourceID.value = variant.id }
    }
    await nextTick()
  } finally { suppressAutosave.value = false }
}
async function selectDraft() { await selectDocument('draft') }
function toggleEditorTool(tool: 'emoji' | 'translation' | 'external') {
  editorTool.value = editorTool.value === tool ? '' : tool
  if (tool === 'translation' && activeVariant.value) translationSourceID.value = activeVariant.value.id
}
function variantFormatLabel(variant: Variant) { return variant.format === 'mfm' ? 'MFM' : '纯文本' }
function variantRelationLabel(variant: Variant) { return variant.relation === 'linked' ? '跟随主稿' : '独立版本' }
function variantStatusLabel(variant: Variant) { return variant.published ? '已有发布记录' : variant.status === 'ready' ? '可发布' : '草稿' }
function platformLabel(platform?: string) {
  return ({ mastodon: 'Mastodon', 'mastodon-compatible': 'Mastodon', misskey: 'Misskey', bluesky: 'Bluesky', x: 'X', weibo: '微博', xiaohongshu: '小红书', qzone: 'QQ 空间' } as Record<string, string>)[platform || ''] || platform || '平台'
}
function variantTabLabel(variant: Variant) {
  const account = variantAccount(variant)
  return account?.name || platformLabel(account?.platform)
}
function topicProgress(topic: Topic) {
  if (selected.value?.id === topic.id) {
    const published = variants.value.filter((variant) => variant.published).length
    return variants.value.length ? `${published}/${variants.value.length} 已发布` : '尚未添加目标'
  }
  return topic.status === 'archived' ? '已归档' : '进行中'
}
function mediaScopeLabel(item: Media) {
  if (!item.variant_id) return '所有平台版本'
  const variant = variants.value.find((candidate) => candidate.id === item.variant_id)
  return variant ? variantAccountLabel(variant) : '原平台版本已移除'
}

function utf8Bytes(text: string) {
  return new TextEncoder().encode(text).length
}

watch(selectedProfileID, () => {
  if (!profileAccounts.value.some((account) => account.id === selectedAccountID.value)) {
    selectedAccountID.value = profileAccounts.value[0]?.id || ''
  }
})
watch(selectedAccountID, (accountID) => {
  const account = accounts.value.find((item) => item.id === accountID)
  if (account && isBridgePlatform(account.platform)) bridgePlatform.value = account.platform
})
watch(externalPlatform, () => {
  if (!externalPlatformAccounts.value.some((account) => account.id === externalAccountID.value)) externalAccountID.value = ''
})
watch(emojiTargetAccountID, (accountID) => {
  if (accountID !== emojiLoadedAccountID.value) {
    emojis.value = []
    emojiLoadedAccountID.value = ''
    emojiFilter.value = ''
    emojiLimit.value = 50
  }
})
watch(filteredVariants, (items) => {
  if (activeVariantID.value === 'draft') return
  if (!items.some((variant) => variant.id === activeVariantID.value)) activeVariantID.value = items[0]?.id || variants.value[0]?.id || ''
})

function routeState() {
  const path = window.location.pathname.replace(/\/+$/, '') || '/'
  const topicMatch = path.match(/^\/topics\/([^/]+)$/)
  if (topicMatch) {
    try { return { section: 'editor', topicID: decodeURIComponent(topicMatch[1]) } } catch { return { section: 'topics', topicID: '' } }
  }
  if (path === '/history' || path === '/publishing') return { section: 'history', topicID: '' }
  if (path === '/accounts') return { section: 'accounts', topicID: '' }
  if (path === '/settings' || path === '/settings/integrations' || path === '/settings/data') return { section: 'settings', topicID: '' }
  return { section: 'topics', topicID: '' }
}
function routePath(next: string) { return next === 'history' ? '/history' : next === 'accounts' ? '/accounts' : next === 'settings' ? '/settings' : next === 'editor' && selected.value ? `/topics/${encodeURIComponent(selected.value.id)}` : '/topics' }
function goSection(next: string, replace = false) {
  if (section.value === 'editor' && next !== 'editor' && hasUnsavedChanges.value && !window.confirm('还有未保存的稿件修改，确定离开并丢弃这些修改吗？')) return
  if (next === 'editor' && !selected.value) { next = 'topics' }
  section.value = next
  const path = routePath(next)
  if (replace) window.history.replaceState({}, '', path)
  else if (window.location.pathname !== path) window.history.pushState({}, '', path)
}
function followSectionLink(event: MouseEvent, next: string) {
  if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return
  event.preventDefault()
  goSection(next)
}
function followTopicLink(event: MouseEvent, topic: Topic) {
  if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return
  event.preventDefault()
  void openTopic(topic)
}

function warnBeforeUnload(event: BeforeUnloadEvent) {
  if (!hasUnsavedChanges.value) return
  event.preventDefault()
  event.returnValue = ''
}
async function applyBrowserRoute() {
  const route = routeState()
  if (route.section === 'editor' && route.topicID) {
    const topic = topics.value.find((item) => item.id === route.topicID)
    if (topic) { await openTopic(topic); return }
  }
  section.value = route.section === 'editor' ? 'topics' : route.section
}
function onPopState() {
  if (section.value === 'editor' && hasUnsavedChanges.value && !window.confirm('还有未保存的稿件修改，确定离开并丢弃这些修改吗？')) {
    window.history.pushState({}, '', routePath('editor'))
    return
  }
  void applyBrowserRoute()
}

function showError(error: unknown) {
  message.value = `操作失败：${error instanceof Error ? error.message : String(error)}`
}
function dismissToast() { toast.value = { text: '', kind: 'success' }; if (toastTimer !== undefined) window.clearTimeout(toastTimer) }
function showToast(text: string) {
  if (!text) return
  const kind = /失败|错误|不可用|拒绝|无法/.test(text) ? 'error' : /^(请|需要|先)|不能为空|只能选择|没有检测到|缺少|警告|建议|注意/.test(text) ? 'warning' : 'success'
  toast.value = { text, kind }
  if (toastTimer !== undefined) window.clearTimeout(toastTimer)
  toastTimer = window.setTimeout(dismissToast, 4500)
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
  if (!profileName.value.trim()) { message.value = '请填写 Profile 名称'; return }
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
  if (!isBridgePlatform(accountPlatform.value) && accountPlatform.value !== 'bluesky' && !baseURL.value.trim()) { message.value = '请填写平台实例 URL'; return }
  if (!isBridgePlatform(accountPlatform.value) && !tokenEnv.value.trim() && !accountSecret.value.trim()) { message.value = '请输入 access token/app password，或展开高级选项填写环境变量名'; return }
  if (tokenEnv.value.trim() && accountSecret.value.trim()) { message.value = '本地凭据和环境变量只能选择一种'; return }
  if (accountPlatform.value === 'bluesky' && !remoteID.value.trim()) { message.value = '请填写 Bluesky handle/identifier'; return }
  try {
    const created = await api<Account>('accounts', { method: 'POST', body: JSON.stringify({ profile_id: selectedProfileID.value, name: accountName.value, platform: accountPlatform.value, remote_id: remoteID.value, base_url: baseURL.value, token_env: tokenEnv.value, secret: accountSecret.value }) })
    accountName.value = ''; baseURL.value = ''; tokenEnv.value = ''; accountSecret.value = ''; remoteID.value = ''
    await load()
    if (!accounts.value.some((account) => account.id === created.id)) accounts.value.push(created)
    message.value = '账号已添加'
    if (created.platform === 'bluesky') await diagnoseBluesky(created)
    if (isMastodon(created)) await diagnoseMastodon(created)
  } catch (error) { showError(error) }
}
function beginEditAccount(account: Account) {
  editingAccountID.value = account.id
  editAccountName.value = account.name
  editBaseURL.value = account.base_url || ''
  editRemoteID.value = account.remote_id || ''
  editTokenEnv.value = ''
  editAccountSecret.value = ''
}
function cancelEditAccount() { editingAccountID.value = ''; editAccountSecret.value = ''; editTokenEnv.value = '' }
async function updateAccount(account: Account) {
  if (!editAccountName.value.trim()) { message.value = '账号名称不能为空'; return }
  if (editTokenEnv.value.trim() && editAccountSecret.value.trim()) { message.value = '新凭据和环境变量只能选择一种'; return }
  try {
    const updated = await api<Account>(`accounts/${account.id}/connection`, { method: 'PUT', body: JSON.stringify({ name: editAccountName.value, remote_id: editRemoteID.value, base_url: editBaseURL.value, token_env: editTokenEnv.value, secret: editAccountSecret.value }) })
    Object.assign(account, updated)
    cancelEditAccount()
    message.value = '账号连接信息已更新'
    if (account.platform === 'bluesky') await diagnoseBluesky(account)
    if (isMastodon(account)) await diagnoseMastodon(account)
  } catch (error) { showError(error) }
}
async function diagnoseBluesky(account: Account) {
  if (account.platform !== 'bluesky') return
  setDiagnosticBusy(account, true)
  try {
    const result = await api<BlueskyDiagnostic>(`accounts/${account.id}/diagnostic`, { method: 'POST' })
    blueskyDiagnostic.value[account.id] = result
    connectionMessage.value = result.ok
      ? `Bluesky 连接成功：@${result.handle || account.remote_id}（${result.elapsed_ms} ms）`
      : `Bluesky 连接失败［${result.stage}/${result.category || 'unknown'}］：${result.message}${result.hint ? `。建议：${result.hint}` : ''}`
  } catch (error) {
    connectionMessage.value = `Bluesky 诊断请求失败：${error instanceof Error ? error.message : String(error)}。请查看 bpm 进程日志。`
  } finally { setDiagnosticBusy(account, false) }
}
async function diagnoseMastodon(account: Account) {
  if (!isMastodon(account)) return
  setDiagnosticBusy(account, true)
  try {
    const result = await api<MastodonDiagnostic>(`accounts/${account.id}/diagnostic`, { method: 'POST' })
    mastodonDiagnostic.value[account.id] = result
    connectionMessage.value = result.ok
      ? `${result.category ? 'Mastodon 检测完成（有警告）' : 'Mastodon 连接成功'}：@${result.username || account.name}（${result.elapsed_ms} ms）${result.hint ? `。${result.hint}` : ''}${result.status_write?.message ? ` · ${result.status_write.message}` : ''}${result.media_write?.message ? ` · ${result.media_write.message}` : ''}`
      : `Mastodon 连接失败［${result.stage}/${result.category || 'unknown'}］：${result.message}${result.hint ? `。建议：${result.hint}` : ''}`
  } catch (error) {
    connectionMessage.value = `Mastodon 诊断请求失败：${error instanceof Error ? error.message : String(error)}。请查看 bpm 进程日志。`
  } finally { setDiagnosticBusy(account, false) }
}
async function removeAccount(account: Account) {
  if (!window.confirm(`停用账号“${account.name}”？历史版本和发布记录会保留。`)) return
  try {
    await api(`accounts/${account.id}/deactivate`, { method: 'DELETE' })
    await load()
    message.value = '账号已停用，本地凭据已移除；历史记录仍保留'
  } catch (error) { showError(error) }
}
async function permanentlyDeleteAccount(account: Account) {
  if (!window.confirm(`永久删除账号“${account.name}”的本地版本、发布记录和关联？不会删除远端帖子。此操作不可恢复。`)) return
  try {
    await api(`accounts/${account.id}`, { method: 'DELETE' })
    await load()
    message.value = '账号及其本地关联已永久删除，远端帖子未受影响'
  } catch (error) { showError(error) }
}
function prepareReconnect(account: Account) {
  accountName.value = account.name
  accountPlatform.value = account.platform
  remoteID.value = account.remote_id || ''
  baseURL.value = account.base_url || ''
  tokenEnv.value = ''
  accountSecret.value = ''
  connectionMessage.value = ''
  message.value = account.remote_id ? '已填入停用账号信息；重新授权或手动添加后会恢复原账号及历史关联' : '该账号缺少可识别的远端 ID；请重新添加连接，历史记录仍保留在停用账号下'
}

async function addTopic() {
  if (!selectedProfileID.value) { message.value = '请先创建或选择 Profile'; return }
  if (!topicTitle.value.trim()) { message.value = '请填写话题标题'; return }
  try {
    const created = await api<Topic>('topics', { method: 'POST', body: JSON.stringify({ profile_id: selectedProfileID.value, title: topicTitle.value }) })
    topicTitle.value = ''
    await load()
    await openTopic(created)
  } catch (error) { showError(error) }
}

async function setTopicStatus(topic: Topic, status: 'active' | 'archived') {
  if (status === 'archived' && !window.confirm(`归档话题“${topic.title}”？内容和发布历史都会保留。`)) return
  try {
    Object.assign(topic, await api<Topic>(`topics/${topic.id}`, { method: 'PUT', body: JSON.stringify({ title: topic.title, status }) }))
    message.value = status === 'archived' ? '话题已归档' : '话题已恢复'
  } catch (error) { showError(error) }
}

async function renameSelectedTopic(silent = false) {
  if (!selected.value) return
  if (!topicTitleEdit.value.trim()) { message.value = '话题标题不能为空'; return }
  try {
    const updated = await api<Topic>(`topics/${selected.value.id}`, { method: 'PUT', body: JSON.stringify({ title: topicTitleEdit.value, status: selected.value.status }) })
    Object.assign(selected.value, updated)
    const listed = topics.value.find((topic) => topic.id === updated.id)
    if (listed) Object.assign(listed, updated)
    topicTitleEdit.value = updated.title
    return true
  } catch (error) { if (!silent) showError(error); return false }
}

async function openTopic(topic: Topic) {
  try {
    suppressAutosave.value = true
    editorReady.value = false
    selected.value = topic
    selectedProfileID.value = topic.profile_id
    const loaded = await Promise.all([
      api<Draft>(`topics/${topic.id}/draft`), api<Variant[]>(`variants?topic_id=${topic.id}`),
      api<Media[]>(`media?topic_id=${topic.id}`), api<ExternalPost[]>(`external-posts?topic_id=${topic.id}`),
    ])
    draft.value = loaded[0]; variants.value = listOrEmpty(loaded[1]); rememberVariants(variants.value); media.value = listOrEmpty(loaded[2]); externalPosts.value = listOrEmpty(loaded[3])
    text.value = draft.value.text
    topicTitleEdit.value = topic.title
    mediaVariantID.value = ''
    variantSearch.value = ''
    activeVariantID.value = 'draft'
    emojis.value = []
    emojiLoadedAccountID.value = ''
    emojiTargetAccountID.value = variants.value.find((variant) => variant.id === activeVariantID.value && variantCapability(variant)?.custom_emoji)?.account_id || variants.value.find((variant) => variantCapability(variant)?.custom_emoji)?.account_id || ''
    emojiFilter.value = ''
    emojiLimit.value = 50
    translationSourceID.value = variants.value[0]?.id || ''
    translationTargetAccountID.value = profileAccounts.value[0]?.id || ''
    externalImportAccountID.value = profileAccounts.value.find((account) => account.platform === 'misskey' || account.platform === 'bluesky')?.id || ''
    section.value = 'editor'
    if (window.location.pathname !== `/topics/${encodeURIComponent(topic.id)}`) window.history.pushState({}, '', `/topics/${encodeURIComponent(topic.id)}`)
    editorReady.value = true
    await nextTick()
    suppressAutosave.value = false
  } catch (error) { showError(error) }
}

async function persistDraft(announce: boolean, silent = false) {
  if (!selected.value) return false
  try {
    const changed = draftDirty.value
    draft.value = await api<Draft>(`topics/${selected.value.id}/draft`, { method: 'PUT', body: JSON.stringify({ text: text.value, format: 'plain', language: '' }) })
    if (changed) variants.value.forEach((variant) => { if (variant.relation === 'linked' && !variant.published) variant.outdated = true })
    if (announce) message.value = '基础稿已保存'
    return true
  } catch (error) { if (!silent) showError(error); return false }
}
async function saveDraft() { await flushAutoSave() }

async function flushAutoSave() {
  if (!editorReady.value || !selected.value) return
  const needsDraft = draftDirty.value
  const needsTitle = topicTitleDirty.value
  const current = activeVariant.value
  const needsVariant = Boolean(current && variantDirty(current))
  if (!needsDraft && !needsTitle && !needsVariant) { saveState.value = 'idle'; return }
  if (saveTimer !== undefined) { window.clearTimeout(saveTimer); saveTimer = undefined }
  saveState.value = 'saving'
  try {
    if (needsTitle && !await renameSelectedTopic(true)) throw new Error('title save failed')
    if (needsDraft && !await persistDraft(false, true)) throw new Error('draft save failed')
    if (current && variantDirty(current) && !await saveVariant(current, false, true)) throw new Error('variant save failed')
    saveState.value = 'idle'
  } catch {
    saveState.value = 'error'
  }
}
function scheduleAutoSave() {
  if (!editorReady.value || suppressAutosave.value) return
  saveState.value = 'saving'
  if (saveTimer !== undefined) window.clearTimeout(saveTimer)
  saveTimer = window.setTimeout(() => { saveTimer = undefined; void flushAutoSave() }, 700)
}
watch([topicTitleEdit, editorText], scheduleAutoSave)

async function openPublishDialog() {
  if (!activeVariant.value) {
    if (!selected.value || !selectedAccountID.value) { message.value = '请先选择目标账号'; return }
    if (draftDirty.value && !await persistDraft(false)) return
    try {
      const created = await api<Variant>('variants', { method: 'POST', body: JSON.stringify({ topic_id: selected.value.id, account_id: selectedAccountID.value }) })
      variants.value.push(created)
      variantBaselines.value[created.id] = variantSignature(created)
      activeVariantID.value = created.id
      await nextTick()
    } catch (error) { showError(error); return }
  }
  if (activeVariant.value.published) {
    message.value = '这个版本已有发布记录，请复制为新版本后再发布'
    return
  }
  if (draftDirty.value && !await persistDraft(false)) return
  if (variantDirty(activeVariant.value) && !await saveVariant(activeVariant.value, false)) return
  publishDialogReturnFocus = document.activeElement as HTMLElement | null
  bridgeAutoPublish.value = false
  publishDialogOpen.value = true
  await nextTick()
  publishDialogEl.value?.querySelector<HTMLElement>('button, [tabindex="0"]')?.focus()
}
function closePublishDialog() {
  publishDialogOpen.value = false
  nextTick(() => publishDialogReturnFocus?.focus())
}
function onDialogKeydown(event: KeyboardEvent) {
  if (mediaDialogOpen.value && event.key === 'Escape') { event.preventDefault(); mediaDialogOpen.value = false; return }
  if (!publishDialogOpen.value) {
    if (section.value !== 'editor' || (!event.metaKey && !event.ctrlKey)) return
    if (event.key.toLowerCase() === 's') { event.preventDefault(); void flushAutoSave() }
    else if (event.key === 'Enter') { event.preventDefault(); void openPublishDialog() }
    return
  }
  if (event.key === 'Escape') { event.preventDefault(); closePublishDialog(); return }
  if (event.key !== 'Tab') return
  const dialog = publishDialogEl.value
  if (!dialog) return
  const focusable = Array.from(dialog.querySelectorAll<HTMLElement>('button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [href], [tabindex]:not([tabindex="-1"])'))
  if (!focusable.length) return
  const first = focusable[0]
  const last = focusable[focusable.length - 1]
  if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus() }
  else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus() }
}
async function confirmPublish() {
  if (!activeVariant.value || publishDialogBusy.value) return
  publishDialogBusy.value = true
  try {
    const account = variantAccount(activeVariant.value)
    if (isBridgePlatform(account?.platform || '')) await fillBrowser(activeVariant.value, bridgeAutoPublish.value)
    else await publish(activeVariant.value)
    closePublishDialog()
  } finally {
    publishDialogBusy.value = false
  }
}

async function newVariant() {
  if (!selected.value) return
  if (!selectedAccountID.value) { message.value = '请先添加并选择目标账号'; return }
  if (draftDirty.value && !await persistDraft(false)) return
  if (!await saveAllDirtyVariants()) return
  try {
    const created = await api<Variant>('variants', { method: 'POST', body: JSON.stringify({ topic_id: selected.value.id, account_id: selectedAccountID.value }) })
    variants.value.push(created)
    variantBaselines.value[created.id] = variantSignature(created)
    variantSearch.value = ''
    activeVariantID.value = created.id
    showTargetAdd.value = false
    emojiTargetAccountID.value = selectedAccountID.value
    translationSourceID.value ||= variants.value[0]?.id || ''
    message.value = '平台版本已生成'
  } catch (error) { showError(error) }
}
async function publishFromDraft() {
  if (draftPublishBusy.value) return
  if (!selected.value || !selectedAccountID.value) { message.value = '请先选择目标账号'; return }
  if (draftDirty.value && !await persistDraft(false)) return
  if (selectedAccount.value?.disabled) { message.value = '目标账号已停用，无法发布'; return }
  draftPublishBusy.value = true
  try {
    const created = await api<Variant>('variants', { method: 'POST', body: JSON.stringify({ topic_id: selected.value.id, account_id: selectedAccountID.value }) })
    variants.value.push(created)
    variantBaselines.value[created.id] = variantSignature(created)
    variantSearch.value = ''
    activeVariantID.value = created.id
    emojiTargetAccountID.value = created.account_id
    if (isBridgePlatform(selectedAccount.value?.platform || '')) {
      if (await fillBrowser(created)) message.value = '已从基础稿生成快照并发送填入请求；桥接平台仍需在平台页确认发布'
      return
    }
    if (await publish(created)) message.value = '已从基础稿生成快照并发布成功'
  } catch (error) { showError(error) }
  finally { draftPublishBusy.value = false }
}

async function saveVariant(variant: Variant, announce = true, silent = false) {
  try {
    Object.assign(variant, await api<Variant>(`variants/${variant.id}`, { method: 'PUT', body: JSON.stringify(variant) }))
    variantBaselines.value[variant.id] = variantSignature(variant)
    if (announce) message.value = '平台版本已保存并与基础稿分离'
    return true
  } catch (error) { if (!silent) showError(error); return false }
}
async function saveAllDirtyVariants() {
  for (const variant of variants.value) {
    if (variantDirty(variant) && !await saveVariant(variant, false)) return false
  }
  return true
}

async function applyVariant(variant: Variant) {
  if (variantDirty(variant) && !window.confirm('应用基础稿更新会覆盖这个平台版本尚未保存的修改，确定继续吗？')) return
  try {
    Object.assign(variant, await api<Variant>(`variants/${variant.id}/apply`, { method: 'POST' }))
    variantBaselines.value[variant.id] = variantSignature(variant)
    message.value = '已应用基础稿更新'
  } catch (error) { showError(error) }
}

async function publish(variant: Variant) {
  if (variantBusy(variant)) return false
  setVariantBusy(variant, true)
  if (variantDirty(variant) && !await saveVariant(variant, false)) { setVariantBusy(variant, false); return false }
  try {
    await api('publications', { method: 'POST', body: JSON.stringify({ variant_id: variant.id }) })
    variant.published = true
    message.value = '发布完成（请查看历史）'
    return true
  } catch (error) { message.value = `发布失败：${error instanceof Error ? error.message : String(error)}`; return false }
  finally { setVariantBusy(variant, false) }
}

async function cloneVariant(variant: Variant) {
  if (!selected.value || variantBusy(variant)) return
  setVariantBusy(variant, true)
  if (variantDirty(variant) && !await saveVariant(variant, false)) { setVariantBusy(variant, false); return }
  try {
    const created = await api<Variant>('variants', { method: 'POST', body: JSON.stringify({ topic_id: selected.value.id, account_id: variant.account_id, source_variant_id: variant.id }) })
    variants.value.push(created)
    variantBaselines.value[created.id] = variantSignature(created)
    variantSearch.value = ''
    activeVariantID.value = created.id
    emojiTargetAccountID.value = created.account_id
    translationSourceID.value = created.id
    message.value = '已复制为可编辑的新版本'
  } catch (error) { showError(error) }
  finally { setVariantBusy(variant, false) }
}

async function markExternal() {
  if (!selected.value) return
  if (!externalURL.value.trim()) { message.value = '请填写要关联的帖子 URL'; return }
  try {
    await api('external-posts', { method: 'POST', body: JSON.stringify({ topic_id: selected.value.id, account_id: externalAccountID.value, platform: externalPlatform.value, url: externalURL.value }) })
    externalURL.value = ''
    externalPosts.value = listOrEmpty(await api<ExternalPost[]>(`external-posts?topic_id=${selected.value.id}`))
    message.value = '已标记站外 URL'
  } catch (error) { showError(error) }
}
async function importExternal() { if (!selected.value) return; if (!externalImportAccountID.value || !externalReference.value.trim()) { message.value='请选择导入账号并填写帖子 ID/URL'; return }; try { const post=await api<ExternalPost>('external-posts/import',{method:'POST',body:JSON.stringify({topic_id:selected.value.id,account_id:externalImportAccountID.value,reference:externalReference.value})});externalPosts.value.unshift(post);externalReference.value='';message.value='已导入快照'} catch(error){showError(error)} }
async function deleteExternalPost(post: ExternalPost) {
  if (!window.confirm(`移除这条 ${post.platform} 站外关联？只删除本地记录，不会操作远端帖子。`)) return
  try {
    await api(`external-posts/${post.id}`, { method: 'DELETE' })
    externalPosts.value = externalPosts.value.filter((item) => item.id !== post.id)
    message.value = '站外关联已移除，远端帖子未受影响'
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
    media.value.push(body); altText.value = ''; input.value = ''; mediaDialogOpen.value = false
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
  if (!window.confirm('移除这个平台版本及本地发布记录和图片？不会删除远端帖子。')) return
  try {
    await api(`variants/${variant.id}`, { method: 'DELETE' })
    variants.value = variants.value.filter((item) => item.id !== variant.id)
    media.value = media.value.filter((item) => item.variant_id !== variant.id)
    delete variantBaselines.value[variant.id]
    if (mediaVariantID.value === variant.id) mediaVariantID.value = ''
    if (translationSourceID.value === variant.id) translationSourceID.value = variants.value[0]?.id || ''
    message.value = '平台版本和本地发布记录已移除，远端帖子未受影响'
  } catch (error) { showError(error) }
}

async function loadEmoji(accountID = selectedAccountID.value) {
  if (!accountID || emojiLoading.value) return
  if (emojiLoadedAccountID.value === accountID && emojis.value.length) return
  emojiLoading.value = true
  emojis.value = []
  emojiLoadedAccountID.value = ''
  emojiFilter.value = ''
  emojiLimit.value = 50
  try {
    emojis.value = listOrEmpty(await api<Emoji[]>(`accounts/${accountID}/emojis`))
    emojiLoadedAccountID.value = accountID
    emojiFilter.value = ''
    emojiLimit.value = 50
    message.value = `已加载 ${emojis.value.length} 个实例 emoji（首屏显示 50 个）`
  } catch (error) { message.value = `emoji 加载失败：${error instanceof Error ? error.message : String(error)}` }
  finally { emojiLoading.value = false }
}
function showMoreEmoji() { emojiLimit.value += 50 }
async function insertEmoji(emoji: Emoji, variant: Variant) {
  if (variant.published) return
  const value = `:${emoji.shortcode}:`
  const area = variantEditor.value
  const start = area?.selectionStart ?? variant.text.length
  const end = area?.selectionEnd ?? start
  variant.text = `${variant.text.slice(0, start)}${value}${variant.text.slice(end)}`
  await nextTick()
  area?.focus()
  const cursor = start + value.length
  area?.setSelectionRange(cursor, cursor)
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
  if (variantBusy(variant)) return false
  const account = accounts.value.find((item) => item.id === variant.account_id)
  if (autoPublish && !window.confirm(`自动填入并点击 ${account?.name || account?.platform || '平台'} 当前页面的可见发布按钮？带图片时会为避免漏图而退回人工发布；页面结构变化时也可能失败，请随后核对结果。`)) return false
  setVariantBusy(variant, true)
  if (variantDirty(variant) && !await saveVariant(variant, false)) { setVariantBusy(variant, false); return false }
  const pairing = connectedPairing(account?.platform)
  if (!pairing) {
    const authorized = bridgePairings.value.some((item) => item.code_used && !item.revoked && item.platform === account?.platform)
    bridgeMessage.value = authorized ? '已有授权，但没有检测到平台脚本近期心跳；请打开平台页并检查右下角 bridge 状态' : '请先完成匹配平台的配对；脚本会把当前页面安全回传给本地服务'
    setVariantBusy(variant, false)
    return false
  }
  try {
    bridgeRequest.value = await api<BridgeRequest>('bridge/requests/fill', { method: 'POST', body: JSON.stringify({ pairing_id: pairing.id, variant_id: variant.id, page_url: pairing.page_url || '', auto_publish: autoPublish }) })
    variant.published = true
    bridgeMessage.value = autoPublish ? '已发送自动发布请求；脚本只会点击已知、可见且启用的发布按钮，请随后核对平台结果' : '已发送，等待脚本领取；填入后请人工检查并点击平台发布'
    return true
  } catch (error) { showError(error); return false }
  finally { setVariantBusy(variant, false) }
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
  const source = variants.value.find((variant) => variant.id === translationSourceID.value)
  if (source && variantDirty(source) && !await saveVariant(source, false)) { translationMessage.value = '源版本保存失败，未开始翻译'; return }
  try {
    const result = await api<{ variant: Variant; translation: unknown }>('translations', { method: 'POST', body: JSON.stringify({ source_variant_id: translationSourceID.value, target_account_id: translationTargetAccountID.value, source_language: translationSourceLanguage.value, target_language: translationTargetLanguage.value, config_id: translationConfigID.value }) })
    variants.value.push(result.variant); variantBaselines.value[result.variant.id] = variantSignature(result.variant); variantSearch.value = ''; activeVariantID.value = result.variant.id; emojiTargetAccountID.value = result.variant.account_id; translationMessage.value = '已生成新的 detached 翻译版本，原文未修改，已切换到新版本'
  } catch (error) { translationMessage.value = `翻译失败：${error instanceof Error ? error.message : String(error)}；原文与目标版本均未改动` }
}
async function translateActiveVariant() {
  if (activeVariant.value) translationSourceID.value = activeVariant.value.id
  await translateVariant()
}
let bridgeSyncing = false
let bridgeRefreshTimer: number | undefined
async function syncBridgeState(announce: boolean) {
  if (bridgeSyncing) return
  bridgeSyncing = true
  try {
    const existing = new Map(bridgePairings.value.map((pairing) => [pairing.id, pairing]))
    bridgePairings.value = listOrEmpty(await api<BridgePairing[]>('bridge/pairings')).map((pairing) => ({ ...existing.get(pairing.id), ...pairing }))
    if (bridgeRequest.value) bridgeRequest.value = await api<BridgeRequest>(`bridge/requests/${bridgeRequest.value.id}`)
    if (selected.value) externalPosts.value = listOrEmpty(await api<ExternalPost[]>(`external-posts?topic_id=${selected.value.id}`))
    if (announce) bridgeMessage.value = '桥接状态与站外快照已刷新'
  } catch (error) { if (announce) showError(error) }
  finally { bridgeSyncing = false }
}
async function refreshBridgeState() { await syncBridgeState(true) }

onMounted(async () => {
  window.addEventListener('beforeunload', warnBeforeUnload)
  window.addEventListener('popstate', onPopState)
  window.addEventListener('keydown', onDialogKeydown)
  await load()
  await applyBrowserRoute()
  const params = new URLSearchParams(window.location.search)
  const connected = params.get('connection')
  if (connected === 'mastodon' || connected === 'misskey') {
    section.value = 'settings'
    connectionMessage.value = `${connected === 'mastodon' ? 'Mastodon' : 'Misskey'} 连接成功，本地凭据已保存。`
    params.delete('connection')
    const query = params.toString()
    window.history.replaceState({}, '', `/settings${query ? `?${query}` : ''}${window.location.hash}`)
  }
  bridgeRefreshTimer = window.setInterval(() => { if (section.value === 'editor') void syncBridgeState(false) }, 3_000)
})
onBeforeUnmount(() => {
  window.removeEventListener('beforeunload', warnBeforeUnload)
  window.removeEventListener('popstate', onPopState)
  window.removeEventListener('keydown', onDialogKeydown)
  if (bridgeRefreshTimer !== undefined) window.clearInterval(bridgeRefreshTimer)
  if (toastTimer !== undefined) window.clearTimeout(toastTimer)
  if (saveTimer !== undefined) window.clearTimeout(saveTimer)
})
</script>

<template>
  <div class="shell">
    <aside class="sidebar" aria-label="工作区导航">
      <div class="brand"><span class="brand-mark" aria-hidden="true"><svg viewBox="0 0 20 20"><path d="M4 16 16 4M7 4h9v9" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="1.8"/></svg></span><div><strong>Best Post</strong><small>Manager</small></div></div>
      <div class="topic-heading" role="tablist" aria-label="话题操作"><button type="button" class="topic-tab" :class="{active:sidebarTool==='create'}" role="tab" :aria-selected="sidebarTool==='create'" @click="sidebarTool='create'">新建</button><button type="button" class="topic-tab" :class="{active:sidebarTool==='profile'}" role="tab" :aria-selected="sidebarTool==='profile'" @click="sidebarTool='profile'">Profile</button><button type="button" class="topic-tab" :class="{active:sidebarTool==='search'}" role="tab" :aria-selected="sidebarTool==='search'" @click="sidebarTool='search'">搜索</button></div>
      <div class="topic-panel"><form v-show="sidebarTool==='create'" class="topic-panel-body" @submit.prevent="addTopic"><input v-model="topicTitle" placeholder="新话题标题，回车创建" aria-label="新建话题标题" :disabled="!selectedProfileID"></form><div v-show="sidebarTool==='profile'" class="topic-panel-body"><select class="profile-select" v-model="selectedProfileID" aria-label="Profile"><option v-for="profile in profiles" :key="profile.id" :value="profile.id">{{profile.name}}</option></select></div><div v-show="sidebarTool==='search'" class="topic-panel-body"><input v-model="topicSearch" aria-label="搜索话题标题" placeholder="搜索标题"></div></div>
      <ul class="quick-topics" aria-label="最近话题"><li v-for="topic in visibleTopics.slice(0, 8)" :key="topic.id"><a :class="{active:selected?.id===topic.id}" :href="`/topics/${encodeURIComponent(topic.id)}`" @click="followTopicLink($event, topic)"><strong>{{topic.title}}</strong><small>{{topicProgress(topic)}}</small></a></li><li v-if="!visibleTopics.length" class="quick-empty">暂无话题</li></ul>
      <nav aria-label="主导航" class="primary-nav">
        <a class="nav-link" :class="{active:section==='topics'}" href="/topics" @click="followSectionLink($event, 'topics')"><span class="nav-icon" aria-hidden="true"><svg viewBox="0 0 20 20"><path d="m3 9 7-6 7 6v7a1 1 0 0 1-1 1h-4v-5H8v5H4a1 1 0 0 1-1-1V9Z" fill="none" stroke="currentColor" stroke-linejoin="round" stroke-width="1.5"/></svg></span> 管理话题<span class="sr-only">内容</span></a>
        <a class="nav-link" :class="{active:section==='history'}" href="/history" @click="followSectionLink($event, 'history')"><span class="nav-icon" aria-hidden="true"><svg viewBox="0 0 20 20"><path d="M4 5h12v11H4zM7 3v4M13 3v4M4 9h12" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5"/></svg></span> 发布中心</a>
        <a class="nav-link" :class="{active:section==='accounts'}" href="/accounts" @click="followSectionLink($event, 'accounts')"><span class="nav-icon" aria-hidden="true"><svg viewBox="0 0 20 20"><circle cx="10" cy="7" r="3" fill="none" stroke="currentColor" stroke-width="1.5"/><path d="M4.5 17c.7-3 2.5-4.5 5.5-4.5s4.8 1.5 5.5 4.5" fill="none" stroke="currentColor" stroke-linecap="round" stroke-width="1.5"/></svg></span> 账号管理</a>
        <a class="nav-link" :class="{active:section==='settings'}" href="/settings" @click="followSectionLink($event, 'settings')"><span class="nav-icon" aria-hidden="true"><svg viewBox="0 0 20 20"><path d="m8 3 .5 1.5a6 6 0 0 1 3 0L12 3l2 1.2-.7 1.5a6 6 0 0 1 2 2l1.5-.2v2.3l-1.5.2a6 6 0 0 1 0 3l1.5.5-2 1.2-1.5-.7a6 6 0 0 1-2 2l.2 1.5H9.2L9 17a6 6 0 0 1-3 0l-.5 1.5-2-1.2.7-1.5a6 6 0 0 1-2-2l-1.5.2v-2.3l1.5-.2a6 6 0 0 1 0-3L2.7 8l2-1.2 1.5.7a6 6 0 0 1 2-2L8 4.5 8 3Z" fill="none" stroke="currentColor" stroke-linejoin="round" stroke-width="1.1"/><circle cx="10" cy="10" r="2" fill="none" stroke="currentColor" stroke-width="1.3"/></svg></span> 设置</a>
      </nav>
      <span class="sr-only">本地单用户</span>
    </aside>
    <main class="canvas">
      <div v-if="toast.text" class="toast" :class="`toast-${toast.kind}`" :role="toast.kind === 'success' ? 'status' : 'alert'" :aria-live="toast.kind === 'success' ? 'polite' : 'assertive'"><span>{{toast.text}}</span><button type="button" aria-label="关闭提示" title="关闭提示" @click="dismissToast">×</button></div>
      <section v-if="section==='topics'" class="page-section topics-page">
        <header class="management-head"><h1 id="topics-title">全部话题</h1></header>
        <form class="topic-create mobile-topic-create" @submit.prevent="addTopic"><input v-model="topicTitle" placeholder="新话题标题" :disabled="!selectedProfileID"><button class="primary-action" :disabled="!selectedProfileID">＋ 新建话题</button></form>
        <section class="topic-browser" aria-labelledby="topic-browser-title"><header class="topic-browser-head"><div class="section-label topic-count"><span id="topic-browser-title">话题</span><small>{{visibleTopics.length}} 个</small></div><div class="topic-management-tools">
          <label class="management-search"><svg viewBox="0 0 20 20" aria-hidden="true"><circle cx="8.5" cy="8.5" r="5" fill="none" stroke="currentColor" stroke-width="1.5"/><path d="m12.5 12.5 4 4" fill="none" stroke="currentColor" stroke-linecap="round" stroke-width="1.5"/></svg><input v-model="topicSearch" placeholder="搜索标题" aria-label="搜索全部话题"></label>
          <label class="archived-filter"><input type="checkbox" v-model="showArchivedTopics">显示已归档</label>
          <select id="content-profile" class="profile-select" v-model="selectedProfileID" aria-label="Profile"><option v-for="profile in profiles" :key="profile.id" :value="profile.id">{{profile.name}}</option></select>
        </div></header>
        <p v-if="profiles.length===0" class="empty-state">还没有 Profile。前往账号管理创建后，就可以开始第一个话题。</p><p v-else-if="visibleTopics.length===0" class="empty-state">这个 Profile 还没有话题。</p><ul v-else class="rows management-list topic-list"><li v-for="topic in visibleTopics" :key="topic.id" class="management-row"><div class="row-main"><a class="link" :href="`/topics/${encodeURIComponent(topic.id)}`" @click="followTopicLink($event, topic)">{{topic.title}}</a></div><span class="topic-progress">{{topicProgress(topic)}}</span><IconButton :icon="topic.status==='archived' ? 'refresh' : 'archive'" :label="topic.status==='archived' ? '恢复话题' : '归档话题'" @click="setTopicStatus(topic, topic.status==='archived'?'active':'archived')" /></li></ul></section>
      </section>
      <section v-else-if="section==='editor' && selected" class="page-section editor-page">
        <header class="editor-head">
          <div class="editor-head-main"><div class="title-row"><input class="topic-title" v-model="topicTitleEdit" aria-label="话题标题"><span class="save-state" aria-live="polite">{{saveState === 'saving' ? '保存中…' : saveState === 'error' ? '保存失败' : ''}}</span><button v-if="saveState==='error'" type="button" class="quiet-action" @click="flushAutoSave">重试</button></div></div>
          <details class="topic-more"><summary aria-label="更多话题操作" title="更多话题操作"><svg viewBox="0 0 20 20" aria-hidden="true"><circle cx="4" cy="10" r="1.2"/><circle cx="10" cy="10" r="1.2"/><circle cx="16" cy="10" r="1.2"/></svg></summary><div class="topic-more-menu"><button type="button" @click="goSection('topics')">返回话题列表</button></div></details>
        </header>
        <div class="document-tabs" role="tablist" aria-label="稿件版本"><button type="button" role="tab" :aria-selected="activeVariantID==='draft'" :class="['document-tab', {active:activeVariantID==='draft'}]" @click="selectDraft"><span class="tab-icon" aria-hidden="true"><svg viewBox="0 0 20 20"><path d="M5 3h7l3 3v11H5zM12 3v4h3" fill="none" stroke="currentColor" stroke-width="1.4"/></svg></span><span>主稿</span><small>内容源头</small></button><button v-for="variant in variants" :key="variant.id" type="button" role="tab" :aria-selected="activeVariantID===variant.id" :class="['document-tab', {active:activeVariantID===variant.id}]" :title="variantAccountLabel(variant)" @click="selectDocument(variant.id)"><span class="tab-icon"><PlatformIcon :platform="variantAccount(variant)?.platform" /></span><strong>{{variantTabLabel(variant)}}</strong><small>{{variantStatusLabel(variant)}}<b v-if="variant.outdated"> · 主稿有更新</b></small></button><button type="button" class="document-tab-add" aria-label="添加目标" @click="showTargetAdd=!showTargetAdd">＋</button></div>
        <div v-if="showTargetAdd" class="target-add target-add-inline"><select v-model="selectedAccountID" aria-label="选择目标账号"><option value="">选择账号</option><option v-for="account in profileAccounts" :key="account.id" :value="account.id">{{account.name}} · {{platformLabel(account.platform)}}</option></select><button type="button" @click="newVariant" :disabled="!selectedAccountID">添加目标</button></div>
        <div class="editor-shell"><div class="editor-paper"><textarea class="draft-editor" v-model="editorText" placeholder="从这里开始写…" :disabled="Boolean(activeVariant?.published)" aria-label="当前稿件正文" @blur="flushAutoSave"></textarea></div><div class="editor-toolbar"><div class="editor-tool-group"><button type="button" class="tool-button" :class="{active:editorTool==='emoji'}" @click="toggleEditorTool('emoji')" :disabled="!activeVariant || !variantCapability(activeVariant)?.custom_emoji"><svg viewBox="0 0 20 20" aria-hidden="true"><circle cx="10" cy="10" r="7" fill="none" stroke="currentColor" stroke-width="1.4"/><path d="M7 11.5s1 1.6 3 1.6 3-1.6 3-1.6" fill="none" stroke="currentColor" stroke-linecap="round" stroke-width="1.4"/><circle cx="7.5" cy="8" r=".7"/><circle cx="12.5" cy="8" r=".7"/></svg>Emoji</button><button type="button" class="tool-button" :class="{active:editorTool==='translation'}" @click="toggleEditorTool('translation')" :disabled="!activeVariant"><svg viewBox="0 0 20 20" aria-hidden="true"><path d="M3 5h7M6.5 3v2M4 7c1.2 2.2 3 3.8 5.2 4.8M8.5 6c-.7 2.5-2.4 4.7-5 6M11 17l3.3-8 3.2 8M12.3 14.3h4" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="1.3"/></svg>翻译</button><details class="inline-more"><summary><svg viewBox="0 0 20 20" aria-hidden="true"><path d="M6 3v14M14 3v14M3 6h14M3 14h14" fill="none" stroke="currentColor" stroke-width="1.3"/></svg>更多</summary><div class="inline-more-menu"><button type="button" @click="toggleEditorTool('external')">站外关联</button><button v-if="activeVariant?.published" type="button" @click="cloneVariant(activeVariant)" :disabled="variantBusy(activeVariant)">复制为新版本</button><button v-if="activeVariant && !variantAccount(activeVariant)?.disabled" type="button" aria-label="移除本地版本（不会删除远端帖子）" @click="deleteVariant(activeVariant)" :disabled="variantBusy(activeVariant)">移除版本</button></div></details></div><div class="editor-toolbar-right"><button v-if="activeVariant?.outdated&&!activeVariant.published" type="button" class="source-update" @click="applyVariant(activeVariant)" :disabled="variantBusy(activeVariant)">主稿已修改，点击更新</button><span class="draft-count">{{editorText.length}} 字符<span v-if="variantCapability(activeVariant)?.text_max_bytes"> · {{utf8Bytes(editorText)}} / {{variantCapability(activeVariant)?.text_max_bytes}} bytes</span></span></div></div><section v-if="activeVariant && variantCapability(activeVariant)?.custom_emoji" v-show="editorTool==='emoji'" class="editor-tool-panel" aria-label="实例 Emoji"><button type="button" @click="loadEmoji(activeVariant.account_id)" :disabled="emojiLoading">{{emojiLoading ? '加载中…' : '加载实例 emoji'}}</button><input v-model="emojiFilter" placeholder="搜索 shortcode" :disabled="emojiLoadedAccountID!==activeVariant.account_id"><div v-if="emojiLoadedAccountID===activeVariant.account_id" class="emoji-browser"><small>共 {{emojis.length}} 个，匹配 {{matchingEmojis.length}} 个，当前显示 {{visibleEmojis.length}} 个</small><div class="emoji-list"><button v-for="emoji in visibleEmojis" :key="emoji.shortcode" type="button" class="emoji" @click="insertEmoji(emoji, activeVariant)"><img loading="lazy" :src="emoji.static_url||emoji.url" alt="">:{{emoji.shortcode}}:</button></div><button v-if="hasMoreEmojis" type="button" @click="showMoreEmoji">继续显示 50 个</button></div></section><section v-if="activeVariant" v-show="editorTool==='translation'" class="editor-tool-panel translation-tool" aria-label="翻译此版本"><label><span>目标账号</span><select v-model="translationTargetAccountID" aria-label="翻译目标账号"><option v-for="account in profileAccounts" :key="account.id" :value="account.id">{{account.name}}</option></select></label><label><span>源语言</span><input v-model="translationSourceLanguage" placeholder="自动识别"></label><label><span>目标语言</span><input v-model="translationTargetLanguage" placeholder="例如 zh"></label><label><span>翻译配置</span><select v-model="translationConfigID" aria-label="翻译配置"><option value="">选择翻译配置</option><option v-for="config in translationConfigs" :key="config.id" :value="config.id">{{config.provider}} · {{config.model || config.project_id || config.id}}</option></select></label><button type="button" @click="translateActiveVariant" :disabled="!translationConfigID">生成版本</button></section><section v-show="editorTool==='external'" class="editor-tool-panel external-tool" aria-label="站外关联"><div class="actions"><button type="button" @click="requestCapture">捕获当前页面</button><a href="/userscript/bpm-bridge.user.js">安装桥接脚本</a></div><p v-if="bridgeRequest" class="bridge-request-status">最近请求：{{bridgeRequestState(bridgeRequest)}}<span v-if="bridgeRequest.warning"> · {{bridgeRequest.warning}}</span><span v-if="bridgeRequest.error"> · {{bridgeRequest.error}}</span></p><form @submit.prevent="markExternal"><select v-model="externalPlatform"><option>mastodon</option><option>misskey</option><option>bluesky</option><option value="x">X</option><option value="weibo">微博</option><option value="xiaohongshu">小红书</option><option value="qzone">QQ 空间</option></select><select v-model="externalAccountID"><option value="">不关联账号</option><option v-for="account in externalPlatformAccounts" :key="account.id" :value="account.id">{{account.name}}</option></select><input v-model="externalURL" placeholder="仅标记 URL，不抓取"><button>标记 URL</button></form><form @submit.prevent="importExternal"><select v-model="externalImportAccountID" aria-label="导入账号"><option value="">选择 Misskey / Bluesky 账号</option><option v-for="account in profileAccounts.filter(item=>item.platform==='misskey'||item.platform==='bluesky')" :key="account.id" :value="account.id">{{account.name}} · {{platformLabel(account.platform)}}</option></select><input v-model="externalReference" placeholder="帖子 ID、URL 或 at://"><button :disabled="!canImportOfficially">导入快照</button></form><ul class="rows external-post-list"><li v-for="post in externalPosts" :key="post.id"><span>{{platformLabel(post.platform)}}<em v-if="post.incomplete">不完整：{{post.incomplete_reason}}</em></span><a :href="post.url" target="_blank" rel="noreferrer">{{post.url}}</a><small v-if="post.text_snapshot">{{post.text_snapshot.slice(0,120)}}</small><button class="danger-text" @click="deleteExternalPost(post)">移除本地关联</button></li></ul></section></div>
        <section class="flow-section media-flow"><div class="flow-head"><strong>媒体</strong><span>{{currentMedia.length}} 张 · 此版本使用通用媒体</span></div><ul v-if="currentMedia.length" class="media-list media-row"><li v-for="item in currentMedia" :key="item.id" class="media-item"><img :src="`/media/${item.id}`" :alt="item.alt_text||item.filename"><span class="media-label" :class="{missing:!item.alt_text}">{{item.alt_text ? '替代文本已填写' : '需要替代文本'}}</span><span class="media-filename" :title="item.filename">{{item.filename}}</span><small>{{mediaScopeLabel(item)}}</small><IconButton icon="trash" :label="`删除图片 ${item.filename}`" @click="deleteMedia(item)" /></li><li class="media-add-item"><button type="button" class="add-media" aria-label="添加图片" @click="mediaDialogOpen=true"><svg viewBox="0 0 20 20" aria-hidden="true"><path d="M10 4v12M4 10h12" fill="none" stroke="currentColor" stroke-linecap="round" stroke-width="1.5"/></svg></button></li></ul><div v-else class="media-row"><button type="button" class="add-media" aria-label="添加图片" @click="mediaDialogOpen=true"><svg viewBox="0 0 20 20" aria-hidden="true"><path d="M10 4v12M4 10h12" fill="none" stroke="currentColor" stroke-linecap="round" stroke-width="1.5"/></svg></button></div></section>
        <div v-if="variants.length===0" class="empty-state">还没有平台版本；选择目标账号后从基础稿生成。</div>
        <section v-if="activeVariant" class="flow-section variant-settings" aria-labelledby="variant-settings-title"><div class="flow-head"><strong id="variant-settings-title">发布设置</strong></div><div class="variant-options"><label v-if="variantCapability(activeVariant)?.visibilities?.length"><span>可见范围</span><select v-model="activeVariant.visibility" :disabled="activeVariant.published"><option v-for="visibility in variantCapability(activeVariant)?.visibilities || []" :key="visibility">{{visibility}}</option></select></label><label v-if="variantCapability(activeVariant)?.supports_cw"><span>内容警告</span><input v-model="activeVariant.spoiler_text" placeholder="内容警告（可选）" :disabled="activeVariant.published"></label><label v-if="variantCapability(activeVariant)?.supports_local_only" class="checkbox-option"><input type="checkbox" v-model="activeVariant.local_only" :disabled="activeVariant.published">仅本地</label></div></section>
        <section v-if="selected" class="publish-flow" aria-labelledby="publish-flow-title">
          <div class="flow-head"><strong id="publish-flow-title">发布前检查</strong><span>{{preflightSummary}}</span></div>
          <ul class="preflight-list">
            <li v-for="check in preflightChecks" :key="check.title" :class="`preflight-${check.tone}`">
              <span class="check-icon" aria-hidden="true">{{check.tone === 'ok' ? '✓' : check.tone === 'warning' ? '!' : '×'}}</span>
              <span class="check-copy"><strong>{{check.title}}</strong><small>{{check.detail}}</small></span>
              <span class="check-result">{{check.result}}</span>
            </li>
          </ul>
          <div class="publish-area"><button type="button" class="primary-action publish-cta publish-button" :disabled="!selectedAccountID || Boolean(activeVariant?.published) || preflightBlocking || publishDialogBusy" @click="openPublishDialog"><svg viewBox="0 0 20 20" aria-hidden="true"><path d="m3 10 14-7-5 14-2-5-7-2Z" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5"/></svg>继续发布</button></div>
        </section>
      </section>
      <section v-else-if="section==='editor'" class="page-section empty-editor">
        <div class="eyebrow">正在编辑</div><h1>先选择一个话题</h1><p class="lede">从内容列表打开话题后，基础稿和平台版本会显示在这里。</p><button class="primary-action" @click="goSection('topics')">返回内容</button>
      </section>
      <section v-else-if="section==='accounts'" class="page-section accounts-page">
        <header class="management-head">
          <div><h1>账号管理</h1></div>
          <details class="account-create-panel">
            <summary class="primary-action">＋ 连接账号</summary>
            <div class="account-create-content">
              <details class="inline-help platform-description"><summary>平台说明</summary><p class="settings-note">{{platformDescription}}</p></details>
              <form class="account-setup" @submit.prevent="addAccount"><div class="account-identity-grid"><label><span>账号名称</span><input v-model="accountName" placeholder="账号名称" :disabled="!selectedProfileID"></label><label><span>平台</span><select v-model="accountPlatform" @change="accountPlatformChanged" :disabled="!selectedProfileID"><option value="mastodon">Mastodon</option><option value="misskey">Misskey</option><option value="bluesky">Bluesky</option><option value="x">X</option><option value="weibo">微博</option><option value="xiaohongshu">小红书</option><option value="qzone">QQ 空间</option></select></label><label v-if="accountPlatform==='bluesky'"><span>远端身份</span><input v-model="remoteID" placeholder="handle / identifier"></label></div><label v-if="!isBridgePlatform(accountPlatform)" class="account-field"><span>实例地址</span><input v-model="baseURL" :placeholder="baseURLPlaceholder"></label><div v-if="accountPlatform==='mastodon' || accountPlatform==='misskey'" class="connection-inline"><button type="button" class="primary-action" @click="startConnection(accountPlatform)" :disabled="!selectedProfileID || !baseURL.trim()">{{accountPlatform==='mastodon'?'OAuth 自动连接':'MiAuth 自动连接'}}</button></div><div v-if="!isBridgePlatform(accountPlatform)" class="manual-account-row"><label class="account-field"><span>本地凭据</span><input v-model="accountSecret" type="password" autocomplete="new-password" placeholder="access token / app password"></label><details class="credential-advanced"><summary>使用环境变量</summary><input v-model="tokenEnv" :placeholder="tokenEnvPlaceholder"></details><button :disabled="!selectedProfileID">添加账号</button></div><button v-else class="primary-action" :disabled="!selectedProfileID">添加账号</button></form>
            </div>
          </details>
        </header>
        <div class="account-management-tools"><label for="account-profile">Profile</label><select id="account-profile" v-model="selectedProfileID"><option v-for="profile in profiles" :key="profile.id" :value="profile.id">{{profile.name}}</option></select></div>
        <div class="section-label"><span>已连接账号</span><small>{{profileAccounts.length}} 个</small></div><ul class="account-list account-grid"><li v-for="account in profileAccounts" :key="account.id" class="account-card"><div class="account-row-main"><span><strong>{{account.name}}</strong><small>{{platformLabel(account.platform)}}<template v-if="account.remote_id"> · {{account.remote_id}}</template></small></span><div class="row-actions"><button v-if="account.platform==='bluesky' || isMastodon(account)" type="button" @click="account.platform==='bluesky'?diagnoseBluesky(account):diagnoseMastodon(account)" :disabled="diagnosticIsBusy(account)">{{diagnosticIsBusy(account)?'检测中…':'测试连接'}}</button><IconButton icon="edit" label="编辑连接" @click="beginEditAccount(account)" /><IconButton icon="archive" label="停用账号并移除凭据" @click="removeAccount(account)" /></div></div><details v-if="editingAccountID===account.id" class="account-edit" open><summary>编辑连接信息</summary><form @submit.prevent="updateAccount(account)"><label>名称<input v-model="editAccountName"></label><label v-if="account.platform==='bluesky'">handle / DID<input v-model="editRemoteID"></label><label v-if="!isBridgePlatform(account.platform)">服务地址<input v-model="editBaseURL"></label><label v-if="!isBridgePlatform(account.platform)">新凭据<input v-model="editAccountSecret" type="password" autocomplete="new-password"></label><details v-if="!isBridgePlatform(account.platform)" class="credential-advanced"><summary>环境变量</summary><input v-model="editTokenEnv" placeholder="环境变量名"><small>保存后使用环境变量读取凭据。</small></details><div class="row-actions"><button type="submit" class="primary-action">保存</button><button type="button" @click="cancelEditAccount">取消</button></div></form></details><div v-if="account.platform==='bluesky' && blueskyDiagnostic[account.id]" class="diagnostic-result" :class="blueskyDiagnostic[account.id].ok?'is-ok':'is-error'">{{blueskyDiagnostic[account.id].message}}<small v-if="blueskyDiagnostic[account.id].hint">{{blueskyDiagnostic[account.id].hint}}</small></div><div v-if="isMastodon(account) && mastodonDiagnostic[account.id]" class="diagnostic-result" :class="mastodonDiagnostic[account.id].ok?'is-ok':'is-error'">{{mastodonDiagnostic[account.id].message}}</div></li></ul><p v-if="!profileAccounts.length" class="empty-state">还没有账号。</p>
        <div v-if="disabledProfileAccounts.length" class="disabled-accounts"><div class="section-label"><span>已停用账号</span></div><ul class="rows"><li v-for="account in disabledProfileAccounts" :key="account.id"><span>{{account.name}} · {{platformLabel(account.platform)}}</span><span class="row-actions"><IconButton icon="refresh" label="准备重新连接" @click="prepareReconnect(account)" /><IconButton icon="trash" label="永久删除本地账号数据" @click="permanentlyDeleteAccount(account)" /></span></li></ul></div>
      </section>
      <section v-else-if="section==='settings'" class="page-section settings-page">
        <h1>设置</h1>
        <section class="settings-block profile-settings" aria-labelledby="profile-settings-title"><header class="settings-block-head"><h2 id="profile-settings-title">Profile</h2></header><form @submit.prevent="addProfile"><input v-model="profileName" placeholder="新 Profile"><button class="primary-action">添加 Profile</button></form></section>
        <section class="settings-block translation-settings" aria-labelledby="translation-settings-title"><header class="settings-block-head"><h2 id="translation-settings-title">翻译 provider</h2></header><form class="actions" @submit.prevent="addTranslationConfig"><select v-model="translationProvider" aria-label="翻译 provider"><option value="openai">OpenAI-compatible</option><option value="deepl">DeepL</option><option value="google">Google Cloud</option><option value="google-gtx">Google translate_a</option></select><p class="settings-note platform-description">{{translationProviderDescription}}</p><input v-if="translationProvider!=='google-gtx'" v-model="translationCredentialEnv" placeholder="token/key 环境变量名"><input v-model="translationBaseURL" placeholder="自定义 API 地址（高级，可选）"><input v-if="translationProvider!=='google-gtx'" v-model="translationProjectOrModel" :placeholder="translationProvider==='google'?'Google project id':'可选模型名'"><button>保存翻译配置</button></form></section>
        <section class="settings-block bridge-settings" aria-labelledby="bridge-settings-title"><header class="settings-block-head"><h2 id="bridge-settings-title">浏览器桥接</h2></header><div class="tool-content"><div class="actions bridge-controls"><select v-model="bridgePlatform" aria-label="桥接平台"><option>x</option><option>weibo</option><option>xiaohongshu</option><option>qzone</option></select><button type="button" @click="createBridgePairing">生成配对 code</button><button type="button" @click="refreshBridgeState">刷新连接状态</button></div><ul v-if="bridgePairings.length" class="rows bridge-pairings"><li v-for="pairing in bridgePairings" :key="pairing.id"><strong>{{pairing.platform}}：{{pairingState(pairing)}}</strong><code v-if="pairing.code">{{pairing.code}}</code><button v-if="pairing.code" type="button" @click="copyPairingCode(pairing)">复制 code</button><button v-if="!pairing.revoked" type="button" @click="revokeBridgePairing(pairing)">撤销</button></li></ul><p v-else class="empty-state">尚未配对</p></div></section>
      </section>
      <section v-if="section==='history'" class="page-section history-page"><header><h1>发布中心</h1></header><PublicationList @notify="showToast" /></section>
    </main>
    <div v-if="mediaDialogOpen" class="modal-backdrop" role="presentation" @click.self="mediaDialogOpen=false"><section class="media-dialog" role="dialog" aria-modal="true" aria-labelledby="media-dialog-title"><header><h2 id="media-dialog-title">添加图片</h2><button type="button" class="icon-button" aria-label="关闭" title="关闭" @click="mediaDialogOpen=false">×</button></header><div class="media-dialog-fields"><label><span>适用范围</span><select v-model="mediaVariantID" aria-label="图片适用范围"><option value="">所有平台版本</option><option v-for="variant in editableVariants" :key="variant.id" :value="variant.id">仅 {{variantAccountLabel(variant)}}</option></select></label><label><span>替代文本</span><input v-model="altText" placeholder="描述图片内容"></label></div><footer><button type="button" @click="mediaDialogOpen=false">取消</button><label class="primary-action upload">选择图片<input type="file" accept="image/*" @change="uploadMedia"></label></footer></section></div>
    <div v-if="publishDialogOpen && activeVariant" class="modal-backdrop" role="presentation" @click.self="closePublishDialog"><section ref="publishDialogEl" class="publish-dialog" role="dialog" aria-modal="true" aria-labelledby="publish-dialog-title"><header><div><span class="eyebrow">最终确认</span><h2 id="publish-dialog-title">发布到 {{variantAccountLabel(activeVariant)}}</h2></div><button type="button" class="icon-button" aria-label="返回编辑" title="返回编辑" @click="closePublishDialog">×</button></header><div class="dialog-target"><strong>{{platformLabel(variantAccount(activeVariant)?.platform)}} · {{variantAccount(activeVariant)?.name}}</strong><span>{{variantRelationLabel(activeVariant)}} · {{activeVariant.visibility}}</span></div><article class="dialog-post-preview"><pre>{{activeVariant.text}}</pre><div v-if="currentMedia.length" class="dialog-media" :class="`media-count-${Math.min(currentMedia.length, 4)}`"><img v-for="item in currentMedia.slice(0, 4)" :key="item.id" :src="`/media/${item.id}`" :alt="item.alt_text || item.filename"><span v-if="currentMedia.length>4" class="media-more">+{{currentMedia.length-4}}</span></div><p v-else class="no-media">无媒体</p></article><label v-if="isBridgePlatform(variantAccount(activeVariant)?.platform || '')" class="dialog-auto-publish"><input v-model="bridgeAutoPublish" type="checkbox">本次请求自动发布</label><footer><button type="button" @click="closePublishDialog">返回编辑</button><button type="button" class="primary-action" :disabled="publishDialogBusy" @click="confirmPublish">{{publishDialogBusy?'处理中…':isBridgePlatform(variantAccount(activeVariant)?.platform||'')?'发送到平台页面':'确认发布'}}</button></footer></section></div>
  </div>
</template>
