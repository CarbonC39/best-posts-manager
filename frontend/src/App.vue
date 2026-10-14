<script setup lang="ts">
import { computed, customRef, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Archive, ArrowRight, Check, ChevronDown, ChevronLeft, ChevronRight, Circle, CircleAlert, CircleCheck, Clock3, Copy, Download, Ellipsis, Eye, FileText, Hash, Image, KeyRound, Languages, Link2, Minus, Pencil, Plus, Power, RefreshCw, Save, ScrollText, Search, Send, Settings, Smile, Trash2, Triangle, UserRound, X } from '@lucide/vue'
import Sortable, { type SortableEvent } from 'sortablejs'
import { api } from './api'
import { usePublicationStatus } from './composables/usePublicationStatus'
import { useAccountManagement } from './composables/useAccountManagement'
import { accountBelongsToProfile, isMastodon, type Account } from './domain/account'
import { canAutoPublish, isBridgePlatform, platformLabel } from './platformRegistry'
import IconButton from './IconButton.vue'
import PlatformIcon from './PlatformIcon.vue'
import PostPreviewDialog from './PostPreviewDialog.vue'
import PublicationCenterView from './views/PublicationCenterView.vue'
import TopicsView from './views/TopicsView.vue'
import { vDialogDrag } from './directives/dialogDrag'

type Profile = { id: string; name: string }
type Topic = { id: string; profile_id: string; title: string; status: string; updated_at?: string; target_count?: number; published_count?: number; needs_action_count?: number; in_progress_count?: number; platforms?: string[] }
type Draft = { topic_id: string; text: string; format: string; revision: number }
type Variant = { id: string; topic_id: string; account_id: string; text: string; format: string; status: string; relation: string; source_revision?: string; visibility: string; spoiler_text: string; local_only: boolean; outdated: boolean; published?: boolean; publication_locked?: boolean; publication_status?: string }
type Media = { id: string; topic_id?: string; variant_id: string; filename: string; mime?: string; alt_text: string }
type ExternalPost = { id: string; platform: string; url: string; text_snapshot: string; incomplete: boolean; incomplete_reason: string }
type Emoji = { shortcode: string; url: string; static_url: string }
type BridgePairing = { id: string; platform: string; code?: string; expires_at: string; code_used: boolean; revoked: boolean; connected?: boolean; page_url?: string; last_seen_at?: string }
type BridgeRequest = { id: string; kind: string; status: string; warning?: string; error?: string; external_post_id?: string }
type Capability = { platform: string; format: string; text_max_bytes: number; max_media: number; max_media_bytes: number; visibilities: string[]; supports_cw: boolean; supports_local_only: boolean; custom_emoji: boolean; visibility_note?: string }
type TranslationConfig = { id: string; provider: string; base_url: string; project_id: string; location: string; token_env: string; key_env: string; model: string }
function listOrEmpty<T>(value: T[] | null | undefined): T[] { return Array.isArray(value) ? value : [] }
function notificationRef() {
  let value = ''
  return customRef<string>((track, trigger) => ({
    get() { track(); return value },
    set(next) { value = next; trigger(); if (next && notificationKind(next) !== 'success') showToast(next); else if (next && toast.value.kind === 'error') dismissToast() },
  }))
}
function notificationKind(text: string): 'success' | 'error' | 'warning' {
  if (/失败|错误|不可用|拒绝|无法/.test(text)) return 'error'
  if (/^(请|需要|先)|不能为空|只能选择|没有检测到|缺少|警告|建议|注意/.test(text)) return 'warning'
  return 'success'
}

const section = ref('editor')
const sidebarCollapsed = ref(typeof window !== 'undefined' && window.innerWidth < 900)
const mobileViewport = ref(typeof window !== 'undefined' && window.innerWidth <= 640)
const mobileDrawerOpen = ref(false)
const mobileMenuButton = ref<HTMLButtonElement | null>(null)
const mobileMenuPosition = ref({ top: 60, left: 12 })
const workspaceModal = ref<'' | 'accounts' | 'settings' | 'history'>('')
const workspaceReturnSection = ref('topics')
const workspaceModalReturnFocus = ref<HTMLElement | null>(null)
const targetAccountSearch = ref('')
const targetSelectedAccountID = ref('')
const targetAddPosition = ref({ top: 0, left: 0 })
let targetPickerReturnFocus: HTMLElement | null = null
const pendingDraftAction = ref<'' | 'publish' | 'translate'>('')
const profiles = ref<Profile[]>([])
const accounts = ref<Account[]>([])
const topics = ref<Topic[]>([])
const variants = ref<Variant[]>([])
const media = ref<Media[]>([])
const mediaOrder = ref<string[]>([])
const mediaListEl = ref<HTMLElement | null>(null)
const externalPosts = ref<ExternalPost[]>([])
const emojis = ref<Emoji[]>([])
const capabilities = ref<Record<string, Capability>>({})
const draft = ref<Draft | null>(null)
const selected = ref<Topic | null>(null)
const newTopicMode = ref(true)
const newTopicBusy = ref(false)
let newTopicCreatePromise: Promise<Topic | null> | null = null
const message = notificationRef()
const toast = ref<{ text: string; kind: 'success' | 'error' | 'warning' }>({ text: '', kind: 'success' })
let toastTimer: number | undefined
const selectedProfileID = ref('')
const selectedAccountID = ref('')
const pendingTargetAccountID = ref('')
const mediaVariantID = ref('')
const profileName = ref('')
const profileCreateOpen = ref(false)
const expandedProfileID = ref('')
const expandedAccountID = ref('')
const accountLinkProfileID = ref('')
const accountLinkBusy = ref(false)
const topicTitle = ref('')
const text = ref('')
const altText = ref('')
const pendingMediaFile = ref<File | null>(null)
const pendingMediaAltText = ref('')
const externalURL = ref('')
const externalReference = ref('')
const externalPlatform = ref('mastodon')
const externalAccountID = ref('')
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
const emojiFilter = ref('')
const emojiLimit = ref(50)
const emojiLoadedAccountID = ref('')
const emojiLoading = ref(false)
const emojiTargetAccountID = ref('')
const variantSearch = ref('')
const activeVariantID = ref('')
const requestedDocumentID = ref('')
let documentNavigation: Promise<boolean> | null = null
const draftEditor = ref<HTMLTextAreaElement | null>(null)
const draftPublishBusy = ref(false)
const unicodeEmoji = ['😀', '🙂', '😂', '🥹', '😍', '🤔', '🙌', '👏', '✨', '🎉', '❤️', '👍', '🙏', '☀️', '🌿', '🐈', '☕', '📷', '💡', '💙']
const showArchivedTopics = ref(false)
const topicTitleEdit = ref('')
const externalImportAccountID = ref('')
const variantBaselines = ref<Record<string, string>>({})
const busyVariantIDs = ref<Set<string>>(new Set())
const topicSearch = ref('')
const openTopicMenuID = ref('')
const topicMenuPosition = ref({ left: 0, top: 0 })
const sidebarTool = ref<'create' | 'profile' | 'search'>('create')
function translationProviderName(provider: string) {
  return ({ openai: 'OpenAI-compatible', deepl: 'DeepL', google: 'Google Cloud', 'google-gtx': 'Google 翻译' } as Record<string, string>)[provider] || provider
}
const showTargetAdd = ref(false)
const showDocumentPicker = ref(false)
const editorTool = ref<'' | 'hashtag' | 'emoji' | 'translation' | 'external'>('')
const hashtagText = ref('')
let editorToolReturnFocus: HTMLElement | null = null
const mediaDialogOpen = ref(false)
const mediaFileInput = ref<HTMLInputElement | null>(null)
let mediaDialogReturnFocus: HTMLElement | null = null
const mediaAltDialogOpen = ref(false)
const mediaAltTarget = ref<Media | null>(null)
const mediaAltIsPending = ref(false)
const mediaAltDraft = ref('')
const mediaAltError = ref('')
const mediaAltTextarea = ref<HTMLTextAreaElement | null>(null)
const mediaAltReturnFocus = ref<HTMLElement | null>(null)
let mediaSortable: Sortable | undefined
let mediaOrderRequestID = 0
let mediaOrderBusy = false
const postPreviewOpen = ref(false)
const translationDialogOpen = ref(false)
const translationBusy = ref(false)
const translationDialogEl = ref<HTMLElement | null>(null)
let translationReturnFocus: HTMLElement | null = null
let modalPreviousBodyOverflow = ''
let modalBodyScrollLocked = false
const publishDialogOpen = ref(false)
const publishDialogBusy = ref(false)
const publishSelectedIDs = ref<string[]>([])
const publishAutoIDs = ref<string[]>([])
const publishTask = ref<Array<{ id: string; label: string; state: 'working' | 'done' | 'waiting' | 'failed'; detail?: string }>>([])
watch([mediaAltDialogOpen, mediaDialogOpen, workspaceModal, publishDialogOpen, postPreviewOpen, translationDialogOpen, showTargetAdd, showDocumentPicker], (states) => {
  if (states.some(Boolean)) closeTopicMenu()
})
watch([workspaceModal, mediaDialogOpen, mediaAltDialogOpen, publishDialogOpen, postPreviewOpen], (states) => {
  if (states.some(Boolean)) {
    if (!modalBodyScrollLocked) {
      modalPreviousBodyOverflow = document.body.style.overflow
      modalBodyScrollLocked = true
    }
    document.body.style.overflow = 'hidden'
  } else if (modalBodyScrollLocked) {
    document.body.style.overflow = modalPreviousBodyOverflow
    modalBodyScrollLocked = false
  }
})
const { summary: publicationSummary, hasFailures: publicationHasFailures, refresh: refreshPublicationSummary } = usePublicationStatus(selectedProfileID)
const {
  accountName, accountCreateOpen, baseURL, tokenEnv, accountSecret, remoteID, accountPlatform,
  editingAccountID, editAccountName, editBaseURL, editRemoteID, editTokenEnv, editAccountSecret,
  blueskyDiagnostic, mastodonDiagnostic,
  profileAccounts,
  accountPlatformChanged, diagnosticIsBusy, addAccount, attachAccount, beginEditAccount, cancelEditAccount,
  updateAccount, diagnoseBluesky, diagnoseMastodon, removeAccount, permanentlyDeleteAccount, prepareReconnect,
  clearAccountCredentials, toggleAccountCreate,
} = useAccountManagement({
  accounts,
  selectedProfileID,
  reloadWorkspace: load,
  ensureAccountCapability,
  notify: (text) => { message.value = text },
  notifyConnection: (text) => { connectionMessage.value = text },
})
const publishDialogEl = ref<HTMLElement | null>(null)
let publishDialogReturnFocus: HTMLElement | null = null
const saveState = ref<'idle' | 'saving' | 'error'>('idle')
const editorReady = ref(false)
const suppressAutosave = ref(false)
let saveTimer: number | undefined
const baseURLPlaceholder = computed(() => accountPlatform.value === 'bluesky' ? 'Bluesky PDS URL' : '平台实例 URL')
const tokenEnvPlaceholder = computed(() => accountPlatform.value === 'bluesky' ? 'app password 环境变量名' : 'token 环境变量名')
const accountSetupPairing = computed(() => bridgePairings.value.find((pairing) => pairing.platform === accountPlatform.value && !pairing.revoked))
const profileTopics = computed(() => topics.value.filter((topic) => topic.profile_id === selectedProfileID.value && (showArchivedTopics.value || topic.status === 'active')))
const archivedTopicCount = computed(() => topics.value.filter((topic) => topic.profile_id === selectedProfileID.value && topic.status === 'archived').length)
const visibleTopics = computed(() => {
  const query = topicSearch.value.trim().toLowerCase()
  return query ? profileTopics.value.filter((topic) => topic.title.toLowerCase().includes(query)) : profileTopics.value
})
const targetAccounts = computed(() => {
  const query = targetAccountSearch.value.trim().toLowerCase()
  return query ? profileAccounts.value.filter((account) => `${account.name} ${platformLabel(account.platform)}`.toLowerCase().includes(query)) : profileAccounts.value
})
const editableVariants = computed(() => variants.value.filter((variant) => !variantLocked(variant)))
const selectedAccount = computed(() => accounts.value.find((account) => account.id === selectedAccountID.value))
const filteredVariants = computed(() => {
  const query = variantSearch.value.trim().toLowerCase()
  if (!query) return variants.value
  return variants.value.filter((variant) => variantAccountLabel(variant).toLowerCase().includes(query) || variantFormatLabel(variant).toLowerCase().includes(query) || variantStatusLabel(variant).toLowerCase().includes(query) || variant.text.toLowerCase().includes(query))
})
const activeVariant = computed(() => {
  if (activeVariantID.value === 'draft') return undefined
  return variants.value.find((variant) => variant.id === activeVariantID.value) || variants.value[0]
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
function variantLocked(variant?: Variant) { return Boolean(variant && (variant.publication_locked ?? variant.published)) }
const draftDirty = computed(() => Boolean(draft.value && text.value !== draft.value.text))
const topicTitleDirty = computed(() => Boolean(selected.value && topicTitleEdit.value !== selected.value.title))
const hasUnsavedChanges = computed(() => topicTitleDirty.value || draftDirty.value || variants.value.some(variantDirty) || Boolean(pendingMediaFile.value) || (newTopicMode.value && Boolean(topicTitleEdit.value.trim() || text.value.trim() || pendingTargetAccountID.value)))
const editorText = computed({
  get: () => activeVariant.value ? (activeVariant.value.text || '') : (text.value || ''),
  set: (value: string) => { if (activeVariant.value) activeVariant.value.text = value; else text.value = value },
})
const currentMedia = computed(() => {
  const order = new Map(mediaOrder.value.map((id, index) => [id, index]))
  return media.value
    .filter((item) => !item.variant_id || item.variant_id === activeVariant.value?.id)
    .slice()
    .sort((a, b) => (order.get(a.id) ?? Number.MAX_SAFE_INTEGER) - (order.get(b.id) ?? Number.MAX_SAFE_INTEGER))
})
const publishCandidates = computed(() => variants.value.filter((variant) => !variantLocked(variant)))
const pendingTopicActions = computed(() => topics.value.reduce((total, topic) => total + (topic.needs_action_count || 0), 0))
const publicationActionCount = computed(() => publicationSummary.value?.action_count ?? pendingTopicActions.value)
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
  if (isBridgePlatform(account.platform)) {
    if (connectedPairing(account.platform)) checks.push({ tone: 'ok', title: '发布通道', detail: `${platformLabel(account.platform)} 页面已连接`, result: '可填入' })
    else checks.push({ tone: 'error', title: '发布通道', detail: `未检测到 ${platformLabel(account.platform)} 页面`, result: '需连接' })
  }
  if (variant?.outdated) checks.push({ tone: 'warning', title: '主稿更新', detail: '当前版本基于较早的主稿', result: '请核对' })
  return checks
})
const preflightBlocking = computed(() => preflightChecks.value.some((check) => check.tone === 'error'))
const preflightSummary = computed(() => {
  const issues = preflightChecks.value.filter((check) => check.tone !== 'ok').length
  return issues ? `${issues} 项${preflightBlocking.value ? '需处理' : '建议'}` : '全部通过'
})
function variantBusy(variant: Variant) { return busyVariantIDs.value.has(variant.id) }
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

