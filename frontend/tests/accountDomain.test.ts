import { describe, expect, it } from 'vitest'
import { accountBelongsToProfile, isMastodon, type Account } from '../src/domain/account'

function account(overrides: Partial<Account> = {}): Account {
  return { id: 'a1', profile_id: 'legacy', platform: 'mastodon', name: 'Main', remote_id: '', ...overrides }
}

describe('account domain', () => {
  it('prefers profile_ids membership and falls back to legacy profile_id', () => {
    expect(accountBelongsToProfile(account(), 'legacy')).toBe(true)
    expect(accountBelongsToProfile(account({ profile_ids: ['shared'] }), 'shared')).toBe(true)
    expect(accountBelongsToProfile(account({ profile_ids: ['shared'] }), 'legacy')).toBe(false)
  })

  it('recognizes both Mastodon account kinds', () => {
    expect(isMastodon(account())).toBe(true)
    expect(isMastodon(account({ platform: 'mastodon-compatible' }))).toBe(true)
    expect(isMastodon(account({ platform: 'bluesky' }))).toBe(false)
  })
})
