import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import PublicationList from '../src/PublicationList.vue'
import PublicationCenterView from '../src/views/PublicationCenterView.vue'

describe('publication center view', () => {
  it('passes the profile to the list and forwards list events', () => {
    const wrapper = mount(PublicationCenterView, {
      props: { profileId: 'profile-2' },
      global: { stubs: { PublicationList: true } },
    })

    expect(wrapper.find('h1').exists()).toBe(false)
    expect(wrapper.classes()).toEqual(expect.arrayContaining(['page-section', 'history-page']))
    const list = wrapper.getComponent(PublicationList)
    expect(list.props('profileId')).toBe('profile-2')
    list.vm.$emit('notify', '刷新失败')
    list.vm.$emit('changed')
    expect(wrapper.emitted('notify')).toEqual([['刷新失败']])
    expect(wrapper.emitted('changed')).toEqual([[]])
  })
})
