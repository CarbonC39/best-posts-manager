import { computed, onBeforeUnmount, onMounted, ref, watch, type Ref } from 'vue'
import { getPublicationSummary } from '../api/publications'
import type { PublicationStatusSummary } from '../domain/publication'

export function usePublicationStatus(profileID: Ref<string>) {
  const summary = ref<PublicationStatusSummary | null>(null)
  const cache = new Map<string, PublicationStatusSummary>()
  let refreshTimer: number | undefined
  let inFlight: Promise<void> | null = null
  let refreshQueued = false
  let stopped = false

  async function refresh() {
    const requestedProfileID = profileID.value.trim()
    if (!requestedProfileID || stopped || document.visibilityState === 'hidden') return
    if (inFlight) {
      refreshQueued = true
      return inFlight
    }

    inFlight = (async () => {
      try {
        const next = await getPublicationSummary(requestedProfileID)
        if (stopped) return
        cache.set(requestedProfileID, next)
        if (profileID.value.trim() === requestedProfileID) summary.value = next
      } catch {
        // Background refreshes retain the last known value and never notify the user.
      } finally {
        inFlight = null
        if (refreshQueued && !stopped) {
          refreshQueued = false
          void refresh()
        }
      }
    })()
    return inFlight
  }

  function refreshWhenVisible() {
    if (document.visibilityState !== 'hidden') void refresh()
  }

  watch(profileID, (nextProfileID) => {
    summary.value = cache.get(nextProfileID.trim()) || null
    void refresh()
  }, { immediate: true })

  onMounted(() => {
    refreshTimer = window.setInterval(refreshWhenVisible, 3_000)
    window.addEventListener('focus', refreshWhenVisible)
    document.addEventListener('visibilitychange', refreshWhenVisible)
  })

  onBeforeUnmount(() => {
    stopped = true
    if (refreshTimer !== undefined) window.clearInterval(refreshTimer)
    window.removeEventListener('focus', refreshWhenVisible)
    document.removeEventListener('visibilitychange', refreshWhenVisible)
  })

  return {
    summary,
    actionCount: computed(() => summary.value?.action_count || 0),
    hasFailures: computed(() => Boolean(summary.value?.failed_count)),
    refresh,
    invalidate: refresh,
  }
}