function variantCapability(variant?: Variant) {
  return variant ? capabilities.value[variant.account_id] : undefined
}

function visibilityLabel(variant: Variant, visibility: string) {
  const platform = variantAccount(variant)?.platform
  if (platform === 'misskey') {
    return ({ public: '公开', home: '主页时间线（公开）', followers: '仅关注者' } as Record<string, string>)[visibility] || visibility
  }
  if (platform === 'mastodon' || platform === 'mastodon-compatible') {
    return ({ public: '公开', unlisted: '不列入公共时间线', private: '关注者及提及用户', direct: '仅提及用户' } as Record<string, string>)[visibility] || visibility
  }
  return visibility === 'public' ? '公开' : visibility
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
function insertUnicodeEmoji(emoji: string) {
  insertEditorText(emoji)
}
function insertEditorText(value: string) {
  if (variantLocked(activeVariant.value)) return
  const editor = draftEditor.value
  const current = editorText.value
  const start = editor?.selectionStart ?? current.length
  const end = editor?.selectionEnd ?? start
  editorText.value = current.slice(0, start) + value + current.slice(end)
  void nextTick(() => {
    const nextEditor = draftEditor.value
    if (!nextEditor) return
    nextEditor.focus()
    const caret = start + value.length
    nextEditor.setSelectionRange(caret, caret)
  })
}
function openEmojiPicker() {
  closeActionMenus()
  editorToolReturnFocus = document.activeElement as HTMLElement | null
  translationDialogOpen.value = false
  editorTool.value = editorTool.value === 'emoji' ? '' : 'emoji'
  const variant = activeVariant.value
  if (editorTool.value === 'emoji' && variant && variantCapability(variant)?.custom_emoji) void loadEmoji(variant.account_id)
  if (editorTool.value === 'emoji') void nextTick(() => document.querySelector<HTMLElement>('.unicode-emoji-grid button')?.focus())
}
function closeEditorTool() {
  editorTool.value = ''
  void nextTick(() => {
    if (editorToolReturnFocus?.isConnected) editorToolReturnFocus.focus()
    else document.querySelector<HTMLElement>('.variant-picker-current')?.focus()
  })
}
function insertHashtag() {
  const tag = hashtagText.value.trim().replace(/^#+/, '').replace(/\s+/g, '_')
  if (!tag) return
  insertEditorText(`#${tag}`)
  editorTool.value = ''
}
function openMediaDialog() {
  closeActionMenus()
  editorTool.value = ''
  translationDialogOpen.value = false
  mediaDialogReturnFocus = document.activeElement as HTMLElement | null
  mediaVariantID.value = activeVariant.value?.id || ''
  mediaDialogOpen.value = true
  void nextTick(() => document.querySelector<HTMLElement>('.media-dialog button')?.focus())
}
function closeMediaDialog() {
  mediaDialogOpen.value = false
  void nextTick(() => mediaDialogReturnFocus?.focus())
}
function openComposerPublish() {
  closeActionMenus()
  editorTool.value = ''
  translationDialogOpen.value = false
  if (activeVariant.value) void openPublishDialog(activeVariant.value.id)
  else void openPublishFromDraft()
}
async function selectDocument(id: string) {
  if (!editorReady.value) return
  requestedDocumentID.value = id
  const previous = documentNavigation
  const topicID = selected.value?.id
  const navigate = async () => {
    if (previous && !await previous) return false
    if (!editorReady.value || topicID !== selected.value?.id || (id !== 'draft' && !variants.value.some((variant) => variant.id === id))) return false
    suppressAutosave.value = true
    try {
      await flushAutoSave()
      if (!editorReady.value || saveState.value === 'error' || topicID !== selected.value?.id) return false
      while (draftDirty.value || topicTitleDirty.value || (activeVariant.value && variantDirty(activeVariant.value))) {
        await flushAutoSave()
        if (!editorReady.value || saveState.value === 'error' || topicID !== selected.value?.id) return false
      }
      activeVariantID.value = id
      editorTool.value = ''
      translationDialogOpen.value = false
      if (id !== 'draft') {
        const variant = variants.value.find((item) => item.id === id)
        if (variant) { emojiTargetAccountID.value = variant.account_id; translationSourceID.value = variant.id }
      }
      await nextTick()
      return true
    } finally { suppressAutosave.value = false }
  }
  const pending = navigate()
  documentNavigation = pending
  try { await pending }
  finally {
    if (documentNavigation === pending) {
      documentNavigation = null
      requestedDocumentID.value = ''
    }
  }
}
async function selectDraft() { await selectDocument('draft') }
function toggleEditorTool(tool: 'hashtag' | 'emoji' | 'translation' | 'external') {
  if (tool === 'translation') {
    if (translationDialogOpen.value) closeTranslationDialog()
    else if (activeVariant.value) openTranslationDialog()
    else void openTranslateFromDraft()
    return
  }
  editorToolReturnFocus = document.activeElement as HTMLElement | null
  translationDialogOpen.value = false
  editorTool.value = editorTool.value === tool ? '' : tool
  if (tool === 'hashtag' && editorTool.value) {
    hashtagText.value = ''
    void nextTick(() => document.querySelector<HTMLInputElement>('.hashtag-tool input')?.focus())
  }
  if (tool === 'external' && editorTool.value) void nextTick(() => document.querySelector<HTMLElement>('.external-tool select')?.focus())
}
function closeActionMenus(except?: HTMLDetailsElement) {
  document.querySelectorAll<HTMLDetailsElement>('.action-menu[open]').forEach((menu) => { if (menu !== except) menu.open = false })
  if (!except || !except.classList.contains('topic-row-menu')) closeTopicMenu()
}
function updateActionMenu(event: Event) {
  const details = event.currentTarget as HTMLDetailsElement
  if (details.open) closeActionMenus(details)
}
function onActionMenuOutside(event: PointerEvent) {
  const target = event.target
  if (mobileDrawerOpen.value && target instanceof Element && !target.closest('.topic-dock, .mobile-menu-toggle, .topic-menu-floating')) closeMobileDrawer(false)
  if (target instanceof Element && (target.closest('.action-menu') || target.closest('.topic-menu-floating'))) return
  closeActionMenus()
}
async function openTargetPickerForDraftAction(action: 'publish' | 'translate') {
  closeActionMenus()
  showDocumentPicker.value = false
  if ((!selected.value && !newTopicMode.value) || !profileAccounts.value.length) {
    message.value = '先添加并选择发布账号'
    return
  }
  pendingDraftAction.value = action
  targetPickerReturnFocus = document.activeElement as HTMLElement | null
  targetAccountSearch.value = ''
  targetSelectedAccountID.value = ''
  showTargetAdd.value = true
  await nextTick()
  const trigger = document.querySelector<HTMLElement>('.variant-add')
  const popover = document.querySelector<HTMLElement>('#target-add-popover')
  if (!trigger || !popover) return
  const anchor = trigger.getBoundingClientRect()
  const size = popover.getBoundingClientRect()
  const left = Math.max(12, Math.min(window.innerWidth - size.width - 12, anchor.right - size.width))
  const topBelow = anchor.bottom + 8
  const top = topBelow + size.height <= window.innerHeight - 12 ? topBelow : Math.max(12, anchor.top - size.height - 8)
  targetAddPosition.value = { left, top }
  await nextTick()
  popover.querySelector<HTMLInputElement>('input[type="search"]')?.focus()
}
async function openPublishFromDraft() { await openTargetPickerForDraftAction('publish') }
async function openTranslateFromDraft() { await openTargetPickerForDraftAction('translate') }

async function toggleTargetAdd() {
  closeActionMenus()
  showDocumentPicker.value = false
  if (showTargetAdd.value) { closeTargetPicker(); return }
  pendingDraftAction.value = ''
  targetPickerReturnFocus = document.activeElement as HTMLElement | null
  targetAccountSearch.value = ''
  targetSelectedAccountID.value = ''
  showTargetAdd.value = true
  await nextTick()
  const trigger = document.querySelector<HTMLElement>('.variant-add')
  const popover = document.querySelector<HTMLElement>('#target-add-popover')
  if (!trigger || !popover) return
  const anchor = trigger.getBoundingClientRect()
  const size = popover.getBoundingClientRect()
  const left = Math.max(12, Math.min(window.innerWidth - size.width - 12, anchor.right - size.width))
  const topBelow = anchor.bottom + 8
  const top = topBelow + size.height <= window.innerHeight - 12 ? topBelow : Math.max(12, anchor.top - size.height - 8)
  targetAddPosition.value = { left, top }
  await nextTick()
  popover.querySelector<HTMLInputElement>('input[type="search"]')?.focus()
}
function closeTargetPicker() {
  showTargetAdd.value = false
  pendingDraftAction.value = ''
  void nextTick(() => targetPickerReturnFocus?.focus())
}
function closeDocumentPicker() {
  showDocumentPicker.value = false
  void nextTick(() => document.querySelector<HTMLElement>('.variant-picker-current')?.focus())
}
function toggleDocumentPicker() {
  closeActionMenus()
  showTargetAdd.value = false
  showDocumentPicker.value = !showDocumentPicker.value
  if (showDocumentPicker.value) {
    variantSearch.value = ''
    void nextTick(() => document.querySelector<HTMLInputElement>('.variant-picker-menu input[type="search"]')?.focus())
  }
}
async function openMobileDrawer() {
  closeActionMenus()
  const anchor = mobileMenuButton.value?.getBoundingClientRect()
  const width = Math.min(220, window.innerWidth - 24)
  const left = anchor ? Math.max(12, Math.min(window.innerWidth - width - 12, anchor.left)) : 12
  mobileMenuPosition.value = { left, top: anchor ? anchor.bottom + 8 : 60 }
  mobileDrawerOpen.value = true
  await nextTick()
  document.querySelector<HTMLInputElement>('.topic-dock.mobile-open .topic-search input')?.focus()
}
async function openTranslationDialog() {
  if (!activeVariant.value) return
  translationReturnFocus = document.querySelector<HTMLElement>('.editor-tool-group [aria-label="翻译"]')
  closeActionMenus()
  showTargetAdd.value = false
  showDocumentPicker.value = false
  postPreviewOpen.value = false
  editorTool.value = 'translation'
  translationSourceID.value = activeVariant.value.id
  translationDialogOpen.value = true
  closeTopicMenu()
  await nextTick()
  translationDialogEl.value?.querySelector<HTMLElement>('select, input, button')?.focus()
}
function closeTranslationDialog() {
  translationDialogOpen.value = false
  if (editorTool.value === 'translation') editorTool.value = ''
  void nextTick(() => translationReturnFocus?.focus())
}
function openPostPreview() {
  closeActionMenus()
  showTargetAdd.value = false
  showDocumentPicker.value = false
  translationDialogOpen.value = false
  editorTool.value = ''
  postPreviewOpen.value = true
  closeTopicMenu()
}
function closePostPreview() {
  postPreviewOpen.value = false
  void nextTick(() => document.querySelector<HTMLElement>('.variant-picker-current')?.focus())
}
function variantFormatLabel(variant: Variant) { return variant.format === 'mfm' ? 'MFM' : '纯文本' }
function variantRelationLabel(variant: Variant) { return variant.relation === 'linked' ? '跟随主稿' : '独立版本' }
function variantStatusLabel(variant: Variant) {
  if (variant.published) return '已发布'
  return ({ failed: '发布失败', needs_confirmation: '待确认', pending: '等待处理', publishing: '正在发布', cancelled: '确认未发布' } as Record<string, string>)[variant.publication_status || ''] || (variant.status === 'ready' ? '可发布' : '草稿')
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
  if (topic.needs_action_count) return `${topic.needs_action_count} 项待处理`
  if (topic.in_progress_count) return `${topic.in_progress_count} 项进行中`
  if (typeof topic.target_count === 'number') return topic.target_count ? `${topic.published_count || 0}/${topic.target_count} 已发布` : '尚未添加目标'
  return topic.status === 'archived' ? '已归档' : '进行中'
}
function topicStatusTone(topic: Topic) {
  if (topic.status === 'archived') return 'archived'
  if (topic.needs_action_count) return 'needs-action'
  if (topic.target_count && topic.published_count === topic.target_count) return 'complete'
  if (topic.target_count) return 'in-progress'
  return 'draft'
}
function topicStatusIcon(topic: Topic) {
  const tone = topicStatusTone(topic)
  if (tone === 'complete') return CircleCheck
  if (tone === 'needs-action') return CircleAlert
  if (tone === 'in-progress') return Clock3
  if (tone === 'archived') return Minus
  return Circle
}
function topicTime(topic: Topic) {
  if (!topic.updated_at) return ''
  const then = new Date(topic.updated_at).getTime()
  if (!Number.isFinite(then)) return ''
  const minutes = Math.max(0, Math.floor((Date.now() - then) / 60000))
  if (minutes < 1) return '刚刚'
  if (minutes < 60) return `${minutes} 分钟前`
  if (minutes < 1440) return `${Math.floor(minutes / 60)} 小时前`
  return new Date(then).toLocaleDateString()
}
function mediaForVariant(variant: Variant) { return media.value.filter((item) => !item.variant_id || item.variant_id === variant.id) }
function variantBlockingReason(variant: Variant) {
  const account = variantAccount(variant)
  const capability = variantCapability(variant)
  const selectedMedia = mediaForVariant(variant)
  if (!account || account.disabled) return '账号不可用'
  if (capability?.text_max_bytes && utf8Bytes(variant.text || '') > capability.text_max_bytes) return '正文超出限制'
  if (capability?.max_media && selectedMedia.length > capability.max_media) return '图片数量超出限制'
  if (isBridgePlatform(account.platform) && !connectedPairing(account.platform)) return '浏览器桥接未连接'
  return ''
}
function publishTargetState(variant: Variant) {
  const blocking = variantBlockingReason(variant)
  if (blocking) return blocking
  if (isBridgePlatform(variantAccount(variant)?.platform || '')) return '将填入平台页面'
  return variant.outdated ? '主稿有更新，可继续发布' : '可以发布'
}
function utf8Bytes(text: string) {
  return new TextEncoder().encode(text).length
}

watch(selectedProfileID, async (profileID) => {
  if (!profileAccounts.value.some((account) => account.id === selectedAccountID.value)) {
    selectedAccountID.value = profileAccounts.value[0]?.id || ''
  }
  if (selected.value && selected.value.profile_id !== profileID) await startNewTopic(false)
})
watch(section, (next, previous) => {
  if (previous === 'accounts' && next !== 'accounts') resetManagementMenus()
})
watch(workspaceModal, (next, previous) => {
  if (previous === 'accounts' && next !== 'accounts') resetManagementMenus()
})
watch(targetAccountSearch, () => { targetSelectedAccountID.value = '' })
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
watch(variants, (items) => {
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
  return { section: 'editor', topicID: '' }
}
function routePath(next: string) { return next === 'history' ? '/history' : next === 'accounts' ? '/accounts' : next === 'settings' ? '/settings' : next === 'editor' && selected.value ? `/topics/${encodeURIComponent(selected.value.id)}` : '/topics' }
async function goSection(next: string, replace = false) {
  if (section.value === 'editor' && next !== 'editor') {
    if (!newTopicMode.value && selected.value && hasUnsavedChanges.value) {
      await flushAutoSave()
      if (saveState.value === 'error' && !window.confirm('保存失败，仍要离开并放弃当前修改吗？')) return
    }
  }
  workspaceModal.value = ''
  resetEditorOverlays()
  section.value = next
  const path = routePath(next)
  if (replace) window.history.replaceState({}, '', path)
  else if (window.location.pathname !== path) window.history.pushState({}, '', path)
}
async function openWorkspacePanel(next: 'accounts' | 'settings' | 'history') {
  workspaceModalReturnFocus.value = document.activeElement as HTMLElement | null
  workspaceReturnSection.value = workspaceModal.value ? workspaceReturnSection.value : section.value
  closeActionMenus()
  closeMobileDrawer(false)
  showDocumentPicker.value = false
  showTargetAdd.value = false
  editorTool.value = ''
  translationDialogOpen.value = false
  workspaceModal.value = next
  const path = routePath(next)
  if (window.location.pathname !== path) window.history.pushState({}, '', path)
  await nextTick()
  document.querySelector<HTMLElement>('.workspace-modal-active [aria-modal="true"] button, .workspace-modal-active [aria-modal="true"] input, .workspace-modal-active [aria-modal="true"] select')?.focus()
}
function closeWorkspacePanel() {
  workspaceModal.value = ''
  window.history.replaceState({}, '', routePath(workspaceReturnSection.value))
  void nextTick(() => workspaceModalReturnFocus.value?.focus())
}
function profileLinkedAccounts(profileID: string) {
  return accounts.value.filter((account) => accountBelongsToProfile(account, profileID))
}
function resetManagementMenus() {
  expandedProfileID.value = ''
  expandedAccountID.value = ''
  cancelEditAccount()
  clearAccountCredentials()
}
function unlinkedProfiles(account: Account) {
  return profiles.value.filter((profile) => !accountBelongsToProfile(account, profile.id))
}
function toggleAccountMenu(account: Account) {
  cancelEditAccount()
  expandedAccountID.value = expandedAccountID.value === account.id ? '' : account.id
  const available = unlinkedProfiles(account)
  accountLinkProfileID.value = available.find((profile) => profile.id === selectedProfileID.value)?.id || available[0]?.id || ''
}
async function linkAccountFromPool(account: Account) {
  if (!accountLinkProfileID.value || accountLinkBusy.value) return
  const profileID = accountLinkProfileID.value
  accountLinkBusy.value = true
  try {
    if (await attachAccount(account, profileID)) accountLinkProfileID.value = unlinkedProfiles(account)[0]?.id || ''
  } finally { accountLinkBusy.value = false }
}
function selectRelativeDocument(direction: -1 | 1) {
  const ids = ['draft', ...variants.value.map((variant) => variant.id)]
  const current = Math.max(0, ids.indexOf(requestedDocumentID.value || activeVariantID.value || 'draft'))
  if (ids.length < 2) return
  const target = ids[(current + direction + ids.length) % ids.length]
  if (target === 'draft') void selectDraft()
  else void selectDocument(target)
}
function documentStepDisabled(direction: -1 | 1) {
  const ids = ['draft', ...variants.value.map((variant) => variant.id)]
  void direction
  return !editorReady.value || ids.length < 2
}
function accountVariant(accountID: string) {
  return variants.value.find((variant) => variant.account_id === accountID)
}
async function submitTargetAccount() {
  const accountID = targetSelectedAccountID.value
  if (!accountID) return
  if (pendingDraftAction.value) {
    const action = pendingDraftAction.value
    pendingDraftAction.value = ''
    showTargetAdd.value = false
    selectedAccountID.value = accountID
    if (!selected.value && newTopicMode.value) {
      if (action === 'translate' && !text.value.trim()) { message.value = '请先输入正文'; return }
      if (action === 'publish' && !topicTitleEdit.value.trim() && !text.value.trim()) topicTitleEdit.value = '未命名话题'
      pendingTargetAccountID.value = accountID
      if (!await createTopicFromEditor()) return
    }
    let variant = accountVariant(accountID)
    if (!variant) {
      await newVariant()
      variant = accountVariant(accountID)
    }
    if (!variant) return
    await selectDocument(variant.id)
    if (action === 'publish') {
      if (variantLocked(variant)) {
        message.value = '这个账号版本已锁定，请先复制版本'
        return
      }
      await openPublishDialog(variant.id)
    } else {
      await openTranslationDialog()
    }
    return
  }
  if (!selected.value && newTopicMode.value) {
    pendingTargetAccountID.value = accountID
    selectedAccountID.value = accountID
    showTargetAdd.value = false
    return
  }
  const existing = accountVariant(accountID)
  if (existing) {
    showTargetAdd.value = false
    await selectDocument(existing.id)
    return
  }
  selectedAccountID.value = accountID
  await newVariant()
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
  resetEditorOverlays()
  const route = routeState()
  if (route.section === 'editor' && route.topicID) {
    const topic = topics.value.find((item) => item.id === route.topicID)
    if (topic) { await openTopic(topic); return }
  }
  if (route.section === 'editor') { await startNewTopic(false); return }
  section.value = route.section
}
function onPopState() {
  const returningFromWindow = Boolean(workspaceModal.value)
  const nextRoute = routeState()
  workspaceModal.value = ''
  if (returningFromWindow && nextRoute.section === workspaceReturnSection.value && (nextRoute.section !== 'editor' || !nextRoute.topicID || nextRoute.topicID === selected.value?.id)) {
    window.history.replaceState({}, '', routePath(workspaceReturnSection.value))
    void nextTick(() => workspaceModalReturnFocus.value?.focus())
    return
  }
  if (section.value === 'editor' && hasUnsavedChanges.value && !window.confirm('还有未保存的稿件修改，确定离开并丢弃这些修改吗？')) {
    window.history.pushState({}, '', routePath('editor'))
    return
  }
  void applyBrowserRoute()
}
function resetEditorOverlays() {
  requestedDocumentID.value = ''
  editorTool.value = ''
  translationDialogOpen.value = false
  postPreviewOpen.value = false
  mediaDialogOpen.value = false
  mediaAltDialogOpen.value = false
  publishDialogOpen.value = false
  showTargetAdd.value = false
  showDocumentPicker.value = false
}

function showError(error: unknown) {
  message.value = `操作失败：${error instanceof Error ? error.message : String(error)}`
}
function dismissToast() { toast.value = { text: '', kind: 'success' }; if (toastTimer !== undefined) window.clearTimeout(toastTimer) }
async function retryAutoSave() {
  await flushAutoSave()
  if (saveState.value !== 'error') dismissToast()
}
function showToast(text: string) {
  if (!text) { dismissToast(); return }
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
    return true
  } catch (error) {
    message.value = `刷新工作区失败：${error instanceof Error ? error.message : String(error)}`
    return false
  }
}

async function ensureAccountCapability(accountID: string) {
  if (capabilities.value[accountID]) return
  const capability = await api<Capability>(`accounts/${accountID}/capability`)
  capabilities.value = { ...capabilities.value, [accountID]: capability }
}

async function addProfile() {
  if (!profileName.value.trim()) { message.value = '请填写 Profile 名称'; return }
  try {
    const created = await api<Profile>('profiles', { method: 'POST', body: JSON.stringify({ name: profileName.value }) })
    profileName.value = ''
    profileCreateOpen.value = false
    await load()
    selectedProfileID.value = created.id
  } catch (error) { showError(error) }
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
  if (status === 'archived' && !window.confirm(`归档话题“${topic.title}”？内容和发布历史都会保留。`)) return false
  try {
    const updated = await api<Topic>(`topics/${topic.id}`, { method: 'PUT', body: JSON.stringify({ title: topic.title, status }) })
    Object.assign(topic, updated)
    const listed = topics.value.find((item) => item.id === updated.id)
    if (listed && listed !== topic) Object.assign(listed, updated)
    if (selected.value?.id === updated.id && selected.value !== topic) Object.assign(selected.value, updated)
    message.value = status === 'archived' ? '话题已归档' : '话题已恢复'
    return true
  } catch (error) { showError(error); return false }
}

async function setSelectedTopicStatus() {
  const topic = selected.value
  if (!topic) return
  await flushAutoSave()
  if (saveState.value === 'error') return
  await setTopicStatus(topic, topic.status === 'archived' ? 'active' : 'archived')
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

async function startNewTopic(updateRoute = true) {
  closeActionMenus()
  if (newTopicCreatePromise) await newTopicCreatePromise
  if (newTopicMode.value && (topicTitleEdit.value.trim() || text.value.trim() || pendingTargetAccountID.value || pendingMediaFile.value) && !window.confirm('新话题尚未保存，清空这份草稿？')) return
  if (selected.value && pendingMediaFile.value && !window.confirm('图片尚未上传，仍要离开当前话题吗？')) return
  if (selected.value) {
    await flushAutoSave()
    if (saveState.value === 'error' && !window.confirm('保存失败，仍要新建话题并放弃当前修改吗？')) return
  }
  if (updateRoute) workspaceModal.value = ''
  resetEditorOverlays()
  if (saveTimer !== undefined) { window.clearTimeout(saveTimer); saveTimer = undefined }
  suppressAutosave.value = true
  editorReady.value = false
  selected.value = null
  draft.value = null
  variants.value = []
  media.value = []
  mediaOrder.value = []
  externalPosts.value = []
  text.value = ''
  topicTitleEdit.value = ''
  activeVariantID.value = 'draft'
  variantSearch.value = ''
  mediaVariantID.value = ''
  pendingTargetAccountID.value = ''
  pendingMediaFile.value = null
  pendingMediaAltText.value = ''
  newTopicMode.value = true
  section.value = 'editor'
  editorTool.value = ''
  saveState.value = 'idle'
  showTargetAdd.value = false
  showDocumentPicker.value = false
  if (updateRoute && window.location.pathname !== '/topics') window.history.pushState({}, '', '/topics')
  await nextTick()
  editorReady.value = true
  suppressAutosave.value = false
  closeMobileDrawer()
}

function createTopicFromEditor(): Promise<Topic | null> {
  if (selected.value) return Promise.resolve(selected.value)
  if (newTopicCreatePromise) return newTopicCreatePromise
  const title = topicTitleEdit.value.trim() || (text.value.trim() ? '未命名话题' : '')
  const body = text.value
  if (!title && !pendingMediaFile.value && !pendingTargetAccountID.value) { message.value = '先输入话题名或正文'; return Promise.resolve(null) }
  const profileID = selectedProfileID.value
  if (!profileID) { message.value = '先创建或选择 Profile'; return Promise.resolve(null) }
  const generationText = body
  const promise = (async () => {
    newTopicBusy.value = true
    saveState.value = 'saving'
    try {
      const created = await api<Topic>('topics', { method: 'POST', body: JSON.stringify({ profile_id: profileID, title: title || '新话题' }) })
      topics.value.unshift(created)
      await openTopic(created, true)
      newTopicMode.value = false
      if (generationText) {
        text.value = generationText
        if (!await persistDraft(false, true)) { saveState.value = 'error'; showError(new Error('正文保存失败，请重试')) }
      }
      const targetAccountID = pendingTargetAccountID.value
      if (targetAccountID) {
        const variant = await api<Variant>('variants', { method: 'POST', body: JSON.stringify({ topic_id: created.id, account_id: targetAccountID }) })
        variants.value.push(variant)
        variantBaselines.value[variant.id] = variantSignature(variant)
        activeVariantID.value = variant.id
        selectedAccountID.value = targetAccountID
        pendingTargetAccountID.value = ''
      }
      if (pendingMediaFile.value && !await retryPendingMedia()) saveState.value = 'error'
      if (saveState.value !== 'error') saveState.value = 'idle'
      return created
    } catch (error) {
      showError(error)
      saveState.value = 'error'
      return null
    } finally { newTopicBusy.value = false }
  })()
  newTopicCreatePromise = promise
  void promise.finally(() => { if (newTopicCreatePromise === promise) newTopicCreatePromise = null })
  return promise
}

async function openTopic(topic: Topic, fromNewCreation = false) {
  if (!fromNewCreation) {
    if (newTopicCreatePromise) await newTopicCreatePromise
    if (newTopicMode.value && (topicTitleEdit.value.trim() || text.value.trim() || pendingTargetAccountID.value || pendingMediaFile.value) && !window.confirm('新话题尚未保存，切换话题会放弃草稿。仍要继续吗？')) return
    if (selected.value && selected.value.id !== topic.id && pendingMediaFile.value && !window.confirm('图片尚未上传，仍要切换话题吗？')) return
    if (selected.value && selected.value.id !== topic.id && hasUnsavedChanges.value) {
      await flushAutoSave()
      if (saveState.value === 'error' && !window.confirm('保存失败，仍要切换话题并放弃当前修改吗？')) return
    }
  }
  closeActionMenus()
  closeMobileDrawer()
  if (!fromNewCreation) resetEditorOverlays()
  try {
    suppressAutosave.value = true
    editorReady.value = false
    selected.value = topic
    newTopicMode.value = false
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
    if (!workspaceModal.value && window.location.pathname !== `/topics/${encodeURIComponent(topic.id)}`) window.history.pushState({}, '', `/topics/${encodeURIComponent(topic.id)}`)
    editorReady.value = true
    await nextTick()
    suppressAutosave.value = false
    closeMobileDrawer()
  } catch (error) { showError(error) }
}

async function persistDraft(announce: boolean, silent = false) {
  if (!selected.value) return false
  try {
    const changed = draftDirty.value
    draft.value = await api<Draft>(`topics/${selected.value.id}/draft`, { method: 'PUT', body: JSON.stringify({ text: text.value, format: 'plain', language: '' }) })
    if (changed) variants.value.forEach((variant) => { if (variant.relation === 'linked' && !variantLocked(variant)) variant.outdated = true })
    if (announce) message.value = '基础稿已保存'
    return true
  } catch (error) { if (!silent) showError(error); return false }
}
async function saveDraft() { await flushAutoSave() }
async function saveTopicTitle() {
  if (newTopicMode.value) {
    if (!topicTitleEdit.value.trim() && !text.value.trim() && !pendingMediaFile.value) {
      message.value = '先输入话题名或正文'
      return
    }
    await createTopicFromEditor()
    return
  }
  await flushAutoSave()
}

let autoSaveRequest: Promise<void> | undefined
async function flushAutoSave() {
  if (autoSaveRequest) return autoSaveRequest
  const pending = performAutoSave()
  autoSaveRequest = pending
  try { await pending }
  finally { if (autoSaveRequest === pending) autoSaveRequest = undefined }
}
async function performAutoSave() {
  if (!editorReady.value) return
  if (!selected.value && newTopicMode.value) {
    if (!topicTitleEdit.value.trim() && !text.value.trim() && !pendingMediaFile.value) { saveState.value = 'idle'; return }
    if (saveTimer !== undefined) { window.clearTimeout(saveTimer); saveTimer = undefined }
    const created = await createTopicFromEditor()
    if (!created) saveState.value = 'error'
    return
  }
  if (!selected.value) return
  const needsDraft = draftDirty.value
  const needsTitle = topicTitleDirty.value
  const current = activeVariant.value
  const needsVariant = Boolean(current && variantDirty(current))
  if (!needsDraft && !needsTitle && !needsVariant) { saveState.value = 'idle'; return }
  if (saveTimer !== undefined) { window.clearTimeout(saveTimer); saveTimer = undefined }
  saveState.value = 'saving'
  try {
    if (needsTitle && !await renameSelectedTopic(true)) throw new Error('话题名称保存失败，请重试')
    if (needsDraft && !await persistDraft(false, true)) throw new Error('正文保存失败，请重试')
    if (current && variantDirty(current) && !await saveVariant(current, false, true)) throw new Error('平台版本保存失败，请重试')
    saveState.value = 'idle'
  } catch (error) {
    saveState.value = 'error'
    showError(error)
  }
}
function scheduleAutoSave() {
  if (!editorReady.value || suppressAutosave.value) return
  if (!selected.value && !topicTitleEdit.value.trim() && !text.value.trim()) { saveState.value = 'idle'; return }
  saveState.value = 'saving'
  if (saveTimer !== undefined) window.clearTimeout(saveTimer)
  saveTimer = window.setTimeout(() => { saveTimer = undefined; void flushAutoSave() }, 700)
}
watch([topicTitleEdit, editorText], scheduleAutoSave)
watch(() => activeVariant.value ? [activeVariant.value.id, activeVariant.value.visibility, activeVariant.value.local_only] as const : null, (next, previous) => {
  if (next && previous && next[0] === previous[0] && (next[1] !== previous[1] || next[2] !== previous[2])) scheduleAutoSave()
})
watch([() => selected.value?.id, () => activeVariant.value?.id, editorReady], () => { void loadMediaOrder() }, { flush: 'post' })
watch(currentMedia, () => { void setupMediaSortable() }, { flush: 'post' })

async function openPublishDialog(selectedOnlyID?: string) {
  if (!publishCandidates.value.length) { message.value = variants.value.length ? '当前话题没有可发布的新版本' : '请先添加目标账号并生成平台版本'; return }
  if (draftDirty.value && !await persistDraft(false)) return
  if (!await saveAllDirtyVariants()) return
  const available = publishCandidates.value.filter((variant) => !variantBlockingReason(variant))
  publishSelectedIDs.value = selectedOnlyID
    ? available.some((variant) => variant.id === selectedOnlyID) ? [selectedOnlyID] : []
    : available.map((variant) => variant.id)
  publishAutoIDs.value = []
  if (!publishSelectedIDs.value.length) { message.value = '所有平台版本都有阻塞项，请先处理'; return }
  publishDialogReturnFocus = document.activeElement as HTMLElement | null
  publishDialogOpen.value = true
  await nextTick()
  publishDialogEl.value?.querySelector<HTMLElement>('button, [tabindex="0"]')?.focus()
}
function closePublishDialog() {
  publishDialogOpen.value = false
  nextTick(() => publishDialogReturnFocus?.focus())
}
function closeMobileDrawer(returnFocus = true) {
  if (!mobileDrawerOpen.value) return
  closeActionMenus()
  mobileDrawerOpen.value = false
  if (returnFocus) void nextTick(() => mobileMenuButton.value?.focus())
}
async function updateTopicMenu(topicID: string, event: Event) {
  const details = event.currentTarget as HTMLDetailsElement
  if (!details.open) { if (openTopicMenuID.value === topicID) openTopicMenuID.value = ''; return }
  closeActionMenus(details)
  document.querySelectorAll<HTMLDetailsElement>('.topic-row-menu[open]').forEach((item) => { if (item !== details) item.open = false })
  openTopicMenuID.value = topicID
  await nextTick()
  const summary = details.querySelector('summary')
  const menu = document.querySelector<HTMLElement>('.topic-menu-floating')
  if (!summary || !menu) return
  const anchor = summary.getBoundingClientRect()
  const size = menu.getBoundingClientRect()
  const left = Math.max(8, Math.min(window.innerWidth - size.width - 8, anchor.right - size.width))
  const topBelow = anchor.bottom + 4
  const top = topBelow + size.height <= window.innerHeight - 8 ? topBelow : Math.max(8, anchor.top - size.height - 4)
  topicMenuPosition.value = { left, top }
}
function closeTopicMenu() {
  document.querySelectorAll<HTMLDetailsElement>('.topic-row-menu[open]').forEach((item) => { item.open = false })
  openTopicMenuID.value = ''
}
function updateMobileViewport() {
  mobileViewport.value = window.innerWidth <= 640
  if (!mobileViewport.value && mobileDrawerOpen.value) closeMobileDrawer(false)
  else if (mobileDrawerOpen.value) void openMobileDrawer()
}
async function openMediaAltDialog(item: Media | null, pending = false, trigger?: HTMLElement) {
  if (mobileDrawerOpen.value) closeMobileDrawer(false)
  closeActionMenus()
  mediaAltTarget.value = item
  mediaAltIsPending.value = pending
  mediaAltDraft.value = pending ? pendingMediaAltText.value : item?.alt_text || ''
  mediaAltError.value = ''
  mediaAltReturnFocus.value = trigger || document.activeElement as HTMLElement
  mediaAltDialogOpen.value = true
  await nextTick()
  mediaAltTextarea.value?.focus()
}
function closeMediaAltDialog() {
  mediaAltDialogOpen.value = false
  mediaAltError.value = ''
  void nextTick(() => mediaAltReturnFocus.value?.focus())
}
async function saveMediaAltDialog() {
  if (mediaAltIsPending.value) {
    pendingMediaAltText.value = mediaAltDraft.value
    closeMediaAltDialog()
    return
  }
  const item = mediaAltTarget.value
  if (!item) return
  try {
    const updated = await api<Media>(`media/${item.id}`, { method: 'PATCH', body: JSON.stringify({ alt_text: mediaAltDraft.value }) })
    Object.assign(item, updated)
    closeMediaAltDialog()
  } catch (error) {
    mediaAltError.value = error instanceof Error ? error.message : String(error)
  }
}
function onDialogKeydown(event: KeyboardEvent) {
  if (openTopicMenuID.value && event.key === 'Escape') { event.preventDefault(); closeTopicMenu(); return }
  if (event.key === 'Escape' && document.querySelector('.action-menu[open]')) { event.preventDefault(); closeActionMenus(); return }
  if (mobileDrawerOpen.value && event.key === 'Escape') { event.preventDefault(); closeMobileDrawer(); return }
  if (mediaAltDialogOpen.value) {
    if (event.key === 'Escape') { event.preventDefault(); closeMediaAltDialog(); return }
    if (event.key === 'Tab') {
      const dialog = document.querySelector<HTMLElement>('.media-alt-dialog')
      const focusable = dialog ? Array.from(dialog.querySelectorAll<HTMLElement>('button:not([disabled]), textarea:not([disabled]), [tabindex="0"]')) : []
      if (focusable.length) {
        const first = focusable[0], last = focusable[focusable.length - 1]
        if (event.shiftKey && (document.activeElement === first || !dialog?.contains(document.activeElement))) { event.preventDefault(); last.focus() }
        else if (!event.shiftKey && (document.activeElement === last || !dialog?.contains(document.activeElement))) { event.preventDefault(); first.focus() }
      }
      return
    }
    return
  }
  if (translationDialogOpen.value) {
    if (event.key === 'Escape') { event.preventDefault(); closeTranslationDialog(); return }
  }
  if (showTargetAdd.value && event.key === 'Escape') { event.preventDefault(); closeTargetPicker(); return }
  if (showDocumentPicker.value && event.key === 'Escape') { event.preventDefault(); closeDocumentPicker(); return }
  if (editorTool.value && event.key === 'Escape') { event.preventDefault(); closeEditorTool(); return }
  if (mediaDialogOpen.value) {
    if (event.key === 'Escape') { event.preventDefault(); closeMediaDialog(); return }
    if (event.key === 'Tab') {
      const dialog = document.querySelector<HTMLElement>('.media-dialog')
      const controls = dialog ? Array.from(dialog.querySelectorAll<HTMLElement>('button:not([disabled]), select:not([disabled]), input:not([disabled]):not([type="file"]), [tabindex="0"]')) : []
      const first = controls[0], last = controls[controls.length - 1]
      if (first && last) {
        if (event.shiftKey && (document.activeElement === first || !dialog?.contains(document.activeElement))) { event.preventDefault(); last.focus() }
        else if (!event.shiftKey && (document.activeElement === last || !dialog?.contains(document.activeElement))) { event.preventDefault(); first.focus() }
      }
    }
    return
  }
  if (workspaceModal.value) {
    if (event.key === 'Escape') { event.preventDefault(); closeWorkspacePanel(); return }
    if (event.key === 'Tab') {
      const dialog = document.querySelector<HTMLElement>('.workspace-modal-active [aria-modal="true"]')
      const focusable = dialog ? Array.from(dialog.querySelectorAll<HTMLElement>('button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), a[href], [tabindex]:not([tabindex="-1"])')) : []
      if (focusable.length) {
        const first = focusable[0], last = focusable[focusable.length - 1]
        if (event.shiftKey && (document.activeElement === first || !dialog?.contains(document.activeElement))) { event.preventDefault(); last.focus() }
        else if (!event.shiftKey && (document.activeElement === last || !dialog?.contains(document.activeElement))) { event.preventDefault(); first.focus() }
      }
      return
    }
    return
  }
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
  if (!publishSelectedIDs.value.length || publishDialogBusy.value) return
  publishDialogBusy.value = true
  if (!await saveAllDirtyVariants()) { publishDialogBusy.value = false; return }
  const targets = publishCandidates.value.filter((variant) => publishSelectedIDs.value.includes(variant.id))
  publishTask.value = targets.map((variant) => ({ id: variant.id, label: variantAccountLabel(variant), state: 'working' }))
  let firstFailure = ''
  try {
    for (const variant of targets) {
      const account = variantAccount(variant)
      const bridgeTarget = isBridgePlatform(account?.platform || '')
      const ok = bridgeTarget ? await fillBrowser(variant, publishAutoIDs.value.includes(variant.id)) : await publish(variant)
      const task = publishTask.value.find((item) => item.id === variant.id)
      if (task) {
        task.state = ok ? (bridgeTarget ? 'waiting' : 'done') : 'failed'
        if (!ok) {
          task.detail = message.value || bridgeMessage.value || '发布未完成'
          firstFailure ||= task.detail
        }
      }
    }
    closePublishDialog()
    const failed = publishTask.value.filter((item) => item.state === 'failed').length
    const waiting = publishTask.value.filter((item) => item.state === 'waiting').length
    message.value = failed ? firstFailure : waiting ? `已发送；${waiting} 个目标等待平台确认` : `${targets.length} 个版本发布完成`
    void refreshPublicationSummary()
    try {
      const topicID = selected.value?.id
      const [refreshed, refreshedVariants] = await Promise.all([
        api<Topic[]>('topics'),
        topicID ? api<Variant[]>(`variants?topic_id=${topicID}`) : Promise.resolve([]),
      ])
      topics.value = listOrEmpty(refreshed)
      const previousVariants = new Map(variants.value.map((variant) => [variant.id, variant]))
      variants.value = listOrEmpty(refreshedVariants).map((variant) => ({
        ...variant,
        publication_locked: variant.publication_locked ?? previousVariants.get(variant.id)?.publication_locked,
        publication_status: variant.publication_status || previousVariants.get(variant.id)?.publication_status,
      }))
      rememberVariants(variants.value)
      const current = topics.value.find((topic) => topic.id === topicID)
      if (current && selected.value) Object.assign(selected.value, current)
    } catch { /* publishing results remain visible in the task bar */ }
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
    variant.publication_locked = true
    variant.publication_status = 'published'
    message.value = '发布完成（请查看历史）'
    return true
  } catch (error) { message.value = `发布失败：${error instanceof Error ? error.message : String(error)}`; return false }
  finally { setVariantBusy(variant, false); void refreshPublicationSummary() }
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

async function saveMediaFile(file: File, alt: string, input?: HTMLInputElement) {
  if (!selected.value) return false
  const form = new FormData()
  form.append('topic_id', selected.value.id)
  if (mediaVariantID.value) form.append('variant_id', mediaVariantID.value)
  form.append('alt_text', alt); form.append('file', file)
  try {
    const response = await fetch('/api/media', { method: 'POST', body: form })
    const body = await response.json()
    if (!response.ok) throw new Error(body.error || `HTTP ${response.status}`)
    media.value.push(body); if (!body.variant_id || body.variant_id === activeVariant.value?.id) mediaOrder.value = [...mediaOrder.value, body.id]; altText.value = ''; if (input) input.value = ''; closeMediaDialog()
    message.value = '图片已保存'
    return true
  } catch (error) { showError(error); return false }
}
async function loadMediaOrder() {
  const topicID = selected.value?.id
  const variantID = activeVariant.value?.id || ''
  const requestID = ++mediaOrderRequestID
  if (!topicID || !editorReady.value) { mediaOrder.value = []; return }
  const query = new URLSearchParams({ topic_id: topicID, variant_id: variantID })
  try {
    const scopedMedia = await api<Media[]>(`media?${query.toString()}`)
    if (requestID !== mediaOrderRequestID || selected.value?.id !== topicID || (activeVariant.value?.id || '') !== variantID) return
    mediaOrder.value = listOrEmpty(scopedMedia).map((item) => item.id)
  } catch (error) {
    if (requestID === mediaOrderRequestID) showError(error)
  }
}
async function saveMediaOrder(event: SortableEvent) {
  if (event.oldIndex === event.newIndex || !mediaSortable || mediaOrderBusy) return
  const topicID = selected.value?.id
  const variantID = activeVariant.value?.id || ''
  if (!topicID) return
  const ids = Array.from(mediaListEl.value?.querySelectorAll<HTMLElement>(':scope > .media-item[data-media-id]') || []).map((item) => item.dataset.mediaId || '')
  const previous = mediaOrder.value.slice()
  if (ids.length !== currentMedia.value.length || ids.every((id, index) => id === previous[index])) return
  mediaOrderBusy = true
  mediaSortable.option('disabled', true)
  try {
    const saved = await api<{ media_ids: string[] }>('media/order', { method: 'PUT', body: JSON.stringify({ topic_id: topicID, variant_id: variantID, media_ids: ids }) })
    if (selected.value?.id === topicID && (activeVariant.value?.id || '') === variantID) mediaOrder.value = saved.media_ids
  } catch (error) {
    if (selected.value?.id === topicID && (activeVariant.value?.id || '') === variantID) mediaOrder.value = previous
    showError(error)
  } finally {
    mediaOrderBusy = false
    mediaSortable?.option('disabled', false)
  }
}
async function setupMediaSortable() {
  await nextTick()
  mediaSortable?.destroy()
  mediaSortable = undefined
  if (!selected.value || !mediaListEl.value || currentMedia.value.length < 2) return
  mediaSortable = Sortable.create(mediaListEl.value, {
    animation: window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 0 : 150,
    dataIdAttr: 'data-media-id',
    disabled: mediaOrderBusy,
    draggable: '.media-item:not(.pending-media-item)',
    handle: '.media-preview-frame',
    filter: '.media-delete, .media-alt-badge, .pending-media-item, button, input',
    preventOnFilter: false,
    delay: 180,
    delayOnTouchOnly: true,
    touchStartThreshold: 4,
    fallbackTolerance: 4,
    ghostClass: 'sortable-ghost',
    chosenClass: 'sortable-chosen',
    dragClass: 'sortable-drag',
    onEnd: (event) => { void saveMediaOrder(event) },
  })
}
async function retryPendingMedia() {
  if (!pendingMediaFile.value || !selected.value) return false
  const saved = await saveMediaFile(pendingMediaFile.value, pendingMediaAltText.value)
  if (saved) { pendingMediaFile.value = null; pendingMediaAltText.value = '' }
  return saved
}
async function uploadMedia(event: Event) {
  const input = event.target as HTMLInputElement
  if (!input.files?.[0]) return
  if (!selected.value && newTopicMode.value) {
    pendingMediaFile.value = input.files[0]
    pendingMediaAltText.value = altText.value
    input.value = ''
    closeMediaDialog()
    message.value = '图片已暂存，保存话题后上传'
    return
  }
  await saveMediaFile(input.files[0], altText.value, input)
}
async function deleteMedia(item: Media) {
  if (!window.confirm(`删除图片“${item.filename}”？本地文件也会移除。`)) return
  try {
    await api(`media/${item.id}`, { method: 'DELETE' })
    media.value = media.value.filter((mediaItem) => mediaItem.id !== item.id)
    mediaOrder.value = mediaOrder.value.filter((id) => id !== item.id)
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
    void refreshPublicationSummary()
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
  if (variantLocked(variant)) return
  insertEditorText(`:${emoji.shortcode}:`)
}
async function createBridgePairing() {
  try {
    const pairing = await api<BridgePairing>('bridge/pairings', { method: 'POST', body: JSON.stringify({ platform: bridgePlatform.value }) })
    bridgePairings.value = [pairing, ...bridgePairings.value.filter((item) => item.platform !== pairing.platform)]
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
async function createAccountBridgePairing() {
  bridgePlatform.value = accountPlatform.value
  await createBridgePairing()
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
    variant.publication_locked = true
    variant.publication_status = 'needs_confirmation'
    bridgeMessage.value = autoPublish ? '已发送自动发布请求；脚本只会点击已知、可见且启用的发布按钮，请随后核对平台结果' : '已发送，等待脚本领取；填入后请人工检查并点击平台发布'
    void refreshPublicationSummary()
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
  const request = { source_variant_id: translationSourceID.value, target_account_id: translationTargetAccountID.value, source_language: translationSourceLanguage.value, target_language: translationTargetLanguage.value, config_id: translationConfigID.value }
  if (source && variantDirty(source) && !await saveVariant(source, false)) { translationMessage.value = '源版本保存失败，未开始翻译'; return }
  try {
    const result = await api<{ variant: Variant; translation: unknown }>('translations', { method: 'POST', body: JSON.stringify(request) })
    if (result.variant.topic_id !== selected.value?.id) return
    variants.value.push(result.variant); variantBaselines.value[result.variant.id] = variantSignature(result.variant); variantSearch.value = ''; activeVariantID.value = result.variant.id; emojiTargetAccountID.value = result.variant.account_id; translationMessage.value = '已生成新的 detached 翻译版本，原文未修改，已切换到新版本'
  } catch (error) { translationMessage.value = `翻译失败：${error instanceof Error ? error.message : String(error)}；原文与目标版本均未改动` }
}
async function translateActiveVariant() {
  if (translationBusy.value) return
  if (activeVariant.value) translationSourceID.value = activeVariant.value.id
  translationBusy.value = true
  try { await translateVariant() }
  finally { translationBusy.value = false }
}
let bridgeSyncing = false
let bridgeRefreshTimer: number | undefined
async function syncBridgeState(announce: boolean) {
  if (bridgeSyncing) return
  bridgeSyncing = true
  try {
    const existing = new Map(bridgePairings.value.map((pairing) => [pairing.id, pairing]))
    bridgePairings.value = listOrEmpty(await api<BridgePairing[]>('bridge/pairings')).map((pairing) => ({ ...existing.get(pairing.id), ...pairing }))
    if (bridgeRequest.value) {
      bridgeRequest.value = await api<BridgeRequest>(`bridge/requests/${bridgeRequest.value.id}`)
      topics.value = listOrEmpty(await api<Topic[]>('topics'))
      void refreshPublicationSummary()
    }
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
  window.addEventListener('resize', updateMobileViewport)
  document.addEventListener('pointerdown', onActionMenuOutside)
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
  if (modalBodyScrollLocked) document.body.style.overflow = modalPreviousBodyOverflow
  mediaSortable?.destroy()
  mediaSortable = undefined
  clearAccountCredentials()
  window.removeEventListener('beforeunload', warnBeforeUnload)
  window.removeEventListener('popstate', onPopState)
  window.removeEventListener('keydown', onDialogKeydown)
  window.removeEventListener('resize', updateMobileViewport)
  document.removeEventListener('pointerdown', onActionMenuOutside)
  if (bridgeRefreshTimer !== undefined) window.clearInterval(bridgeRefreshTimer)
  if (toastTimer !== undefined) window.clearTimeout(toastTimer)
  if (saveTimer !== undefined) window.clearTimeout(saveTimer)
})
</script>

<template>
  <div class="shell" :class="{'sidebar-collapsed':sidebarCollapsed,'workspace-modal-active':Boolean(workspaceModal), 'mobile-drawer-open':mobileDrawerOpen}">
    <aside class="topic-dock" :class="{collapsed:sidebarCollapsed && !mobileDrawerOpen,'mobile-open':mobileDrawerOpen}" :style="mobileViewport && mobileDrawerOpen ? {top:`${mobileMenuPosition.top}px`,left:`${mobileMenuPosition.left}px`} : undefined" :aria-label="mobileViewport && mobileDrawerOpen ? '话题导航菜单' : '话题列表'" :role="mobileViewport && mobileDrawerOpen ? 'dialog' : undefined">
      <div class="dock-search-row"><label class="topic-search"><input v-model="topicSearch" aria-label="搜索话题" placeholder="搜索"><Search aria-hidden="true"/></label><button v-if="archivedTopicCount" type="button" class="archived-topic-toggle" :aria-label="showArchivedTopics?'隐藏归档话题':'显示归档话题'" :title="showArchivedTopics?'隐藏归档话题':'显示归档话题'" :aria-pressed="showArchivedTopics" @click="showArchivedTopics=!showArchivedTopics"><Archive aria-hidden="true"/></button></div>
      <ul class="quick-topics"><li v-for="topic in visibleTopics" :key="topic.id" class="topic-row" :class="{active:selected?.id===topic.id}" @click.self="closeTopicMenu"><button type="button" class="topic-row-select" :title="topicProgress(topic)" :aria-label="`${topic.title} · ${topicProgress(topic)}`" @click="openTopic(topic)"><strong>{{topic.title}}</strong></button><component :is="topicStatusIcon(topic)" class="topic-status" :class="topicStatusTone(topic)" aria-hidden="true"/><details class="topic-row-menu action-menu" @toggle="updateTopicMenu(topic.id, $event)"><summary :aria-label="`更多操作：${topic.title}`" title="更多操作"><Ellipsis aria-hidden="true"/></summary><Teleport to="body"><div v-if="openTopicMenuID===topic.id" class="topic-menu-floating" :style="{left:`${topicMenuPosition.left}px`,top:`${topicMenuPosition.top}px`}"><button type="button" @click="setTopicStatus(topic, topic.status === 'archived' ? 'active' : 'archived'); closeTopicMenu()"><Archive aria-hidden="true"/>{{topic.status === 'archived' ? '恢复话题' : '归档话题'}}</button></div></Teleport></details></li><li v-if="!visibleTopics.length" class="quick-empty">{{topicSearch ? '没有匹配的话题' : '还没有话题'}}</li></ul>
      <button class="new-topic-action" type="button" @click="startNewTopic()"><Plus aria-hidden="true"/><span>新建话题</span></button>
      <button class="dock-collapse" type="button" :aria-label="sidebarCollapsed?'展开话题列表':'收起话题列表'" @click="sidebarCollapsed=!sidebarCollapsed"><Triangle class="dock-triangle" :class="{collapsed:sidebarCollapsed}" aria-hidden="true"/></button>
    </aside>
    <main class="canvas">
      <div v-if="toast.text" class="toast" :class="`toast-${toast.kind}`" :role="toast.kind === 'success' ? 'status' : 'alert'" :aria-live="toast.kind === 'success' ? 'polite' : 'assertive'"><span>{{toast.text}}</span><button v-if="saveState==='error'" type="button" class="toast-retry" aria-label="重试保存" title="重试保存" @click="retryAutoSave"><RefreshCw aria-hidden="true"/><span>重试</span></button><button type="button" aria-label="关闭提示" title="关闭提示" @click="dismissToast">×</button></div>
      <header class="workspace-header">
        <div class="mobile-header-top"><button ref="mobileMenuButton" type="button" class="mobile-menu-toggle" :aria-label="mobileDrawerOpen?'收起话题导航':'打开话题导航'" aria-haspopup="dialog" :aria-expanded="mobileDrawerOpen" @click="mobileDrawerOpen?closeMobileDrawer():openMobileDrawer()"><Triangle class="mobile-menu-triangle" :class="{'is-open':mobileDrawerOpen}" aria-hidden="true"/></button><span class="mobile-wordmark">Best Posts Manager</span></div>
        <a class="workspace-brand" href="/topics" @click.prevent="startNewTopic()"><span><strong>Best Posts</strong><strong>Manager</strong></span></a>
        <div class="header-control-row"><label class="header-profile"><span class="sr-only">Profile</span><select v-model="selectedProfileID" aria-label="切换 Profile" :disabled="!profiles.length"><option v-for="profile in profiles" :key="profile.id" :value="profile.id">{{profile.name}}</option></select><ChevronDown aria-hidden="true"/></label>
        <div v-if="section==='editor'" class="header-topic">
          <div class="header-topic-copy"><span class="sr-only">话题名</span><input class="topic-title" v-model="topicTitleEdit" aria-label="话题名" :placeholder="newTopicMode?'输入话题名':'话题名'"><button v-if="newTopicMode || topicTitleDirty" type="button" class="topic-title-save" aria-label="保存话题名" title="保存" :disabled="newTopicBusy" @click="saveTopicTitle"><Check aria-hidden="true"/></button><span class="save-state" aria-live="polite">{{saveState === 'saving' ? '保存中…' : ''}}</span></div>
          <details v-if="!newTopicMode" class="topic-more action-menu" @toggle="updateActionMenu"><summary aria-label="话题菜单" title="话题菜单"><Ellipsis aria-hidden="true"/></summary><div class="topic-more-menu" @click="closeActionMenus()"><button v-if="sidebarCollapsed || (mobileViewport && !mobileDrawerOpen)" type="button" @click="startNewTopic()"><Plus aria-hidden="true"/><span>新建话题</span></button><button type="button" @click="setSelectedTopicStatus"><Archive aria-hidden="true"/><span>{{selected?.status === 'archived' ? '恢复话题' : '归档话题'}}</span></button></div></details>
        </div>
        </div>
      </header>
      <div v-if="workspaceModal" class="workspace-modal-backdrop" @click.self="closeWorkspacePanel"></div>
      <TopicsView v-if="section==='topics'" v-model:selected-profile-id="selectedProfileID" v-model:topic-search="topicSearch" v-model:topic-title="topicTitle" v-model:show-archived="showArchivedTopics" :profiles="profiles" :topics="visibleTopics" :progress-label="topicProgress" :time-label="topicTime" @create="addTopic" @select-topic="followTopicLink" @toggle-archive="setTopicStatus" />
      <section v-else-if="section==='editor'" class="page-section editor-page">
        <section class="editor-composer" aria-label="稿件编辑器">
        <div class="document-tabs-wrap">
          <button type="button" class="variant-stepper" aria-label="上一个版本" :disabled="documentStepDisabled(-1)" @click="selectRelativeDocument(-1)"><ChevronLeft aria-hidden="true"/></button>
          <div class="variant-picker-group">
            <button type="button" class="variant-picker-current" aria-haspopup="dialog" aria-controls="variant-picker-popover" :aria-expanded="showDocumentPicker" @click="toggleDocumentPicker">
              <span class="variant-current-icon"><PlatformIcon v-if="activeVariant" :platform="variantAccount(activeVariant)?.platform" /><PlatformIcon v-else-if="pendingTargetAccountID" :platform="accounts.find(account=>account.id===pendingTargetAccountID)?.platform" /><svg v-else viewBox="0 0 20 20" aria-hidden="true"><path d="M5 3h7l3 3v11H5zM12 3v4h3" fill="none" stroke="currentColor" stroke-width="1.4"/></svg></span>
              <span class="variant-current-copy"><strong>{{activeVariant ? variantTabLabel(activeVariant) : pendingTargetAccountID ? (accounts.find(account=>account.id===pendingTargetAccountID)?.name || '主稿') : '主稿'}}</strong></span>
              <ChevronDown class="variant-picker-chevron" aria-hidden="true"/>
            </button>
          </div>
          <button type="button" class="variant-stepper" aria-label="下一个版本" :disabled="documentStepDisabled(1)" @click="selectRelativeDocument(1)"><ChevronRight aria-hidden="true"/></button>
            <button type="button" class="variant-add" aria-label="添加发布账号" title="添加发布账号" aria-controls="target-add-popover" :aria-expanded="showTargetAdd" @click="toggleTargetAdd">
              <Plus aria-hidden="true"/>
            </button>
          <div v-if="showDocumentPicker" class="variant-picker-backdrop" @click="closeDocumentPicker"></div>
          <section v-if="showDocumentPicker" id="variant-picker-popover" class="variant-picker-menu" role="dialog" aria-label="稿件版本与操作">
            <input v-model="variantSearch" type="search" aria-label="搜索稿件版本" placeholder="搜索版本">
            <div class="variant-picker-list" role="listbox" aria-label="选择稿件版本">
              <button v-if="!variantSearch.trim() || '主稿'.includes(variantSearch.trim().toLowerCase())" type="button" role="option" :aria-selected="activeVariantID==='draft'" @click="selectDraft(); showDocumentPicker=false"><span class="variant-current-icon"><FileText aria-hidden="true"/></span><span><strong>主稿</strong></span></button>
              <button v-for="variant in filteredVariants" :key="variant.id" type="button" role="option" :aria-selected="activeVariantID===variant.id" @click="selectDocument(variant.id); showDocumentPicker=false"><span class="variant-current-icon"><PlatformIcon :platform="variantAccount(variant)?.platform" /></span><span><strong>{{variantTabLabel(variant)}}</strong><small>{{variantStatusLabel(variant)}}<template v-if="variant.outdated"> · 主稿有更新</template></small></span></button>
              <p v-if="variantSearch.trim() && !filteredVariants.length && !'主稿'.includes(variantSearch.trim().toLowerCase())" class="quick-empty">没有匹配的版本</p>
            </div>
            <div class="variant-picker-actions" aria-label="当前版本操作">
              <label v-if="activeVariant && variantCapability(activeVariant)?.visibilities?.length" class="variant-picker-visibility"><span>可见性</span><select v-model="activeVariant.visibility" :aria-label="`${variantAccountLabel(activeVariant)}可见性`" :disabled="variantLocked(activeVariant)"><option v-for="visibility in variantCapability(activeVariant)?.visibilities || []" :key="visibility" :value="visibility">{{visibilityLabel(activeVariant, visibility)}}</option></select></label>
              <label v-if="activeVariant && variantCapability(activeVariant)?.supports_local_only" class="variant-picker-local-only"><input type="checkbox" v-model="activeVariant.local_only" :disabled="variantLocked(activeVariant)">仅本地</label>
              <button v-if="selected && activeVariant" type="button" @click="showDocumentPicker=false; openPostPreview()"><Eye aria-hidden="true"/><span>预览</span></button>
              <button v-if="selected" type="button" @click="showDocumentPicker=false; toggleEditorTool('external')"><Link2 aria-hidden="true"/><span>站外关联</span></button>
              <button v-if="activeVariant?.outdated && !variantLocked(activeVariant)" type="button" :disabled="variantBusy(activeVariant)" @click="showDocumentPicker=false; applyVariant(activeVariant)"><RefreshCw aria-hidden="true"/><span>应用主稿更新</span></button>
              <button v-if="variantLocked(activeVariant)" type="button" :disabled="variantBusy(activeVariant)" @click="showDocumentPicker=false; cloneVariant(activeVariant)"><Copy aria-hidden="true"/><span>复制版本</span></button>
              <button v-if="activeVariant && !variantAccount(activeVariant)?.disabled" type="button" aria-label="移除本地版本（不会删除远端帖子）" :disabled="variantBusy(activeVariant)" @click="showDocumentPicker=false; deleteVariant(activeVariant)"><Trash2 aria-hidden="true"/><span>移除版本</span></button>
            </div>
          </section>
          <div v-if="showTargetAdd" class="target-add-backdrop" role="presentation" @click="closeTargetPicker"></div>
          <section v-if="showTargetAdd" id="target-add-popover" class="target-add target-add-inline" role="dialog" aria-labelledby="target-add-title" :style="{top:`${targetAddPosition.top}px`,left:`${targetAddPosition.left}px`}"><h2 id="target-add-title">{{pendingDraftAction==='translate'?'选择翻译源账号':'选择发布账号'}}</h2><input v-model="targetAccountSearch" type="search" placeholder="搜索账号" aria-label="搜索账号"><ul v-if="targetAccounts.length" class="target-account-results" aria-label="账号"><li v-for="account in targetAccounts" :key="account.id"><button type="button" :class="{selected:targetSelectedAccountID===account.id}" :aria-pressed="targetSelectedAccountID===account.id" @click="targetSelectedAccountID=account.id"><PlatformIcon :platform="account.platform" /><span class="target-account-copy"><strong>{{account.name}}</strong><small>{{platformLabel(account.platform)}}</small></span><small v-if="accountVariant(account.id)" class="target-account-state">已添加</small></button></li></ul><p v-else class="target-account-empty">没有匹配账号</p><button type="button" class="primary-action target-account-submit" @click="submitTargetAccount" :disabled="!targetSelectedAccountID"><ArrowRight v-if="pendingDraftAction || accountVariant(targetSelectedAccountID)" aria-hidden="true"/><Plus v-else aria-hidden="true"/><span>{{pendingDraftAction?'继续':accountVariant(targetSelectedAccountID)?'切换':'添加'}}</span></button></section>
        </div>
        <div class="editor-shell"><div class="editor-paper"><textarea ref="draftEditor" class="draft-editor" v-model="editorText" placeholder="输入文字…" :disabled="variantLocked(activeVariant)" aria-label="当前稿件正文" @blur="flushAutoSave"></textarea></div>
        <section v-if="currentMedia.length || pendingMediaFile" class="flow-section media-flow" aria-label="媒体"><ul ref="mediaListEl" class="media-list media-row"><li v-for="item in currentMedia" :key="item.id" class="media-item" :data-media-id="item.id"><div class="media-preview-frame"><img :src="`/media/${item.id}`" :alt="item.alt_text || item.filename"><button type="button" class="media-delete" :aria-label="`删除图片 ${item.filename}`" :title="`删除图片 ${item.filename}`" @click="deleteMedia(item)"><X aria-hidden="true"/></button><button type="button" class="media-alt-badge" :aria-label="item.alt_text?.trim() ? `编辑替代文本：${item.alt_text}` : '添加替代文本'" :title="item.alt_text?.trim() ? '编辑替代文本' : '添加替代文本'" @click="openMediaAltDialog(item, false, $event.currentTarget as HTMLElement)"><Check v-if="item.alt_text?.trim()" aria-hidden="true"/><Plus v-else aria-hidden="true"/><span>ALT</span></button></div></li><li v-if="pendingMediaFile" class="media-item pending-media-item"><div class="media-preview-frame pending-media-frame"><span>{{pendingMediaFile.name}}</span><button type="button" class="media-alt-badge" :aria-label="pendingMediaAltText.trim() ? '编辑待上传图片替代文本' : '添加待上传图片替代文本'" @click="openMediaAltDialog(null, true, $event.currentTarget as HTMLElement)"><Check v-if="pendingMediaAltText.trim()" aria-hidden="true"/><Plus v-else aria-hidden="true"/><span>ALT</span></button></div></li></ul></section>
        <section v-if="editorTool==='emoji'" v-dialog-drag class="emoji-picker editor-floating-tool" role="dialog" aria-labelledby="emoji-tool-title"><header class="floating-tool-header"><div><Smile aria-hidden="true"/><h2 id="emoji-tool-title">Emoji</h2></div><button type="button" aria-label="关闭 Emoji" title="关闭" @click="closeEditorTool"><X aria-hidden="true"/></button></header><div class="floating-tool-body"><div class="unicode-emoji-grid"><button v-for="emoji in unicodeEmoji" :key="emoji" type="button" :aria-label="`插入 ${emoji}`" @click="insertUnicodeEmoji(emoji)">{{emoji}}</button></div><template v-if="activeVariant && variantCapability(activeVariant)?.custom_emoji"><input v-model="emojiFilter" type="search" aria-label="搜索实例 emoji" placeholder="搜索 emoji"><div v-if="emojiLoading" class="emoji-loading" role="status">加载中…</div><div v-else class="custom-emoji-grid"><button v-for="emoji in visibleEmojis" :key="emoji.shortcode" type="button" :aria-label="`插入 :${emoji.shortcode}:`" :title="`:${emoji.shortcode}:`" @click="insertEmoji(emoji, activeVariant!)"><img :src="emoji.static_url || emoji.url" :alt="emoji.shortcode"></button><button v-if="hasMoreEmojis" type="button" class="emoji-more" @click="showMoreEmoji">更多</button></div></template></div></section>
        <section v-if="editorTool==='hashtag'" v-dialog-drag class="editor-floating-tool hashtag-tool" role="dialog" aria-labelledby="hashtag-tool-title"><header class="floating-tool-header"><div><Hash aria-hidden="true"/><h2 id="hashtag-tool-title">话题标签</h2></div><button type="button" aria-label="关闭话题标签" title="关闭" @click="closeEditorTool"><X aria-hidden="true"/></button></header><form class="floating-tool-body" @submit.prevent="insertHashtag"><label><span>标签</span><input v-model="hashtagText" aria-label="话题标签" placeholder="#标签"></label><button type="submit" class="primary-action" :disabled="!hashtagText.trim().replace(/^#+/, '')"><Plus aria-hidden="true"/><span>插入</span></button></form></section>
        <footer class="editor-toolbar" aria-label="编辑器工具">
          <div class="editor-tool-group">
            <button type="button" class="tool-button" aria-label="插入话题标签" title="话题标签" :class="{active:editorTool==='hashtag'}" :aria-expanded="editorTool==='hashtag'" :disabled="variantLocked(activeVariant)" @click="toggleEditorTool('hashtag')"><Hash aria-hidden="true"/></button>
            <button type="button" class="tool-button" :class="{active:editorTool==='emoji'}" aria-label="Emoji" title="Emoji" :aria-expanded="editorTool==='emoji'" :disabled="variantLocked(activeVariant)" @click="openEmojiPicker"><Smile aria-hidden="true"/></button>
            <button type="button" class="tool-button" :class="{active:translationDialogOpen}" aria-label="翻译" title="翻译" :aria-expanded="translationDialogOpen" @click="toggleEditorTool('translation')"><Languages aria-hidden="true"/></button>
            <button type="button" class="tool-button" aria-label="图片" title="图片" :disabled="variantLocked(activeVariant)" @click="openMediaDialog"><Image aria-hidden="true"/></button>
          </div>
          <div class="editor-toolbar-right"><button type="button" class="primary-action post-preview-trigger" aria-label="检查并发布" :disabled="newTopicBusy || Boolean(activeVariant && (variantLocked(activeVariant) || variantBusy(activeVariant))) || publishDialogBusy" @click="openComposerPublish"><Send aria-hidden="true"/><span>发布</span></button></div>
        </footer></div>
        </section>
        <section v-if="editorTool==='external' && selected" v-dialog-drag class="editor-tool-panel external-tool editor-floating-tool" role="dialog" aria-labelledby="external-tool-title"><header class="floating-tool-header"><div><Link2 aria-hidden="true"/><h2 id="external-tool-title">站外关联</h2></div><button type="button" aria-label="关闭站外关联" title="关闭" @click="closeEditorTool"><X aria-hidden="true"/></button></header><div class="floating-tool-body">
          <form @submit.prevent="markExternal">
            <select v-model="externalPlatform" aria-label="平台">
              <option v-for="platform in [...new Set(profileAccounts.map(account=>account.platform))]" :key="platform" :value="platform">{{platformLabel(platform)}}</option>
            </select>
            <select v-model="externalAccountID" aria-label="关联账号" :disabled="!externalPlatformAccounts.length">
              <option value="">选择账号</option>
              <option v-for="account in externalPlatformAccounts" :key="account.id" :value="account.id">{{account.name}}</option>
            </select>
            <input v-model="externalURL" type="url" aria-label="站外帖子 URL" placeholder="帖子 URL" required>
            <button type="submit" class="primary-action" :disabled="!externalAccountID || !externalURL.trim()"><Link2 aria-hidden="true"/><span>关联</span></button>
          </form>
          <ul v-if="externalPosts.length" class="external-post-list" aria-label="站外关联列表">
            <li v-for="post in externalPosts" :key="post.id"><a :href="post.url" target="_blank" rel="noreferrer">{{platformLabel(post.platform)}} · {{post.url}}</a><button type="button" :aria-label="`移除 ${platformLabel(post.platform)} 站外关联`" @click="deleteExternalPost(post)"><Trash2 aria-hidden="true"/></button></li>
          </ul>
        </div></section>

      </section>
      <section v-else-if="section==='editor'" class="page-section empty-editor">
        <div class="eyebrow">正在编辑</div><h1>先选择一个话题</h1><p class="lede">从内容列表打开话题后，基础稿和平台版本会显示在这里。</p><button class="primary-action" @click="goSection('topics')">返回内容</button>
      </section>
      <section v-if="workspaceModal ? workspaceModal==='accounts' : section==='accounts'" v-dialog-drag="workspaceModal==='accounts'" class="page-section accounts-page" :role="workspaceModal==='accounts'?'dialog':undefined" :aria-modal="workspaceModal==='accounts'?'true':undefined" aria-labelledby="accounts-title">
        <header class="management-head modal-titlebar">
          <div class="modal-title-label"><UserRound aria-hidden="true"/><h1 id="accounts-title">账号管理</h1></div>
          <button v-if="workspaceModal==='accounts'" type="button" class="workspace-modal-close" aria-label="关闭管理窗口" title="关闭" @click="closeWorkspacePanel"><X aria-hidden="true"/></button>
        </header>
        <div class="workspace-modal-body">
        <section class="profile-management" aria-labelledby="profile-management-title">
          <header><h2 id="profile-management-title">Profiles</h2><button type="button" class="profile-create-toggle" aria-label="新建分组" title="新建分组" aria-controls="profile-create-form" :aria-expanded="profileCreateOpen" @click="profileCreateOpen=!profileCreateOpen"><Plus aria-hidden="true"/></button></header>
          <form v-if="profileCreateOpen" id="profile-create-form" @submit.prevent="addProfile"><label><span>分组名称</span><input v-model="profileName" aria-label="新分组名称" placeholder="分组名称"></label><button class="primary-action"><Plus aria-hidden="true"/>添加</button></form>
          <ul class="profile-list"><li v-for="profile in profiles" :key="profile.id" class="profile-card" :class="{'is-expanded':expandedProfileID===profile.id}">
            <button type="button" class="profile-menu-trigger" :aria-current="profile.id===selectedProfileID?'true':undefined" :aria-label="`Profile 操作：${profile.name}`" :aria-expanded="expandedProfileID===profile.id" :aria-controls="`profile-menu-${profile.id}`" @click="expandedProfileID=expandedProfileID===profile.id?'':profile.id"><UserRound aria-hidden="true"/><span>{{profile.name}}</span><ChevronDown aria-hidden="true" class="inline-menu-chevron"/></button>
            <section v-if="expandedProfileID===profile.id" :id="`profile-menu-${profile.id}`" class="profile-inline-menu" :aria-label="`${profile.name} 账号列表`">
              <button type="button" class="profile-switch-action" :disabled="profile.id===selectedProfileID" @click="selectedProfileID=profile.id"><Check v-if="profile.id===selectedProfileID" aria-hidden="true"/><ArrowRight v-else aria-hidden="true"/>{{profile.id===selectedProfileID?'当前 Profile':'切换'}}</button>
              <ul class="profile-account-list"><li v-for="account in profileLinkedAccounts(profile.id)" :key="account.id"><PlatformIcon :platform="account.platform"/><span>{{account.name}}</span><small v-if="account.disabled">已停用</small></li></ul>
              <p v-if="!profileLinkedAccounts(profile.id).length" class="empty-state">还没有关联账号</p>
            </section>
          </li></ul>
        </section>
          <section class="connected-accounts" aria-labelledby="connected-accounts-title"><header class="connected-accounts-head"><h2 id="connected-accounts-title">账号池</h2><button type="button" class="primary-action account-create-toggle" aria-label="添加账号" title="添加账号" aria-controls="account-create-panel" :aria-expanded="accountCreateOpen" @click="toggleAccountCreate"><Plus aria-hidden="true"/></button></header>
<section v-show="accountCreateOpen" id="account-create-panel" class="account-create-panel" aria-label="连接新账号">
            <div class="account-create-content">
              <form class="account-setup" @submit.prevent="addAccount"><div class="account-identity-grid"><label><span>初始 Profile</span><select v-model="selectedProfileID" aria-label="新账号初始 Profile"><option v-for="profile in profiles" :key="profile.id" :value="profile.id">{{profile.name}}</option></select></label><label><span>账号名称</span><input v-model="accountName" placeholder="账号名称" :disabled="!selectedProfileID"></label><label><span>平台</span><select v-model="accountPlatform" @change="accountPlatformChanged" :disabled="!selectedProfileID"><option value="mastodon">Mastodon</option><option value="misskey">Misskey</option><option value="bluesky">Bluesky</option><option value="x">X</option><option value="weibo">微博</option><option value="xiaohongshu">小红书</option><option value="qzone">QQ 空间</option></select></label><label v-if="accountPlatform==='bluesky'"><span>远端身份</span><input v-model="remoteID" placeholder="handle / identifier"></label></div><label v-if="!isBridgePlatform(accountPlatform)" class="account-field"><span>实例地址</span><input v-model="baseURL" :placeholder="baseURLPlaceholder"></label><div v-if="accountPlatform==='mastodon' || accountPlatform==='misskey'" class="connection-inline"><button type="button" class="primary-action" @click="startConnection(accountPlatform)" :disabled="!selectedProfileID || !baseURL.trim()"><svg viewBox="0 0 20 20" aria-hidden="true"><path d="M7.5 12.5 12.5 7.5M6.5 6l-2 2a2.8 2.8 0 0 0 4 4l2-2M13.5 14l2-2a2.8 2.8 0 0 0-4-4l-2 2" fill="none" stroke="currentColor" stroke-linecap="round" stroke-width="1.4"/></svg>{{accountPlatform==='mastodon'?'OAuth 授权':'MiAuth 授权'}}</button></div><details v-if="accountPlatform==='mastodon' || accountPlatform==='misskey'" class="manual-connection"><summary>使用访问令牌</summary><div class="manual-account-row"><label><span>访问令牌</span><input v-model="accountSecret" type="password" autocomplete="new-password" placeholder="访问令牌"></label><details class="credential-advanced"><summary>使用环境变量</summary><input v-model="tokenEnv" :placeholder="tokenEnvPlaceholder"></details><button class="primary-action" :disabled="!selectedProfileID"><Plus aria-hidden="true"/>添加账号</button></div></details><div v-else-if="accountPlatform==='bluesky'" class="manual-account-row"><label><span>App password</span><input v-model="accountSecret" type="password" autocomplete="new-password" placeholder="app password"></label><details class="credential-advanced"><summary>使用环境变量</summary><input v-model="tokenEnv" :placeholder="tokenEnvPlaceholder"></details><button class="primary-action" :disabled="!selectedProfileID"><Plus aria-hidden="true"/>添加账号</button></div><button v-else-if="!isBridgePlatform(accountPlatform)" class="primary-action" :disabled="!selectedProfileID"><Plus aria-hidden="true"/>添加账号</button>
                <section v-if="isBridgePlatform(accountPlatform)" class="account-bridge-setup" aria-labelledby="account-bridge-title"><header><strong id="account-bridge-title">浏览器桥接</strong><span>{{accountSetupPairing ? pairingState(accountSetupPairing) : '未配对'}}</span></header><div class="account-bridge-actions"><a class="bridge-script-link" href="/userscript/bpm-bridge.user.js" target="_blank"><Download aria-hidden="true"/>安装脚本</a><button type="button" @click="createAccountBridgePairing"><KeyRound aria-hidden="true"/>{{accountSetupPairing ? '重新配对' : '生成配对码'}}</button><code v-if="accountSetupPairing?.code">{{accountSetupPairing.code}}</code><button v-if="accountSetupPairing?.code" type="button" @click="copyPairingCode(accountSetupPairing)"><Copy aria-hidden="true"/>复制</button></div><footer class="account-bridge-footer"><button class="primary-action" :disabled="!selectedProfileID"><Plus aria-hidden="true"/>添加账号</button></footer></section>
              </form>
            </div>
          </section>
        <ul class="account-list account-grid" aria-label="全局账号池">
          <li v-for="account in accounts" :key="account.id" class="account-card" :class="{'is-expanded':expandedAccountID===account.id}" :data-account-id="account.id">
            <button type="button" class="account-row-main account-menu-trigger" :aria-label="`账号操作：${account.name}`" :aria-expanded="expandedAccountID===account.id" :aria-controls="`account-menu-${account.id}`" @click="toggleAccountMenu(account)">
              <PlatformIcon class="account-platform-mark" :platform="account.platform" />
              <span class="account-row-identity"><strong>{{account.name}}</strong><small>{{platformLabel(account.platform)}}<template v-if="account.disabled"> · 已停用</template></small></span>
              <ChevronDown aria-hidden="true" class="inline-menu-chevron"/>
            </button>
            <section v-if="expandedAccountID===account.id" :id="`account-menu-${account.id}`" class="account-inline-menu" :aria-label="`${account.name} 操作`">
              <div class="row-actions account-inline-actions">
                <template v-if="!account.disabled">
                  <button type="button" aria-label="编辑账号" @click="beginEditAccount(account)"><Pencil aria-hidden="true"/>编辑</button>
                  <button v-if="account.platform==='bluesky' || isMastodon(account)" class="account-diagnostic-action" type="button" @click="account.platform==='bluesky'?diagnoseBluesky(account):diagnoseMastodon(account)" :aria-label="diagnosticIsBusy(account)?'正在测试连接':'测试连接'" :disabled="diagnosticIsBusy(account)"><RefreshCw aria-hidden="true"/>{{diagnosticIsBusy(account)?'检测中':'测试连接'}}</button>
                  <button type="button" aria-label="停用账号" @click="removeAccount(account)"><Power aria-hidden="true"/>停用</button>
                </template>
                <template v-else>
                  <button type="button" aria-label="重新连接账号" @click="prepareReconnect(account)"><RefreshCw aria-hidden="true"/>重新连接</button>
                  <button type="button" aria-label="永久删除本地账号数据" @click="permanentlyDeleteAccount(account)"><Trash2 aria-hidden="true"/>删除</button>
                </template>
              </div>
              <ul class="account-memberships" aria-label="已关联 Profile"><li v-for="profile in profiles.filter(item=>accountBelongsToProfile(account,item.id))" :key="profile.id"><Check aria-hidden="true"/>{{profile.name}}</li></ul>
              <form v-if="!account.disabled && unlinkedProfiles(account).length" class="account-profile-link" @submit.prevent="linkAccountFromPool(account)">
                <label><span>关联 Profile</span><select v-model="accountLinkProfileID" aria-label="关联到 Profile" :disabled="accountLinkBusy"><option v-for="profile in unlinkedProfiles(account)" :key="profile.id" :value="profile.id">{{profile.name}}</option></select></label>
                <button type="submit" class="account-link-action" :disabled="!accountLinkProfileID || accountLinkBusy"><Link2 aria-hidden="true"/>{{accountLinkBusy?'关联中…':'关联'}}</button>
              </form>
              <form v-if="editingAccountID===account.id" class="account-edit" @submit.prevent="updateAccount(account)">
                <label>名称<input v-model="editAccountName" aria-label="账号名称"></label>
                <label v-if="account.platform==='bluesky'">handle / DID<input v-model="editRemoteID" aria-label="远端身份"></label>
                <label v-if="!isBridgePlatform(account.platform)">服务地址<input v-model="editBaseURL" aria-label="账号服务地址"></label>
                <label v-if="!isBridgePlatform(account.platform)">新凭据<input v-model="editAccountSecret" type="password" autocomplete="new-password"></label>
                <details v-if="!isBridgePlatform(account.platform)" class="credential-advanced"><summary>环境变量</summary><input v-model="editTokenEnv" placeholder="环境变量名"></details>
                <div class="row-actions"><button type="submit" class="primary-action"><Save aria-hidden="true"/>保存</button><button type="button" @click="cancelEditAccount"><X aria-hidden="true"/>取消</button></div>
              </form>
              <div v-if="account.platform==='bluesky' && blueskyDiagnostic[account.id]" class="diagnostic-result" :class="blueskyDiagnostic[account.id].ok?'is-ok':'is-error'">{{blueskyDiagnostic[account.id].message}}<small v-if="blueskyDiagnostic[account.id].hint">{{blueskyDiagnostic[account.id].hint}}</small></div>
              <div v-if="isMastodon(account) && mastodonDiagnostic[account.id]" class="diagnostic-result" :class="mastodonDiagnostic[account.id].ok?'is-ok':'is-error'">{{mastodonDiagnostic[account.id].message}}</div>
            </section>
          </li>
        </ul>
        <p v-if="!accounts.length" class="empty-state">还没有账号。</p>
        </section>
              </div>
      </section>
      <section v-else-if="workspaceModal ? workspaceModal==='settings' : section==='settings'" v-dialog-drag="workspaceModal==='settings'" class="page-section settings-page" :role="workspaceModal==='settings'?'dialog':undefined" :aria-modal="workspaceModal==='settings'?'true':undefined" aria-labelledby="settings-title">
        <header class="modal-titlebar"><div class="modal-title-label"><Settings aria-hidden="true"/><h1 id="settings-title">设置</h1></div><button v-if="workspaceModal==='settings'" type="button" class="workspace-modal-close" aria-label="关闭设置窗口" title="关闭" @click="closeWorkspacePanel"><X aria-hidden="true"/></button></header>
        <div class="workspace-modal-body">
        <section class="settings-block translation-settings" aria-labelledby="translation-settings-title"><header class="settings-block-head"><h2 id="translation-settings-title">翻译服务</h2></header><form class="translation-config-form" @submit.prevent="addTranslationConfig"><label><span>服务</span><select v-model="translationProvider" aria-label="翻译服务"><option value="openai">OpenAI-compatible</option><option value="deepl">DeepL</option><option value="google">Google Cloud</option><option value="google-gtx">Google 翻译</option></select></label><label v-if="translationProvider!=='google-gtx'"><span>密钥环境变量</span><input v-model="translationCredentialEnv" aria-label="密钥环境变量" placeholder="环境变量名"></label><label><span>API 地址</span><input v-model="translationBaseURL" aria-label="自定义 API 地址" placeholder="默认服务地址"></label><label v-if="translationProvider!=='google-gtx'"><span>{{translationProvider==='google'?'Project ID':'模型'}}</span><input v-model="translationProjectOrModel" :aria-label="translationProvider==='google'?'Google Project ID':'翻译模型'" :placeholder="translationProvider==='google'?'Project ID':'默认模型'"></label><button class="translation-config-submit"><Save aria-hidden="true"/>保存翻译配置</button></form><ul v-if="translationConfigs.length" class="translation-config-list" aria-label="已保存翻译配置"><li v-for="config in translationConfigs" :key="config.id"><strong>{{translationProviderName(config.provider)}}</strong><span v-if="config.model">模型：{{config.model}}</span><span v-if="config.project_id">Project ID：{{config.project_id}}</span><span v-if="config.location">位置：{{config.location}}</span><span v-if="config.base_url">API：{{config.base_url}}</span><span v-if="config.token_env || config.key_env">凭据环境变量：{{config.token_env || config.key_env}}</span></li></ul></section>
        <section class="settings-block bridge-settings" aria-labelledby="bridge-settings-title"><header class="settings-block-head bridge-settings-head"><h2 id="bridge-settings-title">浏览器桥接</h2><a class="bridge-script-link" href="/userscript/bpm-bridge.user.js" target="_blank"><Download aria-hidden="true"/>安装脚本</a></header><div class="tool-content"><div class="actions bridge-controls"><select v-model="bridgePlatform" aria-label="桥接平台"><option value="x">X</option><option value="weibo">微博</option><option value="xiaohongshu">小红书</option><option value="qzone">QQ 空间</option></select><button type="button" @click="createBridgePairing"><KeyRound aria-hidden="true"/>生成配对码</button><button type="button" @click="refreshBridgeState"><RefreshCw aria-hidden="true"/>刷新状态</button></div><ul v-if="bridgePairings.length" class="rows bridge-pairings"><li v-for="pairing in bridgePairings" :key="pairing.id"><strong>{{platformLabel(pairing.platform)}}：{{pairingState(pairing)}}</strong><code v-if="pairing.code">{{pairing.code}}</code><button v-if="pairing.code" type="button" @click="copyPairingCode(pairing)">复制</button><button v-if="!pairing.revoked" type="button" @click="revokeBridgePairing(pairing)">撤销</button></li></ul><p v-else class="empty-state">尚未配对</p></div></section>
              </div>
      </section>
      <section v-else-if="workspaceModal ? workspaceModal==='history' : section==='history'" v-dialog-drag="workspaceModal==='history'" class="history-modal-host" :role="workspaceModal==='history'?'dialog':undefined" :aria-modal="workspaceModal==='history'?'true':undefined" aria-labelledby="publication-center-title"><header v-if="workspaceModal==='history'" class="modal-titlebar"><div class="modal-title-label"><ScrollText aria-hidden="true"/><h1 id="publication-center-title">发布中心</h1></div><button type="button" class="workspace-modal-close" aria-label="关闭发布中心" title="关闭发布中心" @click="closeWorkspacePanel"><X aria-hidden="true"/></button></header><div class="workspace-modal-body"><PublicationCenterView :profile-id="selectedProfileID" @notify="showToast" @changed="refreshPublicationSummary" /></div></section>
    </main>
    <nav class="utility-dock" aria-label="工作区工具">
      <button type="button" :class="{active:workspaceModal==='accounts'}" title="管理" aria-label="管理" @click="openWorkspacePanel('accounts')"><UserRound aria-hidden="true"/></button>
      <button type="button" :class="{active:workspaceModal==='settings'}" title="设置" aria-label="设置" @click="openWorkspacePanel('settings')"><Settings aria-hidden="true"/></button>
      <button type="button" :class="{active:workspaceModal==='history'}" title="发布记录" aria-label="发布记录" @click="openWorkspacePanel('history')"><ScrollText aria-hidden="true"/><i v-if="publicationActionCount" :class="{failure:publicationHasFailures}"></i></button>
    </nav>
    <section v-if="translationDialogOpen && activeVariant" ref="translationDialogEl" v-dialog-drag class="translation-dialog" role="dialog" aria-labelledby="translation-dialog-title"><header class="translation-dialog-header"><div><Languages aria-hidden="true"/><h2 id="translation-dialog-title">翻译</h2></div><button type="button" class="translation-dialog-close" aria-label="关闭翻译" title="关闭" @click="closeTranslationDialog"><X aria-hidden="true"/></button></header><div class="translation-dialog-body"><section class="translation-tool" aria-label="翻译此版本"><label><span>目标账号</span><select v-model="translationTargetAccountID" aria-label="翻译目标账号"><option v-for="account in profileAccounts" :key="account.id" :value="account.id">{{account.name}}</option></select></label><label><span>源语言</span><input v-model="translationSourceLanguage" placeholder="自动识别"></label><label><span>目标语言</span><input v-model="translationTargetLanguage" placeholder="例如 zh"></label><label><span>翻译配置</span><select v-model="translationConfigID" aria-label="翻译配置"><option value="">选择翻译配置</option><option v-for="config in translationConfigs" :key="config.id" :value="config.id">{{config.provider}} · {{config.model || config.project_id || config.id}}</option></select></label></section></div><footer class="translation-dialog-actions"><button type="button" class="translation-generate" @click="translateActiveVariant" :disabled="translationBusy || !translationConfigID || !translationTargetAccountID || !translationTargetLanguage.trim()"><Languages aria-hidden="true"/><span>{{translationBusy?'翻译中…':'生成版本'}}</span></button></footer></section>
    <PostPreviewDialog v-if="postPreviewOpen && activeVariant && variantAccount(activeVariant)" :account-name="variantAccount(activeVariant)!.name" :platform="variantAccount(activeVariant)!.platform" :platform-name="platformLabel(variantAccount(activeVariant)!.platform)" :text="activeVariant.text" :spoiler-text="activeVariant.spoiler_text" :visibility="activeVariant.visibility" :local-only="activeVariant.local_only" :media="currentMedia" @close="closePostPreview" />
    <div v-if="mediaDialogOpen" class="media-dialog-backdrop" role="presentation" @click.self="closeMediaDialog"><section v-dialog-drag class="media-dialog" role="dialog" aria-modal="true" aria-labelledby="media-dialog-title"><header><div><Image aria-hidden="true"/><h2 id="media-dialog-title">添加图片</h2></div><button type="button" class="icon-button" aria-label="关闭" title="关闭" @click="closeMediaDialog"><X aria-hidden="true"/></button></header><form class="media-dialog-body" @submit.prevent><div class="media-dialog-fields"><label><span>适用范围</span><select v-model="mediaVariantID" aria-label="图片适用范围"><option value="">所有平台版本</option><option v-for="variant in editableVariants" :key="variant.id" :value="variant.id">仅 {{variantAccountLabel(variant)}}</option></select></label><label><span>替代文本</span><input v-model="altText" placeholder="描述图片内容"></label></div><p v-if="pendingMediaFile" class="pending-media-note">待上传：{{pendingMediaFile.name}}</p><footer><button type="button" @click="closeMediaDialog">取消</button><button type="button" class="primary-action upload" @click="mediaFileInput?.click()"><Image aria-hidden="true"/><span>选择图片</span></button><input ref="mediaFileInput" type="file" class="sr-only" tabindex="-1" aria-label="选择图片文件" accept="image/*" @change="uploadMedia"></footer></form></section></div>
    <div v-if="mediaAltDialogOpen" class="media-alt-backdrop" role="presentation" @click.self="closeMediaAltDialog"><section v-dialog-drag class="media-alt-dialog" role="dialog" aria-modal="true" aria-labelledby="media-alt-title"><header><div><Image aria-hidden="true"/><h2 id="media-alt-title">ALT</h2></div><button type="button" class="media-alt-close" aria-label="关闭" title="关闭" @click="closeMediaAltDialog"><X aria-hidden="true"/></button></header><form @submit.prevent="saveMediaAltDialog"><textarea ref="mediaAltTextarea" v-model="mediaAltDraft" aria-label="图片替代文本"></textarea><p v-if="mediaAltError" class="media-alt-error" role="alert">{{mediaAltError}}</p><footer><button type="submit" class="primary-action"><Check aria-hidden="true"/><span>保存</span></button></footer></form></section></div>

    <div v-if="publishDialogOpen" class="modal-backdrop publish-dialog-backdrop" role="presentation" @click.self="closePublishDialog"><section ref="publishDialogEl" v-dialog-drag class="publish-dialog" role="dialog" aria-modal="true" aria-labelledby="publish-dialog-title"><header><h2 id="publish-dialog-title">发布 {{publishSelectedIDs.length}} 个版本</h2><button type="button" class="icon-button" aria-label="返回编辑" title="返回编辑" @click="closePublishDialog">×</button></header><div class="publish-dialog-content"><section class="publish-dialog-checks" aria-label="发布前检查"><h3>发布前检查 <span>{{preflightSummary}}</span></h3><ul class="preflight-list"><li v-for="check in preflightChecks" :key="check.title" :class="`preflight-${check.tone}`"><span class="check-icon" aria-hidden="true">{{check.tone === 'ok' ? '✓' : check.tone === 'warning' ? '!' : '×'}}</span><span class="check-copy"><strong>{{check.title}}</strong><small>{{check.detail}}</small></span><span class="check-result">{{check.result}}</span></li></ul></section><details v-if="activeVariant && variantCapability(activeVariant)?.supports_cw" class="publish-dialog-options"><summary>发布设置</summary><div class="variant-options"><label v-if="variantCapability(activeVariant)?.supports_cw"><span>内容警告</span><input v-model="activeVariant.spoiler_text" placeholder="内容警告（可选）" :disabled="variantLocked(activeVariant)"></label></div></details><ul class="dialog-targets"><li v-for="variant in publishCandidates" :key="variant.id" :class="{blocked:Boolean(variantBlockingReason(variant))}"><label><input v-model="publishSelectedIDs" type="checkbox" :value="variant.id" :disabled="Boolean(variantBlockingReason(variant))"><PlatformIcon :platform="variantAccount(variant)?.platform" /><span><strong>{{variantAccountLabel(variant)}}</strong><small>{{visibilityLabel(variant, variant.visibility)}} · {{mediaForVariant(variant).length}} 张图片</small></span><em>{{publishTargetState(variant)}}</em></label><label v-if="canAutoPublish(variantAccount(variant)?.platform||'') && publishSelectedIDs.includes(variant.id)" class="dialog-auto-publish"><input v-model="publishAutoIDs" type="checkbox" :value="variant.id">填入后自动发布</label><article v-if="publishSelectedIDs.includes(variant.id)" class="dialog-post-preview"><pre>{{variant.text}}</pre><div v-if="mediaForVariant(variant).length" class="dialog-media" :class="`media-count-${Math.min(mediaForVariant(variant).length, 4)}`"><img v-for="item in mediaForVariant(variant).slice(0, 4)" :key="item.id" :src="`/media/${item.id}`" :alt="item.alt_text || item.filename"><span v-if="mediaForVariant(variant).length>4" class="media-more">+{{mediaForVariant(variant).length-4}}</span></div></article></li></ul></div><footer><button type="button" @click="closePublishDialog">返回编辑</button><button type="button" class="primary-action" :disabled="publishDialogBusy || !publishSelectedIDs.length" @click="confirmPublish">{{publishDialogBusy?'处理中…':`确认发布 ${publishSelectedIDs.length} 个版本`}}</button></footer></section></div>
    <aside v-if="publishTask.length" class="publish-taskbar" aria-live="polite"><strong>本次发布</strong><span v-for="task in publishTask" :key="task.id" :class="`task-${task.state}`" :title="task.detail">{{task.label}} {{task.state==='done'?'✓':task.state==='waiting'?'待确认':task.state==='failed'?'失败':'…'}}<small v-if="task.detail">{{task.detail}}</small></span><a href="/history" @click="followSectionLink($event, 'history')">查看详情</a><button type="button" aria-label="关闭发布进度" @click="publishTask=[]">×</button></aside>
  </div>
</template>
