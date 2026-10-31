import { expect, mock, test } from 'bun:test'

import { flushSync, mount, unmount } from 'svelte'

import { cell } from './root.svelte.ts'

const pending = new Map<number, (v: unknown) => void>()
const real = await import('@/shared/api/client.ts')
globalThis.fetch = (async () => new Response('[]', { status: 200 })) as typeof fetch
mock.module('@/shared/api/client.ts', () => ({
  ...real,
  listValues: (_kind: string, id: number) => new Promise((resolve) => pending.set(id, resolve)),
}))
const { default: NoteCanvas } = await import('@/pages/notes/ui/NoteCanvas.svelte')
const tick = () => new Promise((resolve) => setTimeout(resolve, 0))
const values = (hp: number) => ({ own: [], inherited: [], effective: [{ name: 'hp', type: 'int', value: hp }] })

test('opening and switching notes never renders raw variables or values from the previous note', async () => {
  const selected = cell(259)
  const target = document.createElement('div')
  document.body.append(target)
  const noop = () => {}
  const props = {
    ed: { title: 'Персонаж', description: 'Здоровье: {{hp}}', detail: {}, dirty: false },
    get selectedId() {
      return selected.value
    },
    titleReadonly: true,
    noteIsOutdated: false,
    exportBusy: false,
    crumbs: [],
    viewMode: 'formatted',
    viewModes: [],
    onOpenNoteMenu: noop,
    onTitleDblClick: noop,
    onTitleBlur: noop,
    onTitleKeydown: noop,
    onTogglePin: noop,
    onToggleReadme: noop,
    onTogglePrivate: noop,
    onToggleCategory: noop,
    onToggleOutdated: noop,
    onRevert: noop,
    onSave: noop,
    onExport: noop,
    onDelete: noop,
    onCrumb: noop,
    onParentPicker: noop,
    onViewMode: noop,
    onEditKeydown: noop,
    onStartBodyEdit: noop,
    onChanged: noop,
  }
  const app = mount(NoteCanvas, { target, props: props as any })
  try {
    flushSync()
    const body = () => document.querySelector('.notes__preview')!.textContent!.trim()
    expect(body()).toBe('Здоровье: …')
    pending.get(259)!(values(20))
    await tick()
    flushSync()
    expect(body()).toBe('Здоровье: 20')
    selected.value = 260
    flushSync()
    expect(body()).toBe('Здоровье: …')
    pending.get(260)!(values(12))
    await tick()
    flushSync()
    expect(body()).toBe('Здоровье: 12')
  } finally {
    await unmount(app)
    target.remove()
  }
})
