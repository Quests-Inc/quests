// Редактор открытой заметки: форма, сохранённая копия, черновик, загрузка, внешние правки и сохранение.
// Страница только показывает поля и зовёт методы; правила «что считается правкой» живут здесь.
import { tick, untrack } from 'svelte'

import {
  clearNoteDraft,
  getNoteDraft,
  type NoteForm,
  noteFormFromRow,
  noteFormsEqual,
  putNoteDraft,
} from '@/entities/note'
import { getNote, updateNote } from '@/shared/api/client.ts'
import type { NotePayload } from '@/shared/api/knowledge_http.gen.ts'
import { toast } from '@/shared/lib/toasts.svelte.ts'

const EMPTY: NoteForm = {
  title: '',
  description: '',
  parentId: '',
  pinned: false,
  isReadme: false,
  isPrivate: false,
  isCategory: false,
}

const messageOf = (e: unknown) => (e as { message?: string })?.message || String(e)

export function createNoteEditor(deps: {
  selectedId: () => number | null
  notes: () => NotePayload[]
  onChanged: () => void
}) {
  let detail = $state<NotePayload | null>(null)
  let title = $state('')
  let description = $state('')
  let parentId = $state('')
  let pinned = $state(false)
  let isReadme = $state(false)
  let isPrivate = $state(false)
  let isCategory = $state(false)
  let privateRevealed = $state(false)
  let saving = $state(false)
  /** Заметку изменили снаружи (агент, другая вкладка, бот), пока тут есть несохранённые правки:
   * входящая версия ждёт решения пользователя, а не заменяет его текст. */
  let external = $state<NoteForm | null>(null)
  let flashing = $state(false)
  let error = $state('')
  let saved = $state<NoteForm>({ ...EMPTY })
  let loadedId = $state<number | null>(null)

  const dirty = $derived(!noteFormsEqual(formDraft(), saved))

  function formDraft(): NoteForm {
    return { title, description, parentId, pinned, isReadme, isPrivate, isCategory }
  }
  function apply(src: NoteForm) {
    title = src.title
    description = src.description
    parentId = src.parentId
    pinned = src.pinned
    isReadme = src.isReadme
    isPrivate = src.isPrivate
    isCategory = src.isCategory
    privateRevealed = false
  }
  function syncDraft(id: number | null) {
    if (id == null) return
    const form = formDraft()
    if (noteFormsEqual(form, saved)) clearNoteDraft(id)
    else putNoteDraft(id, form)
  }

  function revert() {
    external = null
    apply(saved)
    const id = deps.selectedId()
    if (id != null) clearNoteDraft(id)
  }

  // Загрузка выбранной заметки; уходя с неё, запоминаем черновик.
  $effect(() => {
    const id = deps.selectedId()
    untrack(() => {
      if (loadedId != null && loadedId !== id) syncDraft(loadedId)
    })
    if (id == null) {
      detail = null
      apply(EMPTY)
      saved = { ...EMPTY }
      loadedId = null
      return
    }
    let cancelled = false
    getNote(id)
      .then((row) => {
        if (cancelled) return
        detail = row
        const next = noteFormFromRow(row)
        apply({ ...next, ...getNoteDraft(id) })
        saved = next
        loadedId = id
        error = ''
      })
      .catch((e) => {
        if (!cancelled) error = messageOf(e)
      })
    return () => {
      cancelled = true
      untrack(() => syncDraft(id))
    }
  })

  // Черновик пишется по мере правки.
  $effect(() => {
    const id = deps.selectedId()
    if (id == null || loadedId !== id) return
    const form = formDraft()
    untrack(() => {
      if (noteFormsEqual(form, saved)) clearNoteDraft(id)
      else putNoteDraft(id, form)
    })
  })

  async function flashBody() {
    flashing = false
    await tick()
    flashing = true
    setTimeout(() => (flashing = false), 1500)
  }

  /** Принять текущую версию заметки с сервера как форму. */
  async function adoptExternal(id: number) {
    try {
      const row = await getNote(id)
      if (deps.selectedId() !== id) return
      detail = row
      const next = noteFormFromRow(row)
      apply(next)
      saved = next
      clearNoteDraft(id)
      external = null
      void flashBody()
    } catch (e) {
      error = messageOf(e)
    }
  }

  // Живые обновления: журнал перечитывает список заметок на каждое событие сервера, открытая заметка
  // сравнивается со своей строкой. Более новая и отличающаяся строка — значит, правил кто-то другой.
  $effect(() => {
    const id = loadedId
    const row = id != null ? deps.notes().find((n) => n.id === id) : null
    if (!row || id == null) return
    untrack(() => {
      if (saving) return
      // Список, полученный до нашего сохранения, несёт более старую строку.
      if (detail?.updated_at && row.updated_at && row.updated_at <= detail.updated_at) return
      const next = noteFormFromRow(row)
      if (noteFormsEqual(next, saved)) return
      if (noteFormsEqual(formDraft(), saved)) void adoptExternal(id)
      else external = next
    })
  })

  async function save() {
    const id = deps.selectedId()
    if (id == null || saving) return
    const t = title.trim()
    if (!t) {
      error = 'Нужен заголовок'
      toast(error, { kind: 'error' })
      return
    }
    saving = true
    error = ''
    try {
      const row = await updateNote(id, {
        title: t,
        description,
        pinned,
        is_readme: isReadme,
        is_private: isPrivate,
        is_category: isCategory,
        parent_id: parentId === '' ? null : Number(parentId),
      })
      detail = row
      saved = noteFormFromRow(row)
      apply(saved)
      clearNoteDraft(id)
      external = null
      deps.onChanged()
      toast('Сохранено', { kind: 'success', ttl: 1400 })
    } catch (e) {
      error = messageOf(e)
      toast(error, { kind: 'error' })
    } finally {
      saving = false
    }
  }

  /** Заголовок сохраняется отдельно от описания: по blur/Enter. */
  async function commitTitle() {
    const id = deps.selectedId()
    if (id == null) return
    const t = title.trim()
    if (!t || t === saved.title) {
      title = saved.title
      return
    }
    try {
      detail = await updateNote(id, { title: t })
      saved = { ...saved, title: t }
      title = t
      deps.onChanged()
    } catch (e) {
      error = messageOf(e)
      toast(error, { kind: 'error' })
    }
  }

  /**
   * Быстрое сохранение одного поля (из дерева или меню): если заметка открыта, форма и сохранённая копия
   * получают значение, а правка поля в черновике сохраняется. Нетронутое поле формы заменяется значением,
   * тронутое — остаётся, если `keepEdited`.
   */
  function patched(id: number, row: NotePayload, patch: Partial<NoteForm>, keepEdited = false) {
    if (id !== deps.selectedId()) return
    const untouched = (Object.keys(patch) as (keyof NoteForm)[]).every((k) => form()[k] === saved[k])
    saved = { ...saved, ...patch }
    if (!keepEdited || untouched) Object.assign(fields, patch)
    detail = row
    const draft = getNoteDraft(id)
    if (draft) putNoteDraft(id, { ...draft, ...patch })
  }
  const form = formDraft
  const fields = {
    set title(v: string) {
      title = v
    },
    set parentId(v: string) {
      parentId = v
    },
    set pinned(v: boolean) {
      pinned = v
    },
    set isReadme(v: boolean) {
      isReadme = v
    },
    set isPrivate(v: boolean) {
      isPrivate = v
    },
    set isCategory(v: boolean) {
      isCategory = v
    },
  }

  return {
    get title() {
      return title
    },
    set title(v: string) {
      title = v
    },
    get description() {
      return description
    },
    set description(v: string) {
      description = v
    },
    get parentId() {
      return parentId
    },
    get pinned() {
      return pinned
    },
    get isReadme() {
      return isReadme
    },
    get isPrivate() {
      return isPrivate
    },
    get isCategory() {
      return isCategory
    },
    get privateRevealed() {
      return privateRevealed
    },
    set privateRevealed(v: boolean) {
      privateRevealed = v
    },
    get saving() {
      return saving
    },
    get external() {
      return external
    },
    set external(v: NoteForm | null) {
      external = v
    },
    get flashing() {
      return flashing
    },
    get error() {
      return error
    },
    set error(v: string) {
      error = v
    },
    get detail() {
      return detail
    },
    set detail(v: NotePayload | null) {
      detail = v
    },
    get saved() {
      return saved
    },
    get loadedId() {
      return loadedId
    },
    get dirty() {
      return dirty
    },
    form,
    revert,
    save,
    commitTitle,
    patched,
    adoptExternal,
  }
}
