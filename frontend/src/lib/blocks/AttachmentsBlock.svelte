<script lang="ts">
  import Icon from '../ui/Icon.svelte'
  import FileKindIcon from '../ui/FileKindIcon.svelte'
  import ContextMenu from '../ui/ContextMenu.svelte'
  import ConfirmModal from '../modals/ConfirmModal.svelte'
  import {
    deleteAttachment,
    deleteAttachmentRevision,
    listAttachments,
    listAttachmentRevisions,
    setAttachmentCurrentRevision,
    setAttachmentHidden,
    updateAttachmentComment,
    uploadAttachment,
    uploadAttachmentRevision,
    attachmentDownloadUrl,
  } from '../js/api.ts'
  import { formatLocal } from '../js/time.ts'
  import { copyText } from '../js/clipboard.ts'
  import { toast } from '../js/toasts.svelte.ts'
  import {
    peekAttachmentList,
    setAttachmentList,
    onAttachmentLiveInvalidate,
  } from '../js/attachmentCache.ts'

  let { ownerType, ownerId, onOpenOwner, label = 'Вложения', labelClass = 'block__label' }: { ownerType: 'quest' | 'questline' | 'note', ownerId: number, onOpenOwner?: (kind: string, id: number) => void, label?: string, labelClass?: string } = $props()

  /** Файлы свёрнуты по умолчанию; строка «Прикрепить файл» видна всегда. */
  let open = $state(false)

  let items: any[] = $state([])
  /** Hidden cards live only on the Attachments tab. */
  let visible = $derived(items.filter((a) => !a.hidden))
  let pendingDelete: { att: any, revision: number | null } | null = $state(null)
  let deleteBusy = $state(false)
  let uploading = $state(false)
  let error = $state('')
  let dragOver = $state(false)
  let fileInput = $state((null as HTMLInputElement | null))
  let versionInput = $state((null as HTMLInputElement | null))
  let versionForId: number | null = $state(null)
  let commentDraft: Record<number, string> = $state({})
  let commentBusy: number | null = $state(null)
  let historyOpenId: number | null = $state(null)
  let historyById: Record<number, any[]> = $state({})
  let historyBusy = $state(false)
  let ctxOpen = $state(false)
  let ctxX = $state(0)
  let ctxY = $state(0)
  let ctxAtt = $state((null as any | null))

  let ctxItems = $derived.by(() => {
    const a = ctxAtt
    if (!a) return []
    const items: { id: string, label?: string, sep?: boolean, danger?: boolean, disabled?: boolean }[] = [
      { id: 'download', label: 'Скачать', disabled: a.available === false },
      { id: 'new-version', label: 'Новая версия…' },
    ]
    if ((a.revision_count || 1) > 1 || historyOpenId === a.id) {
      items.push({
        id: 'history',
        label: historyOpenId === a.id ? 'Скрыть историю' : 'История версий',
      })
    }
    if (onOpenOwner) items.push({ id: 'open', label: 'Открыть владельца' })
    items.push(
      { id: 'copy-id', label: `Копировать attachment=${a.id}` },
      { id: 'hide', label: 'Скрыть' },
      { id: 'sep-danger', sep: true },
      { id: 'delete', label: 'Удалить…', danger: true },
    )
    return items
  })

  function openAttMenu(event: MouseEvent, a: any) {
    const target = event.target
    if (target instanceof Element && target.closest('a, button, input, textarea')) return
    event.preventDefault()
    event.stopPropagation()
    ctxAtt = a
    ctxX = event.clientX
    ctxY = event.clientY
    ctxOpen = true
  }

  function downloadAtt(a: { id: number; filename: string }, revision: number | string | null = null) {
    const link = document.createElement('a')
    link.href = attachmentDownloadUrl(ownerType, ownerId, a.id, revision)
    link.download = a.filename || ''
    link.rel = 'noopener'
    document.body.appendChild(link)
    link.click()
    link.remove()
  }

  async function toggleHistory(a: { id: number }) {
    if (historyOpenId === a.id) {
      historyOpenId = null
      return
    }
    historyBusy = true
    error = ''
    try {
      const data = await listAttachmentRevisions(ownerType, ownerId, a.id)
      historyById = { ...historyById, [a.id]: data?.revisions || [] }
      historyOpenId = a.id
    } catch (e) {
      error = e?.message || String(e)
    } finally {
      historyBusy = false
    }
  }

  async function makeCurrent(a: { id: number }, revision: string|number) {
    error = ''
    try {
      const updated = await setAttachmentCurrentRevision(ownerType, ownerId, a.id, revision)
      applyRows(
        ownerType,
        ownerId,
        items.map((row) => (row.id === a.id ? { ...row, ...updated } : row)),
        { probed: peekAttachmentList(ownerType, ownerId)?.probed ?? true },
      )
      const data = await listAttachmentRevisions(ownerType, ownerId, a.id)
      historyById = { ...historyById, [a.id]: data?.revisions || [] }
      toast(`Версия ${revision} — текущая`, { kind: 'success', ttl: 1600 })
    } catch (e) {
      error = e?.message || String(e)
    }
  }

  async function confirmDelete() {
    const p = pendingDelete
    if (!p) return
    deleteBusy = true
    try {
      if (p.revision == null) {
        await remove(p.att)
        if (!error) toast('Вложение удалено', { kind: 'success' })
      } else {
        await removeRevision(p.att, p.revision)
      }
    } finally {
      deleteBusy = false
      pendingDelete = null
    }
  }

  async function hide(a: { id: number }) {
    error = ''
    try {
      const updated = await setAttachmentHidden(ownerType, ownerId, a.id, true)
      applyRows(
        ownerType,
        ownerId,
        items.map((row) => (row.id === a.id ? { ...row, ...updated } : row)),
        { probed: peekAttachmentList(ownerType, ownerId)?.probed ?? true },
      )
      if (historyOpenId === a.id) historyOpenId = null
      toast('Вложение скрыто — вернуть можно на вкладке «Вложения»', { kind: 'success' })
    } catch (e) {
      error = e?.message || String(e)
    }
  }

  async function removeRevision(a: { id: number }, revision: number) {
    error = ''
    try {
      await deleteAttachmentRevision(ownerType, ownerId, a.id, revision)
      const data = await listAttachmentRevisions(ownerType, ownerId, a.id)
      historyById = { ...historyById, [a.id]: data?.revisions || [] }
      applyRows(ownerType, ownerId, (await listAttachments(ownerType, ownerId)) || [], {
        probed: true,
      })
    } catch (e) {
      error = e?.message || String(e)
    }
  }

  async function onAttSelect(action: string) {
    const a = ctxAtt
    if (!a) return
    if (action === 'download') {
      if (a.available === false) return
      downloadAtt(a)
      return
    }
    if (action === 'new-version') {
      versionForId = a.id
      queueMicrotask(() => versionInput?.click())
      return
    }
    if (action === 'history') {
      await toggleHistory(a)
      return
    }
    if (action === 'open') {
      onOpenOwner?.(ownerType, Number(ownerId))
      return
    }
    if (action === 'copy-id') {
      try {
        const text = `attachment=${a.id}`
        await copyText(text)
        toast(`Скопировано ${text}`, { kind: 'success', ttl: 1600 })
      } catch (e) {
        toast(e?.message || String(e), { kind: 'error' })
      }
      return
    }
    if (action === 'hide') {
      await hide(a)
      return
    }
    if (action === 'delete') pendingDelete = { att: a, revision: null }
  }

  function draftsFrom(rows: any[]) {
    const drafts = ({} as Record<number, string>)
    for (const a of rows || []) drafts[a.id] = a.comment || ''
    return drafts
  }

  function applyRows(type: string, id: number, rows: any[], { probed = false } = {}) {
    const next = rows || []
    setAttachmentList(type, id, next, { probed })
    if (type !== ownerType || id !== ownerId) return
    items = next
    commentDraft = draftsFrom(next)
  }

  $effect.pre(() => {
    const type = ownerType
    const id = ownerId
    const hit = peekAttachmentList(type, id)
    const next = hit?.items ?? []
    error = ''
    items = next
    commentDraft = draftsFrom(next)
    historyOpenId = null
  })

  let liveNudge = $state(0)
  $effect(() => onAttachmentLiveInvalidate(() => {
    liveNudge += 1
  }))

  $effect(() => {
    const type = ownerType
    const id = ownerId
    void liveNudge
    let cancelled = false
    if (!id) return
    const hit = peekAttachmentList(type, id)
    if (hit?.probed) return
    listAttachments(type, id)
      .then((rows) => {
        setAttachmentList(type, id, rows || [], { probed: true })
        if (cancelled) return
        items = rows || []
        commentDraft = draftsFrom(items)
      })
      .catch((e) => {
        if (cancelled) return
        error = e?.message || String(e)
        if (!hit) applyRows(type, id, [], { probed: false })
      })
    return () => {
      cancelled = true
    }
  })

  function formatSize(n: any) {
    const bytes = Number(n) || 0
    if (bytes < 1024) return `${bytes} B`
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
  }

  async function sendFiles(files: any) {
    const list = [...files].filter(Boolean)
    if (!list.length || uploading) return
    uploading = true
    error = ''
    try {
      for (const file of list) {
        await uploadAttachment(ownerType, ownerId, file)
      }
      applyRows(ownerType, ownerId, (await listAttachments(ownerType, ownerId)) || [], {
        probed: true,
      })
      open = true
    } catch (e) {
      error = e?.message || String(e)
    } finally {
      uploading = false
      if (fileInput) fileInput.value = ''
    }
  }

  async function onVersionPick(event: Event & { currentTarget: HTMLInputElement }) {
    const file = event.currentTarget.files?.[0]
    const aid = versionForId
    versionForId = null
    if (versionInput) versionInput.value = ''
    if (!file || aid == null) return
    uploading = true
    error = ''
    try {
      await uploadAttachmentRevision(ownerType, ownerId, aid, file)
      applyRows(ownerType, ownerId, (await listAttachments(ownerType, ownerId)) || [], {
        probed: true,
      })
      if (historyOpenId === aid) {
        const data = await listAttachmentRevisions(ownerType, ownerId, aid)
        historyById = { ...historyById, [aid]: data?.revisions || [] }
      }
      toast('Новая версия загружена', { kind: 'success', ttl: 1600 })
    } catch (e) {
      error = e?.message || String(e)
    } finally {
      uploading = false
    }
  }

  function onPick(event: { currentTarget: { files: any } }) {
    sendFiles(event.currentTarget.files || [])
  }

  function onDrop(event: { preventDefault: () => void; dataTransfer: { files: any } }) {
    event.preventDefault()
    dragOver = false
    sendFiles(event.dataTransfer?.files || [])
  }

  async function saveComment(a: { id: number; comment: any }) {
    const next = (commentDraft[a.id] ?? '').trim()
    if (next === (a.comment || '')) return
    commentBusy = a.id
    error = ''
    try {
      const updated = await updateAttachmentComment(ownerType, ownerId, a.id, next)
      applyRows(
        ownerType,
        ownerId,
        items.map((row) => (row.id === a.id ? { ...row, ...updated } : row)),
        { probed: peekAttachmentList(ownerType, ownerId)?.probed ?? true },
      )
    } catch (e) {
      error = e?.message || String(e)
    } finally {
      commentBusy = null
    }
  }

  async function remove(a: { id: number }) {
    error = ''
    try {
      await deleteAttachment(ownerType, ownerId, a.id)
      applyRows(
        ownerType,
        ownerId,
        items.filter((row) => row.id !== a.id),
        { probed: peekAttachmentList(ownerType, ownerId)?.probed ?? true },
      )
      if (historyOpenId === a.id) historyOpenId = null
    } catch (e) {
      error = e?.message || String(e)
    }
  }
