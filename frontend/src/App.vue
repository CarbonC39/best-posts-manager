<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { api } from './api'
import { canAutoPublish } from './bridgeAdapters'
import PublicationList from './PublicationList.vue'

type Profile = { id: string; name: string }
type Account = { id: string; profile_id: string; platform: string; name: string; remote_id: string; base_url?: string; credential_ref?: string; disabled?: boolean }
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
const blueskyDiagnostic = ref<Record<string, BlueskyDiagnostic>>({})
const mastodonDiagnostic = ref<Record<string, MastodonDiagnostic>>({})
const diagnosticBusy = ref<Set<string>>(new Set())
const emojiFilter = ref('')
const emojiLimit = ref(50)
const emojiLoadedAccountID = ref('')
const emojiLoading = ref(false)
const emojiTargetAccountID = ref('')
const showArchivedTopics = ref(false)
const topicTitleEdit = ref('')
const externalImportAccountID = ref('')
const variantBaselines = ref<Record<string, string>>({})
const busyVariantIDs = ref<Set<string>>(new Set())
const baseURLPlaceholder = computed(() => accountPlatform.value === 'bluesky' ? 'Bluesky PDS URL' : '平台实例 URL')
const tokenEnvPlaceholder = computed(() => accountPlatform.value === 'bluesky' ? 'app password 环境变量名' : 'token 环境变量名')
const platformDescription = computed(() => ({
  mastodon: 'Mastodon-compatible：推荐 OAuth 自动连接；也可手动填写实例 URL 与 token 环境变量。',
  misskey: 'Misskey：推荐 MiAuth 自动连接；也可手动填写实例 URL 与 token 环境变量。',
  bluesky: 'Bluesky：填写 handle/identifier、PDS URL 与 app password 环境变量。',
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
const selectedCapability = computed(() => capabilities.value[selectedAccountID.value])
const editableVariants = computed(() => variants.value.filter((variant) => !variant.published))
const selectedAccount = computed(() => accounts.value.find((account) => account.id === selectedAccountID.value))
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
function variantBusy(variant: Variant) { return busyVariantIDs.value.has(variant.id) }
function diagnosticIsBusy(account: Account) { return diagnosticBusy.value.has(account.id) }
function setDiagnosticBusy(account: Account, busy: boolean) {
  const next = new Set(diagnosticBusy.value)
  if (busy) next.add(account.id); else next.delete(account.id)
  diagnosticBusy.value = next
}
function isMastodon(account?: Account) { return account?.platform === 'mastodon' || account?.platform === 'mastodon-compatible' }
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
function variantFormatLabel(variant: Variant) { return variant.format === 'mfm' ? 'MFM' : '纯文本' }
function variantRelationLabel(variant: Variant) { return variant.relation === 'linked' ? '跟随基础稿' : '独立版本' }
function variantStatusLabel(variant: Variant) { return variant.published ? '已有发布记录' : variant.status === 'ready' ? '可发布' : '草稿' }
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

function goSection(next: string) {
  if (section.value === 'editor' && next !== 'editor' && hasUnsavedChanges.value && !window.confirm('还有未保存的稿件修改，确定离开并丢弃这些修改吗？')) return
  section.value = next
}

function warnBeforeUnload(event: BeforeUnloadEvent) {
  if (!hasUnsavedChanges.value) return
  event.preventDefault()
  event.returnValue = ''
}

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
  if (!isBridgePlatform(accountPlatform.value) && !baseURL.value.trim()) { message.value = '请填写平台实例 URL'; return }
  if (!isBridgePlatform(accountPlatform.value) && !tokenEnv.value.trim()) { message.value = '手动添加账号需要填写 token/app password 环境变量名；也可以使用上方自动连接'; return }
  if (accountPlatform.value === 'bluesky' && !remoteID.value.trim()) { message.value = '请填写 Bluesky handle/identifier'; return }
  try {
    const created = await api<Account>('accounts', { method: 'POST', body: JSON.stringify({ profile_id: selectedProfileID.value, name: accountName.value, platform: accountPlatform.value, remote_id: remoteID.value, base_url: baseURL.value, token_env: tokenEnv.value }) })
    accountName.value = ''; baseURL.value = ''; tokenEnv.value = ''; remoteID.value = ''
    await load()
    if (!accounts.value.some((account) => account.id === created.id)) accounts.value.push(created)
    message.value = '账号已添加'
    if (created.platform === 'bluesky') await diagnoseBluesky(created)
    if (isMastodon(created)) await diagnoseMastodon(created)
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
      ? `${result.category ? 'Mastodon 检测完成（有警告）' : 'Mastodon 连接成功'}：@${result.username || account.name}（${result.elapsed_ms} ms）${result.hint ? `。${result.hint}` : ''}`
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
function prepareReconnect(account: Account) {
  accountName.value = account.name
  accountPlatform.value = account.platform
  remoteID.value = account.remote_id || ''
  baseURL.value = account.base_url || ''
  tokenEnv.value = ''
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

async function renameSelectedTopic() {
  if (!selected.value) return
  if (!topicTitleEdit.value.trim()) { message.value = '话题标题不能为空'; return }
  try {
    const updated = await api<Topic>(`topics/${selected.value.id}`, { method: 'PUT', body: JSON.stringify({ title: topicTitleEdit.value, status: selected.value.status }) })
    Object.assign(selected.value, updated)
    const listed = topics.value.find((topic) => topic.id === updated.id)
    if (listed) Object.assign(listed, updated)
    topicTitleEdit.value = updated.title
    message.value = '话题标题已更新'
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
    draft.value = loaded[0]; variants.value = listOrEmpty(loaded[1]); rememberVariants(variants.value); media.value = listOrEmpty(loaded[2]); externalPosts.value = listOrEmpty(loaded[3])
    text.value = draft.value.text
    topicTitleEdit.value = topic.title
    mediaVariantID.value = ''
    emojis.value = []
    emojiLoadedAccountID.value = ''
    emojiTargetAccountID.value = variants.value.find((variant) => variantCapability(variant)?.custom_emoji)?.account_id || ''
    emojiFilter.value = ''
    emojiLimit.value = 50
    translationSourceID.value = variants.value[0]?.id || ''
    translationTargetAccountID.value = profileAccounts.value[0]?.id || ''
    externalImportAccountID.value = profileAccounts.value.find((account) => account.platform === 'misskey' || account.platform === 'bluesky')?.id || ''
    section.value = 'editor'
  } catch (error) { showError(error) }
}

async function persistDraft(announce: boolean) {
  if (!selected.value) return false
  try {
    const changed = draftDirty.value
    draft.value = await api<Draft>(`topics/${selected.value.id}/draft`, { method: 'PUT', body: JSON.stringify({ text: text.value, format: 'plain', language: '' }) })
    if (changed) variants.value.forEach((variant) => { if (variant.relation === 'linked' && !variant.published) variant.outdated = true })
    if (announce) message.value = '基础稿已保存'
    return true
  } catch (error) { showError(error); return false }
}
async function saveDraft() { await persistDraft(true) }

async function newVariant() {
  if (!selected.value) return
  if (!selectedAccountID.value) { message.value = '请先添加并选择目标账号'; return }
  if (draftDirty.value && !await persistDraft(false)) return
  if (!await saveAllDirtyVariants()) return
  try {
    await api('variants', { method: 'POST', body: JSON.stringify({ topic_id: selected.value.id, account_id: selectedAccountID.value }) })
    variants.value = listOrEmpty(await api<Variant[]>(`variants?topic_id=${selected.value.id}`))
    rememberVariants(variants.value)
    translationSourceID.value ||= variants.value[0]?.id || ''
    message.value = '平台版本已生成'
  } catch (error) { showError(error) }
}

async function saveVariant(variant: Variant, announce = true) {
  try {
    Object.assign(variant, await api<Variant>(`variants/${variant.id}`, { method: 'PUT', body: JSON.stringify(variant) }))
    variantBaselines.value[variant.id] = variantSignature(variant)
    if (announce) message.value = '平台版本已保存并与基础稿分离'
    return true
  } catch (error) { showError(error); return false }
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
  if (variantBusy(variant)) return
  setVariantBusy(variant, true)
  if (variantDirty(variant) && !await saveVariant(variant, false)) { setVariantBusy(variant, false); return }
  try {
    await api('publications', { method: 'POST', body: JSON.stringify({ variant_id: variant.id }) })
    variant.published = true
    message.value = '发布完成（请查看历史）'
  } catch (error) { message.value = `发布失败：${error instanceof Error ? error.message : String(error)}` }
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
async function importExternal() { if (!selected.value) return; if (!externalImportAccountID.value || !externalReference.value.trim()) { message.value='请选择导入账号并填写帖子 ID/URL'; return }; try { const post=await api<ExternalPost>('external-posts/import',{method:'POST',body:JSON.stringify({topic_id:selected.value.id,account_id:externalImportAccountID.value,reference:externalReference.value})});externalPosts.value.unshift(post);externalReference.value='';message.value='已通过官方 API 导入快照'} catch(error){showError(error)} }
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
    delete variantBaselines.value[variant.id]
    if (mediaVariantID.value === variant.id) mediaVariantID.value = ''
    if (translationSourceID.value === variant.id) translationSourceID.value = variants.value[0]?.id || ''
    message.value = '平台版本已删除'
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
  if (variantBusy(variant)) return
  const account = accounts.value.find((item) => item.id === variant.account_id)
  if (autoPublish && !window.confirm(`自动填入并点击 ${account?.name || account?.platform || '平台'} 当前页面的可见发布按钮？带图片时会为避免漏图而退回人工发布；页面结构变化时也可能失败，请随后核对结果。`)) return
  setVariantBusy(variant, true)
  if (variantDirty(variant) && !await saveVariant(variant, false)) { setVariantBusy(variant, false); return }
  const pairing = connectedPairing(account?.platform)
  if (!pairing) {
    const authorized = bridgePairings.value.some((item) => item.code_used && !item.revoked && item.platform === account?.platform)
    bridgeMessage.value = authorized ? '已有授权，但没有检测到平台脚本近期心跳；请打开平台页并检查右下角 bridge 状态' : '请先完成匹配平台的配对；脚本会把当前页面安全回传给本地服务'
    setVariantBusy(variant, false)
    return
  }
  try {
    bridgeRequest.value = await api<BridgeRequest>('bridge/requests/fill', { method: 'POST', body: JSON.stringify({ pairing_id: pairing.id, variant_id: variant.id, page_url: pairing.page_url || '', auto_publish: autoPublish }) })
    variant.published = true
    bridgeMessage.value = autoPublish ? '已发送自动发布请求；脚本只会点击已知、可见且启用的发布按钮，请随后核对平台结果' : '已发送，等待脚本领取；填入后请人工检查并点击平台发布'
  } catch (error) { showError(error) }
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
    variants.value.push(result.variant); variantBaselines.value[result.variant.id] = variantSignature(result.variant); translationMessage.value = '已生成新的 detached 翻译版本，原文未修改'
  } catch (error) { translationMessage.value = `翻译失败：${error instanceof Error ? error.message : String(error)}；原文与目标版本均未改动` }
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
  bridgeRefreshTimer = window.setInterval(() => { if (section.value === 'editor') void syncBridgeState(false) }, 3_000)
})
onBeforeUnmount(() => {
  window.removeEventListener('beforeunload', warnBeforeUnload)
  if (bridgeRefreshTimer !== undefined) window.clearInterval(bridgeRefreshTimer)
})
</script>

<template>
  <div class="shell">
    <aside class="sidebar">
      <div class="brand"><span class="brand-mark">↗</span><div><strong>Best Post</strong><small>Manager</small></div></div>
      <nav aria-label="主导航">
        <button :class="{active:section==='topics'}" @click="goSection('topics')"><span>⌂</span> 内容</button>
        <button :class="{active:section==='editor'}" @click="goSection('editor')"><span>✎</span> 当前编辑</button>
        <button :class="{active:section==='history'}" @click="goSection('history')"><span>↗</span> 发布记录</button>
        <button :class="{active:section==='settings'}" @click="goSection('settings')"><span>⚙</span> 设置</button>
      </nav>
      <div class="local-status"><span class="status-dot"></span><div><strong>本地单用户</strong><small>数据保存在本机</small></div></div>
    </aside>
    <main class="canvas">
      <div v-if="message" class="notice">{{message}}</div>
      <section v-if="section==='topics'" class="page-section">
        <div class="eyebrow">工作区</div><h1>内容</h1><p class="lede">把想法放进话题，基础稿会在这里保持清晰。</p>
        <div class="profile-picker"><label for="content-profile">当前 Profile</label><select id="content-profile" v-model="selectedProfileID"><option v-for="profile in profiles" :key="profile.id" :value="profile.id">{{profile.name}}</option></select><button class="text-button" @click="goSection('settings')">管理 Profile 与账号 →</button></div>
        <form class="topic-create" @submit.prevent="addTopic"><input v-model="topicTitle" placeholder="新话题标题" :disabled="!selectedProfileID"><button class="primary-action" :disabled="!selectedProfileID">创建话题</button></form>
        <div class="section-label"><span>话题</span><small>{{profileTopics.length}} 个</small><label v-if="topics.some(topic=>topic.profile_id===selectedProfileID&&topic.status==='archived')" class="inline-check"><input type="checkbox" v-model="showArchivedTopics">显示已归档</label></div>
        <p v-if="profiles.length===0" class="empty-state">还没有 Profile。前往设置创建后，就可以开始第一个话题。</p><p v-else-if="profileTopics.length===0" class="empty-state">这个 Profile 还没有话题，从上方写下第一个标题吧。</p><ul v-else class="rows topic-list"><li v-for="topic in profileTopics" :key="topic.id"><button class="link" @click="openTopic(topic)">{{topic.title}}</button><span>{{topic.status==='archived'?'已归档':'进行中'}}</span><button class="text-button" @click="setTopicStatus(topic, topic.status==='archived'?'active':'archived')">{{topic.status==='archived'?'恢复':'归档'}}</button></li></ul>
      </section>
      <section v-else-if="section==='editor' && selected" class="page-section editor-page">
        <button class="back" @click="goSection('topics')">← 返回内容</button><div class="eyebrow">正在编辑</div><div class="topic-title-editor"><input v-model="topicTitleEdit" aria-label="话题标题"><button :disabled="!topicTitleDirty" @click="renameSelectedTopic">重命名</button></div><p class="lede">基础稿是源头；平台版本只在你确认后更新。<span class="help-tip" tabindex="0" role="img" aria-label="平台版本不会随基础稿静默覆盖，需主动应用更新或另建版本"><span aria-hidden="true">i</span><span class="help-tip-popup" role="tooltip">平台版本不会随基础稿静默覆盖，需主动应用更新或另建版本。</span></span></p>
        <textarea class="draft-editor" v-model="text" placeholder="从这里开始写…"></textarea>
        <div class="actions editor-toolbar"><button class="primary-action" @click="saveDraft">{{draftDirty?'保存基础稿（有修改）':'保存基础稿'}}</button><select v-model="selectedAccountID" aria-label="目标账号"><option v-for="account in profileAccounts" :key="account.id" :value="account.id">{{account.name}} ({{account.platform}})</option></select><button @click="newVariant">从基础稿生成平台版本</button></div>
        <div class="media-tools"><div class="section-label"><span>媒体</span><small>默认用于所有平台版本，图片只存储在本机</small></div><div class="actions media-controls"><select v-model="mediaVariantID" aria-label="图片适用范围"><option value="">所有平台版本</option><option v-for="variant in editableVariants" :key="variant.id" :value="variant.id">仅 {{variantAccountLabel(variant)}}</option></select><input v-model="altText" placeholder="图片替代文本"><label class="upload">添加图片<input type="file" accept="image/*" @change="uploadMedia"></label></div><ul v-if="media.length" class="media-list"><li v-for="item in media" :key="item.id"><img :src="`/media/${item.id}`" :alt="item.alt_text||item.filename"><span :title="item.filename">{{item.filename}}</span><small>{{mediaScopeLabel(item)}}</small><button class="text-button" @click="deleteMedia(item)">删除</button></li></ul></div>
        <div class="platform-heading"><div><div class="section-label"><span>平台版本</span><small>{{variants.length}} 个版本</small><span class="help-tip" tabindex="0" role="img" aria-label="平台版本说明"><span aria-hidden="true">i</span><span class="help-tip-popup" role="tooltip">编辑控件由账号能力决定；Misskey 保留 MFM，Bluesky 使用纯文本与 facets。{{selectedCapability?.visibility_note || ''}}</span></span></div></div></div>
        <p v-if="variants.length===0" class="empty-state">还没有平台版本；选择目标账号后从基础稿生成。</p><ul v-else class="variants"><li v-for="variant in variants" :key="variant.id" class="variant-card"><header class="variant-header"><div><strong>{{variantAccountLabel(variant)}}</strong><span>{{variantFormatLabel(variant)}} · {{variantRelationLabel(variant)}} · {{variantStatusLabel(variant)}}<b v-if="variantDirty(variant)" class="unsaved-mark"> · 未保存</b></span></div><em v-if="variant.outdated">基础稿有更新</em></header><textarea class="variant-editor" v-model="variant.text" rows="5" :disabled="variant.published"></textarea><div class="variant-options"><small v-if="variantCapability(variant)?.text_max_bytes">UTF-8 {{utf8Bytes(variant.text)}} / {{variantCapability(variant)?.text_max_bytes}} bytes</small><label><span>可见范围</span><select v-model="variant.visibility" :disabled="variant.published"><option v-for="visibility in variantCapability(variant)?.visibilities || []" :key="visibility">{{visibility}}</option></select></label><input v-if="variantCapability(variant)?.supports_cw" v-model="variant.spoiler_text" placeholder="内容警告（可选）" :disabled="variant.published"><label v-if="variantCapability(variant)?.supports_local_only" class="checkbox-option"><input type="checkbox" v-model="variant.local_only" :disabled="variant.published">仅本地</label></div><div v-if="variantCapability(variant)?.format==='mfm'" class="mfm-preview"><small>原始 MFM 预览（本地不渲染）</small><pre class="preview">{{variant.text}}</pre></div><footer class="variant-actions"><button v-if="!variant.published" class="primary-action" :disabled="variantBusy(variant)" @click="saveVariant(variant)">{{variantBusy(variant)?'处理中…':'保存版本'}}</button><button v-if="variant.outdated&&!variant.published" :disabled="variantBusy(variant)" @click="applyVariant(variant)">应用基础稿更新</button><button v-if="!variant.published&&!variantAccount(variant)?.disabled && !isBridgePlatform(variantAccount(variant)?.platform||'')" :disabled="variantBusy(variant)" @click="publish(variant)">直接发布</button><template v-else-if="!variant.published&&!variantAccount(variant)?.disabled"><button :disabled="variantBusy(variant)" @click="fillBrowser(variant)">仅填入</button><button v-if="canAutoPublish(variantAccount(variant)?.platform||'')" :disabled="variantBusy(variant)" @click="fillBrowser(variant, true)">填入并自动发布</button></template><small v-else-if="variantAccount(variant)?.disabled" class="disabled-account">账号已停用，历史版本仅供查看</small><button v-if="variant.published" :disabled="variantBusy(variant)" @click="cloneVariant(variant)">复制为新版本</button><button v-if="!variant.published&&!variantAccount(variant)?.disabled" class="danger-text" :disabled="variantBusy(variant)" @click="deleteVariant(variant)">删除版本</button></footer></li></ul>
        <div class="bridge-status-panel" aria-label="浏览器桥接状态"><div class="section-label"><span>浏览器填入</span><small>发布入口保持可见</small><span class="help-tip" tabindex="0" role="img" aria-label="浏览器桥接帮助"><span aria-hidden="true">i</span><span class="help-tip-popup" role="tooltip">脚本只处理你当前已登录且可见的页面；自动发布必须按次确认。</span></span></div><details class="inline-help"><summary>查看配对与发布说明</summary><p class="bridge-steps">每个平台首次安装脚本并输入一次 code；此后配对会跨刷新和重启恢复。所有平台支持安全填入；只有已验收发布控件的平台才显示单次自动发布，带图时仍退回人工发布。</p></details><p class="settings-note">先<a href="/userscript/bpm-bridge.user.js">安装或更新本地用户脚本</a>。脚本只处理当前可见页面；自动发布默认关闭。</p><div class="actions bridge-controls"><select v-model="bridgePlatform"><option>x</option><option>weibo</option><option>xiaohongshu</option><option>qzone</option></select><button class="primary-action" @click="createBridgePairing">生成/重置配对 code</button><button @click="refreshBridgeState">刷新连接状态</button><button @click="requestCapture">捕获当前可见帖子</button></div><p v-if="bridgeMessage" class="notice">{{bridgeMessage}}</p><div v-if="bridgePairings.length===0" class="bridge-state">未安装/未配对：安装脚本后生成配对。</div><ul v-else class="rows bridge-pairings"><li v-for="pairing in bridgePairings" :key="pairing.id"><div><strong>{{pairing.platform}}：{{pairingState(pairing)}}</strong><code v-if="pairing.code">{{pairing.code}}</code><details v-if="pairing.page_url || displayableTime(pairing.last_seen_at) || pairing.expires_at" class="pairing-details"><summary>连接详情</summary><small v-if="pairing.page_url">当前页：{{pairing.page_url}}</small><small v-if="displayableTime(pairing.last_seen_at)">最后心跳：{{new Date(pairing.last_seen_at).toLocaleString()}}</small><small v-if="pairing.expires_at">有效期至：{{new Date(pairing.expires_at).toLocaleString()}}</small></details></div><div class="row-actions"><button v-if="pairing.code" @click="copyPairingCode(pairing)">复制 code</button><button v-if="!pairing.revoked" @click="revokeBridgePairing(pairing)">撤销</button></div></li></ul><p v-if="bridgeRequest" class="bridge-request-status">最近请求：{{bridgeRequest.kind}} · {{bridgeRequestState(bridgeRequest)}}<span v-if="bridgeRequest.warning"> · 提示：{{bridgeRequest.warning}}</span><span v-if="bridgeRequest.error"> · 错误：{{bridgeRequest.error}}</span></p></div>
        <details v-if="variants.some(variant=>variantCapability(variant)?.custom_emoji)" class="tool-section emoji-section"><summary>实例 emoji 辅助</summary><div class="tool-content"><div class="actions emoji-controls"><select v-model="emojiTargetAccountID" aria-label="emoji 所属账号"><option v-for="variant in variants.filter(item=>variantCapability(item)?.custom_emoji && variants.find(candidate=>candidate.account_id===item.account_id)?.id===item.id)" :key="variant.account_id" :value="variant.account_id">{{variantAccountLabel(variant)}}</option></select><button class="primary-action" @click="loadEmoji(emojiTargetAccountID)" :disabled="!emojiTargetAccountID || emojiLoading || emojiLoadedAccountID===emojiTargetAccountID">{{emojiLoading?'加载中…':emojiLoadedAccountID===emojiTargetAccountID?'已加载':'按需加载实例 emoji'}}</button></div><div v-if="emojiLoadedAccountID" class="emoji-browser"><input v-model="emojiFilter" placeholder="筛选 shortcode"><small>共 {{emojis.length}} 个，匹配 {{matchingEmojis.length}} 个，当前渲染 {{visibleEmojis.length}} 个</small><div class="emoji-list"><span v-for="emoji in visibleEmojis" :key="emoji.shortcode" class="emoji"><img loading="lazy" :src="emoji.static_url||emoji.url" alt="">:{{emoji.shortcode}}:</span></div><button v-if="hasMoreEmojis" @click="showMoreEmoji">继续显示 50 个</button></div></div></details>
        <details class="tool-section"><summary>发布与导入工具</summary><div class="tool-content"><h3>浏览器桥接 v1</h3><small>默认仅填入；只有你对某次请求选择“填入并自动发布”后，脚本才会点击明确匹配且可见、启用的发布按钮。页面结构不匹配时停止并提示人工处理。</small>
          <h3>站外关联</h3><p class="settings-note">用户脚本安装/更新：<a href="/userscript/bpm-bridge.user.js">打开当前本地脚本</a>（同源地址，不会触发 file:///）。安装到油猴后，在已登录的平台页面使用配对功能。</p><form @submit.prevent="markExternal"><select v-model="externalPlatform"><option>mastodon</option><option>misskey</option><option>bluesky</option><option>x</option><option>weibo</option><option>xiaohongshu</option><option>qzone</option></select><select v-model="externalAccountID"><option value="">不关联账号</option><option v-for="account in externalPlatformAccounts" :key="account.id" :value="account.id">{{account.name}}</option></select><input v-model="externalURL" placeholder="仅标记 URL，不抓取"><button>标记 URL</button></form><form @submit.prevent="importExternal"><select v-model="externalImportAccountID" aria-label="官方导入账号"><option value="">选择 Misskey / Bluesky 账号</option><option v-for="account in profileAccounts.filter(item=>item.platform==='misskey'||item.platform==='bluesky')" :key="account.id" :value="account.id">{{account.name}} · {{account.platform}}</option></select><input v-model="externalReference" placeholder="帖子 ID、URL 或 at://"><button :disabled="!canImportOfficially">官方 API 导入快照</button></form><ul class="rows external-post-list"><li v-for="post in externalPosts" :key="post.id"><span>{{post.platform}}<em v-if="post.incomplete">不完整：{{post.incomplete_reason}}</em></span><a :href="post.url" target="_blank" rel="noreferrer">{{post.url}}</a><small v-if="post.text_snapshot">{{post.text_snapshot.slice(0,120)}}</small><button class="danger-text" @click="deleteExternalPost(post)">移除本地关联</button></li></ul></div></details>
        <details class="tool-section"><summary>翻译工具</summary><div class="tool-content"><small>翻译只生成新的 detached Variant，不覆盖源版本；官方 provider 凭据从本地凭据或环境变量读取。</small><div class="actions"><select v-model="translationSourceID"><option v-for="variant in variants" :key="variant.id" :value="variant.id">{{variantAccountLabel(variant)}} · {{variant.text.slice(0,30)||'空内容'}}</option></select><select v-model="translationTargetAccountID"><option v-for="account in profileAccounts" :key="account.id" :value="account.id">目标：{{account.name}}</option></select><input v-model="translationSourceLanguage" placeholder="源语言（可留空自动识别）"><input v-model="translationTargetLanguage" placeholder="目标语言，如 zh"><select v-model="translationConfigID"><option value="">选择翻译配置</option><option v-for="config in translationConfigs" :key="config.id" :value="config.id">{{config.provider}} · {{config.model || config.project_id || config.id}}</option></select><button @click="translateVariant" :disabled="!translationConfigID">生成翻译版本</button></div><p v-if="translationMessage" class="notice">{{translationMessage}}</p></div></details>
      </section>
      <section v-else-if="section==='editor'" class="page-section empty-editor">
        <div class="eyebrow">正在编辑</div><h1>先选择一个话题</h1><p class="lede">从内容列表打开话题后，基础稿和平台版本会显示在这里。</p><button class="primary-action" @click="goSection('topics')">返回内容</button>
      </section>
      <section v-else-if="section==='settings'" class="page-section settings-page">
        <div class="eyebrow">偏好与连接</div><h1>设置</h1><p class="lede">Profile、账号和本地工具配置。</p>
        <div class="section-label"><span>Profile</span><small>内容分组</small></div><form @submit.prevent="addProfile"><input v-model="profileName" placeholder="新 Profile"><button class="primary-action">添加 Profile</button></form>
        <div class="section-label"><span>平台账号</span><small>自动连接存本地凭据；手动模式引用环境变量</small></div>
        <form class="account-setup" @submit.prevent="addAccount">
          <div class="account-identity-grid">
            <label><span>内容分组</span><select v-model="selectedProfileID"><option v-for="profile in profiles" :key="profile.id" :value="profile.id">{{profile.name}}</option></select></label>
            <label><span>账号名称</span><input v-model="accountName" placeholder="账号名称" :disabled="!selectedProfileID"></label>
            <label><span>平台</span><select v-model="accountPlatform" :disabled="!selectedProfileID"><option value="mastodon">Mastodon-compatible</option><option value="misskey">Misskey</option><option value="bluesky">Bluesky</option><option value="x">X</option><option value="weibo">微博</option><option value="xiaohongshu">小红书</option><option value="qzone">QQ空间</option></select></label>
          </div>
          <details class="inline-help platform-description"><summary>平台说明</summary><p class="settings-note">{{platformDescription}}</p></details>
          <label v-if="accountPlatform==='bluesky'" class="account-field"><span>远端身份</span><input v-model="remoteID" placeholder="Bluesky handle/identifier" :disabled="!selectedProfileID"></label>
          <label v-if="!isBridgePlatform(accountPlatform)" class="account-field"><span>实例地址</span><input v-model="baseURL" :placeholder="baseURLPlaceholder" :disabled="!selectedProfileID"></label>
          <div v-if="accountPlatform==='mastodon' || accountPlatform==='misskey'" class="connection-inline"><button type="button" class="primary-action" @click="startConnection(accountPlatform)" :disabled="!selectedProfileID || !baseURL.trim()">{{accountPlatform==='mastodon'?'OAuth 自动连接':'MiAuth 自动连接'}}</button><small>推荐。授权在当前回环服务完成，成功后自动添加账号。</small></div>
          <p v-if="connectionMessage" class="notice">{{connectionMessage}}</p>
          <div v-if="!isBridgePlatform(accountPlatform)" class="manual-account-row"><label class="account-field"><span>环境变量名</span><input v-model="tokenEnv" :placeholder="tokenEnvPlaceholder" :disabled="!selectedProfileID"></label><button :disabled="!selectedProfileID">手动添加</button></div>
          <button v-else class="primary-action" :disabled="!selectedProfileID">添加桥接账号</button>
        </form>
        <ul class="rows account-list"><li v-for="account in profileAccounts" :key="account.id"><div class="account-row-main"><span>{{account.name}} · {{account.platform}}</span><div class="row-actions"><button v-if="account.platform==='bluesky'" type="button" @click="diagnoseBluesky(account)" :disabled="diagnosticIsBusy(account)">{{diagnosticIsBusy(account)?'检测中…':'测试连接'}}</button><button v-if="isMastodon(account)" type="button" @click="diagnoseMastodon(account)" :disabled="diagnosticIsBusy(account)">{{diagnosticIsBusy(account)?'检测中…':'测试连接'}}</button><button type="button" @click="removeAccount(account)">停用并移除凭据</button></div></div><details v-if="account.platform==='bluesky'" class="account-diagnostic"><summary>Bluesky 连接诊断</summary><p v-if="!blueskyDiagnostic[account.id]" class="settings-note">尚未检测。测试只会创建会话，不会发布内容；凭据不会返回或写入日志。</p><div v-else :class="['diagnostic-result', blueskyDiagnostic[account.id].ok?'is-ok':'is-error']"><strong>{{blueskyDiagnostic[account.id].message}}</strong><small>阶段：{{blueskyDiagnostic[account.id].stage}} · 分类：{{blueskyDiagnostic[account.id].category || 'ok'}} · {{blueskyDiagnostic[account.id].elapsed_ms}} ms</small><small v-if="blueskyDiagnostic[account.id].handle">身份：@{{blueskyDiagnostic[account.id].handle}} · {{blueskyDiagnostic[account.id].did}}</small><small v-if="blueskyDiagnostic[account.id].hint">建议：{{blueskyDiagnostic[account.id].hint}}</small></div></details><details v-else-if="isMastodon(account)" class="account-diagnostic"><summary>Mastodon 连接诊断</summary><p v-if="!mastodonDiagnostic[account.id]" class="settings-note">尚未检测。不会发布动态或上传媒体；OPTIONS 只检查端点兼容性，不能证明写入 scope。</p><div v-else :class="['diagnostic-result', mastodonDiagnostic[account.id].ok?'is-ok':'is-error']"><strong>{{mastodonDiagnostic[account.id].message}}</strong><small>阶段：{{mastodonDiagnostic[account.id].stage}} · 分类：{{mastodonDiagnostic[account.id].category || 'ok'}} · {{mastodonDiagnostic[account.id].elapsed_ms}} ms</small><small v-if="mastodonDiagnostic[account.id].username">身份：@{{mastodonDiagnostic[account.id].username}} · {{mastodonDiagnostic[account.id].account_id}}</small><small>发帖端点：{{mastodonDiagnostic[account.id].status_write.message}}</small><small>媒体端点：{{mastodonDiagnostic[account.id].media_write.message}}</small><small v-if="mastodonDiagnostic[account.id].hint">建议：{{mastodonDiagnostic[account.id].hint}}</small></div></details></li></ul>
        <div v-if="disabledProfileAccounts.length" class="disabled-accounts"><small>已停用账号（历史仍保留）</small><ul class="rows account-list"><li v-for="account in disabledProfileAccounts" :key="account.id"><span>{{account.name}} · {{account.platform}}</span><button type="button" @click="prepareReconnect(account)">准备重新连接</button></li></ul></div><div class="section-label"><span>翻译 provider</span><small>配置不包含 key/token</small></div><form class="actions" @submit.prevent="addTranslationConfig"><select v-model="translationProvider"><option value="openai">OpenAI-compatible</option><option value="deepl">DeepL</option><option value="google">Google Cloud</option><option value="google-gtx">Google translate_a（非官方）</option></select><p class="settings-note platform-description">{{translationProviderDescription}}</p><input v-if="translationProvider!=='google-gtx'" v-model="translationCredentialEnv" placeholder="token/key 环境变量名"><input v-model="translationBaseURL" placeholder="自定义 API 地址（高级，可选）"><input v-if="translationProvider!=='google-gtx'" v-model="translationProjectOrModel" :placeholder="translationProvider==='google'?'Google project id':'可选模型名'"><button>保存翻译配置</button></form><p v-if="translationMessage" class="notice">{{translationMessage}}</p>
        <details class="tool-section backup-section"><summary>备份与凭据 <small>离线可恢复</small></summary><p class="settings-note">用户脚本安装/更新：<a href="/userscript/bpm-bridge.user.js">打开当前同源脚本</a>。每个平台首次输入一次 code，之后保存在油猴隔离存储；可在悬浮窗断开或从 WebUI 撤销。使用 CLI 执行 <code>bpm backup create backup.zip</code> 和 <code>bpm backup restore backup.zip 空目录</code>。备份包含 SQLite、本地图片和校验清单，不包含环境变量值或其他明文凭据。</p></details>
      </section>
      <section v-else class="page-section history-page"><div class="eyebrow">历史</div><h1>发布记录</h1><p class="lede">每次发布的结果和可恢复状态都在这里。</p><PublicationList/></section>
    </main>
  </div>
</template>
