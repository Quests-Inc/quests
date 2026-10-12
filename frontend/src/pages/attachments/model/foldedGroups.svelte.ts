// Свёрнутые группы вложений: запоминаются в браузере, но работа без хранилища не ломается.
const KEY = 'quests.attachments.folded'

function load(): Set<string> {
  try {
    const raw = JSON.parse(localStorage.getItem(KEY) || '[]')
    return new Set(Array.isArray(raw) ? raw.map(String) : [])
  } catch {
    return new Set()
  }
}

export function createFoldedGroups() {
  let folded = $state(load())
  return {
    has: (key: string) => folded.has(key),
    toggle(key: string) {
      const next = new Set(folded)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      folded = next
      try {
        localStorage.setItem(KEY, JSON.stringify([...next]))
      } catch {
        /* private mode */
      }
    },
  }
}
