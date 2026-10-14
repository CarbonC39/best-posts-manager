import { beforeEach, describe, expect, it, vi } from 'vitest'
import { attachAccountToProfile, createAccount, deactivateAccount, deleteAccount, diagnoseBlueskyAccount, updateAccountConnection } from '../src/api/accounts'

describe('account API', () => {
  beforeEach(() => { global.fetch = vi.fn(async () => new Response('{}', { status: 200, headers: { 'Content-Type': 'application/json' } })) as any })

  it('preserves account mutation routes, methods and request bodies', async () => {
    const create = { profile_id: 'p1', name: 'Main', platform: 'bluesky', remote_id: 'me.test', base_url: 'https://bsky.social', token_env: '', secret: 'secret' }
    await createAccount(create)
    await attachAccountToProfile('p2', 'a1')
    await updateAccountConnection('a1', { name: 'Renamed', remote_id: 'did:plc:1', base_url: 'https://pds.test', token_env: 'TOKEN', secret: '' })
    await diagnoseBlueskyAccount('a1')
    await deactivateAccount('a1')
    await deleteAccount('a1')

    const calls = vi.mocked(global.fetch).mock.calls
    expect(calls.map(([url, init]) => [url, init?.method])).toEqual([
      ['/api/accounts', 'POST'],
      ['/api/profile-accounts', 'POST'],
      ['/api/accounts/a1/connection', 'PUT'],
      ['/api/accounts/a1/diagnostic', 'POST'],
      ['/api/accounts/a1/deactivate', 'DELETE'],
      ['/api/accounts/a1', 'DELETE'],
    ])
    expect(JSON.parse(String(calls[0][1]?.body))).toEqual(create)
    expect(JSON.parse(String(calls[1][1]?.body))).toEqual({ profile_id: 'p2', account_id: 'a1' })
  })
})
