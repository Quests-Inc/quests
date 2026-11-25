<script lang="ts">
  import { toast, toastDone, toastProgress } from '@/shared/lib/toasts.svelte.ts'
  import { syncNoteColumns } from '@/shared/lib/syncNoteColumns.ts'
 import {RepositoriesBlock} from '@/entities/repository'
  import { AttachmentsBlock } from '@/entities/attachment'
  import { ValueTable } from '@/entities/value'
  import { EntityHistory } from '@/entities/history'
  import { NoteCheck, NoteFind } from '@/entities/note'
  import { MentionTextarea } from '@/entities/template'
  import type { NoteEditor } from '../model/noteEditor.svelte.ts'
  import Icon from '@/shared/ui/Icon.svelte'
  import RefText from '@/shared/ui/RefText.svelte'
  import MarkdownBody from '@/shared/ui/MarkdownBody.svelte'
  import { applyTaskMarks } from '@/shared/lib/markdown.ts'
  import { getNote } from '@/shared/api/notes.ts'
  import { uploadAttachment } from '@/shared/api/attachments.ts'
  import { fetchAttachmentAudio, transcribeFile } from '@/shared/api/stt.ts'
  import { sttSettings } from '@/shared/model/sttSettings.svelte.ts'
  import ContextMenu from '@/shared/ui/ContextMenu.svelte'
  import ModalHead from '@/shared/ui/ModalHead.svelte'
  import ModalShell from '@/shared/ui/ModalShell.svelte'
  import NoteHead from './NoteHead.svelte'
  import NoteLinks from './NoteLinks.svelte'

  type Ref = { kind: string; id: number; title?: string }
  type Crumb = { id: number | null; title: string }

  let noteValues = $state({} as Record<string, string>)
  let valuesNoteId = $state<number | null>(null)
  let valuesLoading = $state(true)
  /** Тело открытой заметки: шапка, редактор, предпросмотр, ссылки и вложения. Раскладка страницы остаётся у NotesPage. */
  let {
    ed,
    selectedId,
    titleEl = $bindable(null),
    editEl = $bindable(null),
    previewEl = $bindable(null),
    findOpen = $bindable(false),
    refsOpen = $bindable(false),
    backlinksOpen = $bindable(false),
    titleReadonly,
    titleEditing = false,
    titleDraft = $bindable(''),
    noteIsOutdated,
    exportBusy,
    crumbs,
    labels = {},
    onRef,
    viewMode,
    viewModes,
    quests = [],
    questlines = [],
    notes = [],
    attachments = [],
    onOpenNoteMenu,
    onTitleDblClick,
    onTitleBlur,
    onTitleKeydown,
    onTogglePin,
    onToggleReadme,
    onTogglePrivate,
    onToggleCategory,
    onToggleOutdated,
    onRevert,
    onSave,
    onExport,
    onDelete,
    onCrumb,
    onParentPicker,
    onViewMode,
    onEditKeydown,
    onStartBodyEdit,
    onChanged,
    onOpenSecrets,
  }: {
    ed: NoteEditor
    selectedId: number | null
    titleEl?: HTMLTextAreaElement | null
    editEl?: HTMLTextAreaElement | null
    previewEl?: HTMLDivElement | null
    findOpen?: boolean
    refsOpen?: boolean
    backlinksOpen?: boolean
    titleReadonly: boolean
    titleEditing?: boolean
    titleDraft?: string
    noteIsOutdated: boolean
    exportBusy: boolean
    crumbs: Crumb[]
    labels?: Record<string, string>
    onRef?: (kind: string, id: number) => void
    viewMode: string
    viewModes: readonly { id: string; label: string }[]
    quests?: { id: number; title?: string }[]
    questlines?: { id: number; title?: string }[]
    notes?: { id: number; title?: string; is_sample?: boolean }[]
    attachments?: { id: number; title?: string }[]
    onOpenNoteMenu: (e: MouseEvent, note: { id: number }) => void
    onTitleDblClick: () => void
    onTitleBlur: () => void
    onTitleKeydown: (event: KeyboardEvent) => void
    onTogglePin: () => void
    onToggleReadme: () => void
    onTogglePrivate: () => void
    onToggleCategory: () => void
    onToggleOutdated: () => void
    onRevert: () => void
    onSave: () => void
    onExport: (event: MouseEvent) => void
    onDelete: () => void
    onCrumb: (id: number | null) => void
    onParentPicker: (event: MouseEvent) => void
    onViewMode: (mode: string) => void
    onEditKeydown: (event: KeyboardEvent) => void
    onStartBodyEdit: (event?: MouseEvent) => void
    onChanged: () => void
    onOpenSecrets?: (id: number) => void
  } = $props()
  const samples = $derived(notes.filter((n) => n.is_sample && n.id !== selectedId))
  let samplePick = $state(false)
  let insertMenu = $state<{ x: number; y: number } | null>(null)
  let audioMenu = $state<{ x: number; y: number; id: number } | null>(null)
  let savedCaret: { a: number; b: number } | null = null
  let recording = $state(false)
  let recBusy = $state(false)
  let recSeconds = $state(0)
  let recorder: MediaRecorder | null = null
  let pendingRec = $state<{ url: string; blob: Blob; seconds: number } | null>(null)
  let recChunks: Blob[] = []
  let recTick = 0

  function stopClock() {
    window.clearInterval(recTick)
    recTick = 0
  }

  function startClock() {
    stopClock()
    const started = performance.now()
    recSeconds = 0
    recTick = window.setInterval(() => {
      recSeconds = Math.floor((performance.now() - started) / 1000)
    }, 200)
  }

  function recClock(seconds: number) {
    return `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, '0')}`
  }

  function insertAtCaret(text: string) {
    const ta = editEl
    const cur = ed.description || ''
    const a = savedCaret?.a ?? ta?.selectionStart ?? cur.length
    const b = savedCaret?.b ?? ta?.selectionEnd ?? a
    savedCaret = null
    ed.description = cur.slice(0, a) + text + cur.slice(b)
    const pos = a + text.length
    queueMicrotask(() => {
      ta?.focus()
      ta?.setSelectionRange(pos, pos)
    })
  }

  function onInsert(id: string) {
    insertMenu = null
    const table = /^table-(top|left|both|none)-(\d+)x(\d+)$/.exec(id)
    if (table) tableScaffold(table[1] as 'top' | 'left' | 'both' | 'none', Number(table[2]), Number(table[3]))
    else if (id === 'math') insertAtCaret('\n$$\n\n$$\n')
    else if (id === 'link') insertAtCaret('quest=')
    else if (id === 'rec') void toggleRecord()
  }

  function tableScaffold(side: 'top' | 'left' | 'both' | 'none', cols: number, rows: number) {
    const mark = side === 'top' ? '' : `<!-- table: ${side} -->\n`
    const cell = (r: number, c: number) => ((side === 'top' || side === 'both') && r === 0) || ((side === 'left' || side === 'both') && c === 0) ? ' ' : ' '
    const line = (r: number) => `| ${Array.from({ length: cols }, (_, c) => cell(r, c)).join(' | ')} |`
    const body = [line(0), `| ${Array.from({ length: cols }, () => '---').join(' | ')} |`, ...Array.from({ length: rows - 1 }, (_, i) => line(i + 1))]
    insertAtCaret(`\n${mark}${body.join('\n')}\n`)
  }

  async function fillFromSample(id: number) {
    if ((ed.description || '').trim()) return
    samplePick = false
    try {
      const note = await getNote(id)
      ed.description = note.description || ''
      if ((ed.title || '').trim() === 'Новая заметка' && note.title) ed.title = note.title
    } catch (e) {
      toast(e instanceof Error ? e.message : String(e), { kind: 'error' })
    }
  }

  async function toggleRecord() {
    if (!selectedId || sampleBlocked()) return
    if (recording && recorder) {
      recorder.stop()
      return
    }
    let stream: MediaStream
    try {
      stream = await navigator.mediaDevices.getUserMedia({ audio: true })
    } catch (e) {
      toast(e instanceof Error ? e.message : 'Нет доступа к микрофону', { kind: 'error' })
      return
    }
    recChunks = []
    recorder = new MediaRecorder(stream)
    recorder.ondataavailable = (event) => {
      if (event.data.size) recChunks.push(event.data)
    }
    recorder.onstop = () => {
      stopClock()
      stream.getTracks().forEach((track) => track.stop())
      recording = false
      const blob = new Blob(recChunks, { type: recorder?.mimeType || 'audio/webm' })
      if (!blob.size) {
        toast('Пустая запись', { kind: 'error' })
        return
      }
      if (pendingRec) URL.revokeObjectURL(pendingRec.url)
      pendingRec = { url: URL.createObjectURL(blob), blob, seconds: recSeconds }
    }
    recorder.start()
    startClock()
    recording = true
  }

  function discardRecording() {
    if (recBusy) return
    if (pendingRec) URL.revokeObjectURL(pendingRec.url)
    pendingRec = null
  }

  async function keepRecording() {
    const pending = pendingRec
    if (!pending || recBusy) return
    await finishRecording(pending.blob)
    URL.revokeObjectURL(pending.url)
    pendingRec = null
  }

  async function finishRecording(blob: Blob) {
    if (!selectedId || !blob.size) return
    recBusy = true
    try {
      const file = new File([blob], `voice-${Date.now()}.webm`, { type: blob.type || 'audio/webm' })
      const saved = await uploadAttachment('note', selectedId, file)
      const id = Number(saved?.id)
      if (!Number.isFinite(id)) throw new Error('Вложение без id')
      const label = `запись ${recClock(pendingRec?.seconds ?? recSeconds)}`
      insertAtCaret(`\n![${label}](attachment=${id})\n`)
      onChanged()
    } catch (e) {
      toast(e instanceof Error ? e.message : String(e), { kind: 'error' })
    } finally {
      recBusy = false
    }
  }

  const recognizing = new Set<number>()

  function placeTranscript(id: number, text: string) {
    const block = `распознано из attachment=${id}:\n${text}`
    const cur = ed.description || ''
    const marker = `распознано из attachment=${id}:`
    const at = cur.indexOf(marker)
    if (at >= 0) {
      const rest = cur.slice(at + marker.length)
      const end = rest.search(/\n\n/)
      ed.description = cur.slice(0, at) + block + (end >= 0 ? rest.slice(end) : '\n')
    } else {
      const img = new RegExp(`(!\\[[^\\]\\n]*\\]\\(attachment=${id}\\))`)
      ed.description = img.test(cur) ? cur.replace(img, `$1\n\n${block}\n`) : `${cur.replace(/\s*$/, '')}\n\n${block}\n`
    }
    onChanged()
  }

  function recognizeAttachment(id: number) {
    if (!sttSettings.enabled) {
      toast('Распознавание не настроено', { kind: 'error' })
      return
    }
    if (recognizing.has(id)) return
    recognizing.add(id)
    const tid = toastProgress(`stt-${id}`, `Распознаю attachment=${id}`)
    void (async () => {
      try {
        const blob = await fetchAttachmentAudio(id)
        const text = await transcribeFile(new File([blob], `attachment-${id}.webm`, { type: blob.type || 'audio/webm' }))
        if (!text) throw new Error('Пустая расшифровка')
        placeTranscript(id, text)
        toastDone(tid, `Распознано attachment=${id}`)
      } catch (e) {
        toastDone(tid, e instanceof Error ? e.message : 'Распознавание не удалось', 'error')
      } finally {
        recognizing.delete(id)
      }
    })()
  }

  function sampleBlocked() {
    if (!ed.detail?.is_sample) return false
    toast(`Заметка «${ed.title}» помечена как образец. Редактирование запрещено`, { kind: 'error' })
    return true
  }

  /** Bake the preview's scratch checkbox ticks into the note text, then save. Marks outside code and math are positional. */
  function saveMarksToText(marks: boolean[]) {
    if (sampleBlocked()) return
    const next = applyTaskMarks(ed.description, marks)
    if (next == null) {
      toast('Текст заметки изменился — отметки не перенесены, обновите превью', { kind: 'error' })
      return
    }
    if (next !== ed.description) ed.description = next
    onSave()
  }
