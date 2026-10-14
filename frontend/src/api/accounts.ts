import { api } from '../api'
import type { Account, BlueskyDiagnostic, MastodonDiagnostic } from '../domain/account'

export type CreateAccountInput = {
  profile_id: string
  name: string
  platform: string
  remote_id: string
  base_url: string
  token_env: string
  secret: string
}

export type UpdateAccountConnectionInput = {
  name: string
  remote_id: string
  base_url: string
  token_env: string
  secret: string
}

export function createAccount(input: CreateAccountInput): Promise<Account> {
  return api<Account>('accounts', { method: 'POST', body: JSON.stringify(input) })
}

export function attachAccountToProfile(profileID: string, accountID: string): Promise<Account> {
  return api<Account>('profile-accounts', { method: 'POST', body: JSON.stringify({ profile_id: profileID, account_id: accountID }) })
}

export function updateAccountConnection(accountID: string, input: UpdateAccountConnectionInput): Promise<Account> {
  return api<Account>(`accounts/${accountID}/connection`, { method: 'PUT', body: JSON.stringify(input) })
}

export function diagnoseBlueskyAccount(accountID: string): Promise<BlueskyDiagnostic> {
  return api<BlueskyDiagnostic>(`accounts/${accountID}/diagnostic`, { method: 'POST' })
}

export function diagnoseMastodonAccount(accountID: string): Promise<MastodonDiagnostic> {
  return api<MastodonDiagnostic>(`accounts/${accountID}/diagnostic`, { method: 'POST' })
}

export function deactivateAccount(accountID: string): Promise<Account> {
  return api<Account>(`accounts/${accountID}/deactivate`, { method: 'DELETE' })
}

export function deleteAccount(accountID: string): Promise<Account> {
  return api<Account>(`accounts/${accountID}`, { method: 'DELETE' })
}
