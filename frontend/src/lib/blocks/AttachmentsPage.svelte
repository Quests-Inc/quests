<script>
  import Icon from '../ui/Icon.svelte'
  import ContextMenu from '../ui/ContextMenu.svelte'
  import ConfirmModal from '../modals/ConfirmModal.svelte'
  import {
    attachmentDownloadUrl,
    deleteAttachment,
    deleteAttachmentRevision,
    listAttachmentRevisions,
    setAttachmentCurrentRevision,
    setAttachmentHidden,
    updateAttachmentComment,
    uploadAttachmentRevision,
  } from '../js/api.js'
  import { copyText } from '../js/clipboard.js'
  import { toast } from '../js/toasts.svelte.js'

  /** @type {{
   *   attachments?: any[],
   *   quests?: any[],
   *   questlines?: any[],
   *   notes?: any[],
   *   selectedId?: number | null,
   *   onSelect?: (id: number | null) => void,
   *   onOpenOwner?: (kind: string, id: number) => void,
   *   onChanged?: () => void,
   * }} */
  let {
    attachments = [],
    quests = [],
    questlines = [],
    notes = [],
    selectedId = null,
    onSelect,
    onOpenOwner,
    onChanged,
  } = $props()

  const KINDS = [
    { id: 'all', label: 'Все' },
    { id: 'quest', label: 'Квесты' },
    { id: 'questline', label: 'Квестлайны' },
    { id: 'note', label: 'Заметки' },
  ]
  const KIND_ONE = { quest: 'Квест', questline: 'Квестлайн', note: 'Заметка' }
  const SORTS = [
    { id: 'new', label: 'Сначала новые' },
    { id: 'old', label: 'Сначала старые' },
    { id: 'name', label: 'По имени' },
    { id: 'size', label: 'По размеру' },
  ]
  const STATES = [
    { id: 'active', label: 'Активные' },
    { id: 'hidden', label: 'Скрытые' },
    { id: 'all', label: 'Все' },
  ]

  let search = $state('')
  let kindFilter = $state('all')
  let stateFilter = $state('active')
  let sort = $state('new')
  let grouped = $state(true)
  let busyId = $state(/** @type {number | null} */ (null))
  let commentDraft = $state('')
  let commentFor = $state(/** @type {number | null} */ (null))
  let revs = $state(/** @type {any[]} */ ([]))
  let revsFor = $state(/** @type {number | null} */ (null))
  let revsBusy = $state(false)
  /** @type {{ att: any, revision: number | null } | null} */
  let pendingDelete = $state(null)
  let deleteBusy = $state(false)
  let versionInput = $state(/** @type {HTMLInputElement | null} */ (null))
  let versionFor = $state(/** @type {any | null} */ (null))

  let ctxOpen = $state(false)
  let ctxX = $state(0)
  let ctxY = $state(0)
  let ctxAtt = $state(/** @type {any | null} */ (null))

  /** @param {any} a */
  const ownerKind = (a) => a.owner_type || 'quest'

  let titles = $derived.by(() => {
    /** @type {Record<string, string>} */
    const m = {}
    for (const q of quests) m[`quest:${q.id}`] = q.title || `quest=${q.id}`
    for (const l of questlines) m[`questline:${l.id}`] = l.title || `questline=${l.id}`
    for (const n of notes) m[`note:${n.id}`] = n.title || `note=${n.id}`
    return m
  })

  /** @param {any} a */
  function ownerLabel(a) {
    const kind = ownerKind(a)
    return titles[`${kind}:${a.owner_id}`] || `${kind}=${a.owner_id}`
  }

  /** @param {any} a */
  function isRaster(a) {
    const t = (a.content_type_detected || '').split(';')[0].trim().toLowerCase()
    return t === 'image/png' || t === 'image/jpeg' || t === 'image/gif' || t === 'image/webp'
  }

  /** @param {any} a */
  function thumbSrc(a) {
    return `/api/attachments/${a.id}/image?preview=1&revision=${a.revision || 1}`
  }

  /** @param {any} a */
  function extOf(a) {
    const m = /\.([A-Za-z0-9]{1,6})$/.exec(a.filename || '')
    return m ? m[1].toUpperCase() : ''
  }

  let hiddenCount = $derived(attachments.filter((a) => a.hidden).length)
  let activeCount = $derived(attachments.length - hiddenCount)

  let filtered = $derived.by(() => {
    const q = search.trim().toLowerCase()
    const out = attachments.filter((a) => {
      if (kindFilter !== 'all' && ownerKind(a) !== kindFilter) return false
      if (stateFilter === 'active' && a.hidden) return false
      if (stateFilter === 'hidden' && !a.hidden) return false
      if (!q) return true
      return `${a.filename || ''} ${a.comment || ''} ${ownerLabel(a)} ${a.content_type_detected || ''}`
        .toLowerCase()
        .includes(q)
    })
    const byWhen = (x, y) => (y.uploaded_at || '').localeCompare(x.uploaded_at || '') || y.id - x.id
    const cmp = {
      new: byWhen,
      old: (x, y) => -byWhen(x, y),
      name: (x, y) => (x.filename || '').localeCompare(y.filename || '', 'ru'),
      size: (x, y) => (y.size_bytes || 0) - (x.size_bytes || 0),
    }[sort]
    return out.sort(cmp)
  })

  let groups = $derived.by(() => {
    if (!grouped) return []
    /** @type {Map<string, { key: string, kind: string, id: number, title: string, files: any[] }>} */
    const m = new Map()
    for (const a of filtered) {
      const kind = ownerKind(a)
      const key = `${kind}:${a.owner_id}`
      let g = m.get(key)
      if (!g) {
        g = { key, kind, id: Number(a.owner_id), title: ownerLabel(a), files: [] }
        m.set(key, g)
      }
      g.files.push(a)
    }
    return [...m.values()].sort((x, y) => x.title.localeCompare(y.title, 'ru'))
  })

  let selected = $derived(attachments.find((a) => a.id === selectedId) ?? null)

  // Comment draft follows the selection; revisions load lazily per selection.
  $effect(() => {
    const a = selected
    if (!a) {
      commentFor = null
      revsFor = null
      revs = []
      return
    }
    if (commentFor !== a.id) {
      commentFor = a.id
      commentDraft = a.comment || ''
    }
    if (revsFor !== a.id) {
      revsFor = a.id
      loadRevs(a)
    }
  })

  $effect(() => {
    const id = selectedId
    filtered
    if (id == null || typeof document === 'undefined') return
    queueMicrotask(() => {
      document.querySelector(`[data-att-id="${id}"]`)?.scrollIntoView({ block: 'nearest' })
    })
  })

  /** @param {any} a */
  async function loadRevs(a) {
    revsBusy = true
    try {
      const data = await listAttachmentRevisions(ownerKind(a), a.owner_id, a.id)
      if (revsFor === a.id) revs = (data?.revisions || []).slice().reverse()
    } catch (e) {
      toast(e?.message || String(e), { kind: 'error' })
    } finally {
      revsBusy = false
    }
  }

  /** Run a mutation, then refresh the index (and the open card's versions). */
  async function run(a, fn, okMessage) {
    busyId = a.id
    try {
      await fn()
      if (okMessage) toast(okMessage, { kind: 'success', ttl: 1800 })
      onChanged?.()
      if (selectedId === a.id) await loadRevs(a)
    } catch (e) {
      toast(e?.message || String(e), { kind: 'error' })
    } finally {
      busyId = null
    }
  }

  /** @param {any} a */
  function toggleHidden(a) {
    return run(
      a,
      () => setAttachmentHidden(ownerKind(a), a.owner_id, a.id, !a.hidden),
      a.hidden ? 'Вложение возвращено' : 'Вложение скрыто',
    )
  }

  /** @param {any} a @param {number} revision */
  function makeCurrent(a, revision) {
    return run(
      a,
      () => setAttachmentCurrentRevision(ownerKind(a), a.owner_id, a.id, revision),
      `Версия ${revision} — текущая`,
    )
  }

  /** @param {any} a */
  function saveComment(a) {
    const next = commentDraft.trim()
    if (next === (a.comment || '')) return
    return run(a, () => updateAttachmentComment(ownerKind(a), a.owner_id, a.id, next))
  }

  async function confirmDelete() {
    const p = pendingDelete
    if (!p) return
    deleteBusy = true
    try {
      const { att, revision } = p
      const kind = ownerKind(att)
      if (revision == null) {
        await deleteAttachment(kind, att.owner_id, att.id)
        if (selectedId === att.id) onSelect?.(null)
        toast('Вложение удалено', { kind: 'success', ttl: 1800 })
      } else {
        await deleteAttachmentRevision(kind, att.owner_id, att.id, revision)
        toast(`Версия ${revision} удалена`, { kind: 'success', ttl: 1800 })
        if (selectedId === att.id) await loadRevs(att)
      }
      onChanged?.()
    } catch (e) {
      toast(e?.message || String(e), { kind: 'error' })
    } finally {
      deleteBusy = false
      pendingDelete = null
    }
  }

  /** @param {any} a */
  function pickVersion(a) {
    versionFor = a
    versionInput?.click()
  }

  async function onVersionPick(event) {
    const file = event.currentTarget.files?.[0]
    const a = versionFor
    versionFor = null
    event.currentTarget.value = ''
    if (!file || !a) return
    await run(a, () => uploadAttachmentRevision(ownerKind(a), a.owner_id, a.id, file), 'Новая версия загружена')
  }

  /** @param {any} a @param {number | null} [revision] */
  function downloadAtt(a, revision = null) {
    const link = document.createElement('a')
    link.href = attachmentDownloadUrl(ownerKind(a), a.owner_id, a.id, revision)
    link.download = a.filename || ''
    link.rel = 'noopener'
    document.body.appendChild(link)
    link.click()
    link.remove()
  }

  /** @param {any} a */
  async function copyRef(a) {
    try {
      const text = `attachment=${a.id}`
      await copyText(text)
      toast(`Скопировано ${text}`, { kind: 'success', ttl: 1600 })
    } catch (e) {
      toast(e?.message || String(e), { kind: 'error' })
    }
  }

  let ctxItems = $derived.by(() => {
    const a = ctxAtt
    if (!a) return []
    return [
      { id: 'download', label: 'Скачать' },
      { id: 'new-version', label: 'Новая версия…' },
      { id: 'hide', label: a.hidden ? 'Вернуть' : 'Скрыть' },
      { id: 'open', label: 'Открыть владельца' },
      { id: 'copy-id', label: `Копировать attachment=${a.id}` },
      { id: 'sep-danger', sep: true },
      { id: 'delete', label: 'Удалить…', danger: true },
    ]
  })

  /** @param {MouseEvent} event @param {any} a */
  function openAttMenu(event, a) {
    const target = event.target
    if (target instanceof Element && target.closest('a, button, input, textarea')) return
    event.preventDefault()
    event.stopPropagation()
    ctxAtt = a
    ctxX = event.clientX
    ctxY = event.clientY
    ctxOpen = true
  }

  /** @param {string} action */
  function onAttSelect(action) {
    const a = ctxAtt
    if (!a) return
    if (action === 'download') downloadAtt(a)
    else if (action === 'new-version') pickVersion(a)
    else if (action === 'hide') toggleHidden(a)
    else if (action === 'open') onOpenOwner?.(ownerKind(a), Number(a.owner_id))
    else if (action === 'copy-id') copyRef(a)
    else if (action === 'delete') pendingDelete = { att: a, revision: null }
  }

  /** @param {number} n */
  function formatSize(n) {
    const bytes = Number(n) || 0
    if (bytes < 1024) return `${bytes} B`
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
  }

  /** @param {string} iso @param {boolean} [full] */
  function formatWhen(iso, full = false) {
    if (!iso) return '—'
    try {
      return new Date(iso).toLocaleString(
        'ru-RU',
        full
          ? { day: '2-digit', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' }
          : { day: '2-digit', month: 'short', year: '2-digit' },
      )
    } catch {
      return iso
    }
  }

  /** @param {KeyboardEvent} e */
  function onKey(e) {
    if (e.key === 'Escape' && selectedId != null && !pendingDelete) {
      const t = e.target
      if (t instanceof Element && t.closest('input, textarea')) return
      onSelect?.(null)
    }
  }
</script>

<svelte:window onkeydown={onKey} />

<div class="attpage" class:attpage--detail={selected != null}>
  <div class="attpage__tools">
    <div class="attpage__row1">
      <input
        class="search attpage__search"
        type="search"
        placeholder="Поиск по имени, комментарию, владельцу…"
        bind:value={search}
        aria-label="Поиск вложений"
      />
      <select class="attpage__select" bind:value={sort} aria-label="Сортировка">
        {#each SORTS as s (s.id)}
          <option value={s.id}>{s.label}</option>
        {/each}
      </select>
      <button
        type="button"
        class="attpage__tool"
        class:attpage__tool--on={grouped}
        title="Группировать по владельцу"
        aria-pressed={grouped}
        onclick={() => (grouped = !grouped)}
      >
        <Icon name="folder" size={16} />
      </button>
    </div>
    <div class="attpage__row2">
      <div class="attpage__seg" role="group" aria-label="Тип владельца">
        {#each KINDS as k (k.id)}
          <button
            type="button"
            class="attpage__pill"
            class:attpage__pill--on={kindFilter === k.id}
            onclick={() => (kindFilter = k.id)}>{k.label}</button
          >
        {/each}
      </div>
      <div class="attpage__seg" role="group" aria-label="Состояние">
        {#each STATES as s (s.id)}
          <button
            type="button"
            class="attpage__pill"
            class:attpage__pill--on={stateFilter === s.id}
            onclick={() => (stateFilter = s.id)}
          >
            {#if s.id === 'hidden'}<Icon name="eye-off" size={12} />{/if}
            {s.label}
            <span class="attpage__n"
              >{s.id === 'active' ? activeCount : s.id === 'hidden' ? hiddenCount : attachments.length}</span
            >
          </button>
        {/each}
      </div>
    </div>
  </div>

  <div class="attpage__split">
    <div class="attpage__list">
      {#if attachments.length === 0}
        <p class="empty">Вложений пока нет — их вешают на квесты, квестлайны и заметки.</p>
      {:else if filtered.length === 0}
        <p class="empty">Ничего не найдено</p>
      {:else if grouped}
        {#each groups as g (g.key)}
          <section class="attpage__group">
            <button
              type="button"
              class="attpage__ghead"
              title="Открыть владельца"
              onclick={() => onOpenOwner?.(g.kind, g.id)}
            >
              <span class="attpage__kind">{KIND_ONE[g.kind]}</span>
              <span class="attpage__gtitle">{g.title}</span>
              <span class="attpage__n">{g.files.length}</span>
            </button>
            {#each g.files as a (a.id)}
              {@render row(a, false)}
            {/each}
          </section>
        {/each}
      {:else}
        {#each filtered as a (a.id)}
          {@render row(a, true)}
        {/each}
      {/if}
    </div>

    {#if selected}
      {@const a = selected}
      <aside class="attpage__panel" aria-label="Вложение">
        <div class="attpage__phead">
          <button type="button" class="attpage__back" onclick={() => onSelect?.(null)} aria-label="Закрыть">
            <Icon name="close" size={16} />
          </button>
          <span class="attpage__pref">attachment={a.id}</span>
        </div>

        <div class="attpage__preview" class:attpage__preview--hidden={a.hidden}>
          {#if isRaster(a)}
            <img src={thumbSrc(a)} alt={a.filename} loading="lazy" />
          {:else}
            <span class="attpage__bigext">{extOf(a) || 'FILE'}</span>
          {/if}
          {#if a.hidden}
            <span class="attpage__hiddenbadge"><Icon name="eye-off" size={14} /> скрыто</span>
          {/if}
        </div>

        <h2 class="attpage__ptitle">{a.filename || `attachment=${a.id}`}</h2>

        <div class="attpage__pactions">
          <button type="button" class="attpage__btn attpage__btn--main" onclick={() => downloadAtt(a)}>
            <Icon name="download" size={14} /> Скачать
          </button>
          <button type="button" class="attpage__btn" disabled={busyId === a.id} onclick={() => toggleHidden(a)}>
            <Icon name={a.hidden ? 'eye' : 'eye-off'} size={14} />
            {a.hidden ? 'Вернуть' : 'Скрыть'}
          </button>
          <button type="button" class="attpage__btn" disabled={busyId === a.id} onclick={() => pickVersion(a)}>
            <Icon name="renew" size={14} /> Новая версия
          </button>
          <button type="button" class="attpage__btn attpage__btn--danger" onclick={() => (pendingDelete = { att: a, revision: null })}>
            <Icon name="delete" size={14} /> Удалить
          </button>
        </div>

        <dl class="attpage__facts">
          <dt>Владелец</dt>
          <dd>
            <button type="button" class="attpage__link" onclick={() => onOpenOwner?.(ownerKind(a), Number(a.owner_id))}>
              {KIND_ONE[ownerKind(a)]} · {ownerLabel(a)}
            </button>
          </dd>
          <dt>Размер</dt>
          <dd>{formatSize(a.size_bytes)}</dd>
          <dt>Тип</dt>
          <dd class="attpage__mono">{a.content_type_detected || a.content_type_declared || '—'}</dd>
          <dt>Загружено</dt>
          <dd>{formatWhen(a.uploaded_at, true)}</dd>
          <dt>Проверка</dt>
          <dd>{a.scan_status || '—'}</dd>
        </dl>

        <label class="attpage__lbl" for="att-comment">Комментарий</label>
        <textarea
          id="att-comment"
          class="attpage__comment"
          rows="2"
          maxlength="500"
          placeholder="зачем этот файл"
          bind:value={commentDraft}
          onblur={() => saveComment(a)}
        ></textarea>

        <div class="attpage__lbl">Версии ({revs.length || a.revision_count || 1})</div>
        {#if revsBusy && revs.length === 0}
          <p class="attpage__muted">загрузка…</p>
        {:else}
          <ul class="attpage__revs">
            {#each revs as r (r.revision)}
              <li class="attpage__rev" class:attpage__rev--cur={r.is_current}>
                <span class="attpage__rev-v">v{r.revision}</span>
                <span class="attpage__rev-info">
                  {formatWhen(r.uploaded_at)} · {formatSize(r.size_bytes)}
                  {#if r.is_current}<b>· текущая</b>{/if}
                </span>
                <button type="button" class="attpage__ico" title="Скачать версию" onclick={() => downloadAtt(a, r.revision)}>
                  <Icon name="download" size={14} />
                </button>
                {#if !r.is_current}
                  <button
                    type="button"
                    class="attpage__ico"
                    title="Сделать текущей"
                    disabled={busyId === a.id}
                    onclick={() => makeCurrent(a, r.revision)}
                  >
                    <Icon name="arrow-up" size={14} />
                  </button>
                  <button
                    type="button"
                    class="attpage__ico attpage__ico--danger"
                    title="Удалить версию"
                    onclick={() => (pendingDelete = { att: a, revision: r.revision })}
                  >
                    <Icon name="delete" size={14} />
                  </button>
                {/if}
              </li>
            {/each}
          </ul>
        {/if}

        <button type="button" class="attpage__link attpage__copy" onclick={() => copyRef(a)}>
          <Icon name="copy" size={12} /> Копировать attachment={a.id}
        </button>
      </aside>
    {/if}
  </div>
</div>

{#snippet row(a, showOwner)}
  <article
    class="attpage__item"
    class:attpage__item--on={a.id === selectedId}
    class:attpage__item--hidden={a.hidden}
    data-att-id={a.id}
    role="listitem"
    onclick={() => onSelect?.(a.id)}
    oncontextmenu={(e) => openAttMenu(e, a)}
  >
    <div class="attpage__thumb">
      {#if isRaster(a)}
        <img src={thumbSrc(a)} alt="" loading="lazy" />
      {:else}
        <span class="attpage__ext">{extOf(a) || '···'}</span>
      {/if}
    </div>
    <div class="attpage__main">
      <div class="attpage__name">
        <span class="attpage__file">{a.filename || `attachment=${a.id}`}</span>
        {#if a.hidden}
          <span class="attpage__chip attpage__chip--hidden" title="Скрыто"><Icon name="eye-off" size={11} /></span>
        {/if}
        {#if (a.revision_count || 1) > 1}
          <span class="attpage__chip" title="Версия {a.revision} из {a.revision_count}">v{a.revision}/{a.revision_count}</span>
        {/if}
        {#if a.available === false}<span class="attpage__chip attpage__chip--warn">DAV offline</span>{/if}
        {#if a.source_updated}<span class="attpage__chip attpage__chip--fresh">файл новее</span>{/if}
        {#if a.scan_status && a.scan_status !== 'clean'}
          <span class="attpage__chip attpage__chip--warn">{a.scan_status}</span>
        {/if}
      </div>
      {#if a.comment}<p class="attpage__cmt">{a.comment}</p>{/if}
      <div class="attpage__meta">
        {#if showOwner}
          <span class="attpage__owner">{KIND_ONE[ownerKind(a)]} · {ownerLabel(a)}</span>
        {/if}
        <span>{formatSize(a.size_bytes)}</span>
        <span>{formatWhen(a.uploaded_at)}</span>
      </div>
    </div>
    <div class="attpage__acts">
      <a
        class="attpage__ico"
        href={attachmentDownloadUrl(ownerKind(a), a.owner_id, a.id)}
        download={a.filename}
        title="Скачать"
        aria-label="Скачать"
        onclick={(e) => e.stopPropagation()}
      >
        <Icon name="download" size={16} />
      </a>
      <button
        type="button"
        class="attpage__ico"
        title={a.hidden ? 'Вернуть' : 'Скрыть'}
        aria-label={a.hidden ? 'Вернуть' : 'Скрыть'}
        disabled={busyId === a.id}
        onclick={(e) => {
          e.stopPropagation()
          toggleHidden(a)
        }}
      >
        <Icon name={a.hidden ? 'eye' : 'eye-off'} size={16} />
      </button>
      <button
        type="button"
        class="attpage__ico attpage__ico--danger"
        title="Удалить"
        aria-label="Удалить"
        onclick={(e) => {
          e.stopPropagation()
          pendingDelete = { att: a, revision: null }
        }}
      >
        <Icon name="delete" size={16} />
      </button>
    </div>
  </article>
{/snippet}

<input bind:this={versionInput} class="attpage__file-input" type="file" onchange={onVersionPick} />

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
  .attpage {
    display: flex;
    flex-direction: column;
    min-height: 0;
    height: 100%;
  }

  .attpage__tools {
    display: grid;
    gap: 0.55rem;
    padding: 0.75rem 1rem;
    border-bottom: 1px solid var(--color-border, #333);
  }

  .attpage__row1,
  .attpage__row2 {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    flex-wrap: wrap;
  }

  .attpage__search {
    flex: 1 1 14rem;
    min-width: 0;
  }

  .attpage__select {
    flex: 0 0 auto;
    padding: 0.35rem 0.5rem;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-md, 4px);
    background: transparent;
    color: inherit;
    font: inherit;
    font-size: var(--text-sm, 0.875rem);
  }

  .attpage__select option {
    background: var(--color-bg-raised, #1a1a1a);
  }

  .attpage__tool,
  .attpage__pill,
  .attpage__btn,
  .attpage__ico,
  .attpage__back,
  .attpage__link,
  .attpage__ghead {
    font: inherit;
    color: inherit;
    cursor: pointer;
  }

  .attpage__tool {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 2rem;
    height: 2rem;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-md, 4px);
    background: transparent;
    color: var(--color-fg-muted, #9a9a9a);
  }

  .attpage__tool--on {
    color: var(--color-accent, #c9a227);
    border-color: var(--color-accent, #c9a227);
  }

  .attpage__seg {
    display: inline-flex;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-md, 4px);
    overflow: hidden;
  }

  .attpage__pill {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    padding: 0.28rem 0.65rem;
    border: 0;
    border-right: 1px solid var(--color-border, #333);
    background: transparent;
    font-size: var(--text-xs, 0.75rem);
    color: var(--color-fg-muted, #9a9a9a);
  }

  .attpage__pill:last-child {
    border-right: 0;
  }

  .attpage__pill:hover {
    color: var(--color-fg, #e8e8e8);
  }

  .attpage__pill--on {
    color: var(--color-fg, #e8e8e8);
    background: color-mix(in srgb, var(--color-accent, #c9a227) 18%, transparent);
  }

  .attpage__n {
    font-variant-numeric: tabular-nums;
    opacity: 0.7;
    font-size: var(--text-xs, 0.75rem);
  }

  .attpage__split {
    flex: 1 1 auto;
    min-height: 0;
    display: grid;
    grid-template-columns: minmax(0, 1fr);
  }

  .attpage--detail .attpage__split {
    grid-template-columns: minmax(0, 1fr) minmax(18rem, 25rem);
  }

  .attpage__list {
    overflow: auto;
    padding: 0.35rem 0 1rem;
    min-height: 0;
  }

  .empty {
    margin: 1.5rem 1rem;
    color: var(--color-fg-muted, #9a9a9a);
  }

  .attpage__group {
    margin-bottom: 0.5rem;
  }

  .attpage__ghead {
    display: flex;
    align-items: baseline;
    gap: 0.5rem;
    width: 100%;
    padding: 0.5rem 1rem 0.3rem;
    border: 0;
    background: transparent;
    text-align: left;
  }

  .attpage__ghead:hover .attpage__gtitle {
    color: var(--color-accent, #c9a227);
  }

  .attpage__kind {
    flex: 0 0 auto;
    font-size: 0.65rem;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--color-fg-muted, #9a9a9a);
  }

  .attpage__gtitle {
    min-width: 0;
    flex: 1 1 auto;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: var(--font-body, Georgia, serif);
    font-size: var(--text-sm, 0.875rem);
  }

  .attpage__item {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 0.5rem 1rem;
    border-left: 2px solid transparent;
    cursor: pointer;
  }

  .attpage__item:hover {
    background: color-mix(in srgb, var(--color-fg, #e8e8e8) 4%, transparent);
  }

  .attpage__item--on {
    background: color-mix(in srgb, var(--color-fg, #e8e8e8) 6%, var(--color-bg-raised, #1a1a1a));
    border-left-color: var(--color-accent, #c9a227);
  }

  .attpage__item--hidden .attpage__main,
  .attpage__item--hidden .attpage__thumb {
    opacity: 0.5;
  }

  .attpage__thumb {
    flex: 0 0 2.75rem;
    width: 2.75rem;
    height: 2.75rem;
    display: grid;
    place-items: center;
    overflow: hidden;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-md, 4px);
    background: color-mix(in srgb, var(--color-fg, #e8e8e8) 5%, transparent);
  }

  .attpage__thumb img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }

  .attpage__ext {
    font-family: var(--font-mono, monospace);
    font-size: 0.65rem;
    color: var(--color-fg-muted, #9a9a9a);
  }

  .attpage__main {
    flex: 1 1 auto;
    min-width: 0;
    display: grid;
    gap: 0.15rem;
  }

  .attpage__name {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    min-width: 0;
  }

  .attpage__file {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--text-sm, 0.875rem);
    font-weight: 600;
  }

  .attpage__chip {
    flex: 0 0 auto;
    display: inline-flex;
    align-items: center;
    padding: 0 0.3rem;
    border: 1px solid var(--color-border, #3a3a3a);
    border-radius: var(--radius-sm, 3px);
    font-family: var(--font-mono, monospace);
    font-size: 0.65rem;
    color: var(--color-fg-muted, #9a9a9a);
  }

  .attpage__chip--warn {
    color: var(--color-danger, #b54a3a);
    border-color: currentColor;
  }

  .attpage__chip--fresh {
    color: var(--color-accent, #c9a227);
    border-color: currentColor;
  }

  .attpage__cmt {
    margin: 0;
    font-size: var(--text-xs, 0.75rem);
    color: color-mix(in srgb, var(--color-fg, #e8e8e8) 75%, transparent);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .attpage__meta {
    display: flex;
    flex-wrap: wrap;
    gap: 0.1rem 0.7rem;
    font-size: var(--text-xs, 0.75rem);
    color: var(--color-fg-muted, #9a9a9a);
  }

  .attpage__owner {
    max-width: 18rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .attpage__acts {
    flex: 0 0 auto;
    display: flex;
    opacity: 0.35;
    transition: opacity 0.12s;
  }

  .attpage__item:hover .attpage__acts,
  .attpage__item--on .attpage__acts,
  .attpage__item:focus-within .attpage__acts {
    opacity: 1;
  }

  @media (hover: none) {
    .attpage__acts {
      opacity: 1;
    }
  }

  .attpage__ico {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 1.9rem;
    height: 1.9rem;
    padding: 0;
    border: 0;
    border-radius: var(--radius-md, 4px);
    background: transparent;
    color: var(--color-fg-muted, #9a9a9a);
    text-decoration: none;
  }

  .attpage__ico:hover:not(:disabled) {
    color: var(--color-fg, #e8e8e8);
    background: color-mix(in srgb, var(--color-fg, #e8e8e8) 8%, transparent);
  }

  .attpage__ico--danger:hover:not(:disabled) {
    color: var(--color-danger, #b54a3a);
    background: color-mix(in srgb, var(--color-danger, #b54a3a) 14%, transparent);
  }

  .attpage__ico:disabled {
    opacity: 0.4;
    cursor: default;
  }

  /* Detail panel */
  .attpage__panel {
    overflow: auto;
    min-height: 0;
    padding: 0.75rem 1rem 1.5rem;
    border-left: 1px solid var(--color-border, #333);
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  .attpage__phead {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
  }

  .attpage__pref {
    font-family: var(--font-mono, monospace);
    font-size: var(--text-xs, 0.75rem);
    color: var(--color-fg-muted, #9a9a9a);
  }

  .attpage__back {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 1.9rem;
    height: 1.9rem;
    border: 0;
    border-radius: var(--radius-md, 4px);
    background: transparent;
    color: var(--color-fg-muted, #9a9a9a);
  }

  .attpage__back:hover {
    color: var(--color-fg, #e8e8e8);
    background: color-mix(in srgb, var(--color-fg, #e8e8e8) 8%, transparent);
  }

  .attpage__preview {
    position: relative;
    display: grid;
    place-items: center;
    min-height: 7rem;
    max-height: 16rem;
    overflow: hidden;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-md, 4px);
    background: color-mix(in srgb, var(--color-fg, #e8e8e8) 5%, transparent);
  }

  .attpage__preview img {
    max-width: 100%;
    max-height: 16rem;
    object-fit: contain;
  }

  .attpage__preview--hidden img {
    opacity: 0.55;
  }

  .attpage__bigext {
    font-family: var(--font-mono, monospace);
    font-size: 1.6rem;
    color: var(--color-fg-muted, #9a9a9a);
  }

  .attpage__hiddenbadge {
    position: absolute;
    top: 0.4rem;
    left: 0.4rem;
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    padding: 0.1rem 0.45rem;
    border-radius: var(--radius-sm, 3px);
    background: var(--color-bg-raised, #1a1a1a);
    border: 1px solid var(--color-border, #333);
    font-size: var(--text-xs, 0.75rem);
    color: var(--color-fg-muted, #9a9a9a);
  }

  .attpage__ptitle {
    margin: 0;
    font-size: var(--text-md, 1rem);
    font-weight: 600;
    word-break: break-word;
  }

  .attpage__pactions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem;
  }

  .attpage__btn {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    padding: 0.3rem 0.65rem;
    border: 1px solid var(--color-border, #3a3a3a);
    border-radius: var(--radius-md, 4px);
    background: transparent;
    font-size: var(--text-sm, 0.875rem);
  }

  .attpage__btn:hover:not(:disabled) {
    background: color-mix(in srgb, var(--color-fg, #e8e8e8) 8%, transparent);
  }

  .attpage__btn:disabled {
    opacity: 0.5;
    cursor: default;
  }

  .attpage__btn--main {
    color: var(--color-accent, #c9a227);
    border-color: var(--color-accent, #c9a227);
  }

  .attpage__btn--danger:hover:not(:disabled) {
    color: var(--color-danger, #b54a3a);
    border-color: var(--color-danger, #b54a3a);
    background: color-mix(in srgb, var(--color-danger, #b54a3a) 12%, transparent);
  }

  .attpage__facts {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr);
    gap: 0.25rem 0.8rem;
    margin: 0.2rem 0;
    font-size: var(--text-sm, 0.875rem);
  }

  .attpage__facts dt {
    color: var(--color-fg-muted, #9a9a9a);
  }

  .attpage__facts dd {
    margin: 0;
    min-width: 0;
    overflow-wrap: anywhere;
  }

  .attpage__mono {
    font-family: var(--font-mono, monospace);
    font-size: var(--text-xs, 0.75rem);
  }

  .attpage__link {
    padding: 0;
    border: 0;
    background: none;
    text-align: left;
    color: var(--color-accent, #c9a227);
    font-size: var(--text-sm, 0.875rem);
  }

  .attpage__link:hover {
    text-decoration: underline;
  }

  .attpage__copy {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    margin-top: 0.4rem;
    font-size: var(--text-xs, 0.75rem);
    color: var(--color-fg-muted, #9a9a9a);
  }

  .attpage__lbl {
    margin-top: 0.3rem;
    font-size: 0.68rem;
    letter-spacing: 0.1em;
    text-transform: uppercase;
    color: var(--color-fg-subtle, #6e6e6e);
  }

  .attpage__comment {
    width: 100%;
    resize: vertical;
    padding: 0.35rem 0.5rem;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-md, 4px);
    background: transparent;
    color: inherit;
    font: inherit;
    font-size: var(--text-sm, 0.875rem);
  }

  .attpage__muted {
    margin: 0;
    font-size: var(--text-xs, 0.75rem);
    color: var(--color-fg-muted, #9a9a9a);
  }

  .attpage__revs {
    margin: 0;
    padding: 0;
    list-style: none;
    display: grid;
    gap: 0.15rem;
  }

  .attpage__rev {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.1rem 0.3rem;
    border-radius: var(--radius-sm, 3px);
    font-size: var(--text-xs, 0.75rem);
    color: var(--color-fg-muted, #9a9a9a);
  }

  .attpage__rev--cur {
    color: var(--color-fg, #e8e8e8);
    background: color-mix(in srgb, var(--color-accent, #c9a227) 10%, transparent);
  }

  .attpage__rev-v {
    flex: 0 0 auto;
    font-family: var(--font-mono, monospace);
  }

  .attpage__rev-info {
    flex: 1 1 auto;
    min-width: 0;
  }

  .attpage__rev .attpage__ico {
    width: 1.6rem;
    height: 1.6rem;
  }

  .attpage__file-input {
    display: none;
  }

  /* Narrow screens: the panel takes over the whole page, ✕ returns to the list. */
  @media (max-width: 860px) {
    .attpage--detail .attpage__split {
      grid-template-columns: minmax(0, 1fr);
    }

    .attpage--detail .attpage__list {
      display: none;
    }

    .attpage__panel {
      border-left: 0;
    }

    .attpage__item {
      padding-inline: 0.75rem;
    }
  }
</style>
