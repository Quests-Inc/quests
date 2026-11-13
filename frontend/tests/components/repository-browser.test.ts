import { afterEach, expect, test } from 'bun:test'

import { flushSync, mount, unmount } from 'svelte'

import RepositoriesBlock from '@/entities/repository/ui/RepositoriesBlock.svelte'
import RepositoryBrowser from '@/entities/repository/ui/RepositoryBrowser.svelte'
const previous = globalThis.fetch
let cleanup: (() => void)[] = []
afterEach(() => {
  globalThis.fetch = previous
  cleanup.forEach((f) => f())
  cleanup = []
})
const repo = {
  id: 1,
  title: 'Mini',
  kind: 'local' as const,
  branch: 'main',
  url: '',
  has_token: false,
  bytes: 1,
  links: [{ kind: 'quest' as const, id: 5 }],
}
async function settle() {
  for (let i = 0; i < 4; i++) {
    await new Promise((r) => setTimeout(r, 0))
    flushSync()
  }
}
test('переключатель коммитов меняет дерево, файлы не открываются', async () => {
  const requests: string[] = []
  const sha = 'a'.repeat(40)
  globalThis.fetch = (async (input) => {
    const url = String(input)
    requests.push(url)
    return new Response(
      JSON.stringify({
        clone_url: '/api/repositories/1/git',
        branch: 'main',
        branches: ['main', 'feature'],
        selected_commit: sha,
        commits: [{ id: sha, title: 'First', author: 'A', date: '2026' }],
        tree: [
          { name: 'main.py', path: 'main.py', kind: 'file', id: 'def' },
          { name: 'src', path: 'src', kind: 'directory', id: 'ghi' },
        ],
        empty: false,
      })
    )
  }) as typeof fetch
  const target = document.createElement('div')
  document.body.append(target)
  const app = mount(RepositoryBrowser, { target, props: { repo } })
  cleanup.push(() => {
    unmount(app)
    target.remove()
  })
  await settle()
  expect([...target.querySelectorAll('button')].some((b) => b.textContent?.includes('main.py'))).toBe(false)
  const commits = target.querySelector('[aria-label="Коммит репозитория"]') as HTMLSelectElement
  flushSync(() => {
    commits.selectedIndex = 1
    commits.dispatchEvent(new Event('change', { bubbles: true }))
  })
  await settle()
  expect(requests.some((url) => url.includes('commit=' + sha))).toBe(true)
  const folder = [...target.querySelectorAll('button')].find((b) => b.textContent?.includes('src/'))!
  flushSync(() => folder.click())
  await settle()
  expect(requests.some((url) => url.includes('path=src'))).toBe(true)
})
test('карточка владельца содержит ссылки, без встроенного дерева и переключателей', async () => {
  globalThis.fetch = (async () =>
    new Response(JSON.stringify({ items: [repo], quota_bytes: 268435456 }))) as typeof fetch
  const target = document.createElement('div')
  document.body.append(target)
  const app = mount(RepositoriesBlock, { target, props: { owner: { kind: 'quest', id: 5 } } })
  cleanup.push(() => {
    unmount(app)
    target.remove()
  })
  await settle()
  flushSync(() => target.querySelector<HTMLButtonElement>('.repo-heading')!.click())
  expect(target.querySelector('a')?.getAttribute('href')).toBe('?tab=repositories&repo=1')
  expect(target.querySelector('.repo-browser')).toBeNull()
  expect(target.querySelector('[aria-label="Коммит репозитория"]')).toBeNull()
})

test('поиск в боковой панели сохраняет открытый репозиторий и позволяет выбрать другой', async () => {
  globalThis.fetch = (async (input) =>
    new Response(
      JSON.stringify(
        String(input).includes('/browse')
          ? { branches: ['main'], commits: [], tree: [], empty: true }
          : { items: [repo, { ...repo, id: 2, title: 'Other' }], quota_bytes: 268435456 }
      )
    )) as typeof fetch
  const target = document.createElement('div')
  document.body.append(target)
  const app = mount(RepositoriesBlock, { target })
  cleanup.push(() => {
    unmount(app)
    target.remove()
  })
  await settle()
  flushSync(() => target.querySelector<HTMLButtonElement>('.repo-row')!.click())
  await settle()
  const search = target.querySelector<HTMLInputElement>('[aria-label="Поиск репозиториев"]')!
  flushSync(() => {
    search.value = '2'
    search.dispatchEvent(new Event('input', { bubbles: true }))
  })
  await settle()
  expect(target.querySelectorAll('.repo-row').length).toBe(1)
  expect(target.querySelector('.repo-detail h1')?.textContent).toBe('Mini')
  flushSync(() => target.querySelector<HTMLButtonElement>('.repo-row')!.click())
  await settle()
  expect(target.querySelector('.repo-detail h1')?.textContent).toBe('Other')
  expect(target.querySelector('[aria-label="Ширина боковой панели"]')).not.toBeNull()
})
