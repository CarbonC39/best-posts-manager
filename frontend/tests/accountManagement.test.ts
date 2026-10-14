import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick, ref } from 'vue'
import type { Account } from '../src/domain/account'

const accountAPI = vi.hoisted(() => ({
  createAccount: vi.fn(), attachAccountToProfile: vi.fn(), updateAccountConnection: vi.fn(),
  diagnoseBlueskyAccount: vi.fn(), diagnoseMastodonAccount: vi.fn(), deactivateAccount: vi.fn(), deleteAccount: vi.fn(),
}))
vi.mock('../src/api/accounts', () => accountAPI)

import { useAccountManagement } from '../src/composables/useAccountManagement'

function makeAccount(overrides: Partial<Account> = {}): Account {
  return { id: 'a1', profile_id: 'p1', profile_ids: ['p1'], platform: 'bluesky', name: 'Main', remote_id: 'me.test', base_url: 'https://bsky.social', ...overrides }
}

function setup(initial: Account[] = []) {
  const accounts = ref(initial)
  const selectedProfileID = ref('p1')
  const reloadWorkspace = vi.fn(async () => true)
  const ensureAccountCapability = vi.fn(async () => {})
  const notices: string[] = []
  const connectionNotices: string[] = []
  const manager = useAccountManagement({
    accounts, selectedProfileID, reloadWorkspace, ensureAccountCapability,
    notify: (message) => notices.push(message),
    notifyConnection: (message) => connectionNotices.push(message),
  })
  return { accounts, selectedProfileID, reloadWorkspace, ensureAccountCapability, notices, connectionNotices, manager }
}

