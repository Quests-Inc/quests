/**
 * Keyboard control of the journal: one registry of actions and one keydown
 * dispatcher (design: note=76).
 *
 * Keys are matched by event.code, so they work in any keyboard layout.
 * Plain keys stay silent while the user types (a text field has focus), while
 * a modal or a context menu is open, and when a modifier (Ctrl/Alt/Meta) is
 * held — those belong to the browser and to local handlers such as Ctrl+S in
 * the notes editor.
 *
 * An action: {
 *   id, group, label,
 *   codes: string[],            // event.code values that trigger it
 *   keys: string,               // how the help shows it ("1–7", "↑ ↓")
 *   shift?: boolean | 'any',    // default false: Shift must NOT be held
 *   when?: () => boolean,       // enabled only while true
 *   run: (event) => boolean | void,  // return false = did nothing, don't swallow the key
 * }
 */

const TEXT_INPUT_TYPES = new Set([
  'text',
  'search',
  'url',
  'tel',
  'email',
  'password',
  'number',
  'date',
  'time',
  'datetime-local',
  'month',
  'week',
])

/** @type {Map<string, any>} */
const registry: Map<string, any> = new Map()

/** True when the event target is somewhere the user types. */
export function isTypingTarget(target) {
  if (!(target instanceof HTMLElement)) return false
  if (target.isContentEditable) return true
  const tag = target.tagName
  if (tag === 'TEXTAREA' || tag === 'SELECT') return true
  if (tag === 'INPUT') return TEXT_INPUT_TYPES.has((target.getAttribute('type') || 'text').toLowerCase())
  return false
}

/** Something that already owns the keyboard: a modal dialog or an open context menu. */
export function overlayOpen() {
  return !!document.querySelector('[aria-modal="true"], [role="menu"]')
}

/**
 * Controls that react to Space/Enter themselves. A row of the keyboard-walked
 * list is not one: after Enter on it the focus stays there, and Space must
 * still reach the step keys. A group header is (Space folds it).
 */
function isActivatable(target) {
  if (!(target instanceof HTMLElement)) return false
  if (target.closest('[data-nav]:not([aria-expanded])')) return false
  return !!target.closest('button, a[href], summary, [role="button"], input[type="checkbox"], input[type="radio"]')
}

function matches(action, event) {
  if (!action.codes.includes(event.code)) return false
  const shift = action.shift ?? false
  if (shift === 'any') return true
  return event.shiftKey === shift
}

/**
 * Add actions to the registry. Returns a function that removes them again.
 * @param {any[]} actions
 */
export function registerHotkeys(actions: any[]) {
  for (const a of actions) registry.set(a.id, a)
  return () => {
    for (const a of actions) if (registry.get(a.id) === a) registry.delete(a.id)
  }
}

/** Registered actions grouped for the help screen, in registration order. */
export function hotkeyGroups() {
  /** @type {Map<string, any[]>} */
  const groups: Map<string, any[]> = new Map()
  for (const a of registry.values()) {
    if (!groups.has(a.group)) groups.set(a.group, [])
    groups.get(a.group).push({ keys: a.keys, label: a.label })
  }
  return [...groups].map(([title, items]) => ({ title, items }))
}

/** @param {KeyboardEvent} event */
export function handleKeydown(event: KeyboardEvent) {
  if (event.defaultPrevented || event.isComposing) return
  if (event.ctrlKey || event.altKey || event.metaKey) return
  const target = event.target
  const typing = isTypingTarget(target)

  if (event.code === 'Escape' && typing) {
    // Nobody else took the key (suggestions, inline edit and modals do):
    // leave the field so the page keys work again.
    if (overlayOpen()) return
    event.preventDefault()
    target.blur()
    return
  }
  if (typing || overlayOpen()) return

  for (const action of registry.values()) {
    if (!matches(action, event)) continue
    if (action.when && !action.when()) continue
    // Space/Enter on a focused button or link means "press it".
    if ((event.code === 'Space' || event.code === 'Enter') && isActivatable(target)) return
    if (action.run(event) === false) continue
    event.preventDefault()
    return
  }
}

/** Attach the dispatcher to the window. Returns the detach function. */
export function installHotkeys() {
  window.addEventListener('keydown', handleKeydown)
  return () => window.removeEventListener('keydown', handleKeydown)
}
