<script lang="ts">
  import './attachmentCards.css'
  import { parseQuery, passes, tokenValues, setToken, hasToken, helpLine } from '@/shared/lib/searchFlags.ts'
  import FlagHints from '@/shared/ui/FlagHints.svelte'
  import Icon from '@/shared/ui/Icon.svelte'
  import AttachmentGroupHead from './AttachmentGroupHead.svelte'
  import { createFoldedGroups } from '../model/foldedGroups.svelte.ts'
  import FileKindIcon from '@/shared/ui/FileKindIcon.svelte'
  import { fileKind } from '@/shared/lib/fileKind.ts'
  import AttachmentPanel from './AttachmentPanel.svelte'
  import ContextMenu from '@/shared/ui/ContextMenu.svelte'
  import ConfirmModal from '@/shared/ui/ConfirmModal.svelte'
  import { AttachmentPreview, ATT_SPEC, attachmentCatalog, extOf, flattenAttachmentIndex, formatSize, formatWhen, groupByOwner, isRaster, ownerKind, sortAttachments, thumbSrc } from '@/entities/attachment'
  import { noteCatalog } from '@/entities/note'
  import { questCatalog } from '@/entities/quest'
  import { EntityHistory } from '@/entities/history'
  import {
    attachmentDownloadUrl,
    deleteAttachment,
    deleteAttachmentRevision,
    listAttachmentRevisions,
    setAttachmentCurrentRevision,
    setAttachmentHidden,
    updateAttachmentComment,
    uploadAttachmentRevision,
  } from '@/shared/api/client.ts'
  import { createAttachmentOps } from '../model/attachmentOps.svelte.ts'
  import { createAttachmentGroup } from '../model/attachmentGroup.svelte.ts'
  import SidebarPicked from '@/shared/ui/SidebarPicked.svelte'
  import { copyText } from '@/shared/lib/clipboard.ts'
  import { toast } from '@/shared/lib/toasts.svelte.ts'

  let {
    questlines = [],
    selectedId = null,
    onSelect,
    onOpenOwner,
    onOpenGraph,
    onChanged,
  }: {
    questlines?: any[],
    selectedId?: number | null,
    onSelect?: (id: number | null) => void,
    onOpenOwner?: (kind: string, id: number) => void,
    onOpenGraph?: (type: 'quest' | 'questline' | 'note', id: number) => void,
    onChanged?: () => void,
  } = $props()

  let attachments = $derived(flattenAttachmentIndex(attachmentCatalog.index))
  let quests = $derived(questCatalog.list)
  let notes = $derived(noteCatalog.list)

  const KINDS = [
    { id: 'all', label: 'Все' },
    { id: 'quest', label: 'Квесты' },
    { id: 'questline', label: 'Квестлайны' },
    { id: 'note', label: 'Заметки' },
  ]
  const KIND_ONE: Record<string, string> = { quest: 'Квест', questline: 'Квестлайн', note: 'Заметка' }
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
  let sort = $state('new')
  const VIEWS = [
    { id: 'list', label: 'Список' },
    { id: 'grouped', label: 'По владельцу' },
    { id: 'cards', label: 'Карточки' },
  ] as const
  let view = $state<'list' | 'grouped' | 'cards'>('grouped')
  let grouped = $derived(view === 'grouped')
  let versionInput = $state((null as HTMLInputElement | null))
  const foldedGroups = createFoldedGroups()

  let ctxOpen = $state(false)
  let ctxX = $state(0)
  let ctxY = $state(0)
  let ctxAtt = $state((null as any | null))
  let titles = $derived.by(() => {
    const m: Record<string, string> = {}
    for (const q of quests) m[`quest:${q.id}`] = q.title || `quest=${q.id}`
    for (const l of questlines) m[`questline:${l.id}`] = l.title || `questline=${l.id}`
    for (const n of notes) m[`note:${n.id}`] = n.title || `note=${n.id}`
    return m
  })

  function ownerLabel(a: any) {
    const kind = ownerKind(a)
    return titles[`${kind}:${a.owner_id}`] || `${kind}=${a.owner_id}`
  }

  let parsedSearch = $derived(parseQuery(search, ATT_SPEC))
  let kindFilter = $derived(tokenValues(parsedSearch, 'owner')[0] ?? 'all')
  let stateFilter = $derived(hasToken(parsedSearch, 'flag', 'hidden') ? 'hidden' : hasToken(parsedSearch, 'show', 'all') ? 'all' : 'active')
  function setKind(id: string) {
    search = setToken(search, 'owner', id === 'all' ? [] : [id])
  }
  function setState(id: string) {
    search = setToken(setToken(search, 'flag', id === 'hidden' ? ['hidden'] : []), 'show', id === 'all' ? ['all'] : [])
  }

  let hiddenCount = $derived(attachments.filter((a) => a.hidden).length)
  let activeCount = $derived(attachments.length - hiddenCount)

  let filtered = $derived.by(() => {
    const q = parsedSearch.text.toLowerCase()
    const out = attachments.filter((a) => {
      if (!passes(parsedSearch, ATT_SPEC, a)) return false
      if (stateFilter === 'active' && a.hidden) return false
      if (!q) return true
      return `${a.filename || ''} ${a.comment || ''} ${ownerLabel(a)} ${a.content_type_detected || ''}`
        .toLowerCase()
        .includes(q)
    })
    return sortAttachments(out, sort)
  })

  let groups = $derived.by(() => {
    if (!grouped) return []
    return groupByOwner(filtered, ownerLabel)
  })

  let selected = $derived(attachments.find((a) => a.id === selectedId) ?? null)
  const ops = createAttachmentOps({
    selected: () => selected,
    selectedId: () => selectedId,
    onSelect: (id) => onSelect?.(id),
    onChanged: () => onChanged?.(),
    api: {
      listRevisions: listAttachmentRevisions,
      setHidden: setAttachmentHidden,
      setCurrent: setAttachmentCurrentRevision,
      updateComment: updateAttachmentComment,
      remove: deleteAttachment,
      removeRevision: deleteAttachmentRevision,
      uploadRevision: uploadAttachmentRevision,
    },
  })
  const visibleIds = () => (grouped ? groups.filter((g) => !foldedGroups.has(String(g.key))).flatMap((g) => g.files) : filtered).map((a) => a.id)
  const grp = createAttachmentGroup({
    rows: () => attachments,
    selectedId: () => selectedId,
    download: (a) => downloadAtt(a),
    setHidden: setAttachmentHidden,
    remove: deleteAttachment,
    onChanged: () => onChanged?.(),
    onDeleted: (ids) => ids.includes(selectedId ?? -1) && onSelect?.(null),
  })

  $effect(() => {
    const id = selectedId
    filtered
    if (id == null || typeof document === 'undefined') return
    queueMicrotask(() => {
      document.querySelector(`[data-att-id="${id}"]`)?.scrollIntoView({ block: 'nearest' })
    })
  })

  function pickVersion(a: any) {
    ops.expectVersion(a)
    versionInput?.click()
  }

  function onVersionPick(event: Event & { currentTarget: HTMLInputElement }) {
    const file = event.currentTarget.files?.[0]
    event.currentTarget.value = ''
    return ops.uploadVersion(file)
  }

  function downloadAtt(a: any, revision: number | null = null) {
    const link = document.createElement('a')
    link.href = attachmentDownloadUrl(ownerKind(a), a.owner_id, a.id, revision)
    link.download = a.filename || ''
    link.rel = 'noopener'
    document.body.appendChild(link)
    link.click()
    link.remove()
  }

  async function copyRef(a: any) {
    try {
      const text = `attachment=${a.id}`
      await copyText(text)
      toast(`Скопировано ${text}`, { kind: 'success', ttl: 1600 })
    } catch (e) {
      toast(e?.message || String(e), { kind: 'error' })
    }
  }

  let ctxGroup = $state<number[]>([])
  let ctxItems = $derived.by(() => {
    const a = ctxAtt
    if (!a) return []
    if (ctxGroup.length > 1) return grp.items(ctxGroup)
    return [
      { id: 'copy-id', label: `Копировать attachment=${a.id}` },
      { id: 'graph', label: 'Открыть на карте' },
      { id: 'sep-ref', sep: true },
      { id: 'download', label: 'Скачать' },
      { id: 'new-version', label: 'Новая версия…' },
      { id: 'hide', label: a.hidden ? 'Вернуть' : 'Скрыть' },
      { id: 'open', label: 'Открыть владельца' },
      { id: 'sep-danger', sep: true },
      { id: 'delete', label: 'Удалить…', danger: true },
    ]
  })

  function openAttMenu(event: MouseEvent, a: any) {
    const target = event.target
    if (target instanceof Element && target.closest('a, button, input, textarea')) return
    event.preventDefault()
    event.stopPropagation()
    ctxGroup = grp.multi.targets(a.id)
    if (ctxGroup.length < 2) grp.multi.clear()
    ctxAtt = a
    ctxX = event.clientX
    ctxY = event.clientY
    ctxOpen = true
  }

  function onAttSelect(action: string): void {
    const a = ctxAtt
    if (!a) return
    if (ctxGroup.length > 1) return void grp.run(action, ctxGroup)
    if (action === 'graph') onOpenGraph?.(ownerKind(a) as 'quest' | 'questline' | 'note', Number(a.owner_id))
    else if (action === 'download') downloadAtt(a)
    else if (action === 'new-version') pickVersion(a)
    else if (action === 'hide') ops.toggleHidden(a)
    else if (action === 'open') onOpenOwner?.(ownerKind(a), Number(a.owner_id))
    else if (action === 'copy-id') copyRef(a)
    else if (action === 'delete') ops.pending = { att: a, revision: null }
  }

  function onKey(e: KeyboardEvent) {
    const t = e.target
    if (t instanceof Element && t.closest('input, textarea, select')) return
    if (e.key === 'Escape' && grp.multi.size > 0) grp.multi.clear()
    else if (e.key === 'Escape' && selectedId != null && !ops.pending) onSelect?.(null)
    else if ((e.ctrlKey || e.metaKey) && e.code === 'KeyA' && !e.shiftKey) {
      e.preventDefault()
      grp.selectAll(visibleIds())
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
        title={`Фильтры: ${helpLine(ATT_SPEC)}; -key:value — исключить`}
      />
      <select class="attpage__select" bind:value={sort} aria-label="Сортировка">
        {#each SORTS as s (s.id)}
          <option value={s.id}>{s.label}</option>
        {/each}
      </select>
      <div class="attpage__seg" role="group" aria-label="Вид вложений">
        {#each VIEWS as v (v.id)}
          <button
            type="button"
            class="attpage__pill"
            class:attpage__pill--on={view === v.id}
            aria-pressed={view === v.id}
            onclick={() => (view = v.id)}
          >{v.label}</button>
        {/each}
      </div>
    </div>
    <FlagHints value={search} spec={ATT_SPEC} parsed={parsedSearch} onPick={(v) => (search = v)} />
    <div class="attpage__row2">
      <div class="attpage__seg" role="group" aria-label="Тип владельца">
        {#each KINDS as k (k.id)}
          <button
            type="button"
            class="attpage__pill"
            class:attpage__pill--on={kindFilter === k.id}
            onclick={() => setKind(k.id)}>{k.label}</button
          >
        {/each}
      </div>
      <div class="attpage__seg" role="group" aria-label="Состояние">
        {#each STATES as s (s.id)}
          <button
            type="button"
            class="attpage__pill"
            class:attpage__pill--on={stateFilter === s.id}
            onclick={() => setState(s.id)}
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
    <div class="attpage__list" class:attpage__list--cards={view === 'cards'}>
      {#if attachments.length === 0}
        <p class="empty">Вложений пока нет — их вешают на квесты, квестлайны и заметки.</p>
      {:else if filtered.length === 0}
        <p class="empty">Ничего не найдено</p>
      {:else if grouped}
        {#each groups as g (g.key)}
          {@const isFolded = foldedGroups.has(String(g.key))}
          <section class="attpage__group">
            <AttachmentGroupHead
              kind={KIND_ONE[g.kind]}
              title={g.title}
              count={g.files.length}
              folded={isFolded}
              onToggle={() => foldedGroups.toggle(String(g.key))}
              onOpen={() => onOpenOwner?.(g.kind, g.id)}
            />
            {#if !isFolded}
              {#each g.files as a (a.id)}
                {@render row(a, false)}
              {/each}
            {/if}
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
      <AttachmentPanel
        {a}
        busy={ops.busyId === a.id}
        revs={ops.revs}
        revsBusy={ops.revsBusy}
        bind:commentDraft={ops.commentDraft}
        ownerKindLabel={KIND_ONE[ownerKind(a)]}
        ownerLabel={ownerLabel(a)}
        onClose={() => onSelect?.(null)}
        onDownload={(revision = null) => downloadAtt(a, revision)}
        onToggleHidden={() => ops.toggleHidden(a)}
        onPickVersion={() => pickVersion(a)}
        onDelete={() => (ops.pending = { att: a, revision: null })}
        onOpenOwner={() => onOpenOwner?.(ownerKind(a), Number(a.owner_id))}
        onSaveComment={() => ops.saveComment(a)}
        onMakeCurrent={(revision) => ops.makeCurrent(a, revision)}
        onDeleteRevision={(revision) => (ops.pending = { att: a, revision })}
        onCopyRef={() => copyRef(a)}
      />

    {/if}
  </div>
</div>

{#snippet row(a: any, showOwner: boolean)}
  <article
    class="attpage__item"
    style:--file-color={fileKind(a).color}
    class:attpage__item--on={a.id === selectedId}
    class:attpage__item--picked={grp.multi.has(a.id)}
    class:attpage__item--hidden={a.hidden}
    data-att-id={a.id}
    role="listitem"
    onclick={(e) => !grp.click(e, a.id, visibleIds()) && onSelect?.(a.id)}
    oncontextmenu={(e) => openAttMenu(e, a)}
  >
    <div class="attpage__thumb">
      {#if view === 'cards'}
        <AttachmentPreview {a} />
      {:else if isRaster(a)}
        <img src={thumbSrc(a)} alt="" loading="lazy" />
      {:else}
        <span class="attpage__kind">
          <FileKindIcon att={a} size={20} />
          <span class="attpage__ext">{extOf(a) || '···'}</span>
        </span>
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
        disabled={ops.busyId === a.id}
        onclick={(e) => {
          e.stopPropagation()
          ops.toggleHidden(a)
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
          ops.pending = { att: a, revision: null }
        }}
      >
        <Icon name="delete" size={16} />
      </button>
    </div>
  </article>
{/snippet}

<SidebarPicked count={grp.multi.size} onClear={() => grp.multi.clear()} compact />
<ConfirmModal
  open={grp.ask.length > 0}
  title="Удалить вложения?"
  message={`Удалить выбранные вложения (${grp.ask.length}) вместе со всеми версиями? Это необратимо.`}
  busy={grp.busy}
  onCancel={grp.cancelDelete}
  onConfirm={grp.confirmDelete}
/>
<input bind:this={versionInput} class="attpage__file-input" type="file" onchange={onVersionPick} />

<ConfirmModal
  open={ops.pending != null}
  title={ops.pending?.revision == null ? 'Удалить вложение?' : 'Удалить версию?'}
  message={ops.pending
    ? ops.pending.revision == null
      ? `Удалить «${ops.pending.att.filename}» вместе со всеми версиями? Это необратимо. Если файл просто не нужен — лучше скрыть его.`
      : `Удалить версию ${ops.pending.revision} файла «${ops.pending.att.filename}»? Это необратимо.`
    : ''}
  busy={ops.deleteBusy}
  onCancel={() => {
    if (!ops.deleteBusy) ops.pending = null
  }}
  onConfirm={ops.confirmDelete}
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
    position: relative;
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

  .attpage__item--on,
  .attpage__item--picked {
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

  .attpage__kind {
    display: grid;
    justify-items: center;
    gap: 0.1rem;
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

  :global(.attpage__ico) {
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

  :global(.attpage__ico):hover:not(:disabled) {
    color: var(--color-fg, #e8e8e8);
    background: color-mix(in srgb, var(--color-fg, #e8e8e8) 8%, transparent);
  }

  :global(.attpage__ico--danger):hover:not(:disabled) {
    color: var(--color-danger, #b54a3a);
    background: color-mix(in srgb, var(--color-danger, #b54a3a) 14%, transparent);
  }

  :global(.attpage__ico):disabled {
    opacity: 0.4;
    cursor: default;
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

    .attpage__item {
      padding-inline: 0.75rem;
    }
  }
</style>
