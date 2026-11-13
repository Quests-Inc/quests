import { afterEach, expect, test } from 'bun:test'

import { flushSync, mount, unmount } from 'svelte'

import RepositoryBrowser from '@/entities/repository/ui/RepositoryBrowser.svelte'
const previous = globalThis.fetch
let cleanup: (() => void)[] = []
afterEach(() => {
  globalThis.fetch = previous
  cleanup.forEach((f) => f())
  cleanup = []
})
test('дерево репозитория открывает файл как текст без исполнения разметки', async () => {
  const requests: string[] = []
  globalThis.fetch = (async (input) => {
    const url = String(input)
    requests.push(url)
    const file = url.includes('path=main.py')
    return new Response(
      JSON.stringify({
        clone_url: '/api/repositories/1/git',
        branch: 'main',
        branches: ['main'],
        commits: [{ id: 'abc', title: 'First', author: 'A', date: '2026' }],
        tree: file ? [] : [{ name: 'main.py', path: 'main.py', kind: 'file', id: 'def' }],
        file: file ? '<script>bad()</script>' : undefined,
        empty: false,
      })
    )
  }) as typeof fetch
  const target = document.createElement('div')
  document.body.append(target)
  const app = mount(RepositoryBrowser, {
    target,
    props: {
      repo: { id: 1, title: 'Mini', kind: 'local', branch: 'main', url: '', has_token: false, bytes: 1, links: [] },
    },
  })
  cleanup.push(() => {
    unmount(app)
    target.remove()
  })
  for (let i = 0; i < 4; i++) {
    await new Promise((r) => setTimeout(r, 0))
    flushSync()
  }
  const file = [...target.querySelectorAll('button')].find((b) => b.textContent?.includes('main.py'))!
  flushSync(() => file.click())
  for (let i = 0; i < 4; i++) {
    await new Promise((r) => setTimeout(r, 0))
    flushSync()
  }
  expect(target.querySelector('pre')?.textContent).toBe('<script>bad()</script>')
  expect(target.querySelector('script')).toBeNull()
  expect(requests.some((url) => url.includes('path=main.py'))).toBe(true)
})
