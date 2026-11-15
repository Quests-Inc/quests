const key = 'quests.ui.hideNoteHistory'
function readHidden(storageKey = key) {
  try {
    return localStorage.getItem(storageKey) === 'true'
  } catch {
    return false
  }
}
let hideNoteHistory = $state(readHidden())
let hideNoteHistoryIcons = $state(readHidden('quests.ui.hideNoteHistoryIcons'))
export const uiPreferences = {
  get hideNoteHistoryIcons() {
    return hideNoteHistoryIcons
  },
  set hideNoteHistoryIcons(value: boolean) {
    hideNoteHistoryIcons = value
    try {
      localStorage.setItem('quests.ui.hideNoteHistoryIcons', String(value))
    } catch {
      /* Keep the setting for this session. */
    }
  },
  get hideNoteHistory() {
    return hideNoteHistory
  },
  set hideNoteHistory(value: boolean) {
    hideNoteHistory = value
    try {
      localStorage.setItem(key, String(value))
    } catch {
      /* Keep the setting for this session. */
    }
  },
}
