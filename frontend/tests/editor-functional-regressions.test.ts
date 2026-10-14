import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { nextTick } from 'vue'
import App from '../src/App.vue'

type Topic = { id: string; profile_id: string; title: string; status: string }
type Draft = { id: string; topic_id: string; text: string; format: string; language: string; revision: number }
type Variant = {
  id: string; topic_id: string; account_id: string; text: string; format: string; status: string
  relation: string; source_revision: string; visibility: string; spoiler_text: string; local_only: boolean
  outdated: boolean; published: boolean; publication_locked: boolean; publication_status: string
}

function backend(seed: { topics?: Topic[]; drafts?: Draft[]; variants?: Variant[]; media?: Array<{ id: string; topic_id: string; variant_id: string; filename: string; mime: string; alt_text: string }> } = {}) {
  const topics = [...(seed.topics || [])]
  const drafts = new Map((seed.drafts || []).map((draft) => [draft.topic_id, draft]))
  const variants = [...(seed.variants || [])]
  const media = [...(seed.media || [])]
  const requests: Array<{ path: string; method: string; body?: any }> = []
  let topicSequence = topics.length + 1
  let variantSequence = variants.length + 1
  let failedDraftWrites = 0

  const fetch = vi.fn(async (input: RequestInfo | URL, init: RequestInit = {}) => {
    const path = String(input)
    const method = init.method || 'GET'
    const requestBody = init.body ? JSON.parse(String(init.body)) : undefined
    requests.push({ path, method, body: requestBody })
    let body: any = []
    let status = 200

    if (path.endsWith('/profiles')) body = [{ id: 'p1', name: 'Profile' }]
    else if (path.endsWith('/accounts')) body = [{ id: 'a1', profile_id: 'p1', platform: 'mastodon', name: 'Main account', disabled: false }]
    else if (path.endsWith('/topics') && method === 'POST') {
      const topic = { id: `t${topicSequence++}`, profile_id: requestBody.profile_id, title: requestBody.title, status: 'active' }
      topics.unshift(topic)
      const draft: Draft = { id: `d-${topic.id}`, topic_id: topic.id, text: '', format: 'plain', language: '', revision: 1 }
      drafts.set(topic.id, draft)
      body = topic
      status = 201
    } else if (/\/topics\/[^/]+$/.test(path) && method === 'PUT') {
      const topicID = path.match(/\/topics\/([^/]+)$/)?.[1] || ''
      const topic = topics.find((item) => item.id === topicID)
      if (topic) Object.assign(topic, requestBody)
      body = topic || { id: topicID, ...requestBody }
    } else if (path.endsWith('/topics')) body = topics
    else if (/\/topics\/[^/]+\/draft$/.test(path)) {
      const topicID = path.match(/\/topics\/([^/]+)\/draft$/)?.[1] || ''
      const existing = drafts.get(topicID) || { id: `d-${topicID}`, topic_id: topicID, text: '', format: 'plain', language: '', revision: 1 }
      if (method === 'PUT') {
        if (failedDraftWrites > 0) {
          failedDraftWrites -= 1
          body = { error: 'temporary storage failure' }
          status = 503
        } else {
          const next = { ...existing, ...requestBody, revision: existing.revision + Number(requestBody.text !== existing.text) }
          drafts.set(topicID, next)
          body = next
        }
      } else body = existing
    } else if (path.includes('/variants?topic_id=')) body = variants
    else if (path.endsWith('/variants') && method === 'POST') {
      const sourceDraft = drafts.get(requestBody.topic_id)
      const variant: Variant = {
        id: `v${variantSequence++}`, topic_id: requestBody.topic_id, account_id: requestBody.account_id,
        text: sourceDraft?.text || '', format: sourceDraft?.format || 'plain', status: 'draft', relation: 'linked',
        source_revision: String(sourceDraft?.revision || 1), visibility: 'public', spoiler_text: '', local_only: false,
        outdated: false, published: false, publication_locked: false, publication_status: '',
      }
      variants.push(variant)
      body = variant
      status = 201
    } else if (path.includes('/media?')) body = media
    else if (path.includes('/accounts/') && path.endsWith('/capability')) {
      body = { platform: 'mastodon', format: 'plain', visibilities: ['public', 'unlisted', 'private', 'direct'], text_max_bytes: 500 }
    } else if (path.includes('/publications/summary')) body = { action_count: 0, failed_count: 0 }
    else if (path.endsWith('/bridge/pairings') || path.endsWith('/translation-configs') || path.includes('/external-posts?')) body = []
    else if (path.endsWith('/media/order')) body = { status: 'ok' }

    return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
  })

  return { fetch, topics, drafts, variants, media, requests, failNextDraftWrites: (count = 1) => { failedDraftWrites = count } }
}

