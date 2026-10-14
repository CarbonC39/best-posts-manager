export type Account = {
  id: string
  profile_id: string
  profile_ids?: string[]
  platform: string
  name: string
  remote_id: string
  base_url?: string
  token_env?: string
  disabled?: boolean
}

export type BlueskyDiagnostic = {
  ok: boolean
  stage: string
  category?: string
  message: string
  hint?: string
  handle?: string
  did?: string
  elapsed_ms: number
}

export type PermissionSignal = { endpoint: string; status: string; message: string }

export type MastodonDiagnostic = {
  ok: boolean
  stage: string
  category?: string
  message: string
  hint?: string
  username?: string
  account_id?: string
  status_write: PermissionSignal
  media_write: PermissionSignal
  elapsed_ms: number
}

export function accountBelongsToProfile(account: Account, profileID: string): boolean {
  const profileIDs = Array.isArray(account.profile_ids) ? account.profile_ids : [account.profile_id]
  return profileIDs.includes(profileID)
}

export function isMastodon(account?: Account): boolean {
  return account?.platform === 'mastodon' || account?.platform === 'mastodon-compatible'
}
