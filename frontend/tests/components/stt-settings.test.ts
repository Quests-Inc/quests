import { expect, test } from 'bun:test'

import { sttSettings } from '@/shared/model/sttSettings.svelte.ts'

test('адрес распознавания читается с сервера', async () => {
  const previous = globalThis.fetch
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    if (String(input).endsWith('/api/stt')) {
      return new Response(
        JSON.stringify({ base_url: 'http://192.168.1.152:8000', model: 'whisper-1', has_key: false, enabled: true }),
        { status: 200, headers: { 'Content-Type': 'application/json' } }
      )
    }
    return new Response('no', { status: 404 })
  }) as typeof fetch
  try {
    sttSettings.baseUrl = 'http://192.168.1.152:8000'
    await new Promise((resolve) => setTimeout(resolve, 450))
    expect(sttSettings.enabled).toBe(true)
    expect(sttSettings.model).toBe('whisper-1')
  } finally {
    globalThis.fetch = previous
  }
})