</script>

{#if ed.error}
  <p class="notes__error">{ed.error}</p>
{/if}
{#if selectedId == null}
  <p class="notes__empty">Выберите заметку или создайте новую</p>
{:else if ed.loadedId !== selectedId}
  <section class="notes__loading" aria-busy={ed.loading}>
    <h1><RefText source={notes.find(n => n.id === selectedId)?.title ?? 'Заметка'} {labels} {onRef} /></h1>
    {#if ed.loading}<p role="status">Загрузка заметки…</p>{/if}
  </section>
{:else}
  {#if ed.external}
    <div class="notes__external" role="status">
      <span>Заметку изменили снаружи, а у вас есть несохранённые правки.</span>
      <button type="button" class="notes__external-btn" onclick={() => ed.adoptExternal(selectedId)}>
        Загрузить новую версию
      </button>
      <button type="button" class="notes__external-btn" onclick={() => (ed.external = null)}>
        Оставить мою
      </button>
    </div>
  {/if}
  <NoteHead
    bind:title={() => (titleEditing ? titleDraft : ed.title), (v) => (titleDraft = v)}
    {titleEditing}
    bind:titleEl
    {titleReadonly}
    dirty={ed.dirty}
    pinned={ed.pinned}
    isReadme={ed.isReadme}
    isPrivate={ed.isPrivate}
    isCategory={ed.isCategory}
    isSample={!!ed.detail?.is_sample}
    outdated={noteIsOutdated}
    outdatedReason={ed.detail?.outdated_reason ?? ''}
    saving={ed.saving}
    {exportBusy}
    {crumbs}
    {labels}
    {onRef}
    bind:findOpen
    {viewMode}
    {viewModes}
    onContextMenu={(e) => {
      const n = notes.find((item) => item.id === selectedId)
      if (n) onOpenNoteMenu(e, n)
    }}
    onTitleDblClick={() => { if (!sampleBlocked()) onTitleDblClick() }}
    onTitleBlur={onTitleBlur}
    {onTitleKeydown}
    {onTogglePin}
    {onToggleReadme}
    {onTogglePrivate}
    {onToggleCategory}
    {onToggleOutdated}
    {onRevert}
    {onSave}
    {onExport}
    {onDelete}
    {onCrumb}
    {onParentPicker}
    {onViewMode}
  >
    {#snippet tools()}
      {#if selectedId && !(ed.isPrivate && !ed.privateRevealed)}
        <ValueTable noteIsSample={!!ed.detail?.is_sample} readonly={!!ed.detail?.is_sample} variant="icon" owner={{ kind: 'note', id: selectedId }} onEffective={(text, load) => {
          noteValues = text
          valuesNoteId = load.id
          valuesLoading = load.state === 'loading'
        }} />
      {/if}
    {/snippet}
  </NoteHead>
  {#if ed.isPrivate && !ed.privateRevealed}
    <div class="notes__private-gate">
      <Icon name="lock" size={20} />
      <p>Приватная заметка — содержимое скрыто от случайного взгляда.</p>
      <button type="button" class="btn" onclick={() => (ed.privateRevealed = true)}>
        Показать
      </button>
    </div>
  {:else}
    {#if recording || recBusy}
      <div class="notes__rec" role="status">
        <span class="notes__rec-dot" aria-hidden="true"></span>
        <span class="notes__rec-time">{recording ? recClock(recSeconds) : 'Сохраняю'}</span>
        {#if recording}
          <button type="button" onclick={toggleRecord}>Остановить</button>
        {/if}
      </div>
    {/if}
    <ModalShell open={!!pendingRec} onClose={discardRecording} labelledby="voice-keep-title" closeDisabled={recBusy} maxWidth="26rem">
      <ModalHead id="voice-keep-title" title="Запись" icon="play" onClose={discardRecording} closeDisabled={recBusy} />
      <div class="modal__body notes__rec-review">
        {#if pendingRec}
          <audio src={pendingRec.url} controls></audio>
        {/if}
        <div class="notes__rec-review-actions">
          <button type="button" class="btn btn--ghost" disabled={recBusy} onclick={discardRecording}>Отклонить</button>
          <button type="button" class="btn" disabled={recBusy} onclick={() => void keepRecording()}>{recBusy ? 'Сохраняю' : 'Вставить'}</button>
        </div>
      </div>
    </ModalShell>
    {#if !(ed.description || '').trim() && samples.length && !ed.detail?.is_sample}
      <div class="notes__sample">
        {#if !samplePick}
          <button type="button" onclick={() => (samplePick = true)}>Заполнить из образца</button>
        {:else}
          {#each samples as s (s.id)}
            <button type="button" onclick={() => fillFromSample(s.id)}>{s.title || `Заметка ${s.id}`}</button>
          {/each}
          <button type="button" onclick={() => (samplePick = false)}>Отмена</button>
        {/if}
      </div>
    {/if}
    {#if findOpen}
      <NoteFind noteId={selectedId} container={viewMode === 'raw' ? null : previewEl} dep={ed.description} onClose={() => (findOpen = false)} />
    {/if}
    <div
      onbeforeinput={(e) => { if (sampleBlocked()) e.preventDefault() }}
      class="notes__split"
      use:syncNoteColumns={{ enabled: viewMode === 'combined', source: ed.description }}
      class:notes__split--raw={viewMode === 'raw'}
      class:notes__split--combined={viewMode === 'combined'}
      class:notes__split--formatted={viewMode === 'formatted'}
      class:notes__split--flash={ed.flashing}
    >
      {#if viewMode !== 'formatted'}
        <MentionTextarea
          class="notes__edit"
          host={selectedId ? { kind: 'note', id: selectedId } : undefined}
          placement="inside"
          bind:value={ed.description}
          bind:el={editEl}
          {quests}
          {questlines}
          {notes}
          {attachments}
          rows={16}
          placeholder="Markdown. @название — ссылка. Код и конфиг — в блоках ``` … ```"
          onkeydown={onEditKeydown}
          oncontextmenu={(e) => {
            if (ed.detail?.is_sample) return
            e.preventDefault()
            const ta = e.currentTarget
            if (ta instanceof HTMLTextAreaElement) savedCaret = { a: ta.selectionStart ?? 0, b: ta.selectionEnd ?? 0 }
            insertMenu = { x: e.clientX, y: e.clientY }
          }}
        />
      {/if}
      {#if viewMode !== 'raw'}
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div
          class="notes__preview block--prose"
          class:notes__preview--doc={viewMode === 'formatted'}
          bind:this={previewEl}
          ondblclick={(e) => { if (!sampleBlocked()) onStartBodyEdit(e) }}
        >
          <MarkdownBody source={ed.description} {labels} {onRef} values={valuesNoteId === selectedId ? noteValues : {}} valuesLoading={valuesNoteId !== selectedId || valuesLoading} host={selectedId ? { kind: 'note', id: selectedId } : undefined} onSaveMarks={saveMarksToText} onAudioMenu={(id, point) => (audioMenu = { id, ...point })} />
        </div>
      {/if}
    </div>
  {/if}
  <NoteLinks
    refs={(ed.detail?.refs ?? []) as Ref[]}
    backlinks={(ed.detail?.backlinks ?? []) as Ref[]}
    bind:refsOpen
    bind:backlinksOpen
    {onRef}
  />
  {#if selectedId}
    <div class="block notes__attach">
      <NoteCheck initialCheck={ed.detail?.check ?? null} noteId={selectedId} {onChanged} onOpenSecrets={() => onOpenSecrets?.(selectedId)} />
    </div>
    <div class="block notes__attach">
      <RepositoriesBlock owner={{kind:'note',id:selectedId}} />
      <AttachmentsBlock ownerType="note" ownerId={selectedId} onOpenOwner={onRef} />
    </div>
    <div class="block notes__attach">
      <EntityHistory entityType="note" entityId={selectedId} version={ed.detail?.updated_at} />
    </div>
  {/if}
{/if}

<ContextMenu
  open={insertMenu != null}
  x={insertMenu?.x ?? 0}
  y={insertMenu?.y ?? 0}
  items={[
    { id: 'table', label: 'Таблица', children: [
      { id: 'table-top', label: 'Шапка сверху', grid: 10 },
      { id: 'table-left', label: 'Шапка слева', grid: 10 },
      { id: 'table-both', label: 'Шапка с обеих сторон', grid: 10 },
      { id: 'table-none', label: 'Без шапки', grid: 10 },
    ] },
    { id: 'math', label: 'Формула' },
    { id: 'link', label: 'Ссылка' },
    { id: 'rec', label: recording ? 'Остановить запись' : 'Запись', disabled: recBusy || !selectedId || !!pendingRec },
  ]}
  onSelect={onInsert}
  onClose={() => (insertMenu = null)}
/>
<ContextMenu
  open={audioMenu != null}
  x={audioMenu?.x ?? 0}
  y={audioMenu?.y ?? 0}
  items={[{ id: 'recognize', label: 'Распознать' }]}
  onSelect={(id) => {
    const attachment = audioMenu?.id
    audioMenu = null
    if (id === 'recognize' && attachment) recognizeAttachment(attachment)
  }}
  onClose={() => (audioMenu = null)}
/>

<style>
  .notes__loading h1 {
    margin: 0 0 var(--space-3, 0.75rem);
    font-family: var(--font-display, Georgia, serif);
    font-size: clamp(1.35rem, 2.4vw, 1.85rem);
    line-height: 1.15;
    overflow-wrap: anywhere;
  }
  .notes__loading p {
    color: var(--color-fg-muted, #9a9a9a);
  }

  .notes__private-gate {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--space-2, 0.5rem);
    padding: var(--space-5, 2rem) var(--space-3, 0.75rem);
    color: var(--color-fg-muted, #9a9a9a);
    text-align: center;
  }

  .notes__private-gate p {
    margin: 0;
    font-size: 0.9rem;
  }

  .notes__empty {
    color: var(--color-fg-muted, #9a9a9a);
    padding: 1rem;
  }

  .notes__error {
    margin: 0;
    color: var(--color-danger, #b54a3a);
  }

  .notes__external {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.4rem 0.75rem;
    margin: 0;
    padding: 0.4rem 0.6rem;
    border-left: 3px solid var(--color-accent, #c9a227);
    background: color-mix(in srgb, var(--color-accent, #c9a227) 12%, transparent);
    font-size: 0.85rem;
  }

  .notes__external-btn {
    padding: 0;
    border: 0;
    background: none;
    color: var(--color-accent, #c9a227);
    font: inherit;
    text-decoration: underline;
    cursor: pointer;
  }

  .notes__split--flash {
    animation: notes-flash 1.5s ease-out;
  }

  @keyframes notes-flash {
    from {
      background: color-mix(in srgb, var(--color-accent, #c9a227) 22%, transparent);
    }
    to {
      background: transparent;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .notes__split--flash {
      animation: none;
    }
  }

  .notes__rec {
    display: flex;
    align-items: center;
    gap: 0.45rem;
    margin-bottom: 0.4rem;
    color: var(--color-fg);
    font-size: 0.85rem;
  }
  .notes__rec-time {
    font-variant-numeric: tabular-nums;
    min-width: 2.4rem;
  }
  .notes__rec-dot {
    width: 0.55rem;
    height: 0.55rem;
    border-radius: 50%;
    background: #d24a4a;
    box-shadow: 0 0 0 0 #d24a4a;
    animation: notes-rec 1.2s ease-out infinite;
  }
  .notes__rec button {
    margin-left: auto;
    border: 1px solid #d24a4a;
    border-radius: 999px;
    background: transparent;
    color: #d24a4a;
    font: inherit;
    font-size: 0.82rem;
    padding: 0.1rem 0.55rem;
    cursor: pointer;
  }
  @keyframes notes-rec {
    70% { box-shadow: 0 0 0 0.35rem transparent; }
  }
  @media (prefers-reduced-motion: reduce) {
    .notes__rec-dot { animation: none; }
  }
  .notes__rec-review {
    display: grid;
    gap: 0.85rem;
  }
  .notes__rec-review audio {
    display: block;
    width: 100%;
    height: 2.5rem;
  }
  .notes__rec-review-actions {
    display: flex;
    justify-content: flex-end;
    gap: 0.5rem;
  }
  .notes__sample {
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
    margin-bottom: 0.4rem;
  }
  .notes__sample button {
    border: 1px solid var(--color-border, #3a3a3a);
    border-radius: 999px;
    background: transparent;
    color: var(--color-fg-muted);
    font: inherit;
    font-size: 0.82rem;
    padding: 0.1rem 0.55rem;
    cursor: pointer;
  }
  .notes__split {
    display: grid;
    gap: 0;
    min-height: 0;
    flex: 0 0 auto;
  }

  .notes__split--combined {
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
    grid-template-areas: 'edit preview';
    grid-template-rows: minmax(0, 1fr);
    gap: 0 1.1rem;
  }
  .notes__split--combined :global(.mention-field) {
    grid-area: edit;
  }
  .notes__split--combined .notes__preview.block--prose {
    grid-area: preview;
  }

  .notes__split--combined .notes__preview.block--prose { position: sticky; top: calc(-1 * var(--space-5, 1.5rem)); align-self: start; height: calc(100dvh - 3.5rem); scrollbar-width: none; min-height: 0; box-sizing: border-box; overflow: hidden; }

  .notes__split--combined .notes__preview::-webkit-scrollbar { display: none; }

  .notes__split--raw,
  .notes__split--formatted {
    grid-template-columns: minmax(0, 1fr);
  }

  .notes__split--formatted .notes__preview {
    min-height: 0;
    overflow: visible;
  }

  .notes__split :global(textarea.notes__edit) {
    width: 100%;
    min-height: 18rem;
    resize: none;
    overflow: hidden;
    padding: 0.75rem 1rem;
    border: 0;
    border-radius: 0;
    background: transparent;
    color: inherit;
    font-family: var(--font-mono, ui-monospace, monospace);
    font-size: 0.85rem;
    line-height: 1.5;
  }

  .notes__split :global(textarea.notes__edit:focus) {
    outline: none;
  }

  .notes__split :global(.mention-field--inside) {
    min-width: 0;
    border: 1px solid transparent;
    border-radius: var(--pill-radius, 12px);
    background: var(--pill-bg, #0f0f0f);
    box-shadow: var(--pill-shadow, none);
  }

  .notes__split :global(.mention-field--inside:focus-within) {
    border-color: var(--color-accent);
    outline: 2px solid color-mix(in srgb, var(--color-accent) 35%, transparent);
    outline-offset: 2px;
  }

  .notes__preview.block--prose {
    width: 100%;
    margin: 0;
    padding: 0.75rem 1rem;
    overflow: auto;
    min-height: 18rem;
    min-width: 0;
    overflow-wrap: break-word;
  }

  .notes__preview--doc {
    cursor: text;
  }

  .notes__attach {
    margin-top: 0;
    margin-bottom: 0;
    padding-top: var(--space-4, 1rem);
    border-top: 1px solid var(--color-border, #333);
  }

  @media (max-width: 720px) {
    .notes__split--combined .notes__preview.block--prose { position: static; height: auto; overflow: visible; }
    .notes__split--combined {
      grid-template-columns: 1fr;
      gap: 0.85rem;
      background: none;
    }
  }
</style>
