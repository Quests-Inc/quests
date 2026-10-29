import { afterEach, expect, mock, test } from 'bun:test'

import { mount, unmount } from 'svelte'

import { setAttachmentIndex } from '@/entities/attachment'
import type { AttachmentView } from '@/shared/api/knowledge_http.gen.ts'
const real = await import('@/shared/api/client.ts')
mock.module('@/shared/api/client.ts', () => ({ ...real, getQuestLog: async () => ({ items: [] }) }))
const { default: AttachmentsPage } = await import('@/pages/attachments/ui/AttachmentsPage.svelte')
const file = (id: number, filename: string, type: string): AttachmentView => ({
  id,
  filename,
  owner_type: 'note',
  owner_id: 1,
  size_bytes: 2048,
  content_type_declared: type,
  content_type_detected: type,
  comment: 'Комментарий',
  scan_status: 'clean',
  revision: 1,
  revision_count: 1,
  hidden: false,
})
let app: ReturnType<typeof mount>
let target: HTMLDivElement
const tick = () => new Promise((resolve) => setTimeout(resolve, 0))
afterEach(() => {
  unmount(app)
  target.remove()
  setAttachmentIndex(null)
})
test('cards retain previews, file actions and multi-selection; all three layouts remain available', async () => {
  setAttachmentIndex({
    note: {
      '1': [
        { ...file(1, 'Карта.png', 'image/png'), revision: 2, revision_count: 2 },
        file(2, 'Отчёт.pdf', 'application/pdf'),
      ],
    },
  })
  const picked: number[] = []
  target = document.createElement('div')
  document.body.append(target)
  app = mount(AttachmentsPage, { target, props: { onSelect: (id) => id != null && picked.push(id) } })
  await tick()
  const viewButton = () => target.querySelector<HTMLButtonElement>('.attpage__view')!
  expect(target.querySelector('.attpage__group')).not.toBeNull()
  viewButton().click()
  await tick()
  expect(viewButton().dataset.view).toBe('cards')
  expect(viewButton().textContent?.trim()).toBe('')
  expect(target.querySelector('.attpage__list--cards')).not.toBeNull()
  expect(target.querySelectorAll('[data-att-id]')).toHaveLength(2)
  expect(target.querySelector('[data-att-id="1"] img')?.getAttribute('loading')).toBe('lazy')
  expect(target.querySelectorAll('a[download]')).toHaveLength(2)
  expect(target.querySelector('[data-att-id="1"] .attpage__acts .attpage__revision')?.textContent).toBe('v2/2')
  expect(target.querySelector('[data-att-id="1"] .attpage__name .attpage__revision')).toBeNull()
  target.querySelector<HTMLElement>('[data-att-id="1"]')!.click()
  expect(picked).toEqual([1])
  target.querySelector('[data-att-id="2"]')!.dispatchEvent(new MouseEvent('click', { bubbles: true, ctrlKey: true }))
  await tick()
  expect(target.querySelector('[data-att-id="2"]')!.classList.contains('attpage__item--picked')).toBe(true)
  viewButton().click()
  await tick()
  expect(target.querySelector('.attpage__list--cards')).toBeNull()
  expect(target.querySelector('.attpage__group')).toBeNull()
  viewButton().click()
  await tick()
  expect(target.querySelector('.attpage__group')).not.toBeNull()
  const sortButton = target.querySelector<HTMLButtonElement>('[aria-label="Сортировка"]')!
  expect(target.querySelector('select')).toBeNull()
  sortButton.click()
  await tick()
  const byName = [...target.querySelectorAll<HTMLButtonElement>('[role="menuitemradio"]')].find(
    (b) => b.textContent?.trim() === 'По имени'
  )!
  byName.click()
  await tick()
  expect(target.querySelector('[data-att-id]')?.getAttribute('data-att-id')).toBe('1')
  expect(sortButton.textContent).toContain('По имени')
  expect(sortButton.getAttribute('aria-expanded')).toBe('false')
  expect(document.activeElement).toBe(sortButton)
})
