import { test } from 'bun:test'
import assert from 'node:assert/strict'

class FakeEl {
  constructor(tag = 'DIV', attrs = {}) {
    this.tagName = tag
    this.attrs = attrs
    this.isContentEditable = false
    this.blurred = false
  }
  getAttribute(n) {
    return this.attrs[n] ?? null
  }
  closest(sel) {
    if (sel.startsWith('[data-nav]')) return this.attrs.navRow ? this : null
    return this.attrs.activatable && sel.includes('button') ? this : null
  }
  blur() {
    this.blurred = true
  }
}
globalThis.HTMLElement = FakeEl
let overlay = false
globalThis.document = { querySelector: () => (overlay ? {} : null) }
globalThis.window = { addEventListener() {}, removeEventListener() {} }

const { handleKeydown, registerHotkeys, hotkeyGroups } = await import('../src/lib/js/hotkeys.js')

function press(code, { target = new FakeEl(), ...mods } = {}) {
  const ev = {
    code,
    target,
    shiftKey: false,
    ctrlKey: false,
    altKey: false,
    metaKey: false,
    isComposing: false,
    defaultPrevented: false,
    ...mods,
  }
  ev.preventDefault = () => {
    ev.defaultPrevented = true
  }
  handleKeydown(ev)
  return ev
}

test('a plain key runs its action and is swallowed', () => {
  let n = 0
  const off = registerHotkeys([
    {
      id: 't1',
      group: 'g',
      label: 'l',
      keys: 'N',
      codes: ['KeyN'],
      run: () => {
        n++
      },
    },
  ])
  assert.equal(press('KeyN').defaultPrevented, true)
  assert.equal(n, 1)
  off()
  press('KeyN')
  assert.equal(n, 1, 'unregistered action must not run')
})

test('silent while typing, with modifiers and over a modal', () => {
  let n = 0
  const off = registerHotkeys([
    {
      id: 't2',
      group: 'g',
      label: 'l',
      keys: 'N',
      codes: ['KeyN'],
      run: () => {
        n++
      },
    },
  ])
  press('KeyN', { target: new FakeEl('INPUT', { type: 'text' }) })
  press('KeyN', { target: new FakeEl('TEXTAREA') })
  press('KeyN', { ctrlKey: true })
  press('KeyN', { metaKey: true })
  overlay = true
  press('KeyN')
  overlay = false
  assert.equal(n, 0)
  press('KeyN', { target: new FakeEl('INPUT', { type: 'checkbox' }) })
  assert.equal(n, 1, 'a checkbox is not a text field')
  off()
})

test('Esc in a field leaves it, unless a modal is open', () => {
  const field = new FakeEl('TEXTAREA')
  const ev = press('Escape', { target: field })
  assert.equal(field.blurred, true)
  assert.equal(ev.defaultPrevented, true)
  const f2 = new FakeEl('INPUT', { type: 'text' })
  overlay = true
  press('Escape', { target: f2 })
  overlay = false
  assert.equal(f2.blurred, false)
})

test('Shift must match; "any" accepts both', () => {
  let strict = 0,
    loose = 0
  const off = registerHotkeys([
    {
      id: 't3',
      group: 'g',
      label: 'l',
      keys: '?',
      codes: ['Slash'],
      shift: true,
      run: () => {
        strict++
      },
    },
    {
      id: 't4',
      group: 'g',
      label: 'l',
      keys: '+',
      codes: ['Equal'],
      shift: 'any',
      run: () => {
        loose++
      },
    },
  ])
  press('Slash')
  press('Slash', { shiftKey: true })
  press('Equal')
  press('Equal', { shiftKey: true })
  assert.deepEqual([strict, loose], [1, 2])
  off()
})

test('Space on a focused button presses the button, not the hotkey', () => {
  let n = 0
  const off = registerHotkeys([
    {
      id: 't5',
      group: 'g',
      label: 'l',
      keys: 'Space',
      codes: ['Space'],
      run: () => {
        n++
      },
    },
  ])
  press('Space', { target: new FakeEl('BUTTON', { activatable: true }) })
  assert.equal(n, 0)
  press('Space')
  assert.equal(n, 1)
  // a focused list row (after Enter on it) is not a button for Space
  press('Space', { target: new FakeEl('BUTTON', { activatable: true, navRow: true }) })
  assert.equal(n, 2)
  off()
})

test('when() gates, and run() === false lets the key fall through', () => {
  let on = false,
    second = 0
  const off = registerHotkeys([
    { id: 't6', group: 'g', label: 'a', keys: 'J', codes: ['KeyJ'], when: () => on, run: () => {} },
    { id: 't7', group: 'g', label: 'b', keys: 'J', codes: ['KeyJ'], run: () => false },
    {
      id: 't8',
      group: 'g',
      label: 'c',
      keys: 'J',
      codes: ['KeyJ'],
      run: () => {
        second++
      },
    },
  ])
  const ev = press('KeyJ')
  assert.equal(second, 1, 'falls past the disabled and the declining action')
  assert.equal(ev.defaultPrevented, true)
  off()
})

test('a declined key keeps its default behaviour', () => {
  const off = registerHotkeys([{ id: 't9', group: 'g', label: 'l', keys: 'K', codes: ['KeyK'], run: () => false }])
  assert.equal(press('KeyK').defaultPrevented, false)
  off()
})

test('help groups follow the registry', () => {
  const off = registerHotkeys([
    { id: 'h1', group: 'Группа А', label: 'один', keys: '1', codes: ['Digit1'], run: () => {} },
    { id: 'h2', group: 'Группа А', label: 'два', keys: '2', codes: ['Digit2'], run: () => {} },
  ])
  const g = hotkeyGroups().find((x) => x.title === 'Группа А')
  assert.deepEqual(
    g.items.map((i) => i.label),
    ['один', 'два']
  )
  off()
  assert.equal(
    hotkeyGroups().some((x) => x.title === 'Группа А'),
    false
  )
})
