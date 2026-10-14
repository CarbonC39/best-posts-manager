import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import App from '../src/App.vue'
import PublicationList from '../src/PublicationList.vue'
async function selectVariant(wrapper: any, label: string) {
  await wrapper.get('.variant-picker-current').trigger('click')
  await nextTick()
  const option = wrapper.findAll('.variant-picker-menu [role="option"]').find((item: any) => item.text().includes(label))
  expect(option, `variant option containing ${label}`).toBeTruthy()
  await option.trigger('click')
  await flushPromises()
}
async function openTopicRowMenu(wrapper: any, title: string) {
  const row = wrapper.findAll('.topic-row').find((item: any) => item.text().includes(title))
  expect(row, `topic row containing ${title}`).toBeTruthy()
  const menu = row!.get('details.topic-row-menu')
  ;(menu.element as HTMLDetailsElement).open = true
  await menu.trigger('toggle')
  menu.element.dispatchEvent(new Event('toggle'))
  await flushPromises()
}
async function openPickerAction(wrapper: any, label: string) {
  await wrapper.get('.variant-picker-current').trigger('click')
  await nextTick()
  const action = wrapper.findAll('.variant-picker-actions button').find((item: any) => item.text().includes(label))
  expect(action, `picker action containing ${label}`).toBeTruthy()
  await action!.trigger('click')
  await flushPromises()
}
async function selectLastVariantForAccount(wrapper: any, label: string) {
  await wrapper.get('.variant-picker-current').trigger('click')
  await nextTick()
  const options = wrapper.findAll('.variant-picker-menu [role="option"]').filter((item: any) => item.text().includes(label))
  expect(options.length, `variant options containing ${label}`).toBeGreaterThan(0)
  await options.at(-1)!.trigger('click')
  await flushPromises()
}
describe('editor shell',()=>{
  beforeEach(() => { window.history.replaceState({}, '', '/topics') })
  it('shows the topic dock, empty editor and single publish action',async()=>{global.fetch=async()=>new Response('[]',{status:200,headers:{'Content-Type':'application/json'}});const w=mount(App);expect(w.get('.topic-dock').exists()).toBe(true);expect(w.get('.editor-composer').exists()).toBe(true);expect(w.findAll('.post-preview-trigger')).toHaveLength(1);expect(w.get('.new-topic-action').text()).toContain('新建话题');await w.get('.utility-dock button[aria-label="管理"]').trigger('click');expect(w.get('.accounts-page h1').text()).toBe('账号管理');expect(w.text()).not.toContain('浏览器桥接 v1')})
  it('filters the sidebar by the selected profile and opens the chosen topic', async () => {
    const createdBodies: any[] = []
    const fetchMock = vi.fn(async (input: RequestInfo, init?: RequestInit) => {
      const path = String(input)
      let body: any = []
      if (path.endsWith('/profiles')) body = [{ id: 'p1', name: 'First' }, { id: 'p2', name: 'Second' }]
      else if (path.endsWith('/topics') && init?.method === 'POST') {
        const request = JSON.parse(String(init.body)); createdBodies.push(request)
        body = { id: 'new', profile_id: request.profile_id, title: request.title, status: 'active' }
      } else if (path.endsWith('/topics')) body = [{ id: 't1', profile_id: 'p1', title: 'Alpha', status: 'active' }, { id: 't2', profile_id: 'p2', title: 'Beta', status: 'active' }]
      else if (path.includes('/publications/summary?')) body = { action_count: 0, failed_count: 0 }
      else if (path.endsWith('/draft')) body = { topic_id: 't2', text: '', format: 'plain', revision: 1 }
      return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
    })
    global.fetch = fetchMock as any
    const wrapper = mount(App)
    await flushPromises()

    await wrapper.get('select[aria-label="切换 Profile"]').setValue('p2')
    await flushPromises()
    expect(wrapper.get('.topic-dock').text()).toContain('Beta')
    expect(wrapper.get('.topic-dock').text()).not.toContain('Alpha')
    await wrapper.get('input[aria-label="搜索话题"]').setValue('Beta')
    expect(wrapper.get('.topic-dock').text()).toContain('Beta')
    await wrapper.get('input[aria-label="搜索话题"]').setValue('missing')
    expect(wrapper.get('.topic-dock').text()).toContain('没有匹配的话题')
    await wrapper.get('input[aria-label="搜索话题"]').setValue('')
    await wrapper.get('.topic-row-select').trigger('click')
    await flushPromises()
    expect((wrapper.get('.draft-editor').element as HTMLTextAreaElement).value).toBe('')
    expect(window.location.pathname).toBe('/topics/t2')
    expect(createdBodies).toEqual([])
    wrapper.unmount()
  })
  it('opens a topic from the sidebar and loads it in the SPA editor', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo) => {
      const path = String(input)
      const body = path.endsWith('/profiles') ? [{ id: 'p1', name: 'First' }]
        : path.endsWith('/topics') ? [{ id: 'topic/one', profile_id: 'p1', title: 'Linked topic', status: 'active' }]
          : path.includes('/publications/summary?') ? { action_count: 0, failed_count: 0 }
            : []
      return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
    })
    global.fetch = fetchMock as any
    const wrapper = mount(App)
    await flushPromises()
    await wrapper.get('.topic-row-select').trigger('click')
    await flushPromises()
    expect(window.location.pathname).toBe('/topics/topic%2Fone')
    expect((wrapper.get('.draft-editor').element as HTMLTextAreaElement).value).toBe('')
    expect(fetchMock.mock.calls.some(call => String(call[0]).endsWith('/draft'))).toBe(true)
    expect(fetchMock.mock.calls.some(call => String(call[0]).includes('/variants?'))).toBe(true)
    wrapper.unmount()
  })
  it('archives and restores a topic once while sharing archived visibility with the sidebar', async () => {
    const updates: Array<{ title: string; status: string }> = []
    const fetchMock = vi.fn(async (input: RequestInfo, init?: RequestInit) => {
      const path = String(input)
      let body: any = []
      if (path.endsWith('/profiles')) body = [{ id: 'p1', name: 'First' }]
      else if (path.endsWith('/topics/t1') && init?.method === 'PUT') {
        const update = JSON.parse(String(init.body)); updates.push(update)
        body = { id: 't1', profile_id: 'p1', ...update }
      } else if (path.endsWith('/topics')) body = [
        { id: 't1', profile_id: 'p1', title: 'Active topic', status: 'active' },
        { id: 't2', profile_id: 'p1', title: 'Older archive', status: 'archived' },
      ]
      else if (path.includes('/publications/summary?')) body = { action_count: 0, failed_count: 0 }
      return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
    })
    global.fetch = fetchMock as any
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)
    const wrapper = mount(App)
    await flushPromises()

    expect(wrapper.get('.quick-topics').text()).toContain('Active topic')
    expect(wrapper.get('.quick-topics').text()).not.toContain('Older archive')

    await openTopicRowMenu(wrapper, 'Active topic')
    document.body.querySelector<HTMLButtonElement>('.topic-menu-floating button')!.click()
    await flushPromises()
    await flushPromises()
    expect(confirmSpy).toHaveBeenCalledTimes(1)
    expect(updates).toEqual([{ title: 'Active topic', status: 'archived' }])
    expect(wrapper.get('.quick-topics').text()).not.toContain('Active topic')

    await wrapper.get('.archived-topic-toggle').trigger('click')
    await nextTick()
    expect(wrapper.get('.quick-topics').text()).toContain('Active topic')
    expect(wrapper.get('.quick-topics').text()).toContain('Older archive')

    await openTopicRowMenu(wrapper, 'Active topic')
    document.body.querySelector<HTMLButtonElement>('.topic-menu-floating button')!.click()
    await flushPromises()
    await flushPromises()
    expect(updates).toEqual([
      { title: 'Active topic', status: 'archived' },
      { title: 'Active topic', status: 'active' },
    ])
    expect(confirmSpy).toHaveBeenCalledTimes(1)
    confirmSpy.mockRestore()
    wrapper.unmount()
  })
  it('renders the publication center view without changing navigation semantics', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo) => {
      const path = String(input)
      const body = path.endsWith('/profiles') ? [{ id: 'p1', name: 'p' }]
        : path.includes('/publications/summary?profile_id=p1') ? { action_count: 0, failed_count: 0 }
          : []
      return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
    })
    global.fetch = fetchMock as any
    const wrapper = mount(App)
    await flushPromises()
    await wrapper.get('.utility-dock button[aria-label="发布记录"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('.history-modal-host h1').text()).toBe('发布中心')
    expect(wrapper.get('.history-page').exists()).toBe(true)
    expect(wrapper.get('.utility-dock button[aria-label="发布记录"]').classes()).toContain('active')
    expect(fetchMock.mock.calls.some(call => String(call[0]).endsWith('/publications?profile_id=p1'))).toBe(true)
    expect(fetchMock.mock.calls.some(call => String(call[0]).endsWith('/accounts?profile_id=p1'))).toBe(true)
    wrapper.unmount()
  })
  it('routes publication center notifications once and refreshes the current profile badge on changes', async () => {
    let publicationSummary = { action_count: 0, failed_count: 0 }
    const fetchMock = vi.fn(async (input: RequestInfo) => {
      const path = String(input)
      const body = path.endsWith('/profiles') ? [{ id: 'p1', name: 'p' }]
        : path.includes('/publications/summary?profile_id=p1') ? publicationSummary
          : []
      return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
    })
    global.fetch = fetchMock as any
    const wrapper = mount(App, {
      global: {
        stubs: {
          PublicationCenterView: {
            props: ['profileId'],
            emits: ['notify', 'changed'],
            template: '<section><button class="emit-notify" @click="$emit(\'notify\', \'刷新失败\')">notify</button><button class="emit-changed" @click="$emit(\'changed\')">changed</button></section>',
          },
        },
      },
    })
    await flushPromises()
    await wrapper.get('.utility-dock button[aria-label="发布记录"]').trigger('click')
    await flushPromises()

    await wrapper.get('.emit-notify').trigger('click')
    expect(wrapper.findAll('.toast')).toHaveLength(1)
    expect(wrapper.get('.toast').text()).toContain('刷新失败')

    const summaryCallsBeforeChange = fetchMock.mock.calls.filter(call => String(call[0]).includes('/publications/summary?profile_id=p1')).length
    publicationSummary = { action_count: 2, failed_count: 1 }
    await wrapper.get('.emit-changed').trigger('click')
    await flushPromises()

    const summaryCallsAfterChange = fetchMock.mock.calls.filter(call => String(call[0]).includes('/publications/summary?profile_id=p1')).length
    expect(summaryCallsAfterChange).toBe(summaryCallsBeforeChange + 1)
    const badge = wrapper.get('.utility-dock button[aria-label="发布记录"] i')
    expect(badge.exists()).toBe(true)
    expect(badge.classes()).toContain('failure')
    wrapper.unmount()
  })
  it('loads a topic, saves its draft and marks a URL locally',async()=>{
    const fetchMock=vi.fn(async(input:RequestInfo, init?:RequestInit)=>{const path=String(input);let body:any=[];if(path.endsWith('/profiles'))body=[{id:'p1',name:'p'}];else if(path.endsWith('/accounts'))body=[{id:'a1',profile_id:'p1',platform:'mastodon',name:'a'}];else if(path.endsWith('/topics'))body=[{id:'t1',profile_id:'p1',title:'Topic',status:'active'}];else if(path.endsWith('/bridge/pairings'))body=[];else if(path.endsWith('/draft'))body={topic_id:'t1',text:'old',format:'plain',revision:1};else if(path.includes('/variants'))body=[];else if(path.includes('/media'))body=[];else if(path.includes('/external-posts')&&(!init||init.method==='GET'))body=[];else if(path.endsWith('/external-posts'))body={id:'e1'};else body={topic_id:'t1',text:'new',format:'plain',revision:2};return new Response(JSON.stringify(body),{status:200,headers:{'Content-Type':'application/json'}})});global.fetch=fetchMock as any
    const w=mount(App);await flushPromises();await nextTick();await w.get('.topic-row-select').trigger('click');await flushPromises();const area=w.get('.draft-editor');await area.setValue('new');await area.trigger('blur');await flushPromises();expect(fetchMock.mock.calls.some(c=>String(c[0]).endsWith('/draft')&&c[1]?.method==='PUT')).toBe(true);await openPickerAction(w, '站外关联');const url=w.get('input[aria-label="站外帖子 URL"]');await url.setValue('https://example.test/p');const account=w.get('select[aria-label="关联账号"]');await account.setValue('a1');const markerForm=w.get('.external-tool form');await markerForm.trigger('submit');await flushPromises();expect(fetchMock.mock.calls.some(c=>String(c[0]).endsWith('/external-posts')&&c[1]?.method==='POST')).toBe(true)
  })
  it('selects profile/account and handles variant state, apply and publish errors',async()=>{
    const fetchMock=vi.fn(async(input:RequestInfo, init?:RequestInit)=>{const path=String(input);let body:any=[];let status=200;if(path.endsWith('/profiles'))body=[{id:'p1',name:'one'},{id:'p2',name:'two'}];else if(path.endsWith('/accounts'))body=[{id:'a1',profile_id:'p1',platform:'mastodon',name:'a1'},{id:'a2',profile_id:'p2',platform:'mastodon',name:'a2'}];else if(path.endsWith('/topics'))body=[{id:'t1',profile_id:'p2',title:'Topic',status:'active'}];else if(path.endsWith('/draft'))body={topic_id:'t1',text:'old',format:'plain',revision:1};else if(path.includes('/variants?'))body=[{id:'v1',topic_id:'t1',account_id:'a2',text:'variant',format:'plain',status:'draft',relation:'linked',visibility:'public',spoiler_text:'',outdated:true}];else if(path.endsWith('/apply'))body={id:'v1',topic_id:'t1',account_id:'a2',text:'updated',format:'plain',status:'draft',relation:'linked',visibility:'public',spoiler_text:'',outdated:false};else if(path.includes('/variants/'))body={id:'v1',topic_id:'t1',account_id:'a2',text:'edited',format:'plain',status:'ready',relation:'detached',visibility:'private',spoiler_text:'cw',outdated:false};else if(path.endsWith('/publications')){status=400;body={error:'token environment variable is empty'}}else if(path.includes('/media')||path.includes('/external-posts'))body=[];return new Response(JSON.stringify(body),{status,headers:{'Content-Type':'application/json'}})});global.fetch=fetchMock as any
    const w=mount(App);await flushPromises();await w.get('select[aria-label="切换 Profile"]').setValue('p2');await w.get('.topic-row-select').trigger('click');await flushPromises();await selectVariant(w, 'a2');expect(w.get('.variant-picker-current').text()).toContain('a2');vi.spyOn(window,'confirm').mockReturnValue(true);await openPickerAction(w, '应用主稿更新');expect(fetchMock.mock.calls.some(call=>String(call[0]).endsWith('/variants/v1/apply')&&call[1]?.method==='POST')).toBe(true);await w.get('.draft-editor').setValue('edited');await w.get('.draft-editor').trigger('blur');await flushPromises();expect(fetchMock.mock.calls.some(call=>String(call[0]).endsWith('/variants/v1')&&call[1]?.method==='PUT')).toBe(true);await w.get('.post-preview-trigger').trigger('click');await flushPromises();await w.get('[role="dialog"] button.primary-action').trigger('click');await flushPromises();expect(w.text()).toContain('发布失败：token environment variable is empty')
  })
  it('offers retry only for retryable failed publications',async()=>{
    const fetchMock=vi.fn(async(input:RequestInfo)=>new Response(JSON.stringify(String(input).endsWith('/publications/retry')?{}:[{id:'pub1',status:'failed',remote_url:'',error_message:'offline',error_category:'retryable'}]),{status:200,headers:{'Content-Type':'application/json'}}));global.fetch=fetchMock as any
    const w=mount(PublicationList);await flushPromises();expect(w.text()).toContain('重试原始内容');await w.findAll('button').find(button=>button.text()==='重试原始内容')!.trigger('click');await flushPromises();expect(fetchMock.mock.calls.some(c=>String(c[0]).endsWith('/publications/retry'))).toBe(true)
  })
  it('locks a variant after an unresolved failed publication',async()=>{
    const fetchMock=vi.fn(async(input:RequestInfo)=>{const path=String(input);let body:any=[];if(path.endsWith('/profiles'))body=[{id:'p1',name:'p'}];else if(path.endsWith('/accounts'))body=[{id:'a1',profile_id:'p1',platform:'mastodon',name:'main'}];else if(path.endsWith('/topics'))body=[{id:'t1',profile_id:'p1',title:'Topic',status:'active'}];else if(path.endsWith('/draft'))body={topic_id:'t1',text:'draft',format:'plain',revision:1};else if(path.includes('/variants?'))body=[{id:'v1',topic_id:'t1',account_id:'a1',text:'failed body',format:'plain',status:'ready',relation:'linked',visibility:'public',spoiler_text:'',local_only:false,outdated:false,published:false,publication_locked:true,publication_status:'failed'}];else if(path.includes('/capability'))body={platform:'mastodon',format:'plain',visibilities:['public']};return new Response(JSON.stringify(body),{status:200,headers:{'Content-Type':'application/json'}})});global.fetch=fetchMock as any
    const w=mount(App);await flushPromises();await w.get('.topic-row-select').trigger('click');await flushPromises();await selectVariant(w, 'main');await nextTick();expect(w.get('.draft-editor').attributes('disabled')).toBeDefined();expect(w.get('.post-preview-trigger').attributes('disabled')).toBeDefined()
  })
  it('saves an edited variant before publishing it',async()=>{
    const calls:string[]=[];const fetchMock=vi.fn(async(input:RequestInfo,init?:RequestInit)=>{const path=String(input);let body:any=[];if(path.endsWith('/profiles'))body=[{id:'p1',name:'p'}];else if(path.endsWith('/accounts'))body=[{id:'a1',profile_id:'p1',platform:'mastodon',name:'main'}];else if(path.endsWith('/topics'))body=[{id:'t1',profile_id:'p1',title:'Topic',status:'active'}];else if(path.endsWith('/draft'))body={topic_id:'t1',text:'draft',format:'plain',revision:1};else if(path.includes('/variants?'))body=[{id:'v1',topic_id:'t1',account_id:'a1',text:'old',format:'plain',status:'draft',relation:'linked',visibility:'public',spoiler_text:'',local_only:false,outdated:false}];else if(path.endsWith('/variants/v1')&&init?.method==='PUT'){calls.push(`save:${JSON.parse(String(init.body)).text}`);body={id:'v1',topic_id:'t1',account_id:'a1',text:'new text',format:'plain',status:'draft',relation:'detached',visibility:'public',spoiler_text:'',local_only:false,outdated:false}}else if(path.endsWith('/publications')&&init?.method==='POST'){calls.push('publish');body={id:'pub1',status:'published'}}else if(path.includes('/capability'))body={platform:'mastodon',format:'plain',visibilities:['public']};return new Response(JSON.stringify(body),{status:200,headers:{'Content-Type':'application/json'}})});global.fetch=fetchMock as any
    const w=mount(App);await flushPromises();await w.get('.topic-row-select').trigger('click');await flushPromises();await selectVariant(w, 'main');await w.get('.draft-editor').setValue('new text');await w.get('.draft-editor').trigger('blur');await flushPromises();await w.get('.post-preview-trigger').trigger('click');await flushPromises();await w.get('[role="dialog"] button.primary-action').trigger('click');await flushPromises();expect(calls).toEqual(['save:new text','publish'])
  })
  it('does not publish when saving the edited variant fails',async()=>{
    const fetchMock=vi.fn(async(input:RequestInfo,init?:RequestInit)=>{const path=String(input);let body:any=[];let status=200;if(path.endsWith('/profiles'))body=[{id:'p1',name:'p'}];else if(path.endsWith('/accounts'))body=[{id:'a1',profile_id:'p1',platform:'mastodon',name:'main'}];else if(path.endsWith('/topics'))body=[{id:'t1',profile_id:'p1',title:'Topic',status:'active'}];else if(path.endsWith('/draft'))body={topic_id:'t1',text:'draft',format:'plain',revision:1};else if(path.includes('/variants?'))body=[{id:'v1',topic_id:'t1',account_id:'a1',text:'old',format:'plain',status:'draft',relation:'linked',visibility:'public',spoiler_text:'',local_only:false,outdated:false}];else if(path.endsWith('/variants/v1')&&init?.method==='PUT'){status=500;body={error:'disk full'}}else if(path.includes('/capability'))body={platform:'mastodon',format:'plain',visibilities:['public']};return new Response(JSON.stringify(body),{status,headers:{'Content-Type':'application/json'}})});global.fetch=fetchMock as any
    const w=mount(App);await flushPromises();await w.get('.topic-row-select').trigger('click');await flushPromises();await selectVariant(w, 'main');await w.get('.draft-editor').setValue('new text');await w.get('.draft-editor').trigger('blur');await flushPromises();await w.get('.post-preview-trigger').trigger('click');await flushPromises();expect(w.get('.toast').text()).toContain('disk full');expect(fetchMock.mock.calls.some(call=>String(call[0]).endsWith('/publications')&&call[1]?.method==='POST')).toBe(false)
  })
  it('creates a snapshot before publishing the base draft and preserves the real failure',async()=>{const calls:string[]=[];const fetchMock=vi.fn(async(input:RequestInfo,init?:RequestInit)=>{const path=String(input);let body:any=[];let status=200;if(path.endsWith('/profiles'))body=[{id:'p1',name:'p'}];else if(path.endsWith('/accounts'))body=[{id:'a1',profile_id:'p1',platform:'mastodon',name:'main'}];else if(path.endsWith('/topics'))body=[{id:'t1',profile_id:'p1',title:'Topic',status:'active'}];else if(path.endsWith('/draft'))body={topic_id:'t1',text:'draft',format:'plain',revision:1};else if(path.includes('/variants?'))body=[];else if(path.endsWith('/variants')&&init?.method==='POST'){calls.push('snapshot');body={id:'v1',topic_id:'t1',account_id:'a1',text:'draft',format:'plain',status:'draft',relation:'linked',visibility:'public',spoiler_text:'',local_only:false,outdated:false}}else if(path.endsWith('/publications')&&init?.method==='POST'){calls.push('publish');status=400;body={error:'正文提交阶段失败：instance returned 422'}}else if(path.includes('/capability'))body={platform:'mastodon',format:'plain',visibilities:['public']};return new Response(JSON.stringify(body),{status,headers:{'Content-Type':'application/json'}})});global.fetch=fetchMock as any;const w=mount(App);await flushPromises();await w.get('.topic-row-select').trigger('click');await flushPromises();await w.get('.post-preview-trigger').trigger('click');await nextTick();await w.get('.target-account-results button').trigger('click');await w.get('.target-account-submit').trigger('click');await flushPromises();expect(w.get('#publish-dialog-title').exists()).toBe(true);await w.get('[role="dialog"] button.primary-action').trigger('click');await flushPromises();expect(calls).toEqual(['snapshot','publish']);expect(w.text()).toContain('发布失败：正文提交阶段失败');expect(w.get('.variant-picker-current').exists()).toBe(true)})
  it('searches many platform versions while rendering only the selected editor',async()=>{const fetchMock=vi.fn(async(input:RequestInfo)=>{const path=String(input);let body:any=[];if(path.endsWith('/profiles'))body=[{id:'p1',name:'p'}];else if(path.endsWith('/accounts'))body=[{id:'a1',profile_id:'p1',platform:'mastodon',name:'first'},{id:'a2',profile_id:'p1',platform:'bluesky',name:'second'}];else if(path.endsWith('/topics'))body=[{id:'t1',profile_id:'p1',title:'Topic',status:'active'}];else if(path.endsWith('/draft'))body={topic_id:'t1',text:'draft',format:'plain',revision:1};else if(path.includes('/variants?'))body=[{id:'v1',topic_id:'t1',account_id:'a1',text:'ordinary',format:'plain',status:'draft',relation:'linked',visibility:'public',spoiler_text:'',local_only:false,outdated:false},{id:'v2',topic_id:'t1',account_id:'a2',text:'unique second body',format:'plain',status:'draft',relation:'linked',visibility:'public',spoiler_text:'',local_only:false,outdated:false}];else if(path.includes('/capability'))body={platform:path.includes('a2')?'bluesky':'mastodon',format:'plain',visibilities:['public']};return new Response(JSON.stringify(body),{status:200,headers:{'Content-Type':'application/json'}})});global.fetch=fetchMock as any;const w=mount(App);await flushPromises();await w.get('.topic-row-select').trigger('click');await flushPromises();expect(w.findAll('.draft-editor')).toHaveLength(1);await selectVariant(w, 'second');expect((w.get('.draft-editor').element as HTMLTextAreaElement).value).toBe('unique second body');await selectVariant(w, 'first');expect((w.get('.draft-editor').element as HTMLTextAreaElement).value).toBe('ordinary')})
  it('lets an uncertain publication be confirmed from history',async()=>{
    const fetchMock=vi.fn(async(input:RequestInfo,init?:RequestInit)=>{const path=String(input);let body:any=[];if(path.endsWith('/accounts'))body=[{id:'a1',name:'main',platform:'x'}];else if(path.endsWith('/publications'))body=[{id:'pub1',account_id:'a1',status:'needs_confirmation',remote_url:'',remote_id:'',remote_cid:'',error_message:'结果未知',error_category:'unknown',source:'bridge',created_at:new Date().toISOString()}];else if(path.endsWith('/publications/pub1/confirm')&&init?.method==='POST')body={id:'pub1',status:'published'};return new Response(JSON.stringify(body),{status:200,headers:{'Content-Type':'application/json'}})});global.fetch=fetchMock as any
    const w=mount(PublicationList);await flushPromises();expect(w.text()).toContain('需要人工确认');await w.get('input[placeholder="帖子永久链接 https://…"]').setValue('https://x.com/user/status/1');await w.findAll('button').find(button=>button.text()==='确认已发布')!.trigger('click');await flushPromises();const call=fetchMock.mock.calls.find(item=>String(item[0]).endsWith('/publications/pub1/confirm'));expect(JSON.parse(String(call?.[1]?.body)).remote_url).toBe('https://x.com/user/status/1')
  })
  it('uses account capabilities for platform-specific editor controls',async()=>{
    const fetchMock=vi.fn(async(input:RequestInfo)=>{const path=String(input);let body:any=[];if(path.endsWith('/profiles'))body=[{id:'p1',name:'p'}];else if(path.endsWith('/accounts'))body=[{id:'a1',profile_id:'p1',platform:'misskey',name:'mk'}];else if(path.endsWith('/accounts/a1/capability'))body={platform:'misskey',format:'mfm',text_max_bytes:0,max_media:4,max_media_bytes:33554432,visibilities:['public','home','followers'],supports_cw:true,supports_local_only:true,custom_emoji:true};else if(path.endsWith('/topics'))body=[{id:'t1',profile_id:'p1',title:'Topic',status:'active'}];else if(path.endsWith('/draft'))body={topic_id:'t1',text:'old',format:'plain',revision:1};else if(path.includes('/variants'))body=[{id:'v1',topic_id:'t1',account_id:'a1',text:'MFM',format:'mfm',status:'draft',relation:'linked',visibility:'home',spoiler_text:'',local_only:false,outdated:false}];return new Response(JSON.stringify(body),{status:200,headers:{'Content-Type':'application/json'}})});global.fetch=fetchMock as any
    const w=mount(App);await flushPromises();await w.get('.topic-row-select').trigger('click');await flushPromises();await selectVariant(w, 'mk');await w.get('button[aria-label="Emoji"]').trigger('click');await nextTick();expect(w.get('input[aria-label="搜索实例 emoji"]').exists()).toBe(true);await w.get('.variant-picker-current').trigger('click');await nextTick();const actions=w.get('.variant-picker-actions');expect(actions.get('select[aria-label="mk · Misskey可见性"]').text()).toContain('仅关注者');expect(actions.get('input[type=checkbox]').exists()).toBe(true);expect(actions.get('select[aria-label="mk · Misskey可见性"]').text()).not.toContain('私有')
  })
  it('shows the browser bridge controls and manual confirmation boundary',async()=>{
    const fetchMock=vi.fn(async(input:RequestInfo)=>{const path=String(input);let body:any=[];if(path.endsWith('/profiles'))body=[{id:'p1',name:'one'}];else if(path.endsWith('/accounts'))body=[{id:'a1',profile_id:'p1',platform:'x',name:'x'}];else if(path.endsWith('/topics'))body=[{id:'t1',profile_id:'p1',title:'Topic',status:'active'}];else if(path.endsWith('/draft'))body={topic_id:'t1',text:'old',format:'plain',revision:1};else if(path.includes('/variants?'))body=[{id:'v1',topic_id:'t1',account_id:'a1',text:'bridge',format:'plain',status:'draft',relation:'linked',visibility:'public',spoiler_text:'',local_only:false,outdated:false}];else if(path.includes('/capability'))body={platform:'x',format:'plain',visibilities:['public']};return new Response(JSON.stringify(body),{status:200,headers:{'Content-Type':'application/json'}})});global.fetch=fetchMock as any
    const w=mount(App);await flushPromises();await w.get('.utility-dock button[aria-label="设置"]').trigger('click');await flushPromises();expect(w.text()).toContain('浏览器桥接');expect(w.find('.bridge-settings').exists()).toBe(true);expect(w.text()).toContain('生成配对码');expect(w.get('.bridge-settings a[href="/userscript/bpm-bridge.user.js"]').text()).toContain('安装脚本');expect(w.text()).not.toContain('Mastodon 官方 API');expect(w.findAll('.post-preview-trigger')).toHaveLength(1)
  })
  it('copies a new bridge code and explains clipboard failure',async()=>{
    const fetchMock=vi.fn(async(input:RequestInfo,init?:RequestInit)=>{const path=String(input);let body:any=[];if(path.endsWith('/profiles'))body=[{id:'p1',name:'one'}];else if(path.endsWith('/accounts'))body=[{id:'a1',profile_id:'p1',platform:'x',name:'x'}];else if(path.endsWith('/topics'))body=[{id:'t1',profile_id:'p1',title:'Topic',status:'active'}];else if(path.endsWith('/draft'))body={topic_id:'t1',text:'old',format:'plain',revision:1};else if(path.includes('/variants?'))body=[];else if(path.endsWith('/bridge/pairings')&&init?.method==='POST')body={id:'pair1',platform:'x',code:'ABC123',expires_at:new Date(Date.now()+600000).toISOString(),code_used:false,revoked:false};else if(path.includes('/capability'))body={platform:'x',format:'plain',visibilities:['public']};return new Response(JSON.stringify(body),{status:200,headers:{'Content-Type':'application/json'}})});global.fetch=fetchMock as any
    const writeText=vi.fn().mockResolvedValue(undefined);Object.defineProperty(navigator,'clipboard',{value:{writeText},configurable:true});const w=mount(App);await flushPromises();await w.get('.utility-dock button[aria-label="设置"]').trigger('click');await flushPromises();const create=w.findAll('button').find(button=>button.text().includes('生成配对'))!;await create.trigger('click');await flushPromises();expect(writeText).toHaveBeenCalledWith('ABC123');writeText.mockRejectedValueOnce(new Error('denied'));await w.findAll('button').find(button=>button.text()==='复制')!.trigger('click');await flushPromises();expect(w.text()).toContain('无法自动复制')
  })
  it('sends explicit auto-publish intent and deletes an unpublished variant',async()=>{
    const fetchMock=vi.fn(async(input:RequestInfo,init?:RequestInit)=>{const path=String(input);let body:any=[];if(path.endsWith('/profiles'))body=[{id:'p1',name:'one'}];else if(path.endsWith('/accounts'))body=[{id:'a1',profile_id:'p1',platform:'x',name:'x'}];else if(path.endsWith('/topics'))body=[{id:'t1',profile_id:'p1',title:'Topic',status:'active'}];else if(path.endsWith('/bridge/pairings'))body=[{id:'pair1',platform:'x',code_used:true,connected:true,revoked:false,page_url:'https://x.com/compose',expires_at:new Date(Date.now()+600000).toISOString()}];else if(path.endsWith('/draft'))body={topic_id:'t1',text:'old',format:'plain',revision:1};else if(path.includes('/variants?'))body=[{id:'v1',topic_id:'t1',account_id:'a1',text:'bridge',format:'plain',status:'draft',relation:'linked',visibility:'public',spoiler_text:'',local_only:false,outdated:false},{id:'v2',topic_id:'t1',account_id:'a1',text:'spare',format:'plain',status:'draft',relation:'detached',visibility:'public',spoiler_text:'',local_only:false,outdated:false}];else if(path.includes('/media?'))body=[{id:'m1',topic_id:'t1',variant_id:'',filename:'image.png',mime:'image/png'}];else if(path.endsWith('/bridge/requests/fill'))body={id:'request1',kind:'fill',status:'pending'};else if(path.includes('/capability'))body={platform:'x',format:'plain',visibilities:['public']};else if((path.endsWith('/variants/v2')||path.endsWith('/media/m1'))&&init?.method==='DELETE')body={status:'deleted'};return new Response(JSON.stringify(body),{status:200,headers:{'Content-Type':'application/json'}})});global.fetch=fetchMock as any
    vi.spyOn(window,'confirm').mockReturnValue(true);const w=mount(App);await flushPromises();await w.get('.topic-row-select').trigger('click');await flushPromises();await selectVariant(w, 'x');await w.get('.post-preview-trigger').trigger('click');await flushPromises();await w.get('.dialog-auto-publish input').setValue(true);await w.get('[role="dialog"] button.primary-action').trigger('click');await flushPromises();const fill=fetchMock.mock.calls.find(call=>String(call[0]).endsWith('/bridge/requests/fill'));expect(JSON.parse(String(fill?.[1]?.body)).auto_publish).toBe(true);expect(fetchMock.mock.calls.some(call=>String(call[0]).endsWith('/bridge/requests/fill')&&call[1]?.method==='POST')).toBe(true);await w.get('button[aria-label="删除图片 image.png"]').trigger('click');await flushPromises();expect(fetchMock.mock.calls.some(call=>String(call[0]).endsWith('/media/m1')&&call[1]?.method==='DELETE')).toBe(true);await selectLastVariantForAccount(w, 'x');await openPickerAction(w, '移除版本');await flushPromises();expect(fetchMock.mock.calls.some(call=>String(call[0]).endsWith('/variants/v2')&&call[1]?.method==='DELETE')).toBe(true)
  })
  it('keeps the source on provider error and shows recovery guidance',async()=>{const fetchMock=vi.fn(async(input:RequestInfo,init?:RequestInit)=>{const path=String(input);let body:any=[];let status=200;if(path.endsWith('/profiles'))body=[{id:'p1',name:'one'}];else if(path.endsWith('/accounts'))body=[{id:'a1',profile_id:'p1',platform:'mastodon',name:'main'}];else if(path.endsWith('/topics'))body=[{id:'t1',profile_id:'p1',title:'Topic',status:'active'}];else if(path.endsWith('/translation-configs'))body=[{id:'c1',provider:'openai',model:'fake',token_env:'TRANSLATION_TOKEN'}];else if(path.endsWith('/draft'))body={topic_id:'t1',text:'source',format:'plain',revision:1};else if(path.includes('/variants?'))body=[{id:'v1',topic_id:'t1',account_id:'a1',text:'source',format:'plain',status:'draft',relation:'linked',visibility:'public',spoiler_text:'',local_only:false,outdated:false}];else if(path.includes('/capability'))body={platform:'mastodon',format:'plain',visibilities:['public']};else if(path.endsWith('/translations')){status=400;body={error:'provider offline'}};return new Response(JSON.stringify(body),{status,headers:{'Content-Type':'application/json'}})});global.fetch=fetchMock as any
    const w=mount(App);await flushPromises();await w.get('.topic-row-select').trigger('click');await flushPromises();await selectVariant(w, 'main');await w.get('button[aria-label="翻译"]').trigger('click');await nextTick();await w.get('select[aria-label="翻译配置"]').setValue('c1');await w.get('select[aria-label="翻译目标账号"]').setValue('a1');await w.get('input[placeholder="例如 zh"]').setValue('zh');const button=w.findAll('button').find(item=>item.text()==='生成版本');expect(button).toBeTruthy();await button!.trigger('click');await flushPromises();expect(w.text()).toContain('翻译失败：provider offline');expect(w.text()).toContain('原文与目标版本均未改动')
  })
  it('adds a translated detached variant without replacing the source',async()=>{const fetchMock=vi.fn(async(input:RequestInfo,init?:RequestInit)=>{const path=String(input);let body:any=[];if(path.endsWith('/profiles'))body=[{id:'p1',name:'one'}];else if(path.endsWith('/accounts'))body=[{id:'a1',profile_id:'p1',platform:'mastodon',name:'main'}];else if(path.endsWith('/topics'))body=[{id:'t1',profile_id:'p1',title:'Topic',status:'active'}];else if(path.endsWith('/translation-configs'))body=[{id:'c1',provider:'openai',model:'fake',token_env:'TRANSLATION_TOKEN'}];else if(path.endsWith('/draft'))body={topic_id:'t1',text:'source',format:'plain',revision:1};else if(path.includes('/variants?'))body=[{id:'v1',topic_id:'t1',account_id:'a1',text:'source',format:'plain',status:'draft',relation:'linked',visibility:'public',spoiler_text:'',local_only:false,outdated:false}];else if(path.includes('/capability'))body={platform:'mastodon',format:'plain',visibilities:['public']};else if(path.endsWith('/translations')&&init?.method==='POST')body={variant:{id:'v2',topic_id:'t1',account_id:'a1',text:'译文',format:'plain',status:'draft',relation:'detached',visibility:'public',spoiler_text:'',local_only:false,outdated:false},translation:{id:'tr1'}};return new Response(JSON.stringify(body),{status:200,headers:{'Content-Type':'application/json'}})});global.fetch=fetchMock as any
    const w=mount(App);await flushPromises();await w.get('.topic-row-select').trigger('click');await flushPromises();await selectVariant(w, 'main');await w.get('button[aria-label="翻译"]').trigger('click');await nextTick();await w.get('select[aria-label="翻译配置"]').setValue('c1');await w.get('select[aria-label="翻译目标账号"]').setValue('a1');await w.get('input[placeholder="例如 zh"]').setValue('zh');const button=w.findAll('button').find(item=>item.text()==='生成版本');await button!.trigger('click');await flushPromises();expect((w.get('.draft-editor').element as HTMLTextAreaElement).value).toBe('译文');expect(fetchMock.mock.calls.some(call=>String(call[0]).endsWith('/translations')&&call[1]?.method==='POST'&&JSON.parse(String(call[1].body)).source_variant_id==='v1')).toBe(true)
  })
  it('does not crash when an empty database returns null lists',async()=>{global.fetch=vi.fn(async()=>new Response('null',{status:200,headers:{'Content-Type':'application/json'}})) as any;const w=mount(App);await flushPromises();expect(w.get('.editor-composer').exists()).toBe(true);expect(w.get('.quick-empty').text()).toContain('还没有话题');expect(w.text()).not.toContain('TypeError')})
  it('renders an empty publication history when the old API returns null',async()=>{global.fetch=vi.fn(async()=>new Response('null',{status:200,headers:{'Content-Type':'application/json'}})) as any;const w=mount(PublicationList);await flushPromises();expect(w.text()).toContain('还没有发布记录。');expect(w.text()).not.toContain('TypeError')})
  it('stops publication polling when leaving the center',async()=>{global.fetch=vi.fn(async()=>new Response('[]',{status:200,headers:{'Content-Type':'application/json'}})) as any;const clear=vi.spyOn(window,'clearInterval');const w=mount(PublicationList);await flushPromises();w.unmount();expect(clear).toHaveBeenCalled();clear.mockRestore()})
  it('refreshes the main navigation publication badge outside the publication center', async () => {
    let publicationSummary = { action_count: 0, failed_count: 0 }
    const fetchMock = vi.fn(async (input: RequestInfo) => {
      const path = String(input)
      const body = path.endsWith('/profiles') ? [{ id: 'p1', name: 'p' }]
        : path.includes('/publications/summary?profile_id=p1') ? publicationSummary
          : []
      return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
    })
    global.fetch = fetchMock as any
    const w = mount(App)
    await flushPromises()
    expect(w.find('.utility-dock button[aria-label="发布记录"] .pending-count').exists()).toBe(false)

    publicationSummary = { action_count: 1, failed_count: 1 }
    window.dispatchEvent(new Event('focus'))
    await flushPromises()

    const badge = w.get('.utility-dock button[aria-label="发布记录"] i')
    expect(badge.exists()).toBe(true)
    expect(badge.classes()).toContain('failure')
    w.unmount()
  })
  it('normalizes a null variant list while opening a topic',async()=>{const fetchMock=vi.fn(async(input:RequestInfo)=>{const path=String(input);let body:any=null;if(path.endsWith('/profiles'))body=[{id:'p1',name:'p'}];else if(path.endsWith('/accounts'))body=[{id:'a1',profile_id:'p1',platform:'mastodon',name:'a'}];else if(path.endsWith('/topics'))body=[{id:'t1',profile_id:'p1',title:'Topic',status:'active'}];else if(path.endsWith('/draft'))body={topic_id:'t1',text:'draft',format:'plain',revision:1};return new Response(JSON.stringify(body),{status:200,headers:{'Content-Type':'application/json'}})});global.fetch=fetchMock as any;const w=mount(App);await flushPromises();await w.get('.topic-row-select').trigger('click');await flushPromises();expect(w.get('.variant-add').exists()).toBe(true);expect(w.get('button[aria-label="上一个版本"]').attributes('disabled')).toBeDefined();expect(w.text()).not.toContain('TypeError')})
  it('keeps account setup on its management panel with a stable toggle',async()=>{global.fetch=vi.fn(async()=>new Response('[]',{status:200,headers:{'Content-Type':'application/json'}})) as any;const w=mount(App);await flushPromises();expect(w.get('.editor-composer').exists()).toBe(true);expect(w.find('input[placeholder="账号名称"]').exists()).toBe(false);await w.get('.utility-dock button[aria-label="管理"]').trigger('click');await nextTick();expect(w.get('.accounts-page h1').text()).toBe('账号管理');const toggle=w.get('.account-create-toggle');const panel=w.get('#account-create-panel');expect(toggle.attributes('aria-expanded')).toBe('false');expect(panel.attributes('style')).toContain('display: none');await toggle.trigger('click');expect(toggle.attributes('aria-expanded')).toBe('true');expect(panel.attributes('style') || '').not.toContain('display: none');expect(w.find('input[placeholder="账号名称"]').exists()).toBe(true);await w.get('.utility-dock button[aria-label="设置"]').trigger('click');expect(w.find('input[placeholder="账号名称"]').exists()).toBe(false)})
  it('clears platform-specific account fields and expands setup for reconnect', async () => {
    global.fetch = vi.fn(async (input: RequestInfo) => {
      const path = String(input)
      const body = path.endsWith('/profiles') ? [{ id: 'p1', name: 'p' }]
        : path.endsWith('/accounts') ? [{ id: 'a1', profile_id: 'p1', platform: 'x', name: 'old x', remote_id: 'remote-x', disabled: true }, { id: 'a2', profile_id: 'p1', platform: 'bluesky', name: 'active sky', remote_id: 'sky.test' }]
          : []
      return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
    }) as any
    const w = mount(App)
    await flushPromises()
    await w.get('.utility-dock button[aria-label="管理"]').trigger('click')
    await w.get('.account-create-toggle').trigger('click')
    const platform = w.findAll('select').find(select => select.find('option[value="bluesky"]').exists())!
    await w.get('input[placeholder="账号名称"]').setValue('keep name')
    await w.get('input[placeholder="平台实例 URL"]').setValue('https://old.test')
    await w.get('input[placeholder="访问令牌"]').setValue('old-secret')
    await platform.setValue('bluesky')
    expect((w.get('input[placeholder="账号名称"]').element as HTMLInputElement).value).toBe('keep name')
    expect((w.get('input[placeholder="Bluesky PDS URL"]').element as HTMLInputElement).value).toBe('https://bsky.social')
    expect((w.get('input[placeholder="app password"]').element as HTMLInputElement).value).toBe('')
    await w.get('input[placeholder="app password"]').setValue('close-me')
    await w.get('.account-create-toggle').trigger('click')
    expect(w.get('#account-create-panel').attributes('style')).toContain('display: none')
    await w.get('.account-create-toggle').trigger('click')
    expect((w.get('input[placeholder="app password"]').element as HTMLInputElement).value).toBe('')
    await w.get('input[placeholder="app password"]').setValue('route-away')
    await w.get('[data-account-id="a2"] .account-menu-trigger').trigger('click')
    await w.get('button[aria-label="编辑账号"]').trigger('click')
    await w.get('.account-edit input[type="password"]').setValue('edit-route-away')
    await w.get('.workspace-brand').trigger('click')
    await w.get('.utility-dock button[aria-label="管理"]').trigger('click')
    expect((w.get('input[placeholder="app password"]').element as HTMLInputElement).value).toBe('')
    expect(w.find('.account-edit').exists()).toBe(false)
    await w.get('[data-account-id="a2"] .account-menu-trigger').trigger('click')
    await w.get('button[aria-label="编辑账号"]').trigger('click')
    expect((w.get('.account-edit input[type="password"]').element as HTMLInputElement).value).toBe('')
    await w.get('.account-create-toggle').trigger('click')
    expect(w.get('#account-create-panel').attributes('style')).toContain('display: none')
    await w.get('[data-account-id="a1"] .account-menu-trigger').trigger('click')
    await w.get('button[aria-label="重新连接账号"]').trigger('click')
    expect(w.get('.account-create-toggle').attributes('aria-expanded')).toBe('true')
    expect((w.get('input[placeholder="账号名称"]').element as HTMLInputElement).value).toBe('old x')
    expect((w.findAll('select').find(select => select.find('option[value="bluesky"]').exists())!.element as HTMLSelectElement).value).toBe('x')
    const setupState = w.vm as any
    setupState.accountSecret = 'unmount-create'
    setupState.editAccountSecret = 'unmount-edit'
    w.unmount()
    expect(setupState.accountSecret).toBe('')
    expect(setupState.editAccountSecret).toBe('')
  })
  it('explains platform connection choices and the credential-free gtx setup',async()=>{global.fetch=vi.fn(async(input:RequestInfo)=>{const path=String(input);return new Response(path.endsWith('/profiles')?JSON.stringify([{id:'p1',name:'p'}]):'[]',{status:200,headers:{'Content-Type':'application/json'}})}) as any;const w=mount(App);await flushPromises();await w.get('.utility-dock button[aria-label="管理"]').trigger('click');await nextTick();await w.get('.account-create-toggle').trigger('click');expect(w.text()).toContain('OAuth 授权');const platform=w.findAll('select').find(select=>select.find('option[value="qzone"]').exists())!;await platform.setValue('misskey');expect(w.text()).toContain('MiAuth 授权');await platform.setValue('qzone');expect(w.find('.platform-description').exists()).toBe(false);await w.get('.utility-dock button[aria-label="设置"]').trigger('click');await nextTick();const provider=w.get('select[aria-label="翻译服务"]');await provider.setValue('google-gtx');expect((provider.element as HTMLSelectElement).value).toBe('google-gtx');expect(w.find('input[aria-label="密钥环境变量"]').exists()).toBe(false)})
  it('pairs a browser bridge while adding its account',async()=>{const fetchMock=vi.fn(async(input:RequestInfo,init?:RequestInit)=>{const path=String(input);let body:any=[];if(path.endsWith('/profiles'))body=[{id:'p1',name:'p'}];else if(path.endsWith('/bridge/pairings')&&init?.method==='POST')body={id:'pair-qzone',platform:'qzone',code:'QZ1234',expires_at:new Date(Date.now()+600000).toISOString(),code_used:false,revoked:false};return new Response(JSON.stringify(body),{status:200,headers:{'Content-Type':'application/json'}})});global.fetch=fetchMock as any;const writeText=vi.fn().mockResolvedValue(undefined);Object.defineProperty(navigator,'clipboard',{value:{writeText},configurable:true});const w=mount(App);await flushPromises();await w.get('.utility-dock button[aria-label="管理"]').trigger('click');await w.get('.account-create-toggle').trigger('click');const platform=w.findAll('select').find(select=>select.find('option[value="qzone"]').exists())!;await platform.setValue('qzone');expect(w.get('.account-bridge-setup a[href="/userscript/bpm-bridge.user.js"]').text()).toContain('安装脚本');await w.get('.account-bridge-actions button').trigger('click');await flushPromises();const request=fetchMock.mock.calls.find(call=>String(call[0]).endsWith('/bridge/pairings')&&call[1]?.method==='POST');expect(JSON.parse(String(request?.[1]?.body))).toEqual({platform:'qzone'});expect(w.get('.account-bridge-setup').text()).toContain('QZ1234');expect(writeText).toHaveBeenCalledWith('QZ1234')})
  it('returns to settings after a successful local authorization callback',async()=>{window.history.replaceState({},'', '/?connection=misskey');global.fetch=vi.fn(async()=>new Response('[]',{status:200,headers:{'Content-Type':'application/json'}})) as any;const w=mount(App);await flushPromises();expect(w.get('.utility-dock button[aria-label="设置"]').exists()).toBe(true);expect(window.location.search).toBe('')})
  it('creates a manual account with the selected profile and loads its capability',async()=>{const fetchMock=vi.fn(async(input:RequestInfo,init?:RequestInit)=>{const path=String(input);let body:any=[];if(path.endsWith('/profiles'))body=[{id:'p1',name:'p'}];else if(path.endsWith('/accounts')&&init?.method==='POST')body={id:'a1',profile_id:'p1',platform:'qzone',name:'space'};else if(path.endsWith('/accounts/a1/capability'))body={platform:'qzone',format:'plain',visibilities:['public']};else if(path.endsWith('/accounts'))body=[];return new Response(JSON.stringify(body),{status:200,headers:{'Content-Type':'application/json'}})});global.fetch=fetchMock as any;const w=mount(App);await flushPromises();await w.get('.utility-dock button[aria-label="管理"]').trigger('click');await nextTick();await w.get('.account-create-toggle').trigger('click');const platform=w.findAll('select').find(select=>select.find('option[value="qzone"]').exists())!;await platform.setValue('qzone');await w.get('input[placeholder="账号名称"]').setValue('space');const form=w.findAll('form').find(form=>form.find('input[placeholder="账号名称"]').exists())!;await form.trigger('submit');await flushPromises();const post=fetchMock.mock.calls.find(call=>String(call[0]).endsWith('/accounts')&&call[1]?.method==='POST');expect(JSON.parse(String(post?.[1]?.body))).toMatchObject({profile_id:'p1',platform:'qzone',name:'space'});expect(fetchMock.mock.calls.filter(call=>String(call[0]).endsWith('/accounts/a1/capability'))).toHaveLength(1)})
  it('attaches an existing connection to another profile without authorization',async()=>{const fetchMock=vi.fn(async(input:RequestInfo,init?:RequestInit)=>{const path=String(input);let body:any=[];if(path.endsWith('/profiles'))body=[{id:'p1',name:'one'},{id:'p2',name:'two'}];else if(path.endsWith('/profile-accounts')&&init?.method==='POST')body={id:'a1',profile_id:'p1',profile_ids:['p1','p2'],platform:'mastodon',name:'shared',remote_id:'alice'};else if(path.endsWith('/accounts'))body=[{id:'a1',profile_id:'p1',profile_ids:['p1'],platform:'mastodon',name:'shared',remote_id:'alice'}];else if(path.includes('/capability'))body={platform:'mastodon',format:'plain',visibilities:['public']};return new Response(JSON.stringify(body),{status:200,headers:{'Content-Type':'application/json'}})});global.fetch=fetchMock as any;const w=mount(App);await flushPromises();await w.get('.utility-dock button[aria-label="管理"]').trigger('click');await w.get('select[aria-label="切换 Profile"]').setValue('p2');await nextTick();expect(w.get('.account-list').text()).toContain('shared');await w.get('.account-menu-trigger').trigger('click');await w.get('.account-profile-link').trigger('submit');await flushPromises();const request=fetchMock.mock.calls.find(call=>String(call[0]).endsWith('/profile-accounts'));expect(JSON.parse(String(request?.[1]?.body))).toEqual({profile_id:'p2',account_id:'a1'});expect(w.find('.account-profile-link').exists()).toBe(false);expect(w.get('.account-memberships').text()).toContain('two');expect(w.get('.account-list').text()).toContain('shared')})
  it('confirms deactivation and keeps the disabled connection in the global pool',async()=>{
    const account:any={id:'a1',profile_id:'p1',platform:'x',name:'old',disabled:false}
    const fetchMock=vi.fn(async(input:RequestInfo,init?:RequestInit)=>{const path=String(input);let body:any=[];if(path.endsWith('/profiles'))body=[{id:'p1',name:'p'}];else if(path.endsWith('/accounts/a1/deactivate')&&init?.method==='DELETE'){account.disabled=true;body=account}else if(path.endsWith('/accounts'))body=[account];return new Response(JSON.stringify(body),{status:200,headers:{'Content-Type':'application/json'}})})
    global.fetch=fetchMock as any
    vi.spyOn(window,'confirm').mockReturnValue(true)
    const w=mount(App)
    await flushPromises()
    await w.get('.utility-dock button[aria-label="管理"]').trigger('click')
    await nextTick()
    await w.get('.account-menu-trigger').trigger('click')
    await w.get('button[aria-label="停用账号"]').trigger('click')
    await flushPromises()
    expect(fetchMock.mock.calls.filter(call=>String(call[0]).endsWith('/accounts/a1/deactivate')&&call[1]?.method==='DELETE')).toHaveLength(1)
    expect(w.get('.account-list').text()).toContain('old')
    expect(w.get('.account-menu-trigger').text()).toContain('已停用')
    expect(w.find('button[aria-label="停用账号"]').exists()).toBe(false)
    expect(w.find('button[aria-label="永久删除本地账号数据"]').exists()).toBe(true)
    w.unmount()
  })
  it('explains why a manual account was not submitted',async()=>{global.fetch=vi.fn(async(input:RequestInfo)=>new Response(String(input).endsWith('/profiles')?JSON.stringify([{id:'p1',name:'p'}]):'[]',{status:200,headers:{'Content-Type':'application/json'}})) as any;const w=mount(App);await flushPromises();await w.get('.utility-dock button[aria-label="管理"]').trigger('click');await nextTick();await w.get('.account-create-toggle').trigger('click');const form=w.findAll('form').find(form=>form.find('input[placeholder="账号名称"]').exists())!;await form.trigger('submit');expect(w.text()).toContain('请填写账号名称');expect(w.findAll('.toast')).toHaveLength(1);expect(w.get('.toast').classes()).toContain('toast-warning')})
  it('surfaces a Bluesky connection diagnostic instead of silently accepting failure',async()=>{const fetchMock=vi.fn(async(input:RequestInfo,init?:RequestInit)=>{const path=String(input);let body:any=[];if(path.endsWith('/profiles'))body=[{id:'p1',name:'p'}];else if(path.endsWith('/accounts'))body=[{id:'a1',profile_id:'p1',platform:'bluesky',name:'bsky',remote_id:'me.test'}];else if(path.endsWith('/accounts/a1/capability'))body={platform:'bluesky',format:'plain',visibilities:['public']};else if(path.endsWith('/accounts/a1/diagnostic')&&init?.method==='POST')body={ok:false,stage:'auth',category:'auth',message:'app password 无效或已过期',hint:'创建新的 app password',elapsed_ms:42};return new Response(JSON.stringify(body),{status:200,headers:{'Content-Type':'application/json'}})});global.fetch=fetchMock as any;const w=mount(App);await flushPromises();await w.get('.utility-dock button[aria-label="管理"]').trigger('click');await nextTick();await w.get('.account-menu-trigger').trigger('click');await w.get('button[aria-label="测试连接"]').trigger('click');await flushPromises();expect(w.text()).toContain('Bluesky 连接失败');expect(w.text()).toContain('app password 无效或已过期');expect(w.text()).toContain('创建新的 app password');expect(fetchMock.mock.calls.some(call=>String(call[0]).endsWith('/accounts/a1/diagnostic')&&call[1]?.method==='POST')).toBe(true)})
  it('shows Mastodon diagnostic stages without claiming OPTIONS proves write access',async()=>{const fetchMock=vi.fn(async(input:RequestInfo,init?:RequestInit)=>{const path=String(input);let body:any=[];if(path.endsWith('/profiles'))body=[{id:'p1',name:'p'}];else if(path.endsWith('/accounts'))body=[{id:'a1',profile_id:'p1',platform:'mastodon',name:'fediverse'}];else if(path.endsWith('/accounts/a1/capability'))body={platform:'mastodon',format:'plain',visibilities:['public']};else if(path.endsWith('/accounts/a1/diagnostic')&&init?.method==='POST')body={ok:true,stage:'permissions',category:'compatibility',message:'账号身份有效，但实例拒绝了 OPTIONS 探测',hint:'请按发布记录中的阶段排查',username:'alice',account_id:'42',status_write:{endpoint:'/api/v1/statuses',status:'denied',message:'仅是兼容性信号'},media_write:{endpoint:'/api/v2/media',status:'unknown',message:'无法判断'},elapsed_ms:18};return new Response(JSON.stringify(body),{status:200,headers:{'Content-Type':'application/json'}})});global.fetch=fetchMock as any;const w=mount(App);await flushPromises();await w.get('.utility-dock button[aria-label="管理"]').trigger('click');await nextTick();await w.get('.account-menu-trigger').trigger('click');await w.get('button[aria-label="测试连接"]').trigger('click');await flushPromises();expect(w.text()).toContain('Mastodon 检测完成（有警告）');expect(w.text()).toContain('账号身份有效，但实例拒绝了 OPTIONS 探测');expect(w.text()).toContain('仅是兼容性信号')})
  it('opens a topic URL directly and exposes workspace actions', async () => {
    window.history.replaceState({}, '', '/topics/t1')
    global.fetch = vi.fn(async (input: RequestInfo) => {
      const path = String(input)
      let body: any = []
      if (path.endsWith('/profiles')) body = [{ id: 'p1', name: 'p' }]
      else if (path.endsWith('/accounts')) body = [{ id: 'a1', profile_id: 'p1', platform: 'bluesky', name: 'bsky' }]
      else if (path.endsWith('/topics')) body = [{ id: 't1', profile_id: 'p1', title: 'Linked topic', status: 'active' }]
      else if (path.endsWith('/accounts/a1/capability')) body = { platform: 'bluesky', format: 'plain', visibilities: ['public'] }
      else if (path.endsWith('/draft')) body = { topic_id: 't1', text: 'direct draft', format: 'plain', revision: 1 }
      return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
    }) as any
    const w = mount(App)
    await flushPromises()
    expect((w.get('.draft-editor').element as HTMLTextAreaElement).value).toBe('direct draft')
    expect(w.get('.new-topic-action').exists()).toBe(true)
    expect(w.get('.utility-dock button[aria-label="发布记录"]').exists()).toBe(true)
    expect(w.get('.utility-dock button[aria-label="设置"]').exists()).toBe(true)
    await w.get('.utility-dock button[aria-label="发布记录"]').trigger('click')
    await flushPromises()
    expect(window.location.pathname).toBe('/history')
    expect(w.text()).toContain('发布记录')
  })
  it('edits a saved Bluesky credential and automatically runs diagnostics', async () => {
    window.history.replaceState({}, '', '/settings')
    const fetchMock = vi.fn(async (input: RequestInfo, init?: RequestInit) => {
      const path = String(input)
      let body: any = []
      if (path.endsWith('/accounts/a1/connection') && init?.method === 'PUT') body = { id: 'a1', profile_id: 'p1', platform: 'bluesky', name: 'renamed', remote_id: 'new.bsky.social', base_url: 'https://bsky.social' }
      else if (path.endsWith('/accounts/a1/diagnostic') && init?.method === 'POST') body = { ok: true, stage: 'auth', message: 'Bluesky 连接成功', handle: 'new.bsky.social', did: 'did:plc:test', elapsed_ms: 8 }
      else if (path.endsWith('/accounts/a1/capability')) body = { platform: 'bluesky', format: 'plain', visibilities: ['public'] }
      else if (path.endsWith('/profiles')) body = [{ id: 'p1', name: 'p' }]
      else if (path.endsWith('/accounts')) body = [{ id: 'a1', profile_id: 'p1', platform: 'bluesky', name: 'old', remote_id: 'old.bsky.social', base_url: 'https://bsky.social' }]
      return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
    })
    global.fetch = fetchMock as any
    const w = mount(App)
    await flushPromises()
    await w.get('.utility-dock button[aria-label="管理"]').trigger('click')
    await nextTick()
    await w.get('.account-menu-trigger').trigger('click')
    await w.get('button[aria-label="编辑账号"]').trigger('click')
    const inputs = w.findAll('.account-edit input')
    await inputs[0].setValue('renamed')
    await inputs[1].setValue('new.bsky.social')
    await inputs[2].setValue('bsky.social')
    await inputs[3].setValue('new-app-password')
    await w.get('form.account-edit').trigger('submit')
    await flushPromises()
    const update = fetchMock.mock.calls.find(call => String(call[0]).endsWith('/accounts/a1/connection'))
    expect(JSON.parse(String(update?.[1]?.body))).toMatchObject({ name: 'renamed', remote_id: 'new.bsky.social', base_url: 'bsky.social', secret: 'new-app-password' })
    expect(fetchMock.mock.calls.some(call => String(call[0]).endsWith('/accounts/a1/diagnostic'))).toBe(true)
    expect(w.text()).toContain('Bluesky 连接成功')
  })
    it('renders only the first emoji page, inserts at the editor and loads more on demand',async()=>{const fetchMock=vi.fn(async(input:RequestInfo,init?:RequestInit)=>{const path=String(input);let body:any=[];if(path.endsWith('/profiles'))body=[{id:'p1',name:'p'}];else if(path.endsWith('/accounts'))body=[{id:'a1',profile_id:'p1',platform:'misskey',name:'mk'}];else if(path.endsWith('/accounts/a1/capability'))body={platform:'misskey',format:'mfm',visibilities:['public'],custom_emoji:true};else if(path.endsWith('/topics'))body=[{id:'t1',profile_id:'p1',title:'Topic',status:'active'}];else if(path.endsWith('/draft'))body={topic_id:'t1',text:'old',format:'plain',revision:1};else if(path.includes('/variants?'))body=[{id:'v1',topic_id:'t1',account_id:'a1',text:'MFM',format:'mfm',status:'draft',relation:'linked',visibility:'public',spoiler_text:'',local_only:false,outdated:false}];else if(path.endsWith('/emojis'))body=Array.from({length:120},(_,i)=>({shortcode:`emoji-${i}`,url:`https://cdn.test/${i}.png`}));return new Response(JSON.stringify(body),{status:200,headers:{'Content-Type':'application/json'}})});global.fetch=fetchMock as any;const w=mount(App);await flushPromises();await w.get('.topic-row-select').trigger('click');await flushPromises();await selectVariant(w, 'mk');await nextTick();await w.get('button[aria-label="Emoji"]').trigger('click');await flushPromises();expect(w.findAll('.custom-emoji-grid button[aria-label^="插入 :"]').length).toBe(50);await w.get('.custom-emoji-grid button[aria-label="插入 :emoji-0:"]').trigger('click');await nextTick();expect((w.get('.draft-editor').element as HTMLTextAreaElement).value).toContain(':emoji-0:');await w.get('button.emoji-more').trigger('click');expect(w.findAll('.custom-emoji-grid button[aria-label^="插入 :"]').length).toBe(100);expect(fetchMock.mock.calls.some(call=>String(call[0]).endsWith('/emojis'))).toBe(true)})
  it('removes a terminal publication locally and emits one parent notification', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo, init?: RequestInit) => {
      const path = String(input)
      const body = path.endsWith('/publications') ? [{ id: 'pub1', status: 'published', remote_url: 'https://remote.test/1', created_at: new Date().toISOString() }] : []
      return new Response(JSON.stringify(init?.method === 'DELETE' ? { status: 'deleted' } : body), { status: 200, headers: { 'Content-Type': 'application/json' } })
    })
    global.fetch = fetchMock as any
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const w = mount(PublicationList)
    await flushPromises()
    await w.get('button[aria-label^="移除本地记录"]').trigger('click')
    await flushPromises()
    expect(fetchMock.mock.calls.some(call => String(call[0]).endsWith('/publications/pub1') && call[1]?.method === 'DELETE')).toBe(true)
    expect(w.emitted('notify')?.at(-1)).toEqual(['本地发布记录已移除，远端帖子未受影响'])
    expect(w.find('.toast').exists()).toBe(false)
  })
  it('offers local removal for a published platform version', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo, init?: RequestInit) => {
      const path = String(input)
      let body: any = []
      if (path.endsWith('/profiles')) body = [{ id: 'p1', name: 'p' }]
      else if (path.endsWith('/accounts')) body = [{ id: 'a1', profile_id: 'p1', platform: 'mastodon', name: 'main' }]
      else if (path.endsWith('/topics')) body = [{ id: 't1', profile_id: 'p1', title: 'Topic', status: 'active' }]
      else if (path.endsWith('/draft')) body = { topic_id: 't1', text: 'draft', format: 'plain', revision: 1 }
      else if (path.includes('/variants?')) body = [{ id: 'v1', topic_id: 't1', account_id: 'a1', text: 'sent', format: 'plain', status: 'published', published: true, relation: 'detached', visibility: 'public' }]
      else if (path.includes('/capability')) body = { platform: 'mastodon', format: 'plain', visibilities: ['public'] }
      else if (path.endsWith('/variants/v1') && init?.method === 'DELETE') body = { status: 'deleted' }
      return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
    })
    global.fetch = fetchMock as any
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const w = mount(App)
    await flushPromises()
    await w.get('.topic-row-select').trigger('click')
    await flushPromises()
    await selectVariant(w, 'main')
    await openPickerAction(w, '移除版本')
    await flushPromises()
    expect(fetchMock.mock.calls.some(call => String(call[0]).endsWith('/variants/v1') && call[1]?.method === 'DELETE')).toBe(true)
  })
  it('exposes one apply action for the active outdated variant', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo, init?: RequestInit) => {
      const path = String(input)
      let body: any = []
      if (path.endsWith('/profiles')) body = [{ id: 'p1', name: 'p' }]
      else if (path.endsWith('/accounts')) body = [{ id: 'a1', profile_id: 'p1', platform: 'mastodon', name: 'main' }]
      else if (path.endsWith('/topics')) body = [{ id: 't1', profile_id: 'p1', title: 'Topic', status: 'active' }]
      else if (path.endsWith('/draft')) body = { topic_id: 't1', text: 'draft', format: 'plain', revision: 1 }
      else if (path.includes('/variants?')) body = [{ id: 'v1', topic_id: 't1', account_id: 'a1', text: 'variant', format: 'plain', status: 'draft', relation: 'linked', visibility: 'public', outdated: true }]
      else if (path.endsWith('/variants/v1/apply') && init?.method === 'POST') body = { id: 'v1', topic_id: 't1', account_id: 'a1', text: 'updated', format: 'plain', status: 'draft', relation: 'linked', visibility: 'public', outdated: false }
      else if (path.includes('/capability')) body = { platform: 'mastodon', format: 'plain', visibilities: ['public'] }
      return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
    })
    global.fetch = fetchMock as any
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const w = mount(App)
    await flushPromises()
    await w.get('.topic-row-select').trigger('click')
    await flushPromises()
    await selectVariant(w, 'main')
    await nextTick()
    await openPickerAction(w, '应用主稿更新')
    await flushPromises()
    expect(fetchMock.mock.calls.some(call => String(call[0]).endsWith('/variants/v1/apply') && call[1]?.method === 'POST')).toBe(true)
  })
  it('publishes every selected ready target and keeps independent progress', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo, init?: RequestInit) => {
      const path = String(input)
      let body: any = []
      if (path.endsWith('/profiles')) body = [{ id: 'p1', name: 'p' }]
      else if (path.endsWith('/accounts')) body = [{ id: 'a1', profile_id: 'p1', platform: 'mastodon', name: 'main' }, { id: 'a2', profile_id: 'p1', platform: 'bluesky', name: 'sky' }]
      else if (path.endsWith('/topics')) body = [{ id: 't1', profile_id: 'p1', title: 'Batch', status: 'active', target_count: 2, published_count: 0 }]
      else if (path.endsWith('/draft')) body = { topic_id: 't1', text: 'draft', format: 'plain', revision: 1 }
      else if (path.includes('/variants?')) body = [{ id: 'v1', topic_id: 't1', account_id: 'a1', text: 'one', format: 'plain', status: 'draft', relation: 'linked', visibility: 'public' }, { id: 'v2', topic_id: 't1', account_id: 'a2', text: 'two', format: 'plain', status: 'draft', relation: 'linked', visibility: 'public' }]
      else if (path.includes('/capability')) body = { platform: path.includes('a2') ? 'bluesky' : 'mastodon', format: 'plain', visibilities: ['public'] }
      else if (path.endsWith('/publications') && init?.method === 'POST') body = { id: `pub-${JSON.parse(String(init.body)).variant_id}`, status: 'published' }
      return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
    })
    global.fetch = fetchMock as any
    const w = mount(App)
    await flushPromises()
    await w.get('.topic-row-select').trigger('click')
    await flushPromises()
    await selectVariant(w, 'main')
    await w.get('.post-preview-trigger').trigger('click')
    await flushPromises()
    await w.get('.dialog-targets input[value="v2"]').setValue(true)
    await nextTick()
    expect(w.get('#publish-dialog-title').text()).toContain('发布 2 个版本')
    await w.get('[role="dialog"] footer .primary-action').trigger('click')
    await flushPromises()
    const posted = fetchMock.mock.calls.filter(call => String(call[0]).endsWith('/publications') && call[1]?.method === 'POST')
    expect(posted).toHaveLength(2)
    expect(w.get('.publish-taskbar').text()).toContain('main · Mastodon ✓')
    expect(w.get('.publish-taskbar').text()).toContain('sky · Bluesky ✓')
  })
  it('previews the active platform version with its real settings and media', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo) => {
      const path = String(input)
      let body: any = []
      if (path.endsWith('/profiles')) body = [{ id: 'p1', name: 'p' }]
      else if (path.endsWith('/accounts')) body = [{ id: 'a1', profile_id: 'p1', platform: 'mastodon', name: 'social account' }]
      else if (path.endsWith('/topics')) body = [{ id: 't1', profile_id: 'p1', title: 'Preview topic', status: 'active' }]
      else if (path.endsWith('/draft')) body = { topic_id: 't1', text: 'draft', format: 'plain', revision: 1 }
      else if (path.includes('/variants?')) body = [{ id: 'v1', topic_id: 't1', account_id: 'a1', text: 'platform body', format: 'plain', status: 'draft', relation: 'linked', visibility: 'private', spoiler_text: 'spoiler', local_only: true }]
      else if (path.includes('/media?')) body = [{ id: 'm1', variant_id: 'v1', filename: 'photo.jpg', alt_text: 'a photo' }]
      else if (path.includes('/capability')) body = { platform: 'mastodon', format: 'plain', visibilities: ['public', 'private'], supports_cw: true, supports_local_only: true }
      return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
    })
    global.fetch = fetchMock as any
    const w = mount(App)
    await flushPromises()
    await w.get('.topic-row-select').trigger('click')
    await flushPromises()
    await selectVariant(w, 'social account')
    await openPickerAction(w, '预览')
    await nextTick()
    const preview = w.get('[aria-labelledby="post-preview-title"]')
    expect(preview.text()).toContain('social account')
    expect(preview.text()).toContain('platform body')
    expect(preview.text()).toContain('spoiler')
    expect(preview.text()).toContain('private · 仅本地')
    expect(preview.get('img').attributes('src')).toBe('/media/m1')
    await preview.get('button[aria-label="关闭预览"]').trigger('click')
    expect(w.find('[aria-labelledby="post-preview-title"]').exists()).toBe(false)
  })
  it('shows persisted topic progress and contextual publication groups', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo) => {
      const path = String(input)
      let body: any = []
      if (path.endsWith('/profiles')) body = [{ id: 'p1', name: 'p' }]
      else if (path.endsWith('/topics')) body = [{ id: 't1', profile_id: 'p1', title: 'Tracked topic', status: 'active', target_count: 3, published_count: 2, needs_action_count: 1, platforms: ['mastodon', 'x'], updated_at: new Date().toISOString() }]
      else if (path.endsWith('/accounts')) body = [{ id: 'a1', name: 'main', platform: 'mastodon' }]
      else if (path.includes('/publications/summary?profile_id=p1')) body = { action_count: 1, failed_count: 0 }
      else if (path.endsWith('/publications')) body = [{ id: 'wait', account_id: 'a1', status: 'needs_confirmation', error_category: 'unknown', error_message: '请核对', topic_id: 't1', topic_title: 'Tracked topic', text: 'context body', created_at: new Date().toISOString() }, { id: 'done', account_id: 'a1', status: 'published', topic_id: 't1', topic_title: 'Tracked topic', text: 'done body', created_at: new Date().toISOString() }]
      return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
    })
    global.fetch = fetchMock as any
    const app = mount(App)
    await flushPromises()
    expect(app.get('.topic-row-select').attributes('title')).toContain('1 项待处理')
    expect(app.get('.utility-dock button[aria-label="发布记录"] i').exists()).toBe(true)
    const center = mount(PublicationList)
    await flushPromises()
    expect(center.findAll('[role="tab"]').map(item => item.text())).toEqual(['需处理1', '最近完成1'])
    expect(center.findAll('[role="tab"]')[0].classes()).toContain('group-action')
    expect(center.findAll('[role="tab"]')[1].classes()).toContain('group-complete')
    expect(center.get('[role="tab"]').attributes('aria-selected')).toBe('true')
    expect(center.text()).toContain('Tracked topic')
    expect(center.text()).toContain('context body')
    expect(center.text()).not.toContain('done body')
    await center.findAll('[role="tab"]')[1].trigger('click')
    expect(center.text()).toContain('done body')
    expect(center.text()).not.toContain('context body')
  })
  it('scopes publication center rows to the selected profile', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo) => {
      const path = String(input)
      const body = path.includes('/accounts?profile_id=p2') ? [{ id: 'a2', name: 'second', platform: 'mastodon' }]
        : path.includes('/publications?profile_id=p2') ? [{ id: 'pub2', account_id: 'a2', status: 'published', topic_title: 'Second profile topic', created_at: new Date().toISOString() }]
          : []
      return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
    })
    global.fetch = fetchMock as any
    const center = mount(PublicationList, { props: { profileId: 'p2' } })
    await flushPromises()
    expect(center.text()).toContain('Second profile topic')
    expect(fetchMock.mock.calls.some(call => String(call[0]).endsWith('/publications?profile_id=p2'))).toBe(true)
    expect(fetchMock.mock.calls.some(call => String(call[0]).endsWith('/accounts?profile_id=p2'))).toBe(true)
    center.unmount()
  })
  it('saves the current title before archiving and exposes restore in the editor menu', async () => {
    const updates: Array<{ title: string; status: string }> = []
    const fetchMock = vi.fn(async (input: RequestInfo, init?: RequestInit) => {
      const path = String(input)
      let body: any = []
      if (path.endsWith('/profiles')) body = [{ id: 'p1', name: 'p' }]
      else if (path.endsWith('/accounts')) body = []
      else if (path.endsWith('/topics') && init?.method !== 'PUT') body = [{ id: 't1', profile_id: 'p1', title: 'Old title', status: 'active' }]
      else if (path.endsWith('/topics/t1') && init?.method === 'PUT') {
        const update = JSON.parse(String(init.body))
        updates.push(update)
        body = { id: 't1', profile_id: 'p1', ...update }
      } else if (path.endsWith('/draft')) body = { topic_id: 't1', text: 'draft', format: 'plain', revision: 1 }
      return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
    })
    global.fetch = fetchMock as any
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const w = mount(App)
    await flushPromises()
    await w.get('.topic-row-select').trigger('click')
    await flushPromises()
    await w.get('.topic-title').setValue('New title')
    await w.findAll('.topic-more-menu button').find(button => button.text() === '归档话题')!.trigger('click')
    await flushPromises()
    expect(updates).toEqual([
      { title: 'New title', status: 'active' },
      { title: 'New title', status: 'archived' },
    ])
    expect(w.get('.topic-more-menu').text()).toContain('恢复话题')
  })
})
