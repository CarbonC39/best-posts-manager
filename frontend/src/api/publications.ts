import { api } from '../api'
import type { Publication, PublicationStatusSummary } from '../domain/publication'

function profileQuery(profileID: string) {
  const value = profileID.trim()
  return value ? `?${new URLSearchParams({ profile_id: value })}` : ''
}

export async function listPublications(profileID: string) {
  const rows = await api<Publication[] | null>(`publications${profileQuery(profileID)}`)
  return Array.isArray(rows) ? rows : []
}

export function getPublicationSummary(profileID: string) {
  return api<PublicationStatusSummary>(`publications/summary?${new URLSearchParams({ profile_id: profileID.trim() })}`)
}

export function retryPublication(publicationID: string) {
  return api<Publication>('publications/retry', { method: 'POST', body: JSON.stringify({ publication_id: publicationID }) })
}

export function confirmPublication(publicationID: string, remoteURL: string) {
  return api<Publication>(`publications/${publicationID}/confirm`, { method: 'POST', body: JSON.stringify({ remote_url: remoteURL }) })
}

export function cancelPublication(publicationID: string) {
  return api<Publication>(`publications/${publicationID}/cancel`, { method: 'POST' })
}

export function removePublication(publicationID: string) {
  return api<{ status: string }>(`publications/${publicationID}`, { method: 'DELETE' })
}
