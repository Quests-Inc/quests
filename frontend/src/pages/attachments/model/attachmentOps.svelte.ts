// Операции над вложением на странице «Вложения»: версии, комментарий, скрытие, удаление.
// Каждая мутация выполняется под `busyId`, затем индекс и версии открытой карточки перечитываются.
import { ownerKind } from '@/entities/attachment'
import type { RevisionRead } from '@/shared/api/knowledge.gen.ts'
import { toast } from '@/shared/lib/toasts.svelte.ts'

type Att = { id: number; owner_id: number; hidden?: boolean; comment?: string; filename?: string } & Record<
  string,
  unknown
>
type Pending = { att: Att; revision: number | null }
const messageOf = (e: unknown) => (e as { message?: string })?.message || String(e)

export function createAttachmentOps(deps: {
  selected: () => Att | null
  selectedId: () => number | null
  onSelect: (id: number | null) => void
  onChanged: () => void
  api: {
    listRevisions: (kind: string, ownerId: number, id: number) => Promise<{ revisions?: RevisionRead[] } | undefined>
    setHidden: (kind: string, ownerId: number, id: number, hidden: boolean) => Promise<unknown>
    setCurrent: (kind: string, ownerId: number, id: number, revision: number) => Promise<unknown>
    updateComment: (kind: string, ownerId: number, id: number, comment: string) => Promise<unknown>
    remove: (kind: string, ownerId: number, id: number) => Promise<unknown>
    removeRevision: (kind: string, ownerId: number, id: number, revision: number) => Promise<unknown>
    uploadRevision: (kind: string, ownerId: number, id: number, file: File) => Promise<unknown>
  }
}) {
  let busyId = $state<number | null>(null)
  let commentDraft = $state('')
  let commentFor: number | null = null
  let revs = $state<RevisionRead[]>([])
  let revsFor: number | null = null
  let revsBusy = $state(false)
  let pending = $state<Pending | null>(null)
  let deleteBusy = $state(false)
  let versionFor: Att | null = null

  const fail = (e: unknown) => toast(messageOf(e), { kind: 'error' })

  async function loadRevs(a: Att) {
    revsBusy = true
    try {
      const data = await deps.api.listRevisions(ownerKind(a), a.owner_id, a.id)
      if (revsFor === a.id) revs = (data?.revisions || []).slice().reverse()
    } catch (e) {
      fail(e)
    } finally {
      revsBusy = false
    }
  }

  /** Комментарий и версии следуют за выбранной карточкой; версии грузятся лениво. */
  function follow() {
    const a = deps.selected()
    if (!a) {
      commentFor = null
      revsFor = null
      revs = []
      return
    }
    if (commentFor !== a.id) {
      commentFor = a.id
      commentDraft = (a.comment as string) || ''
    }
    if (revsFor !== a.id) {
      revsFor = a.id
      void loadRevs(a)
    }
  }
  $effect(follow)

  async function run(a: Att, fn: () => Promise<unknown>, okMessage?: string) {
    busyId = a.id
    try {
      await fn()
      if (okMessage) toast(okMessage, { kind: 'success', ttl: 1800 })
      deps.onChanged()
      if (deps.selectedId() === a.id) await loadRevs(a)
    } catch (e) {
      fail(e)
    } finally {
      busyId = null
    }
  }

  const toggleHidden = (a: Att) =>
    run(
      a,
      () => deps.api.setHidden(ownerKind(a), a.owner_id, a.id, !a.hidden),
      a.hidden ? 'Вложение возвращено' : 'Вложение скрыто'
    )
  const makeCurrent = (a: Att, revision: number) =>
    run(a, () => deps.api.setCurrent(ownerKind(a), a.owner_id, a.id, revision), `Версия ${revision} — текущая`)
  function saveComment(a: Att) {
    const next = commentDraft.trim()
    if (next === (a.comment || '')) return
    return run(a, () => deps.api.updateComment(ownerKind(a), a.owner_id, a.id, next))
  }

  async function confirmDelete() {
    const p = pending
    if (!p) return
    deleteBusy = true
    try {
      const { att, revision } = p
      const kind = ownerKind(att)
      if (revision == null) {
        await deps.api.remove(kind, att.owner_id, att.id)
        if (deps.selectedId() === att.id) deps.onSelect(null)
        toast('Вложение удалено', { kind: 'success', ttl: 1800 })
      } else {
        await deps.api.removeRevision(kind, att.owner_id, att.id, revision)
        toast(`Версия ${revision} удалена`, { kind: 'success', ttl: 1800 })
        if (deps.selectedId() === att.id) await loadRevs(att)
      }
      deps.onChanged()
    } catch (e) {
      fail(e)
    } finally {
      deleteBusy = false
      pending = null
    }
  }

  /** Запомнить вложение, для которого сейчас выбирают файл новой версии. */
  function expectVersion(a: Att): void {
    versionFor = a
  }
  async function uploadVersion(file: File | undefined) {
    const a = versionFor
    versionFor = null
    if (!file || !a) return
    await run(a, () => deps.api.uploadRevision(ownerKind(a), a.owner_id, a.id, file), 'Новая версия загружена')
  }

  return {
    get busyId() {
      return busyId
    },
    get commentDraft() {
      return commentDraft
    },
    set commentDraft(v: string) {
      commentDraft = v
    },
    get revs() {
      return revs
    },
    get revsBusy() {
      return revsBusy
    },
    get pending() {
      return pending
    },
    set pending(v: Pending | null) {
      pending = v
    },
    get deleteBusy() {
      return deleteBusy
    },
    toggleHidden,
    makeCurrent,
    saveComment,
    confirmDelete,
    expectVersion,
    uploadVersion,
  }
}
