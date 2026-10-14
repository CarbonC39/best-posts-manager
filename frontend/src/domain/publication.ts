export const publicationStatuses = ['pending', 'publishing', 'needs_confirmation', 'failed', 'published', 'cancelled'] as const

export type PublicationStatus = typeof publicationStatuses[number]

export type Publication = {
  id: string
  variant_id: string
  account_id: string
  status: PublicationStatus
  remote_url: string
  remote_id: string
  remote_cid: string
  error_message: string
  error_category: string
  idempotency_key: string
  source: string
  created_at: string
  published_at: string
  topic_id?: string
  topic_title?: string
  text?: string
}

export type PublicationStatusSummary = {
  action_count: number
  failed_count: number
}

const statusLabels: Record<PublicationStatus, string> = {
  pending: '等待处理',
  publishing: '正在发布',
  needs_confirmation: '需要人工确认',
  failed: '发布失败',
  published: '已发布',
  cancelled: '已取消',
}

export function publicationStatusLabel(status: string) {
  return statusLabels[status as PublicationStatus] || status
}

export function publicationNeedsAction(status: string) {
  return status === 'needs_confirmation' || status === 'failed'
}

export function publicationInProgress(status: string) {
  return status === 'pending' || status === 'publishing'
}

export function publicationIsRemovable(status: string) {
  return status === 'published' || status === 'failed' || status === 'cancelled'
}
