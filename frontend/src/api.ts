export async function api<T>(path: string, options: RequestInit = {}): Promise<T> {
  let response: Response
  try {
    response = await fetch(`/api/${path}`, { headers: { Accept: 'application/json', 'Content-Type': 'application/json', ...(options.headers || {}) }, ...options })
  } catch {
    throw new Error('无法连接本地服务。请启动 Go 服务，并确认开发代理指向该服务。')
  }

  const raw = await response.text()
  let body: { error?: string } | T | undefined
  if (raw) {
    try {
      body = JSON.parse(raw) as { error?: string } | T
    } catch {
      if (response.ok) throw new Error(`本地服务返回了非 JSON 内容（HTTP ${response.status}）。请检查服务地址和开发代理。`)
    }
  }
  if (!response.ok) {
    const error = body && typeof body === 'object' && 'error' in body ? body.error : undefined
    throw new Error(error || `HTTP ${response.status}`)
  }
  if (!raw) return undefined as T
  return body as T
}