</script>

<div
  class="attach"
  class:attach--over={dragOver}
  role="region"
  aria-label="Вложения"
  ondragover={(e) => {
    e.preventDefault()
    dragOver = true
  }}
  ondragleave={() => (dragOver = false)}
  ondrop={onDrop}
>
  {#if error}
    <p class="attach__error">{error}</p>
  {/if}

  {#if visible.length}
    <button
      type="button"
      class="{labelClass} attach__toggle"
      onclick={() => (open = !open)}
      aria-expanded={open}
    >
      {label} ({visible.length})
      <Icon name={open ? 'chevron-down' : 'chevron-right'} size={12} />
    </button>
  {:else}
    <span class="{labelClass} attach__toggle attach__toggle--static">{label}</span>
  {/if}

  {#if visible.length && open}
    <ul class="attach__list">
      {#each visible as a (a.id)}
        {@const unavailable = a.available === false}
        {@const revCount = Number(a.revision_count) || 1}
        {@const rev = Number(a.revision) || 1}
        <li
          class="attach__row"
          class:attach__row--off={unavailable}
          oncontextmenu={(e) => openAttMenu(e, a)}
        >
          <span class="attach__file">
            <span class="attach__name">
              <FileKindIcon att={a} size={16} />
              {a.filename}
              {#if revCount > 1}
                <button
                  type="button"
                  class="attach__rev"
                  title="История версий"
                  onclick={() => toggleHistory(a)}
                >
                  v{rev}/{revCount}
                </button>
              {/if}
            </span>
            <span class="attach__meta">
              {formatSize(a.size_bytes)}
              {#if a.content_type_detected}
                · {a.content_type_detected}
              {/if}
              {#if a.scan_status && a.scan_status !== 'clean'}
                · {a.scan_status}
              {/if}
            </span>
            {#if a.uploaded_at}
              <span class="attach__meta">загружен {formatLocal(a.uploaded_at)}</span>
            {/if}
            {#if a.source_updated}
              <span class="attach__flag">источник обновился</span>
            {/if}
            {#if unavailable}
              <span class="attach__flag attach__flag--muted">файл недоступен</span>
            {/if}
          </span>
          <input
            class="attach__comment"
            type="text"
            maxlength="500"
            placeholder="зачем этот файл"
            disabled={commentBusy === a.id}
            bind:value={commentDraft[a.id]}
            onblur={() => saveComment(a)}
            onkeydown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault()
                e.currentTarget.blur()
              }
            }}
          />
          <button
            type="button"
            class="attach__act"
            title="Новая версия"
            aria-label="Загрузить новую версию"
            disabled={uploading}
            onclick={() => {
              versionForId = a.id
              versionInput?.click()
            }}
          >
            <Icon name="renew" size={16} />
          </button>
          {#if unavailable}
            <span class="attach__act attach__act--off" title="WebDAV недоступен" aria-label="Скачать (недоступно)">
              <Icon name="download" size={16} />
            </span>
          {:else}
            <a
              class="attach__act"
              href={attachmentDownloadUrl(ownerType, ownerId, a.id)}
              download={a.filename}
              title="Скачать"
              aria-label="Скачать вложение"
            >
              <Icon name="download" size={16} />
            </a>
          {/if}
          <button
            type="button"
            class="attach__act"
            title="Скрыть"
            aria-label="Скрыть вложение"
            onclick={() => hide(a)}
          >
            <Icon name="eye-off" size={16} />
          </button>
          <button
            type="button"
            class="attach__act attach__act--danger"
            title="Удалить"
            aria-label="Удалить вложение"
            onclick={() => (pendingDelete = { att: a, revision: null })}
          >
            <Icon name="delete" size={16} />
          </button>
        </li>
        {#if historyOpenId === a.id}
          <li class="attach__history">
            {#if historyBusy}
              <span class="attach__meta">загрузка…</span>
            {:else}
              <ul class="attach__rev-list">
                {#each historyById[a.id] || [] as rev (rev.revision)}
                  <li class="attach__rev-row" class:attach__rev-row--cur={rev.is_current}>
                    <span class="attach__rev-label">
                      v{rev.revision}
                      {#if rev.is_current} · текущая{/if}
                      · {formatLocal(rev.uploaded_at, { year: '2-digit' })}
                      · {formatSize(rev.size_bytes)}
                      {#if rev.comment}
                        · {rev.comment}
                      {/if}
                    </span>
                    <a
                      class="attach__act"
                      href={attachmentDownloadUrl(ownerType, ownerId, a.id, rev.revision)}
                      download={rev.filename || a.filename}
                      title="Скачать версию"
                    >
                      <Icon name="download" size={14} />
                    </a>
                    {#if !rev.is_current}
                      <button
                        type="button"
                        class="attach__act"
                        title="Сделать текущей"
                        onclick={() => makeCurrent(a, rev.revision)}
                      >
                        <Icon name="arrow-up" size={14} />
                      </button>
                      <button
                        type="button"
                        class="attach__act attach__act--danger"
                        title="Удалить версию"
                        onclick={() => (pendingDelete = { att: a, revision: rev.revision })}
                      >
                        <Icon name="delete" size={14} />
                      </button>
                    {/if}
                  </li>
                {/each}
              </ul>
            {/if}
          </li>
        {/if}
      {/each}
    </ul>
  {/if}

  <div class="attach__drop">
    <button
      type="button"
      class="attach__pick"
      disabled={uploading}
      onclick={() => fileInput?.click()}
    >
      {uploading ? 'Сканирование…' : 'Прикрепить файл'}
    </button>
    <span class="attach__hint">или перетащить сюда</span>
    <input
      bind:this={fileInput}
      class="attach__input"
      type="file"
      multiple
      onchange={onPick}
    />
    <input
      bind:this={versionInput}
      class="attach__input"
      type="file"
      onchange={onVersionPick}
    />
  </div>
</div>

<ConfirmModal
  open={pendingDelete != null}
  title={pendingDelete?.revision == null ? 'Удалить вложение?' : 'Удалить версию?'}
  message={pendingDelete
    ? pendingDelete.revision == null
      ? `Удалить «${pendingDelete.att.filename}» вместе со всеми версиями? Это необратимо. Если файл просто не нужен — лучше скрыть его.`
      : `Удалить версию ${pendingDelete.revision} файла «${pendingDelete.att.filename}»? Это необратимо.`
    : ''}
  busy={deleteBusy}
  onCancel={() => {
    if (!deleteBusy) pendingDelete = null
  }}
  onConfirm={confirmDelete}
/>

<ContextMenu
  open={ctxOpen}
  x={ctxX}
  y={ctxY}
  items={ctxItems}
  onSelect={onAttSelect}
  onClose={() => (ctxOpen = false)}
/>

<style>
  .attach__toggle {
    display: flex;
    width: fit-content;
    align-items: center;
    gap: 0.3rem;
    border: 0;
    background: transparent;
    margin: 0;
    padding: 0;
    line-height: 1;
    cursor: pointer;
    font: inherit;
    font-family: var(--font-ui, sans-serif);
    font-size: 0.68rem;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    font-variant: small-caps;
    font-weight: 500;
    color: var(--color-fg-subtle, #6e6e6e);
  }

  .attach__toggle:hover {
    color: var(--color-fg-muted, #9a9a9a);
  }

  .attach__toggle--static {
    cursor: default;
  }

  .attach__toggle--static:hover {
    color: var(--color-fg-subtle, #6e6e6e);
  }
</style>
