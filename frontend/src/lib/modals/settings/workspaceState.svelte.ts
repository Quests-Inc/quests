import {
  fetchWorkspaces,
  createWorkspace,
  switchWorkspace,
  fetchMembers,
  removeMember,
  createInvite,
  renameWorkspace,
  deleteWorkspace,
  importWorkspace,
} from '../../js/api.ts'

/** Workspaces of the signed-in account, shared by the settings sections.
 * Switching / creating / deleting reloads the page: every list in the app belongs to one workspace. */
export const wsx = $state({
  enabled: false,
  loaded: false,
  list: /** @type {{ id: number, name: string }[]} */ ([]),
  current: 0,
  members: /** @type {{ user_id: number, username: string, role: string }[]} */ ([]),
  myRole: '',
  /** Attachment bytes the current workspace keeps, and its cap (0 = none). */
  attachmentsUsed: 0,
  attachmentsQuota: 0,
  busy: false,
  error: '',
  notice: '',
})

export function currentWorkspaceName() {
  return wsx.list.find((w) => w.id === wsx.current)?.name ?? ''
}

export async function loadWorkspaceState() {
  try {
    const r = await fetchWorkspaces()
    wsx.enabled = !!r?.enabled
    wsx.list = r?.workspaces ?? []
    wsx.current = r?.current ?? 0
    wsx.attachmentsUsed = r?.attachments_used ?? 0
    wsx.attachmentsQuota = r?.attachments_quota ?? 0
    wsx.loaded = true
    if (wsx.enabled && wsx.current) {
      const m = await fetchMembers(wsx.current)
      wsx.members = m?.members ?? []
      wsx.myRole = m?.role ?? ''
    }
  } catch {
    wsx.enabled = false
  }
}

/** Runs a mutation with the shared busy/error handling. */
async function act(fn, failure) {
  if (wsx.busy) return false
  wsx.busy = true
  wsx.error = ''
  try {
    await fn()
    return true
  } catch (e) {
    wsx.error = e?.message || failure
    return false
  } finally {
    wsx.busy = false
  }
}

export async function pickWorkspace(id) {
  if (id === wsx.current) return
  if (await act(() => switchWorkspace(id), 'не удалось переключить')) location.reload()
}

export async function addWorkspace(name) {
  if (!name.trim()) return false
  const ok = await act(async () => {
    const w = await createWorkspace(name.trim())
    await switchWorkspace(w.id)
  }, 'не удалось создать')
  if (ok) location.reload()
  return ok
}

export async function saveName(name) {
  const n = name.trim()
  if (!n || n === currentWorkspaceName()) return
  await act(async () => {
    await renameWorkspace(wsx.current, n)
    wsx.list = wsx.list.map((w) => (w.id === wsx.current ? { ...w, name: n } : w))
  }, 'не удалось переименовать')
}

export async function removeCurrent(confirmName) {
  if (await act(() => deleteWorkspace(wsx.current, confirmName.trim()), 'не удалось удалить')) location.reload()
}

/** @returns {Promise<string>} the one-time link, or '' on failure */
export async function newInvite(): Promise<string> {
  let url = ''
  await act(async () => {
    const r = await createInvite(wsx.current)
    url = location.origin + r.path
  }, 'не удалось создать приглашение')
  return url
}

export async function dropMember(m, me) {
  const self = m.username === me
  if (!confirm(self ? 'Выйти из этого пространства?' : `Убрать ${m.username} из пространства?`)) return
  const ok = await act(() => removeMember(wsx.current, m.user_id), 'не удалось убрать')
  if (!ok) return
  if (self) location.reload()
  else wsx.members = wsx.members.filter((x) => x.user_id !== m.user_id)
}

export async function doImport(file, secrets) {
  wsx.notice = ''
  const ok = await act(async () => {
    const r = await importWorkspace(file, { secrets })
    const rep = r.report || {}
    wsx.notice = `Создано пространство «${r.name}». Команд без раннера убрано: ${rep.stripped_commands ?? 0}, битых ссылок: ${rep.dangling_refs ?? 0}.`
    await switchWorkspace(r.workspace_id)
  }, 'не удалось импортировать')
  if (ok) setTimeout(() => location.reload(), 1800)
}
