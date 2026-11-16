import { afterEach, expect, test } from 'bun:test'

import { flushSync, mount, unmount } from 'svelte'

import NoteHead from '@/pages/notes/ui/NoteHead.svelte'

let cleanup: (() => void) | undefined
afterEach(() => cleanup?.())
function open(titleEditing = false) {
  const refs: unknown[] = []
  let edits = 0
  const noop = () => {}
  const target = document.createElement('div')
  document.body.append(target)
  const app = mount(NoteHead, {
    target,
    props: {
      title: 'Android-релизы (quest=197)',
      titleEditing,
      titleReadonly: !titleEditing,
      dirty: false,
      pinned: false,
      isReadme: false,
      isPrivate: false,
      isCategory: false,
      outdated: false,
      saving: false,
      exportBusy: false,
      crumbs: [],
      labels: { 'quest:197': 'Автосборка релизов' },
      onRef: (kind, id) => refs.push([kind, id]),
      viewMode: 'view',
      viewModes: [],
      onContextMenu: noop,
      onTitleDblClick: () => {
        edits++
      },
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
    },
  })
  flushSync()
  cleanup = () => {
    unmount(app)
    target.remove()
  }
  return { target, refs, edits: () => edits }
}
test('view title resolves references and opens them without starting rename', () => {
  const { target, refs, edits } = open()
  const heading = target.querySelector('h1')!
  expect(heading.textContent).toContain('Автосборка релизов')
  expect(heading.textContent).not.toContain('quest=197')
  const link = heading.querySelector('a')!
  link.click()
  expect(refs).toEqual([['quest', 197]])
  link.dispatchEvent(new MouseEvent('dblclick', { bubbles: true }))
  expect(edits()).toBe(0)
  heading.dispatchEvent(new MouseEvent('dblclick', { bubbles: true }))
  expect(edits()).toBe(1)
})
test('rename keeps the original reference token', () => {
  const { target } = open(true)
  expect(target.querySelector('textarea')?.value).toBe('Android-релизы (quest=197)')
  expect(target.querySelector('h1')).toBeNull()
})
