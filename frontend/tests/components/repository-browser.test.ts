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
          {
            name: 'main.py',
            path: 'main.py',
            kind: 'file',
            id: 'def',
            last_commit: { id: sha, title: 'Edit main', author: 'Tester', date: '2026-11-13T10:00:00Z' },
          },
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
  expect(target.querySelector('.repo-tree li')?.textContent).toContain('src/')
  expect(target.querySelector('.repo-file .fki')?.getAttribute('aria-label')).toBe('Код / текст')
  expect(target.querySelector('.repo-message[title="Edit main"]')).not.toBeNull()
  expect(target.querySelector('time')?.getAttribute('datetime')).toBe('2026-11-13T10:00:00Z')
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
  expect(target.querySelector('a')?.getAttribute('href')).toBe('?repo=1')
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

test('настройки репозитория предупреждают о смене адреса и сохраняют slug', async () => {
  const requests: { url: string; body: string }[] = []
  globalThis.fetch = (async (input, init) => {
    const url = String(input)
    if (init?.method === 'PATCH') requests.push({ url, body: String(init.body) })
    return new Response(
      JSON.stringify(
        url.includes('/browse')
          ? { branches: ['main'], commits: [], tree: [], empty: true }
          : { items: [{ ...repo, slug: 'mini' }], quota_bytes: 268435456 }
      )
    )
  }) as typeof fetch
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
  flushSync(() =>
    [...target.querySelectorAll('button')]
      .find((b) => b.getAttribute('aria-label') === 'Настройки репозитория')!
      .click()
  )
  await settle()
  const dialog = document.querySelector('[role="dialog"]')!
  const input = dialog.querySelector<HTMLInputElement>('[aria-label="Slug репозитория"]')!
  expect(input.value).toBe('mini')
  flushSync(() => {
    input.value = 'new-name'
    input.dispatchEvent(new Event('input', { bubbles: true }))
  })
  expect(dialog.textContent).toContain('Git-адрес изменится')
  flushSync(() => dialog.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })))
  await settle()
  expect(requests).toHaveLength(1)
  expect(JSON.parse(requests[0].body)).toEqual({ title: 'Mini', slug: 'new-name', branch: 'main' })
})

test('токен в настройках: пустое поле сохраняет, новое заменяет, галочка удаляет', async () => {
  for (const action of ['keep', 'replace', 'remove']) {
    let saved: Record<string, unknown> | null = null
    globalThis.fetch = (async (input, init) => {
      if (init?.method === 'PATCH') saved = JSON.parse(String(init.body))
      return new Response(
        JSON.stringify(
          String(input).includes('/browse')
            ? { branches: ['main'], commits: [], tree: [], empty: true }
            : {
                items: [
                  { ...repo, kind: 'github', url: 'https://github.com/team/project', slug: 'mini', has_token: true },
                ],
                quota_bytes: 1,
              }
        )
      )
    }) as typeof fetch
    const target = document.createElement('div')
    document.body.append(target)
    const app = mount(RepositoriesBlock, { target })
    await settle()
    flushSync(() => target.querySelector<HTMLButtonElement>('.repo-row')!.click())
    await settle()
    flushSync(() =>
      [...target.querySelectorAll('button')]
        .find((b) => b.getAttribute('aria-label') === 'Настройки репозитория')!
        .click()
    )
    await settle()
    const dialog = document.querySelector('[role="dialog"]')!
    const field = dialog.querySelector<HTMLInputElement>('[aria-label="Токен доступа"]')!
    expect(field.value).toBe('')
    expect(field.type).toBe('password')
    if (action === 'replace')
      flushSync(() => {
        field.value = 'fixture-read-token'
        field.dispatchEvent(new Event('input', { bubbles: true }))
      })
    if (action === 'remove') flushSync(() => dialog.querySelector<HTMLInputElement>('input[type="checkbox"]')!.click())
    flushSync(() =>
      dialog.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    )
    await settle()
    expect(saved).not.toBeNull()
    if (action === 'keep') expect(Object.hasOwn(saved!, 'token')).toBe(false)
    else expect(saved!['token']).toBe(action === 'replace' ? 'fixture-read-token' : '')
    await unmount(app)
    target.remove()
  }
})
