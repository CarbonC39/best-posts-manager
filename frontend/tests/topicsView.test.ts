import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import TopicsView from '../src/views/TopicsView.vue'

const profiles = [{ id: 'p1', name: 'First' }, { id: 'p2', name: 'Second' }]
const topics = [
  { id: 't1', profile_id: 'p1', title: 'Topic', status: 'active', needs_action_count: 1, platforms: ['mastodon'] },
  { id: 'archived/topic', profile_id: 'p1', title: 'Archived', status: 'archived' },
]

describe('topics view', () => {
  it('renders the existing controls and emits state and business actions', async () => {
    const wrapper = mount(TopicsView, {
      props: {
        profiles,
        topics,
        selectedProfileId: 'p1',
        topicSearch: '',
        topicTitle: '',
        showArchived: false,
        progressLabel: () => '1 项待处理',
        timeLabel: () => '刚刚',
      },
      attrs: { onSelectTopic: (event: MouseEvent) => event.preventDefault() },
    })

    expect(wrapper.get('#topics-title').text()).toBe('全部话题')
    expect(wrapper.get('.topic-count').text()).toBe('2 个')
    expect(wrapper.get('.topic-progress').text()).toBe('1 项待处理')
    expect(wrapper.get('.topic-platforms').attributes('aria-label')).toBe('目标平台')

    await wrapper.get('.mobile-topic-create input').setValue('New topic')
    await wrapper.get('input[aria-label="搜索全部话题"]').setValue('needle')
    await wrapper.get('#content-profile').setValue('p2')
    await wrapper.get('.archived-filter input').setValue(true)
    await wrapper.get('.mobile-topic-create').trigger('submit')
    const links = wrapper.findAll('.topic-list a')
    expect(links[1].attributes('href')).toBe('/topics/archived%2Ftopic')
    await links[0].trigger('click')
    await links[1].trigger('click', { ctrlKey: true })
    await wrapper.get('button[aria-label="归档话题"]').trigger('click')
    await wrapper.get('button[aria-label="恢复话题"]').trigger('click')

    expect(wrapper.emitted('update:topicTitle')).toEqual([['New topic']])
    expect(wrapper.emitted('update:topicSearch')).toEqual([['needle']])
    expect(wrapper.emitted('update:selectedProfileId')).toEqual([['p2']])
    expect(wrapper.emitted('update:showArchived')).toEqual([[true]])
    expect(wrapper.emitted('create')).toEqual([[]])
    expect(wrapper.emitted('select-topic')).toHaveLength(2)
    expect((wrapper.emitted('select-topic')?.[0]?.[0] as MouseEvent).ctrlKey).toBe(false)
    expect((wrapper.emitted('select-topic')?.[1]?.[0] as MouseEvent).ctrlKey).toBe(true)
    expect(wrapper.emitted('select-topic')?.[0]?.[1]).toEqual(topics[0])
    expect(wrapper.emitted('toggle-archive')).toEqual([[topics[0], 'archived'], [topics[1], 'active']])
  })

  it('keeps both empty states and disables creation without a profile', () => {
    const base = { topics: [], selectedProfileId: '', topicSearch: '', topicTitle: '', showArchived: false, progressLabel: vi.fn(), timeLabel: vi.fn() }
    const withoutProfiles = mount(TopicsView, { props: { ...base, profiles: [] } })
    expect(withoutProfiles.text()).toContain('还没有 Profile。前往账号管理创建后，就可以开始第一个话题。')
    expect(withoutProfiles.get('.mobile-topic-create button').attributes('disabled')).toBeDefined()
    const withoutTopics = mount(TopicsView, { props: { ...base, profiles } })
    expect(withoutTopics.text()).toContain('这个 Profile 还没有话题。')
  })
})
