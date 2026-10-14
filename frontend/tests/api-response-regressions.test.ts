import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from '../src/api'

afterEach(() => vi.unstubAllGlobals())

describe('API response errors', () => {
  it('reports an HTTP error when a server or dev proxy returns HTML', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('<!doctype html><html>Vite fallback</html>', {
      status: 502,
      headers: { 'Content-Type': 'text/html' },
    })))

    await expect(api('topics')).rejects.toThrow(/502/)
  })

  it('preserves the backend error message from a JSON error response', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ error: 'database unavailable' }), {
      status: 503,
      headers: { 'Content-Type': 'application/json' },
    })))

    await expect(api('topics')).rejects.toThrow('database unavailable')
  })
})
