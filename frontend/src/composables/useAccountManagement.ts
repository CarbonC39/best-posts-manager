import { computed, ref, watch, type Ref } from 'vue'
import {
  attachAccountToProfile,
  createAccount,
  deactivateAccount,
  deleteAccount,
  diagnoseBlueskyAccount,
  diagnoseMastodonAccount,
  updateAccountConnection,
} from '../api/accounts'
import { accountBelongsToProfile, isMastodon, type Account, type BlueskyDiagnostic, type MastodonDiagnostic } from '../domain/account'
import { isBridgePlatform } from '../platformRegistry'

type Options = {
  accounts: Ref<Account[]>
  selectedProfileID: Ref<string>
  reloadWorkspace: () => Promise<boolean>
  ensureAccountCapability: (accountID: string) => Promise<void>
  notify: (message: string) => void
  notifyConnection: (message: string) => void
}

export function useAccountManagement({ accounts, selectedProfileID, reloadWorkspace, ensureAccountCapability, notify, notifyConnection }: Options) {
  const accountName = ref('')
  const accountCreateOpen = ref(false)
  const baseURL = ref('')
  const tokenEnv = ref('')
  const accountSecret = ref('')
  const remoteID = ref('')
  const accountPlatform = ref('mastodon')
  let previousAccountPlatform = accountPlatform.value

  const editingAccountID = ref('')
  const editAccountName = ref('')
  const editBaseURL = ref('')
  const editRemoteID = ref('')
  const editTokenEnv = ref('')
  const editAccountSecret = ref('')

  const blueskyDiagnostic = ref<Record<string, BlueskyDiagnostic>>({})
  const mastodonDiagnostic = ref<Record<string, MastodonDiagnostic>>({})
  const diagnosticBusy = ref<Set<string>>(new Set())
  const diagnosticEpoch = new Map<string, number>()

  const profileAccounts = computed(() => accounts.value.filter((account) => accountBelongsToProfile(account, selectedProfileID.value) && !account.disabled))
  const availableProfileAccounts = computed(() => accounts.value.filter((account) => !account.disabled && !accountBelongsToProfile(account, selectedProfileID.value)))
  const disabledProfileAccounts = computed(() => accounts.value.filter((account) => accountBelongsToProfile(account, selectedProfileID.value) && account.disabled))

  function showError(error: unknown) {
    notify(`操作失败：${error instanceof Error ? error.message : String(error)}`)
  }

  function clearCreateCredentials() {
    tokenEnv.value = ''
    accountSecret.value = ''
  }

  function clearEditCredentials() {
    editTokenEnv.value = ''
    editAccountSecret.value = ''
  }

  function clearAccountCredentials() {
    clearCreateCredentials()
    clearEditCredentials()
  }

  watch(accountCreateOpen, (open) => { if (!open) clearCreateCredentials() })

  function toggleAccountCreate() {
    accountCreateOpen.value = !accountCreateOpen.value
    if (!accountCreateOpen.value) clearCreateCredentials()
  }

  function diagnosticIsBusy(account: Account) { return diagnosticBusy.value.has(account.id) }
  function setDiagnosticBusy(account: Account, busy: boolean) {
    const next = new Set(diagnosticBusy.value)
    if (busy) next.add(account.id)
    else next.delete(account.id)
    diagnosticBusy.value = next
  }

  function clearDiagnostic(accountID: string) {
    const nextBluesky = { ...blueskyDiagnostic.value }
    const nextMastodon = { ...mastodonDiagnostic.value }
    delete nextBluesky[accountID]
    delete nextMastodon[accountID]
    blueskyDiagnostic.value = nextBluesky
    mastodonDiagnostic.value = nextMastodon
    const nextBusy = new Set(diagnosticBusy.value)
    nextBusy.delete(accountID)
    diagnosticBusy.value = nextBusy
  }

  function invalidateDiagnostic(accountID: string) {
    diagnosticEpoch.set(accountID, (diagnosticEpoch.get(accountID) || 0) + 1)
  }

  function upsertAccount(updated: Account) {
    const existing = accounts.value.find((account) => account.id === updated.id)
    if (existing) Object.assign(existing, updated)
    else accounts.value.push(updated)
  }

  function removeLocalAccount(accountID: string) {
    accounts.value = accounts.value.filter((account) => account.id !== accountID)
  }

  async function reloadAfterMutation(reconcile: () => void) {
    reconcile()
    let refreshed = false
    try { refreshed = await reloadWorkspace() }
    catch (error) { notify(`刷新工作区失败：${error instanceof Error ? error.message : String(error)}`) }
    reconcile()
    return refreshed
  }

  function accountPlatformChanged() {
    if (accountPlatform.value === previousAccountPlatform) return
    previousAccountPlatform = accountPlatform.value
    remoteID.value = ''
    baseURL.value = accountPlatform.value === 'bluesky' ? 'https://bsky.social' : ''
    tokenEnv.value = ''
    accountSecret.value = ''
    notifyConnection('')
  }

  async function diagnoseBluesky(account: Account) {
    if (account.platform !== 'bluesky') return
    if (diagnosticBusy.value.has(account.id)) return
    const epoch = diagnosticEpoch.get(account.id) || 0
    setDiagnosticBusy(account, true)
    try {
      const result = await diagnoseBlueskyAccount(account.id)
      if ((diagnosticEpoch.get(account.id) || 0) !== epoch) return
      blueskyDiagnostic.value[account.id] = result
      notifyConnection(result.ok
        ? `Bluesky 连接成功：@${result.handle || account.remote_id}（${result.elapsed_ms} ms）`
        : `Bluesky 连接失败［${result.stage}/${result.category || 'unknown'}］：${result.message}${result.hint ? `。建议：${result.hint}` : ''}`)
    } catch (error) {
      if ((diagnosticEpoch.get(account.id) || 0) !== epoch) return
      notifyConnection(`Bluesky 诊断请求失败：${error instanceof Error ? error.message : String(error)}。请查看 bpm 进程日志。`)
    } finally {
      if ((diagnosticEpoch.get(account.id) || 0) === epoch) setDiagnosticBusy(account, false)
    }
  }

  async function diagnoseMastodon(account: Account) {
    if (!isMastodon(account)) return
    if (diagnosticBusy.value.has(account.id)) return
    const epoch = diagnosticEpoch.get(account.id) || 0
    setDiagnosticBusy(account, true)
    try {
      const result = await diagnoseMastodonAccount(account.id)
      if ((diagnosticEpoch.get(account.id) || 0) !== epoch) return
      mastodonDiagnostic.value[account.id] = result
      notifyConnection(result.ok
        ? `${result.category ? 'Mastodon 检测完成（有警告）' : 'Mastodon 连接成功'}：@${result.username || account.name}（${result.elapsed_ms} ms）${result.hint ? `。${result.hint}` : ''}${result.status_write?.message ? ` · ${result.status_write.message}` : ''}${result.media_write?.message ? ` · ${result.media_write.message}` : ''}`
        : `Mastodon 连接失败［${result.stage}/${result.category || 'unknown'}］：${result.message}${result.hint ? `。建议：${result.hint}` : ''}`)
    } catch (error) {
      if ((diagnosticEpoch.get(account.id) || 0) !== epoch) return
      notifyConnection(`Mastodon 诊断请求失败：${error instanceof Error ? error.message : String(error)}。请查看 bpm 进程日志。`)
    } finally {
      if ((diagnosticEpoch.get(account.id) || 0) === epoch) setDiagnosticBusy(account, false)
    }
  }

  async function addAccount() {
    if (!selectedProfileID.value) { notify('请先选择 Profile'); return }
    if (!accountName.value.trim()) { notify('请填写账号名称'); return }
    if (!isBridgePlatform(accountPlatform.value) && accountPlatform.value !== 'bluesky' && !baseURL.value.trim()) { notify('请填写平台实例 URL'); return }
    if (!isBridgePlatform(accountPlatform.value) && !tokenEnv.value.trim() && !accountSecret.value.trim()) { notify('请输入 access token/app password，或展开高级选项填写环境变量名'); return }
    if (tokenEnv.value.trim() && accountSecret.value.trim()) { notify('本地凭据和环境变量只能选择一种'); return }
    if (accountPlatform.value === 'bluesky' && !remoteID.value.trim()) { notify('请填写 Bluesky handle/identifier'); return }
    let created: Account
    try {
      created = await createAccount({
        profile_id: selectedProfileID.value,
        name: accountName.value,
        platform: accountPlatform.value,
        remote_id: remoteID.value,
        base_url: baseURL.value,
        token_env: tokenEnv.value,
        secret: accountSecret.value,
      })
    } catch (error) { showError(error); return }
    accountName.value = ''; baseURL.value = accountPlatform.value === 'bluesky' ? 'https://bsky.social' : ''; clearCreateCredentials(); remoteID.value = ''
    const refreshed = await reloadAfterMutation(() => upsertAccount(created))
    try { await ensureAccountCapability(created.id) }
    catch (error) { notify(`刷新账号能力失败：${error instanceof Error ? error.message : String(error)}`); return }
    if (!refreshed) return
    notify('账号已添加')
    const savedAccount = accounts.value.find((account) => account.id === created.id)
    if (savedAccount?.platform === 'bluesky') await diagnoseBluesky(savedAccount)
    if (savedAccount && isMastodon(savedAccount)) await diagnoseMastodon(savedAccount)
  }

  async function attachAccount(account: Account, profileID = selectedProfileID.value) {
    if (!profileID) return false
    try {
      const updated = await attachAccountToProfile(profileID, account.id)
      Object.assign(account, updated)
      notify('账号已关联')
      return true
    } catch (error) { showError(error); return false }
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
    if (!editAccountName.value.trim()) { notify('账号名称不能为空'); return }
    if (editTokenEnv.value.trim() && editAccountSecret.value.trim()) { notify('新凭据和环境变量只能选择一种'); return }
    try {
      const updated = await updateAccountConnection(account.id, {
        name: editAccountName.value,
        remote_id: editRemoteID.value,
        base_url: editBaseURL.value,
        token_env: editTokenEnv.value,
        secret: editAccountSecret.value,
      })
      Object.assign(account, updated)
      cancelEditAccount()
      notify('账号连接信息已更新')
      if (account.platform === 'bluesky') await diagnoseBluesky(account)
      if (isMastodon(account)) await diagnoseMastodon(account)
    } catch (error) { showError(error) }
  }

  async function removeAccount(account: Account) {
    const otherProfiles = Math.max(0, (account.profile_ids?.length || 1) - 1)
    const sharedWarning = otherProfiles ? `该连接还被 ${otherProfiles} 个其他 Profile 使用，停用会同时影响它们。` : ''
    if (!window.confirm(`停用账号“${account.name}”？${sharedWarning}历史版本和发布记录会保留。`)) return
    invalidateDiagnostic(account.id)
    try {
      const deactivated = await deactivateAccount(account.id)
      clearDiagnostic(account.id)
      const refreshed = await reloadAfterMutation(() => upsertAccount(deactivated))
      if (refreshed) notify('账号已停用，本地凭据已移除；历史记录仍保留')
    } catch (error) { showError(error) }
  }

  async function permanentlyDeleteAccount(account: Account) {
    if (!window.confirm(`永久删除账号“${account.name}”的本地版本、发布记录和关联？不会删除远端帖子。此操作不可恢复。`)) return
    invalidateDiagnostic(account.id)
    try {
      await deleteAccount(account.id)
      clearDiagnostic(account.id)
      const refreshed = await reloadAfterMutation(() => removeLocalAccount(account.id))
      if (refreshed) notify('账号及其本地关联已永久删除，远端帖子未受影响')
    } catch (error) { showError(error) }
  }

  function prepareReconnect(account: Account) {
    accountCreateOpen.value = true
    accountName.value = account.name
    accountPlatform.value = account.platform
    previousAccountPlatform = account.platform
    remoteID.value = account.remote_id || ''
    baseURL.value = account.base_url || ''
    tokenEnv.value = ''
    accountSecret.value = ''
    notifyConnection('')
    notify(account.remote_id ? '已填入停用账号信息；重新授权或手动添加后会恢复原账号及历史关联' : '该账号缺少可识别的远端 ID；请重新添加连接，历史记录仍保留在停用账号下')
  }

  return {
    accountName, accountCreateOpen, baseURL, tokenEnv, accountSecret, remoteID, accountPlatform,
    editingAccountID, editAccountName, editBaseURL, editRemoteID, editTokenEnv, editAccountSecret,
    blueskyDiagnostic, mastodonDiagnostic, diagnosticBusy,
    profileAccounts, availableProfileAccounts, disabledProfileAccounts,
    accountPlatformChanged, diagnosticIsBusy, addAccount, attachAccount, beginEditAccount, cancelEditAccount,
    updateAccount, diagnoseBluesky, diagnoseMastodon, removeAccount, permanentlyDeleteAccount, prepareReconnect,
    clearCreateCredentials, clearEditCredentials, clearAccountCredentials, toggleAccountCreate,
  }
}