async function settleAutosave() {
  await vi.advanceTimersByTimeAsync(800)
  await flushPromises()
  await nextTick()
}

describe('editor functional regressions', () => {
  let mounted: VueWrapper | undefined

  beforeEach(() => {
    vi.useFakeTimers()
    window.history.replaceState({}, '', '/topics')
  })

  afterEach(() => {
    mounted?.unmount()
    mounted = undefined
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('does not create an untouched topic, then autosaves a named topic and restores it after reload', async () => {
    const api = backend()
    vi.stubGlobal('fetch', api.fetch)
    mounted = mount(App)
    await flushPromises()
    await nextTick()

    expect(api.topics).toHaveLength(0)
    await settleAutosave()
    expect(api.topics).toHaveLength(0)
    await mounted.get('input[aria-label="话题名"]').setValue('Release notes')
    await mounted.get('.draft-editor').setValue('Draft body')
    await settleAutosave()

    expect(api.topics).toHaveLength(1)
    expect(api.topics[0].title).toBe('Release notes')
    expect(api.drafts.get(api.topics[0].id)?.text).toBe('Draft body')
    expect(api.requests.some(({ path, method }) => /\/topics\/t\d+\/draft$/.test(path) && method === 'PUT')).toBe(true)

    const topic = api.topics[0]
    mounted.unmount()
    window.history.replaceState({}, '', `/topics/${topic.id}`)
    mounted = mount(App)
    await flushPromises()
    await nextTick()

    expect((mounted.get('.draft-editor').element as HTMLTextAreaElement).value).toBe('Draft body')
    const titleInput = mounted.get('input[aria-label="话题名"]')
    expect((titleInput.element as HTMLInputElement).value).toBe('Release notes')
    await titleInput.setValue('Release notes revised')
    await settleAutosave()
    expect(api.topics[0].title).toBe('Release notes revised')
    expect(api.requests.some(({ path, method }) => path.endsWith('/topics/t1') && method === 'PUT')).toBe(true)
  })

  it('creates an unnamed topic from the first body edit and persists that text', async () => {
    const api = backend()
    vi.stubGlobal('fetch', api.fetch)
    mounted = mount(App)
    await flushPromises()
    await nextTick()

    await mounted.get('.draft-editor').setValue('First unscheduled thought')
    await settleAutosave()

    expect(api.topics).toHaveLength(1)
    expect(api.topics[0].title).toBe('未命名话题')
    expect(api.drafts.get(api.topics[0].id)?.text).toBe('First unscheduled thought')

    const topic = api.topics[0]
    mounted.unmount()
    window.history.replaceState({}, '', `/topics/${topic.id}`)
    mounted = mount(App)
    await flushPromises()
    await nextTick()
    expect((mounted.get('.draft-editor').element as HTMLTextAreaElement).value).toBe('First unscheduled thought')
  })

  it('opens master publish target selection and preflight without sending a publication', async () => {
    const topic: Topic = { id: 't1', profile_id: 'p1', title: 'Topic', status: 'active' }
    const draft: Draft = { id: 'd1', topic_id: 't1', text: 'Ready to review', format: 'plain', language: '', revision: 1 }
    const api = backend({ topics: [topic], drafts: [draft] })
    vi.stubGlobal('fetch', api.fetch)
    window.history.replaceState({}, '', '/topics/t1')
    mounted = mount(App)
    await flushPromises()
    await nextTick()

    expect((mounted.get('.draft-editor').element as HTMLTextAreaElement).value).toBe('Ready to review')
    const publishCTA = mounted.get('.post-preview-trigger')
    expect(publishCTA.attributes('disabled')).toBeUndefined()
    await publishCTA.trigger('click')
    expect(mounted.get('.target-add-inline').text()).toContain('选择发布账号')
    await mounted.get('.target-account-results button').trigger('click')
    await mounted.get('.target-account-submit').trigger('click')
    await flushPromises()

    expect(api.variants).toHaveLength(1)
    expect(api.variants[0].text).toBe('Ready to review')
    expect(mounted.get('.publish-dialog').text()).toContain('发布前检查')
    expect(mounted.get('.publish-dialog').text()).toContain('Ready to review')
    expect(api.requests.some(({ path, method }) => path.endsWith('/publications') && method === 'POST')).toBe(false)
  })

  it('keeps edited text visible after a failed autosave and persists it after recovery', async () => {
    const topic: Topic = { id: 't1', profile_id: 'p1', title: 'Topic', status: 'active' }
    const draft: Draft = { id: 'd1', topic_id: 't1', text: 'Saved text', format: 'plain', language: '', revision: 1 }
    const api = backend({ topics: [topic], drafts: [draft] })
    api.failNextDraftWrites()
    vi.stubGlobal('fetch', api.fetch)
    window.history.replaceState({}, '', '/topics/t1')
    mounted = mount(App)
    await flushPromises()
    await nextTick()

    await mounted.get('.draft-editor').setValue('Unsaved through outage')
    await settleAutosave()
    expect((mounted.get('.draft-editor').element as HTMLTextAreaElement).value).toBe('Unsaved through outage')
    expect(mounted.get('.toast').text()).toContain('正文保存失败')
    expect(api.drafts.get('t1')?.text).toBe('Saved text')

    await mounted.get('.draft-editor').setValue('Recovered and saved')
    await settleAutosave()
    expect(api.drafts.get('t1')?.text).toBe('Recovered and saved')
    expect(mounted.get('.save-state').text()).toBe('')
  })

  it('disables both version arrows when the master draft is the only document', async () => {
    const topic: Topic = { id: 't1', profile_id: 'p1', title: 'Topic', status: 'active' }
    const draft: Draft = { id: 'd1', topic_id: 't1', text: 'Master text', format: 'plain', language: '', revision: 1 }
    const api = backend({ topics: [topic], drafts: [draft] })
    vi.stubGlobal('fetch', api.fetch)
    window.history.replaceState({}, '', '/topics/t1')
    mounted = mount(App)
    await flushPromises()
    await nextTick()

    expect(mounted.get('button[aria-label="上一个版本"]').attributes('disabled')).toBe('')
    expect(mounted.get('button[aria-label="下一个版本"]').attributes('disabled')).toBe('')
  })

  it('renders one toolbar action each and inserts a hashtag and Unicode emoji into the draft', async () => {
    const topic: Topic = { id: 't1', profile_id: 'p1', title: 'Topic', status: 'active' }
    const draft: Draft = { id: 'd1', topic_id: 't1', text: '', format: 'plain', language: '', revision: 1 }
    const media = [{ id: 'm1', topic_id: 't1', variant_id: '', filename: 'test.png', mime: 'image/png', alt_text: '' }]
    const api = backend({ topics: [topic], drafts: [draft], media })
    vi.stubGlobal('fetch', api.fetch)
    window.history.replaceState({}, '', '/topics/t1')
    mounted = mount(App)
    await flushPromises()
    await nextTick()

    const composer = mounted.get('.editor-composer')
    for (const label of ['插入话题标签', 'Emoji', '翻译', '图片', '检查并发布']) {
      expect(composer.findAll(`button[aria-label="${label}"]`)).toHaveLength(1)
    }
    expect(mounted.findAll('button[aria-label="检查并发布"]')).toHaveLength(1)
    expect(composer.find('.media-flow').exists()).toBe(true)
    expect(composer.find('.media-flow img').attributes('src')).toBe('/media/m1')

    await composer.get('button[aria-label="插入话题标签"]').trigger('click')
    await composer.get('input[aria-label="话题标签"]').setValue('创作')
    await composer.get('.hashtag-tool form').trigger('submit')
    await composer.get('button[aria-label="Emoji"]').trigger('click')
    await composer.get('.unicode-emoji-grid button[aria-label="插入 😀"]').trigger('click')
    expect((composer.get('.draft-editor').element as HTMLTextAreaElement).value).toBe('#创作😀')

    await composer.get('button[aria-label="图片"]').trigger('click')
    expect(mounted.get('.media-dialog input[type="file"]').exists()).toBe(true)
  })

  it('routes master Translate through a real source variant without issuing a translation request', async () => {
    const topic: Topic = { id: 't1', profile_id: 'p1', title: 'Topic', status: 'active' }
    const draft: Draft = { id: 'd1', topic_id: 't1', text: 'Source text', format: 'plain', language: '', revision: 1 }
    const api = backend({ topics: [topic], drafts: [draft] })
    vi.stubGlobal('fetch', api.fetch)
    window.history.replaceState({}, '', '/topics/t1')
    mounted = mount(App)
    await flushPromises()
    await nextTick()

    await mounted.get('button[aria-label="翻译"]').trigger('click')
    expect(mounted.get('.target-add-inline').text()).toContain('选择翻译源账号')
    await mounted.get('.target-account-results button').trigger('click')
    await mounted.get('.target-account-submit').trigger('click')
    await flushPromises()

    expect(api.variants).toHaveLength(1)
    expect(api.variants[0].text).toBe('Source text')
    expect(mounted.get('.translation-dialog').exists()).toBe(true)
    expect(api.requests.some(({ path, method }) => path.endsWith('/translations') && method === 'POST')).toBe(false)
  })

  it('moves through master and multiple account variants with the previous/next controls', async () => {
    const topic: Topic = { id: 't1', profile_id: 'p1', title: 'Topic', status: 'active' }
    const draft: Draft = { id: 'd1', topic_id: 't1', text: 'Master text', format: 'plain', language: '', revision: 1 }
    const makeVariant = (id: string, text: string): Variant => ({
      id, topic_id: 't1', account_id: 'a1', text, format: 'plain', status: 'draft', relation: 'linked',
      source_revision: '1', visibility: 'public', spoiler_text: '', local_only: false, outdated: false,
      published: false, publication_locked: false, publication_status: '',
    })
    const api = backend({ topics: [topic], drafts: [draft], variants: [makeVariant('v1', 'Variant one'), makeVariant('v2', 'Variant two')] })
    vi.stubGlobal('fetch', api.fetch)
    window.history.replaceState({}, '', '/topics/t1')
    mounted = mount(App)
    await flushPromises()
    await nextTick()

    const next = mounted.get('button[aria-label="下一个版本"]')
    const previous = mounted.get('button[aria-label="上一个版本"]')
    expect((mounted.get('.draft-editor').element as HTMLTextAreaElement).value).toBe('Master text')
    expect(next.attributes('disabled')).toBeUndefined()
    await next.trigger('click')
    await flushPromises()
    expect((mounted.get('.draft-editor').element as HTMLTextAreaElement).value).toBe('Variant one')
    await next.trigger('click')
    await flushPromises()
    expect((mounted.get('.draft-editor').element as HTMLTextAreaElement).value).toBe('Variant two')
    await next.trigger('click')
    await flushPromises()
    expect((mounted.get('.draft-editor').element as HTMLTextAreaElement).value).toBe('Master text')
    await previous.trigger('click')
    await flushPromises()
    expect((mounted.get('.draft-editor').element as HTMLTextAreaElement).value).toBe('Variant two')
  })

  it('honors consecutive arrow clicks while the edited master is still saving', async () => {
    const topic: Topic = { id: 't1', profile_id: 'p1', title: 'Topic', status: 'active' }
    const draft: Draft = { id: 'd1', topic_id: 't1', text: 'Master', format: 'plain', language: '', revision: 1 }
    const makeVariant = (id: string): Variant => ({ id, topic_id: 't1', account_id: 'a1', text: id, format: 'plain', status: 'draft', relation: 'linked', source_revision: '1', visibility: 'public', spoiler_text: '', local_only: false, outdated: false, published: false, publication_locked: false, publication_status: '' })
    const api = backend({ topics: [topic], drafts: [draft], variants: [makeVariant('v1'), makeVariant('v2')] })
    let releaseSave!: () => void
    const saving = new Promise<void>((resolve) => { releaseSave = resolve })
    vi.stubGlobal('fetch', async (input: RequestInfo | URL, init?: RequestInit) => {
      if (String(input).endsWith('/topics/t1/draft') && init?.method === 'PUT') await saving
      return api.fetch(input, init)
    })
    window.history.replaceState({}, '', '/topics/t1')
    mounted = mount(App)
    await flushPromises()
    await mounted.get('.draft-editor').setValue('Edited master')
    const next = mounted.get('button[aria-label="下一个版本"]')
    await next.trigger('click')
    await next.trigger('click')
    releaseSave()
    await flushPromises()
    expect((mounted.get('.draft-editor').element as HTMLTextAreaElement).value).toBe('v2')
    expect(api.drafts.get('t1')?.text).toBe('Edited master')
    expect(api.requests.filter(({ path, method }) => path.endsWith('/topics/t1/draft') && method === 'PUT')).toHaveLength(1)
  })

  it('saves edits made during an in-flight save before switching documents', async () => {
    const topic: Topic = { id: 't1', profile_id: 'p1', title: 'Topic', status: 'active' }
    const draft: Draft = { id: 'd1', topic_id: 't1', text: 'Master', format: 'plain', language: '', revision: 1 }
    const variant: Variant = { id: 'v1', topic_id: 't1', account_id: 'a1', text: 'Variant', format: 'plain', status: 'draft', relation: 'linked', source_revision: '1', visibility: 'public', spoiler_text: '', local_only: false, outdated: false, published: false, publication_locked: false, publication_status: '' }
    const api = backend({ topics: [topic], drafts: [draft], variants: [variant] })
    let releaseSave!: () => void
    const saving = new Promise<void>((resolve) => { releaseSave = resolve })
    let first = true
    vi.stubGlobal('fetch', async (input: RequestInfo | URL, init?: RequestInit) => {
      if (first && String(input).endsWith('/topics/t1/draft') && init?.method === 'PUT') {
        first = false
        await saving
      }
      return api.fetch(input, init)
    })
    window.history.replaceState({}, '', '/topics/t1')
    mounted = mount(App)
    await flushPromises()
    await mounted.get('.draft-editor').setValue('First edit')
    await mounted.get('.draft-editor').trigger('blur')
    await mounted.get('button[aria-label="下一个版本"]').trigger('click')
    await mounted.get('.draft-editor').setValue('Latest edit')
    releaseSave()
    await flushPromises()
    expect(api.drafts.get('t1')?.text).toBe('Latest edit')
    expect((mounted.get('.draft-editor').element as HTMLTextAreaElement).value).toBe('Variant')
    expect(api.requests.filter(({ path, method }) => path.endsWith('/topics/t1/draft') && method === 'PUT')).toHaveLength(2)
  })

  it('keeps the current document when saving before an arrow switch fails', async () => {
    const topic: Topic = { id: 't1', profile_id: 'p1', title: 'Topic', status: 'active' }
    const draft: Draft = { id: 'd1', topic_id: 't1', text: 'Master', format: 'plain', language: '', revision: 1 }
    const variant: Variant = { id: 'v1', topic_id: 't1', account_id: 'a1', text: 'Variant', format: 'plain', status: 'draft', relation: 'linked', source_revision: '1', visibility: 'public', spoiler_text: '', local_only: false, outdated: false, published: false, publication_locked: false, publication_status: '' }
    const api = backend({ topics: [topic], drafts: [draft], variants: [variant] })
    vi.stubGlobal('fetch', api.fetch)
    window.history.replaceState({}, '', '/topics/t1')
    mounted = mount(App)
    await flushPromises()
    await mounted.get('.draft-editor').setValue('Unsaved master')
    api.failNextDraftWrites()
    await mounted.get('button[aria-label="下一个版本"]').trigger('click')
    await flushPromises()
    expect((mounted.get('.draft-editor').element as HTMLTextAreaElement).value).toBe('Unsaved master')
    expect(mounted.get('.variant-picker-current').text()).toBe('主稿')
    expect(mounted.get('[role="alert"]').text()).toContain('正文保存失败')
  })

  it('filters versions without changing or saving a different active document', async () => {
    const topic: Topic = { id: 't1', profile_id: 'p1', title: 'Topic', status: 'active' }
    const draft: Draft = { id: 'd1', topic_id: 't1', text: 'Master text', format: 'plain', language: '', revision: 1 }
    const makeVariant = (id: string, text: string): Variant => ({
      id, topic_id: 't1', account_id: 'a1', text, format: 'plain', status: 'draft', relation: 'linked',
      source_revision: '1', visibility: 'public', spoiler_text: '', local_only: false, outdated: false,
      published: false, publication_locked: false, publication_status: '',
    })
    const api = backend({ topics: [topic], drafts: [draft], variants: [makeVariant('v1', 'First version'), makeVariant('v2', 'Second version')] })
    vi.stubGlobal('fetch', api.fetch)
    window.history.replaceState({}, '', '/topics/t1')
    mounted = mount(App)
    await flushPromises()
    await mounted.get('button[aria-label="下一个版本"]').trigger('click')
    await mounted.get('.variant-picker-current').trigger('click')
    await mounted.get('input[aria-label="搜索稿件版本"]').setValue('Second version')
    expect(mounted.findAll('.variant-picker-list [role="option"]')).toHaveLength(1)
    expect((mounted.get('.draft-editor').element as HTMLTextAreaElement).value).toBe('First version')
    await mounted.get('input[aria-label="搜索稿件版本"]').setValue('no matching version')
    expect(mounted.get('.variant-picker-list').text()).toBe('没有匹配的版本')
    expect((mounted.get('.draft-editor').element as HTMLTextAreaElement).value).toBe('First version')
    await settleAutosave()
    expect(api.requests.filter(({ path, method }) => /\/variants\/v[12]$/.test(path) && method === 'PUT')).toHaveLength(0)
  })

  it('dismisses a draft action picker with Escape and resets its action before adding an account', async () => {
    const api = backend()
    vi.stubGlobal('fetch', api.fetch)
    mounted = mount(App, { attachTo: document.body })
    await flushPromises()
    await mounted.get('button[aria-label="翻译"]').trigger('click')
    expect(mounted.get('.target-add h2').text()).toBe('选择翻译源账号')
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await nextTick()
    expect(mounted.find('.target-add').exists()).toBe(false)
    await mounted.get('.variant-add').trigger('click')
    expect(mounted.get('.target-add h2').text()).toBe('选择发布账号')
    expect(api.requests.filter(({ method }) => method === 'POST')).toHaveLength(0)
  })

  it('keeps the editor mounted and saves the draft while a workspace window is open', async () => {
    const api = backend()
    vi.stubGlobal('fetch', api.fetch)
    mounted = mount(App, { attachTo: document.body })
    await flushPromises()
    const editor = mounted.get('.draft-editor').element as HTMLTextAreaElement
    await mounted.get('.draft-editor').setValue('Draft while settings are open')
    editor.setSelectionRange(6, 11)
    editor.scrollTop = 24
    await mounted.get('button[aria-label="设置"]').trigger('click')
    expect(mounted.get('.draft-editor').element).toBe(editor)
    expect(editor.selectionStart).toBe(6)
    expect(editor.scrollTop).toBe(24)
    await settleAutosave()
    expect(api.drafts.get(api.topics[0].id)?.text).toBe('Draft while settings are open')
    expect(window.location.pathname).toBe('/settings')
    await mounted.get('button[aria-label="关闭设置窗口"]').trigger('click')
    expect(mounted.get('.draft-editor').element).toBe(editor)
    expect(window.location.pathname).toBe(`/topics/${api.topics[0].id}`)
    expect(mounted.findAll('.post-preview-trigger')).toHaveLength(1)
  })

  it('closes the emoji tool with Escape and restores its trigger focus', async () => {
    vi.stubGlobal('fetch', backend().fetch)
    mounted = mount(App, { attachTo: document.body })
    await flushPromises()
    const trigger = mounted.get('button[aria-label="Emoji"]')
    ;(trigger.element as HTMLButtonElement).focus()
    await trigger.trigger('click')
    expect(mounted.get('.emoji-picker').attributes('role')).toBe('dialog')
    expect(mounted.get('.emoji-picker').attributes('aria-modal')).toBeUndefined()
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await nextTick()
    expect(mounted.find('.emoji-picker').exists()).toBe(false)
    expect(document.activeElement).toBe(trigger.element)
  })

  it('returns from a workspace window without discarding or reloading an unsaved draft', async () => {
    const topic: Topic = { id: 't1', profile_id: 'p1', title: 'Topic', status: 'active' }
    const draft: Draft = { id: 'd1', topic_id: 't1', text: 'Saved', format: 'plain', language: '', revision: 1 }
    vi.stubGlobal('fetch', backend({ topics: [topic], drafts: [draft] }).fetch)
    window.history.replaceState({}, '', '/topics/t1')
    mounted = mount(App)
    await flushPromises()
    await mounted.get('.draft-editor').setValue('Still editing')
    await mounted.get('button[aria-label="设置"]').trigger('click')
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    window.history.replaceState({}, '', '/topics/t1')
    window.dispatchEvent(new PopStateEvent('popstate'))
    await flushPromises()
    expect(confirm).not.toHaveBeenCalled()
    expect(mounted.find('.settings-page').exists()).toBe(false)
    expect((mounted.get('.draft-editor').element as HTMLTextAreaElement).value).toBe('Still editing')
    confirm.mockRestore()
  })

  it('removes a floating editor tool when navigating to another route', async () => {
    vi.stubGlobal('fetch', backend().fetch)
    mounted = mount(App)
    await flushPromises()
    await mounted.get('button[aria-label="图片"]').trigger('click')
    expect(mounted.find('.media-dialog').exists()).toBe(true)
    window.history.replaceState({}, '', '/settings')
    window.dispatchEvent(new PopStateEvent('popstate'))
    await flushPromises()
    expect(mounted.find('.media-dialog').exists()).toBe(false)
    expect(mounted.find('.settings-page').exists()).toBe(true)
  })
})
