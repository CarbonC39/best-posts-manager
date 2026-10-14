import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, toRef } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { usePublicationStatus } from '../src/composables/usePublicationStatus'

const Harness = defineComponent({
  props: { profileId: { type: String, required: true } },
  setup(props) {
    const { summary, hasFailures } = usePublicationStatus(toRef(props, 'profileId'))
    return { summary, hasFailures }
  },
  template: '<span>{{ summary?.action_count ?? "none" }}:{{ hasFailures }}</span>',
})

describe('publication status', () => {
  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('refreshes for profile changes without showing the previous profile summary', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo) => {
      const profileID = new URL(String(input), 'http://local').searchParams.get('profile_id')
      const body = profileID === 'p1' ? { action_count: 2, failed_count: 1 } : { action_count: 0, failed_count: 0 }
      return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
    })
    global.fetch = fetchMock as any
    const wrapper = mount(Harness, { props: { profileId: 'p1' } })
    await flushPromises()
    expect(wrapper.text()).toBe('2:true')

    const changing = wrapper.setProps({ profileId: 'p2' })
    await changing
    expect(wrapper.text()).toBe('none:false')
    await flushPromises()
    expect(wrapper.text()).toBe('0:false')
    expect(fetchMock.mock.calls.some(call => String(call[0]).includes('profile_id=p2'))).toBe(true)
    wrapper.unmount()
  })

  it('deduplicates overlapping refreshes and removes polling listeners', async () => {
    vi.useFakeTimers()
    let resolveRequest: ((response: Response) => void) | undefined
    const fetchMock = vi.fn(() => new Promise<Response>((resolve) => { resolveRequest = resolve }))
    global.fetch = fetchMock as any
    const clearInterval = vi.spyOn(window, 'clearInterval')
    const wrapper = mount(Harness, { props: { profileId: 'p1' } })

    window.dispatchEvent(new Event('focus'))
    vi.advanceTimersByTime(3_000)
    expect(fetchMock).toHaveBeenCalledTimes(1)

    wrapper.unmount()
    expect(clearInterval).toHaveBeenCalled()
    window.dispatchEvent(new Event('focus'))
    expect(fetchMock).toHaveBeenCalledTimes(1)
    resolveRequest?.(new Response(JSON.stringify({ action_count: 1, failed_count: 0 }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    await flushPromises()
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('retains the last summary when a background refresh fails', async () => {
    let succeeds = true
    global.fetch = vi.fn(async () => succeeds
      ? new Response(JSON.stringify({ action_count: 3, failed_count: 1 }), { status: 200, headers: { 'Content-Type': 'application/json' } })
      : new Response(JSON.stringify({ error: 'offline' }), { status: 503, headers: { 'Content-Type': 'application/json' } })) as any
    const wrapper = mount(Harness, { props: { profileId: 'p1' } })
    await flushPromises()
    succeeds = false
    window.dispatchEvent(new Event('focus'))
    await flushPromises()
    expect(wrapper.text()).toBe('3:true')
    wrapper.unmount()
  })
})