describe('account management', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.spyOn(window, 'confirm').mockReturnValue(true)
  })

  it('uses the shared account ref and computes profile membership', () => {
    const state = setup([makeAccount(), makeAccount({ id: 'a2', profile_id: 'old', profile_ids: ['p2'], name: 'Other' })])
    expect(state.manager.profileAccounts.value.map(account => account.id)).toEqual(['a1'])
    state.accounts.value.push(makeAccount({ id: 'a3', name: 'New' }))
    expect(state.manager.profileAccounts.value.map(account => account.id)).toEqual(['a1', 'a3'])
    expect(state.manager.availableProfileAccounts.value.map(account => account.id)).toEqual(['a2'])
  })

  it('validates creation, sends the unchanged body, reloads and clears fields on success', async () => {
    const state = setup()
    await state.manager.addAccount()
    expect(state.notices.at(-1)).toBe('请填写账号名称')

    state.manager.accountName.value = 'Sky'
    state.manager.accountPlatform.value = 'bluesky'
    state.manager.accountPlatformChanged()
    state.manager.remoteID.value = 'me.test'
    state.manager.accountSecret.value = 'app-pass'
    const created = makeAccount({ name: 'Sky' })
    accountAPI.createAccount.mockResolvedValue(created)
    accountAPI.diagnoseBlueskyAccount.mockResolvedValue({ ok: true, stage: 'done', message: 'ok', handle: 'me.test', elapsed_ms: 4 })
    await state.manager.addAccount()

    expect(accountAPI.createAccount).toHaveBeenCalledWith({ profile_id: 'p1', name: 'Sky', platform: 'bluesky', remote_id: 'me.test', base_url: 'https://bsky.social', token_env: '', secret: 'app-pass' })
    expect(state.reloadWorkspace).toHaveBeenCalledOnce()
    expect(state.ensureAccountCapability).toHaveBeenCalledWith(created.id)
    expect(state.accounts.value.map(account => account.id)).toContain(created.id)
    expect(state.manager.accountSecret.value).toBe('')
    expect(state.manager.baseURL.value).toBe('https://bsky.social')
    expect(state.notices).toContain('账号已添加')
  })

  it('retains creation input after a failed request for retry', async () => {
    const state = setup()
    state.manager.accountName.value = 'Bridge'
    state.manager.accountPlatform.value = 'x'
    state.manager.accountPlatformChanged()
    accountAPI.createAccount.mockRejectedValue(new Error('offline'))
    await state.manager.addAccount()
    expect(state.manager.accountName.value).toBe('Bridge')
    expect(state.manager.accountPlatform.value).toBe('x')
    expect(state.notices.at(-1)).toBe('操作失败：offline')
    expect(state.reloadWorkspace).not.toHaveBeenCalled()
  })

  it('reconciles a restored account when workspace reload leaves stale local data', async () => {
    const disabled = makeAccount({ disabled: true })
    const state = setup([disabled])
    state.reloadWorkspace.mockImplementation(async () => {
      state.accounts.value = [makeAccount({ disabled: true })]
      return true
    })
    state.manager.accountName.value = 'Restored'
    state.manager.accountPlatform.value = 'bluesky'
    state.manager.accountPlatformChanged()
    state.manager.remoteID.value = 'me.test'
    state.manager.accountSecret.value = 'app-pass'
    accountAPI.createAccount.mockResolvedValue({ ...disabled, name: 'Restored', disabled: false })
    accountAPI.diagnoseBlueskyAccount.mockResolvedValue({ ok: true, stage: 'done', message: 'ok', elapsed_ms: 1 })
    await state.manager.addAccount()
    expect(state.accounts.value).toHaveLength(1)
    expect(state.accounts.value[0]).toMatchObject({ id: disabled.id, name: 'Restored', disabled: false })
    expect(accountAPI.diagnoseBlueskyAccount).toHaveBeenCalledWith(disabled.id)
  })

  it('clears hidden connection fields only when the platform really changes', () => {
    const state = setup()
    state.manager.accountName.value = 'Keep name'
    state.manager.remoteID.value = 'old-id'
    state.manager.baseURL.value = 'https://old.test'
    state.manager.tokenEnv.value = 'TOKEN'
    state.manager.accountSecret.value = 'secret'
    state.manager.accountPlatformChanged()
    expect(state.manager.baseURL.value).toBe('https://old.test')

    state.manager.accountPlatform.value = 'bluesky'
    state.manager.accountPlatformChanged()
    expect(state.manager.accountName.value).toBe('Keep name')
    expect(state.manager.remoteID.value).toBe('')
    expect(state.manager.baseURL.value).toBe('https://bsky.social')
    expect(state.manager.tokenEnv.value).toBe('')
    expect(state.manager.accountSecret.value).toBe('')
    expect(state.connectionNotices.at(-1)).toBe('')

    state.manager.remoteID.value = 'new-id'
    state.manager.baseURL.value = 'https://new-pds.test'
    state.manager.tokenEnv.value = 'NEW_TOKEN'
    state.manager.accountSecret.value = 'new-secret'
    state.manager.accountPlatform.value = 'mastodon'
    state.manager.accountPlatformChanged()
    expect(state.manager.remoteID.value).toBe('')
    expect(state.manager.baseURL.value).toBe('')
    expect(state.manager.tokenEnv.value).toBe('')
    expect(state.manager.accountSecret.value).toBe('')
  })

  it('clears only create credentials when the account panel closes', async () => {
    const state = setup()
    state.manager.toggleAccountCreate()
    state.manager.tokenEnv.value = 'TOKEN'
    state.manager.accountSecret.value = 'secret'
    state.manager.editTokenEnv.value = 'EDIT_TOKEN'
    state.manager.editAccountSecret.value = 'edit-secret'
    state.manager.toggleAccountCreate()
    await nextTick()
    expect(state.manager.tokenEnv.value).toBe('')
    expect(state.manager.accountSecret.value).toBe('')
    expect(state.manager.editTokenEnv.value).toBe('EDIT_TOKEN')
    expect(state.manager.editAccountSecret.value).toBe('edit-secret')
    state.manager.clearAccountCredentials()
    expect(state.manager.editTokenEnv.value).toBe('')
    expect(state.manager.editAccountSecret.value).toBe('')
  })

  it('opens and fills reconnect while clearing sensitive fields', () => {
    const state = setup()
    state.manager.tokenEnv.value = 'OLD'
    state.manager.accountSecret.value = 'secret'
    state.manager.prepareReconnect(makeAccount())
    expect(state.manager.accountCreateOpen.value).toBe(true)
    expect(state.manager.accountName.value).toBe('Main')
    expect(state.manager.accountSecret.value).toBe('')
    expect(state.manager.tokenEnv.value).toBe('')
  })

  it('updates account data without allowing a failed diagnostic to become a save failure', async () => {
    const account = makeAccount()
    const state = setup([account])
    state.manager.beginEditAccount(account)
    state.manager.editAccountName.value = 'Renamed'
    accountAPI.updateAccountConnection.mockResolvedValue({ ...account, name: 'Renamed' })
    accountAPI.diagnoseBlueskyAccount.mockRejectedValue(new Error('diagnostic offline'))
    await state.manager.updateAccount(account)
    expect(account.name).toBe('Renamed')
    expect(state.notices).toContain('账号连接信息已更新')
    expect(state.notices.some(message => message.includes('操作失败'))).toBe(false)
    expect(state.connectionNotices.at(-1)).toContain('诊断请求失败')
    expect(state.manager.editAccountSecret.value).toBe('')
    expect(state.manager.editTokenEnv.value).toBe('')
  })

  it('retains edit credentials after an update request fails so it can be retried', async () => {
    const account = makeAccount()
    const state = setup([account])
    state.manager.beginEditAccount(account)
    state.manager.editAccountName.value = 'Retry later'
    state.manager.editTokenEnv.value = 'NEW_TOKEN'
    accountAPI.updateAccountConnection.mockRejectedValue(new Error('offline'))
    await state.manager.updateAccount(account)
    expect(state.manager.editingAccountID.value).toBe(account.id)
    expect(state.manager.editAccountSecret.value).toBe('')
    expect(state.manager.editTokenEnv.value).toBe('NEW_TOKEN')
    expect(state.notices.at(-1)).toBe('操作失败：offline')
  })

  it('prevents duplicate diagnostics and replaces the busy Set', async () => {
    const account = makeAccount()
    const state = setup([account])
    let resolve!: (value: any) => void
    accountAPI.diagnoseBlueskyAccount.mockReturnValue(new Promise(done => { resolve = done }))
    const before = state.manager.diagnosticBusy.value
    const first = state.manager.diagnoseBluesky(account)
    const busy = state.manager.diagnosticBusy.value
    const second = state.manager.diagnoseBluesky(account)
    expect(busy).not.toBe(before)
    expect(accountAPI.diagnoseBlueskyAccount).toHaveBeenCalledOnce()
    resolve({ ok: true, stage: 'done', message: 'ok', elapsed_ms: 1 })
    await Promise.all([first, second])
    expect(state.manager.diagnosticBusy.value).not.toBe(busy)
    expect(state.manager.diagnosticIsBusy(account)).toBe(false)
  })

  it('honors deactivate cancellation, reloads mutations, reports failures, and deletes diagnostics', async () => {
    const account = makeAccount()
    const state = setup([account])
    vi.mocked(window.confirm).mockReturnValueOnce(false)
    await state.manager.removeAccount(account)
    expect(accountAPI.deactivateAccount).not.toHaveBeenCalled()

    vi.mocked(window.confirm).mockReturnValueOnce(true)
    accountAPI.deactivateAccount.mockResolvedValue({ ...account, disabled: true })
    await state.manager.removeAccount(account)
    expect(state.reloadWorkspace).toHaveBeenCalledOnce()
    expect(state.accounts.value[0].disabled).toBe(true)

    accountAPI.deleteAccount.mockRejectedValueOnce(new Error('locked'))
    await state.manager.permanentlyDeleteAccount(account)
    expect(state.notices.at(-1)).toBe('操作失败：locked')

    state.manager.blueskyDiagnostic.value[account.id] = { ok: true, stage: 'done', message: 'ok', elapsed_ms: 1 }
    accountAPI.deleteAccount.mockResolvedValue(account)
    await state.manager.permanentlyDeleteAccount(account)
    expect(state.reloadWorkspace).toHaveBeenCalledTimes(2)
    expect(state.accounts.value).toEqual([])
    expect(state.manager.blueskyDiagnostic.value[account.id]).toBeUndefined()
  })

  it('does not restore a diagnostic result that finishes after deactivation', async () => {
    const account = makeAccount()
    const state = setup([account])
    let resolveDiagnostic!: (value: any) => void
    accountAPI.diagnoseBlueskyAccount.mockReturnValue(new Promise(done => { resolveDiagnostic = done }))
    accountAPI.deactivateAccount.mockResolvedValue({ ...account, disabled: true })
    const diagnostic = state.manager.diagnoseBluesky(account)
    await state.manager.removeAccount(account)
    resolveDiagnostic({ ok: true, stage: 'done', message: 'late result', elapsed_ms: 1 })
    await diagnostic
    expect(state.manager.blueskyDiagnostic.value[account.id]).toBeUndefined()
    expect(state.connectionNotices).not.toContain(expect.stringContaining('late result'))
  })

  it('does not let an old diagnostic clear busy for a newer same-ID diagnostic', async () => {
    const account = makeAccount()
    const state = setup([account])
    let resolveOld!: (value: any) => void
    let resolveNew!: (value: any) => void
    accountAPI.diagnoseBlueskyAccount
      .mockReturnValueOnce(new Promise(done => { resolveOld = done }))
      .mockReturnValueOnce(new Promise(done => { resolveNew = done }))
    accountAPI.deactivateAccount.mockResolvedValue({ ...account, disabled: true })

    const oldDiagnostic = state.manager.diagnoseBluesky(account)
    await state.manager.removeAccount(account)
    const newDiagnostic = state.manager.diagnoseBluesky(account)
    expect(state.manager.diagnosticIsBusy(account)).toBe(true)
    resolveOld({ ok: true, stage: 'done', message: 'old', elapsed_ms: 1 })
    await oldDiagnostic
    expect(state.manager.diagnosticIsBusy(account)).toBe(true)
    resolveNew({ ok: true, stage: 'done', message: 'new', elapsed_ms: 1 })
    await newDiagnostic
    expect(state.manager.diagnosticIsBusy(account)).toBe(false)
    expect(state.manager.blueskyDiagnostic.value[account.id]?.message).toBe('new')
  })

  it('reapplies deactivation and deletion after a stale successful reload', async () => {
    const active = makeAccount()
    const deactivation = setup([active])
    deactivation.reloadWorkspace.mockImplementation(async () => {
      deactivation.accounts.value = [makeAccount()]
      return true
    })
    accountAPI.deactivateAccount.mockResolvedValue({ ...active, disabled: true })
    await deactivation.manager.removeAccount(active)
    expect(deactivation.accounts.value).toHaveLength(1)
    expect(deactivation.accounts.value[0].disabled).toBe(true)

    const disabled = makeAccount({ disabled: true })
    const deletion = setup([disabled])
    deletion.reloadWorkspace.mockImplementation(async () => {
      deletion.accounts.value = [makeAccount({ disabled: true })]
      return true
    })
    accountAPI.deleteAccount.mockResolvedValue(disabled)
    await deletion.manager.permanentlyDeleteAccount(disabled)
    expect(deletion.accounts.value).toEqual([])
  })

  it('ensures capability and preserves the refresh error after a successful add', async () => {
    const state = setup()
    state.manager.accountName.value = 'Bridge'
    state.manager.accountPlatform.value = 'x'
    state.manager.accountPlatformChanged()
    const created = makeAccount({ platform: 'x', name: 'Bridge' })
    accountAPI.createAccount.mockResolvedValue(created)
    state.reloadWorkspace.mockRejectedValue(new Error('refresh failed'))
    await state.manager.addAccount()
    expect(state.accounts.value.map(account => account.id)).toContain(created.id)
    expect(state.ensureAccountCapability).toHaveBeenCalledWith(created.id)
    expect(state.notices.at(-1)).toBe('刷新工作区失败：refresh failed')
    expect(state.notices).not.toContain('账号已添加')
  })

  it('keeps successful deactivation and deletion locally when workspace reload rejects', async () => {
    const deactivatedAccount = makeAccount()
    const deactivation = setup([deactivatedAccount])
    deactivation.reloadWorkspace.mockRejectedValue(new Error('refresh failed'))
    accountAPI.deactivateAccount.mockResolvedValue({ ...deactivatedAccount, disabled: true })
    await deactivation.manager.removeAccount(deactivatedAccount)
    expect(deactivation.accounts.value[0].disabled).toBe(true)
    expect(deactivation.notices.at(-1)).toBe('刷新工作区失败：refresh failed')

    const deletedAccount = makeAccount({ disabled: true })
    const deletion = setup([deletedAccount])
    deletion.reloadWorkspace.mockRejectedValue(new Error('refresh failed'))
    accountAPI.deleteAccount.mockResolvedValue(deletedAccount)
    await deletion.manager.permanentlyDeleteAccount(deletedAccount)
    expect(deletion.accounts.value).toEqual([])
    expect(deletion.notices.at(-1)).toBe('刷新工作区失败：refresh failed')
  })
})
