import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import App from '../src/App.vue'
import PublicationList from '../src/PublicationList.vue'

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
}

function poolFixture() {
  const profiles = [{ id: 'p1', name: 'Writing' }, { id: 'p2', name: 'Drawing' }]
  const accounts = [
    { id: 'a1', profile_id: 'p1', profile_ids: ['p1'], platform: 'x', name: 'Writer', disabled: false },
    { id: 'a2', profile_id: 'p2', profile_ids: ['p2'], platform: 'qzone', name: 'Artist', disabled: false },
  ]
  let failLink = false
  const fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = String(input)
    if (path.endsWith('/profiles')) return json(profiles)
    if (path.endsWith('/accounts')) return json(accounts)
    if (path.endsWith('/profile-accounts') && init?.method === 'POST') {
      if (failLink) return json({ error: 'Association failed' }, 503)
      const payload = JSON.parse(String(init.body))
      const account = accounts.find((item) => item.id === payload.account_id)!
      account.profile_ids.push(payload.profile_id)
      return json(account, 201)
    }
    return json([])
  })
  return { profiles, accounts, fetch, setLinkFailure: (value: boolean) => { failLink = value } }
}

describe('management UI API flows', () => {
  beforeEach(() => {
    window.history.replaceState({}, '', '/topics')
    vi.restoreAllMocks()
  })

  it('shows every global account and opens a Profile account list without switching the editor', async () => {
    const fixture = poolFixture()
    global.fetch = fixture.fetch as any
    const wrapper = mount(App)
    await flushPromises()
    await wrapper.get('button[aria-label="管理"]').trigger('click')
    expect(wrapper.get('#connected-accounts-title').text()).toBe('账号池')
    expect(wrapper.findAll('.account-card')).toHaveLength(2)
    expect(wrapper.findAll('.profile-inline-menu')).toHaveLength(0)
    await wrapper.get('button[aria-label="Profile 操作：Drawing"]').trigger('click')
    expect(wrapper.get('.profile-account-list li > span').text()).toBe('Artist')
    expect(wrapper.get('select[aria-label="切换 Profile"]').element).toHaveProperty('value', 'p1')
    await wrapper.get('.profile-switch-action').trigger('click')
    await flushPromises()
    expect(wrapper.get('select[aria-label="切换 Profile"]').element).toHaveProperty('value', 'p2')
    expect(wrapper.findAll('.account-card')).toHaveLength(2)
    expect(wrapper.get('.account-list').text()).toContain('Writer')
    expect(fixture.fetch.mock.calls.some(([path]) => String(path).endsWith('/accounts'))).toBe(true)
    wrapper.unmount()
  })

  it('reveals account actions only on click and closes the editor when another account opens', async () => {
    global.fetch = poolFixture().fetch as any
    const wrapper = mount(App)
    await flushPromises()
    await wrapper.get('button[aria-label="管理"]').trigger('click')
    expect(wrapper.findAll('button[aria-label="编辑账号"]')).toHaveLength(0)
    await wrapper.get('button[aria-label="账号操作：Writer"]').trigger('click')
    expect(wrapper.findAll('button[aria-label="编辑账号"]')).toHaveLength(1)
    expect(wrapper.findAll('button[aria-label="停用账号"]')).toHaveLength(1)
    await wrapper.get('button[aria-label="编辑账号"]').trigger('click')
    await wrapper.get('form.account-edit input').setValue('Changed')
    await wrapper.get('button[aria-label="账号操作：Artist"]').trigger('click')
    expect(wrapper.find('form.account-edit').exists()).toBe(false)
    expect(wrapper.findAll('.account-inline-menu')).toHaveLength(1)
    expect(wrapper.get('button[aria-label="账号操作：Writer"]').attributes('aria-expanded')).toBe('false')
    await wrapper.get('button[aria-label="账号操作：Artist"]').trigger('click')
    expect(wrapper.findAll('.account-inline-menu')).toHaveLength(0)
    wrapper.unmount()
  })

  it('associates a pool account with another Profile without changing the active Profile or hiding the account', async () => {
    const fixture = poolFixture()
    global.fetch = fixture.fetch as any
    const wrapper = mount(App)
    await flushPromises()
    await wrapper.get('button[aria-label="管理"]').trigger('click')
    await wrapper.get('button[aria-label="Profile 操作：Drawing"]').trigger('click')
    await wrapper.get('button[aria-label="账号操作：Writer"]').trigger('click')
    await wrapper.get('.account-profile-link').trigger('submit')
    await flushPromises()
    const call = fixture.fetch.mock.calls.find(([path]) => String(path).endsWith('/profile-accounts'))!
    expect(JSON.parse(String(call[1]?.body))).toEqual({ profile_id: 'p2', account_id: 'a1' })
    expect(wrapper.get('.account-memberships').text()).toContain('Drawing')
    expect(wrapper.get('.profile-account-list').text()).toContain('Writer')
    expect(wrapper.get('.profile-account-list').text()).toContain('Artist')
    expect(wrapper.findAll('.account-card')).toHaveLength(2)
    expect(wrapper.find('.account-profile-link').exists()).toBe(false)
    expect(wrapper.get('select[aria-label="切换 Profile"]').element).toHaveProperty('value', 'p1')
    wrapper.unmount()
  })

  it('keeps a failed association available for retry without inventing membership', async () => {
    const fixture = poolFixture()
    fixture.setLinkFailure(true)
    global.fetch = fixture.fetch as any
    const wrapper = mount(App)
    await flushPromises()
    await wrapper.get('button[aria-label="管理"]').trigger('click')
    await wrapper.get('button[aria-label="账号操作：Writer"]').trigger('click')
    await wrapper.get('.account-profile-link').trigger('submit')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('Association failed')
    expect(wrapper.get('.account-memberships').text()).not.toContain('Drawing')
    expect(wrapper.get('select[aria-label="关联到 Profile"]').element).toHaveProperty('value', 'p2')
    fixture.setLinkFailure(false)
    await wrapper.get('.account-profile-link').trigger('submit')
    await flushPromises()
    expect(wrapper.get('.account-memberships').text()).toContain('Drawing')
    wrapper.unmount()
  })

  it('creates a profile through the form and selects the returned profile', async () => {
    const profiles = [{ id: 'p1', name: 'First' }]
    const fetchMock = vi.fn(async (input: RequestInfo, init?: RequestInit) => {
      const path = String(input)
      if (path.endsWith('/profiles') && init?.method === 'POST') {
        const request = JSON.parse(String(init.body))
        expect(request).toEqual({ name: 'Research' })
        profiles.push({ id: 'p2', name: request.name })
        return json(profiles[1], 201)
      }
      if (path.endsWith('/profiles')) return json(profiles)
      if (path.endsWith('/publications/summary?profile_id=p1') || path.endsWith('/publications/summary?profile_id=p2')) return json({ action_count: 0, failed_count: 0 })
      return json([])
    })
    global.fetch = fetchMock as any

    const wrapper = mount(App)
    await flushPromises()
    await wrapper.get('.utility-dock button[aria-label="管理"]').trigger('click')
    await wrapper.get('button[aria-label="新建分组"]').trigger('click')
    await wrapper.get('#profile-create-form input[aria-label="新分组名称"]').setValue('Research')
    await wrapper.get('#profile-create-form').trigger('submit')
    await flushPromises()

    expect(fetchMock).toHaveBeenCalledWith('/api/profiles', expect.objectContaining({ method: 'POST' }))
    expect(wrapper.get('select[aria-label="切换 Profile"]').element).toHaveProperty('value', 'p2')
    expect(wrapper.get('.profile-management').text()).toContain('Research')
    expect(wrapper.find('#profile-create-form').exists()).toBe(false)
    wrapper.unmount()
  })

  it('keeps a deactivated account in the global pool only after confirmation', async () => {
    const account: Record<string, unknown> = { id: 'a1', profile_id: 'p1', platform: 'mastodon', name: 'Fixture account', disabled: false }
    const fetchMock = vi.fn(async (input: RequestInfo, init?: RequestInit) => {
      const path = String(input)
      if (path.endsWith('/profiles')) return json([{ id: 'p1', name: 'First' }])
      if (path.endsWith('/accounts/a1/deactivate') && init?.method === 'DELETE') {
        account.disabled = true
        return json(account)
      }
      if (path.endsWith('/accounts')) return json([account])
      return json([])
    })
    global.fetch = fetchMock as any
    const confirm = vi.spyOn(window, 'confirm').mockReturnValueOnce(false).mockReturnValueOnce(true)

    const wrapper = mount(App)
    await flushPromises()
    await wrapper.get('.utility-dock button[aria-label="管理"]').trigger('click')
    await wrapper.get('.account-menu-trigger').trigger('click')
    await wrapper.get('button[aria-label="停用账号"]').trigger('click')
    await flushPromises()
    expect(fetchMock.mock.calls.some(call => String(call[0]).endsWith('/accounts/a1/deactivate'))).toBe(false)
    expect(wrapper.get('.account-list').text()).toContain('Fixture account')

    await wrapper.get('button[aria-label="停用账号"]').trigger('click')
    await flushPromises()
    expect(confirm).toHaveBeenCalledTimes(2)
    expect(fetchMock.mock.calls.filter(call => String(call[0]).endsWith('/accounts/a1/deactivate') && call[1]?.method === 'DELETE')).toHaveLength(1)
    expect(wrapper.get('.account-list').text()).toContain('Fixture account')
    expect(wrapper.get('.account-menu-trigger').text()).toContain('已停用')
    expect(wrapper.find('button[aria-label="停用账号"]').exists()).toBe(false)
    expect(wrapper.find('button[aria-label="永久删除本地账号数据"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('keeps account edit input on failure and clears the modal error after a successful retry', async () => {
    const account: Record<string, unknown> = { id: 'a1', profile_id: 'p1', platform: 'x', name: 'Original', disabled: false }
    let updateAttempt = 0
    const fetchMock = vi.fn(async (input: RequestInfo, init?: RequestInit) => {
      const path = String(input)
      if (path.endsWith('/profiles')) return json([{ id: 'p1', name: 'First' }])
      if (path.endsWith('/accounts/a1/connection') && init?.method === 'PUT') {
        updateAttempt += 1
        if (updateAttempt === 1) return json({ error: 'fixture connection error' }, 500)
        Object.assign(account, JSON.parse(String(init.body)))
        return json(account)
      }
      if (path.endsWith('/accounts')) return json([account])
      return json([])
    })
    global.fetch = fetchMock as any

    const wrapper = mount(App)
    await flushPromises()
    await wrapper.get('.utility-dock button[aria-label="管理"]').trigger('click')
    await wrapper.get('.account-menu-trigger').trigger('click')
    await wrapper.get('button[aria-label="编辑账号"]').trigger('click')
    const editForm = wrapper.get('form.account-edit')
    await editForm.get('input').setValue('Updated local account')
    await editForm.trigger('submit')
    await flushPromises()

    expect(wrapper.get('[role="dialog"]').exists()).toBe(true)
    expect(wrapper.get('[role="alert"]').text()).toContain('fixture connection error')
    expect((wrapper.get('.account-edit input').element as HTMLInputElement).value).toBe('Updated local account')

    await editForm.trigger('submit')
    await flushPromises()
    expect(wrapper.get('.connected-accounts').text()).toContain('Updated local account')
    expect(wrapper.find('.account-edit').exists()).toBe(false)
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('sends only the supported nonsecret translation config fields and shows API errors', async () => {
    const requests: Array<{ provider: string; base_url: string; token_env: string; key_env: string; project_id: string; model: string }> = []
    const existingConfig = { id: 'existing', provider: 'openai', base_url: 'https://api.example.test', token_env: 'TEST_TRANSLATION_ENV', key_env: '', project_id: '', model: 'sample-model' }
    let createAttempt = 0
    const fetchMock = vi.fn(async (input: RequestInfo, init?: RequestInit) => {
      const path = String(input)
      if (path.endsWith('/profiles')) return json([{ id: 'p1', name: 'First' }])
      if (path.endsWith('/translation-configs') && init?.method === 'POST') {
        requests.push(JSON.parse(String(init.body)))
        createAttempt += 1
        if (createAttempt === 1) return json({ error: 'fixture save failure' }, 500)
        return json({ id: 'cfg1', provider: 'google-gtx', base_url: 'https://translate.example.test', token_env: '', key_env: '', project_id: '', model: '' }, 201)
      }
      if (path.endsWith('/translation-configs')) return json([existingConfig])
      return json([])
    })
    global.fetch = fetchMock as any

    const wrapper = mount(App)
    await flushPromises()
    await wrapper.get('.utility-dock button[aria-label="设置"]').trigger('click')
    expect(wrapper.get('.translation-config-list').text()).toContain('sample-model')
    expect(wrapper.get('.translation-config-list').text()).toContain('https://api.example.test')
    const form = wrapper.get('.translation-config-form')
    await form.get('select[aria-label="翻译服务"]').setValue('google-gtx')
    expect(form.find('input[aria-label="密钥环境变量"]').exists()).toBe(false)
    await form.get('input[aria-label="自定义 API 地址"]').setValue('https://translate.example.test')
    await form.trigger('submit')
    await flushPromises()

    expect(requests[0]).toEqual({ provider: 'google-gtx', base_url: 'https://translate.example.test', token_env: '', key_env: '', project_id: '', model: '' })
    expect(wrapper.get('[role="alert"]').text()).toContain('fixture save failure')
    expect(wrapper.find('.settings-page').exists()).toBe(true)
    await form.trigger('submit')
    await flushPromises()
    expect(requests).toHaveLength(2)
    expect(requests[1]).toEqual(requests[0])
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(JSON.stringify(requests)).not.toMatch(/secret|credential_ref/i)
    wrapper.unmount()
  })

  it('renders publication status tabs, validates an empty confirmation URL, and refreshes scoped history', async () => {
    const rows = [
      { id: 'uncertain', topic_id: 't1', topic_title: 'Uncertain', account_id: 'a1', status: 'needs_confirmation', text: 'Check URL', created_at: '2026-01-01T00:00:00Z' },
      { id: 'pending', topic_id: 't2', topic_title: 'Queued', account_id: 'a1', status: 'pending', text: 'Waiting', created_at: '2026-01-02T00:00:00Z' },
      { id: 'done', topic_id: 't3', topic_title: 'Published', account_id: 'a1', status: 'published', text: 'Completed', created_at: '2026-01-03T00:00:00Z' },
    ]
    const fetchMock = vi.fn(async (input: RequestInfo, init?: RequestInit) => {
      const path = String(input)
      if (path.endsWith('/publications?profile_id=p1')) return json(rows)
      if (path.endsWith('/accounts?profile_id=p1')) return json([{ id: 'a1', name: 'Fixture', platform: 'mastodon' }])
      if (path.endsWith('/confirm')) throw new Error('Empty URL must not send a request')
      return json({})
    })
    global.fetch = fetchMock as any
    const wrapper = mount(PublicationList, { props: { profileId: 'p1' } })
    await flushPromises()

    expect(wrapper.get('[role="tablist"]').findAll('[role="tab"]').map(tab => tab.text())).toEqual(expect.arrayContaining(['需处理1', '进行中1', '最近完成1']))
    expect(wrapper.text()).toContain('Uncertain')
    await wrapper.get('button.primary-action').trigger('click')
    await flushPromises()
    expect(fetchMock.mock.calls.some(call => String(call[0]).endsWith('/confirm'))).toBe(false)
    expect(wrapper.text()).toContain('请填写帖子永久链接后再确认')

    await wrapper.findAll('[role="tab"]').find(tab => tab.text().includes('进行中'))!.trigger('click')
    expect(wrapper.text()).toContain('Queued')
    expect(wrapper.text()).not.toContain('Uncertain')
    const scopedLoadsBeforeRefresh = fetchMock.mock.calls.filter(call => String(call[0]).endsWith('/publications?profile_id=p1')).length
    await wrapper.get('button[aria-label="刷新记录"]').trigger('click')
    await flushPromises()
    expect(fetchMock.mock.calls.filter(call => String(call[0]).endsWith('/publications?profile_id=p1'))).toHaveLength(scopedLoadsBeforeRefresh + 1)
    wrapper.unmount()
  })

  it('keeps history refresh reachable after the first list request fails and recovers rows on retry', async () => {
    let publicationLoads = 0
    const fetchMock = vi.fn(async (input: RequestInfo) => {
      const path = String(input)
      if (path.endsWith('/profiles')) return json([{ id: 'p1', name: 'First' }])
      if (path.endsWith('/topics')) return json([])
      if (path.includes('/publications/summary?')) return json({ action_count: 0, failed_count: 0 })
      if (path.endsWith('/publications?profile_id=p1')) {
        publicationLoads += 1
        if (publicationLoads === 1) return json({ error: '历史加载失败' }, 503)
        return json([{ id: 'pub1', topic_id: 't1', topic_title: 'Recovered history', account_id: 'a1', status: 'published', text: 'Saved local fixture', created_at: '2026-01-03T00:00:00Z' }])
      }
      if (path.endsWith('/accounts?profile_id=p1')) return json([{ id: 'a1', name: 'Fixture', platform: 'mastodon' }])
      return json([])
    })
    global.fetch = fetchMock as any

    const wrapper = mount(App)
    await flushPromises()
    await wrapper.get('.utility-dock button[aria-label="发布记录"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[role="alert"]').text()).toContain('历史加载失败')
    expect(wrapper.text()).not.toContain('还没有发布记录。')
    expect(wrapper.get('button[aria-label="刷新记录"]').exists()).toBe(true)

    await wrapper.get('button[aria-label="刷新记录"]').trigger('click')
    await flushPromises()
    expect(publicationLoads).toBe(2)
    expect(wrapper.get('.publication-card').text()).toContain('Recovered history')
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('hides stale publication errors after retry is accepted and emits no success notice', async () => {
    let retryAccepted = false
    const fetchMock = vi.fn(async (input: RequestInfo, init?: RequestInit) => {
      const path = String(input)
      if (path.endsWith('/publications/retry') && init?.method === 'POST') {
        retryAccepted = true
        return json({ status: 'pending' })
      }
      if (path.endsWith('/publications?profile_id=p1')) {
        return retryAccepted
          ? json([
            { id: 'retrying', topic_id: 't1', topic_title: 'Accepted retry', account_id: 'a1', status: 'pending', error_message: 'stale network failure', error_category: 'retryable', text: 'Draft', created_at: '2026-01-01T00:00:00Z' },
            { id: 'publishing', topic_id: 't2', topic_title: 'Publishing', account_id: 'a1', status: 'publishing', error_message: 'old timeout', text: 'Draft', created_at: '2026-01-02T00:00:00Z' },
            { id: 'published', topic_id: 't3', topic_title: 'Published', account_id: 'a1', status: 'published', error_message: 'old publish error', text: 'Done', created_at: '2026-01-03T00:00:00Z' },
          ])
          : json([{ id: 'retrying', topic_id: 't1', topic_title: 'Accepted retry', account_id: 'a1', status: 'failed', error_message: 'stale network failure', error_category: 'retryable', text: 'Draft', created_at: '2026-01-01T00:00:00Z' }])
      }
      if (path.endsWith('/accounts?profile_id=p1')) return json([{ id: 'a1', name: 'Fixture', platform: 'mastodon' }])
      return json([])
    })
    global.fetch = fetchMock as any

    const wrapper = mount(PublicationList, { props: { profileId: 'p1' } })
    await flushPromises()
    expect(wrapper.get('.publication-error').text()).toBe('stale network failure')
    await wrapper.findAll('button').find(button => button.text().includes('重试原始内容'))!.trigger('click')
    await flushPromises()

    expect(fetchMock.mock.calls.some(call => String(call[0]).endsWith('/publications/retry') && call[1]?.method === 'POST')).toBe(true)
    expect(wrapper.text()).toContain('等待处理')
    expect(wrapper.text()).toContain('正在发布')
    expect(wrapper.findAll('.publication-error')).toHaveLength(0)
    expect(wrapper.text()).not.toContain('重试完成')
    expect(wrapper.emitted('notify')).toBeUndefined()

    await wrapper.findAll('[role="tab"]').find(tab => tab.text().includes('最近完成'))!.trigger('click')
    expect(wrapper.text()).toContain('已发布')
    expect(wrapper.findAll('.publication-error')).toHaveLength(0)
    wrapper.unmount()
  })
})
