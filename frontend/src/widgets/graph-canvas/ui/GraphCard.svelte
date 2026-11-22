<script lang="ts">
  import { afterPaint } from '@/shared/lib/frameTasks.ts'
  import { untrack } from 'svelte'
  import { getNote } from '@/shared/api/notes.ts'
  import Icon from '@/shared/ui/Icon.svelte'

 import {type GalleryImage} from '@/shared/lib/imageViewer.svelte.ts'
  import { trackOf } from '@/shared/lib/player.svelte.ts'
  import { focusEditorAt, focusWhenShown, sourceCaretFromMarkdownClick } from '../model/noteEditCaret.ts'
 import GraphSquareMedia from './GraphSquareMedia.svelte'
 import NoteSilhouette from './NoteSilhouette.svelte'
  import { CARD, ENTITY_LABELS, SQUARE, TYPES, tinted, type GraphNodeView, type GraphNoteEditing } from '@/entities/graph'
  import { QuestlineIcon } from '@/entities/questline'
  import { MentionTextarea } from '@/entities/template'
  import { rememberRevealed, revealedBody } from '@/shared/lib/privateReveal.svelte.ts'
  import { statusWord } from '@/shared/lib/questStatus.ts'
  import type { MapJump } from '@/shared/lib/mapJumps.ts'
  import MarkdownBody from '@/shared/ui/MarkdownBody.svelte'

  /**
   * Карточка узла карты: заголовок, текст, вложение-миниатюра, редактор заметки на месте, точки «ещё связи» и «есть изменения».
   * Состояние выбора, режима связывания и расчёты приходят готовыми флагами из страницы.
   */
  let {
    n,
    selected = false,
    tint = '',
    lit = false,
    root = false,
    done = false,
    orphan = false,
    dim = false,
    pickable = false,
    anchor = false,
    ownerless = false,
    editingHere = false,
    picked = false,
    ready = false,
    textRevision = 0,
    loadText = true,
    deferContent = false,
    onContentMeasure,
    raster = false,
    imagePixels = 128,
    imageGallery = [],
    tier,
    more = [],
    changed = false,
    editing = $bindable(null),
    onCardDown,
    onRaise,
    z = 0,
    onCtx,
    onStartEdit,
    onFinishEdit,
    onOpen,
    mapJump = null,
    labels = {},
    onOpenMore,
    onShowChanges,
    onResizeStart,
    onResizeReset,
    resizing = false,
    onRevealed,
  }: {
    n: GraphNodeView
    selected?: boolean
    /** A light shade of the entity's own colour (questline, file type) mixed into the card's fill; the border keeps the type colour. */
    tint?: string
    lit?: boolean
    root?: boolean
    done?: boolean
    orphan?: boolean
    dim?: boolean
    pickable?: boolean
    anchor?: boolean
    ownerless?: boolean
    editingHere?: boolean
    picked?: boolean
    ready?: boolean
    textRevision?: number
    loadText?: boolean
    deferContent?: boolean
    onContentMeasure?: (key: string, width: number, height: number) => void
    imageGallery?: GalleryImage[]
    imagePixels?: number
    raster?: boolean
    tier?: string
    more?: { type: string; count: number }[]
    changed?: boolean
    editing?: GraphNoteEditing | null
    onCardDown: (e: PointerEvent, n: GraphNodeView) => void
    /** Touching a card brings it to the foreground. */
    onRaise?: (key: string) => void
    /** Stacking order among cards (higher = nearer). */
    z?: number
    onCtx: (e: MouseEvent, n: GraphNodeView) => void
    onStartEdit(n: GraphNodeView, opts?: { caret?: number | null }): void
    onFinishEdit: (save: boolean) => void
    onOpen?: (kind: string, id: number) => void
    /** Ссылки на сущности этого же холста получают кнопку перехода камеры. */
    mapJump?: MapJump | null
    /** Подписи ссылок kind:id → заголовок, те же что в обычном просмотре. */
    labels?: Record<string, string>
    onOpenMore(n: GraphNodeView, type: string): void
    onShowChanges: (key: string) => void
    /** Pointer down on the corner grip: the page stretches the card. */
    onResizeStart?: (e: PointerEvent, n: GraphNodeView) => void
    /** Double click on the grip: back to the size of the content. */
    onResizeReset?: (n: GraphNodeView) => void
    resizing?: boolean
    onRevealed?: (key: string, text: string) => void
  } = $props()
/** Title of a brand-new note: focused with the whole text selected. */
const focusSelect = (node: HTMLInputElement | HTMLTextAreaElement) => {
  focusWhenShown(node, (el) => {
    ;(el as HTMLInputElement).focus()
    ;(el as HTMLInputElement).select()
  })
}

function beginNoteEdit(event: MouseEvent, node: GraphNodeView) {
  if (event.target instanceof Element && event.target.closest('a, button, textarea, input, a.md-script')) return
  event.preventDefault()
  const mdRoot = cardEl?.querySelector('.gcard__body .md') ?? cardEl?.querySelector('.gcard__body')
  void onStartEdit(node, { caret: sourceCaretFromMarkdownClick(mdRoot, bodyText ?? '', event.clientX, event.clientY) })
}

  let cardEl = $state<HTMLElement | null>(null)
  let editEl = $state<HTMLTextAreaElement | null>(null)
  let focusedEditKey: string | null = null
  $effect(() => {
    if (!editingHere) { focusedEditKey = null; return }
    const ed = editing, ta = editEl
    if (!ed || ed.fresh || !ta || focusedEditKey === ed.key) return
    focusedEditKey = ed.key
    const offset = ed.caret ?? null
    if (ed.caret != null) ed.caret = null
    focusEditorAt(ta, offset)
  })
  let contentReady = $state(!deferContent)
  $effect(() => {
    const enabled = !deferContent || (ready && loadText && tier !== 'lite' && tier !== 'title')
    contentReady = !deferContent
    if (deferContent && enabled) return afterPaint(() => { contentReady = true })
  })

  let fullText = $state<{ preview: string; revision: number; text: string } | null>(null)
  let textError = $state(false)
  let textLoading = $state(false)
  let textRetry = $state(0)
  // The graph carries a 4000-rune preview. Fetch the body only when the
  // visible card renders markdown; draft edits already contain the full text.
  $effect(() => {
    const preview = n.desc ?? ''
    const revision = textRevision
    const id = Number(n.id)
    void textRetry
    if (n.type !== 'note' || n.draft || changed || !ready || !loadText || !contentReady || tier === 'lite' || tier === 'title' ||
      !Number.isInteger(id) || id <= 0 || !preview.endsWith('…') || [...preview].length <= 4000) return
    if (untrack(() => fullText?.preview === preview && fullText.revision === revision)) {
      textLoading = false
      return
    }
    let cancelled = false
    textError = false
    textLoading = true
    void getNote(id).then(note => {
      if (!cancelled) fullText = { preview, revision, text: note.description ?? '' }
    }).catch(() => {
      if (!cancelled) textError = true
    }).finally(() => {
      if (!cancelled) textLoading = false
    })
    return () => { cancelled = true }
  })
  const opened = $derived(n.private ? revealedBody(n.key) : undefined)
  const sealed = $derived(Boolean(n.private) && opened === undefined)
  const bodyText = $derived(sealed ? '' : opened !== undefined ? opened : !n.draft && !changed && fullText && fullText.preview === n.desc ? fullText.text : n.desc)
  let revealing = $state(false)
  let revealError = $state(false)
  async function showPrivate(event: MouseEvent) {
    event.stopPropagation()
    const id = Number(n.id)
    if (revealing || !Number.isInteger(id) || id <= 0) return
    revealing = true
    revealError = false
    try {
      const note = await getNote(id)
      const text = note.description ?? ''
      rememberRevealed(n.key, text)
      onRevealed?.(n.key, text)
    } catch { revealError = true } finally { revealing = false }
  }

  // Refine an automatic size once the real body exists; explicit user sizes stay fixed.
  $effect(() => {
    void bodyText
    const el = cardEl
    if (!deferContent || !contentReady || !ready || editingHere || n.sh != null || !el || !onContentMeasure) return
    untrack(() => {
      const previous = el.style.height
      el.style.height = 'auto'
      const width = el.offsetWidth, height = el.offsetHeight
      el.style.height = previous
      onContentMeasure?.(n.key, width, height)
    })
  })

  const t = $derived(TYPES[n.type] ?? TYPES.step)
  const square = $derived(n.type === 'attachment' || n.type === 'cluster')
  const big = $derived(n.type === 'quest' || n.type === 'note')
  const withText = $derived(n.type === 'step' && !!n.desc)
  /** Whose values and scripts the text speaks for: the entity of the card, when it is a real one (not a draft). */
  const hostOf = $derived.by(() => {
    const id = Number(n.id)
    return !n.draft && Number.isInteger(id) && id > 0 && ['note', 'quest', 'step', 'questline'].includes(n.type) ? { kind: n.type, id } : undefined
  })
</script>

      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <div
        class="gcard gcard--{n.type}"
        class:gcard--sel={selected}
        class:gcard--lit={lit}
        class:gcard--root={root}
        class:gcard--done={done}
        class:gcard--orphan={orphan}
        class:gcard--draft={n.draft}
        class:gcard--dim={dim}
        class:gcard--pickable={pickable}
        class:gcard--anchor={anchor}
        class:gcard--ownerless={ownerless}
        class:gcard--editing={editingHere}
        class:gcard--picked={picked}
        class:gcard--playing={n.type === 'attachment' && trackOf(Number(n.id)) === 'playing'}
        class:gcard--current={n.type === 'attachment' && trackOf(Number(n.id)) === 'paused'}
        class:gcard--pill={!big && !square && !withText}
        style:--c={ownerless ? `color-mix(in srgb, ${t.color} 35%, #e0443e)` : t.color}
        style:--c-fill={tint && !ownerless ? tinted(t.color, tint) : undefined}
        style:--fill-w={tint && !ownerless ? '17%' : undefined}
        style:z-index={editingHere ? 1_000_000 : z}
        style:transform="translate({n.x - n.w / 2}px, {n.y - n.h / 2}px)"
        class:gcard--sized={n.sw != null}
        class:gcard--compact={!editingHere && n.sh != null && n.sh < 80}
        class:gcard--resizing={resizing}
        style:width={n.sw != null ? `${n.sw}px` : big ? `${CARD[n.type].w}px` : withText ? `${CARD.stepBody.w}px` : square ? `${SQUARE}px` : undefined}
        style:height={n.sh != null ? `${editingHere ? Math.max(n.sh, 340) : n.sh}px` : square ? `${SQUARE}px` : ready && n.h ? `${editingHere ? Math.max(n.h, 340) : n.h}px` : undefined}
        bind:this={cardEl}
        data-gkey={n.key}
        title="{t.label}: {n.title}{orphan ? ' — на неё нет ссылок' : ''}"
        onpointerdown={(e) => {
          onRaise?.(n.key)
          onCardDown(e, n)
        }}
        oncontextmenu={(e) => onCtx(e, n)}
        ondblclick={(e) => {
          if (n.type === 'note' && !sealed) beginNoteEdit(e, n)
        }}
      >
        {#if square}
          <div class="gcard__sq">
            {#if n.type === 'cluster'}
              <span class="gcard__count">{n.title.split(' ')[0]}</span>
            {:else}
              <GraphSquareMedia
                id={n.id}
                title={n.title}
                mime={n.mime}
                {raster}
                {imagePixels}
                {imageGallery}
              />
            {/if}
            <span class="gcard__name">{n.type === 'cluster' ? 'вложений' : n.title}</span>
          </div>
        {:else if editingHere && editing}
          <div class="gcard__head">
            {@render glyph(n, t)}
            {#if editing.fresh}
              <input
                class="gcard__titleedit"
                bind:value={editing.title}
                aria-label="Заголовок"
                use:focusSelect
                onkeydown={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault()
                    e.currentTarget.closest('.gcard')?.querySelector('textarea')?.focus()
                  } else if (e.key === 'Escape') {
                    e.stopPropagation()
                    void onFinishEdit(true)
                  }
                }}
              />
            {:else}
              <input class="gcard__titleedit" bind:value={editing.title} aria-label="Заголовок" />
            {/if}
          </div>
          <MentionTextarea
            class="gcard__editor"
            bind:value={editing.text}
            bind:el={editEl}
            host={hostOf}
            placement="inside"
            placeholder="Markdown"
            onkeydown={(e) => {
              if (e.key === 'Escape') {
                e.stopPropagation()
                void onFinishEdit(false)
              } else if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
                e.preventDefault()
                void onFinishEdit(true)
              }
            }}
          />
          <div class="gcard__editbar">
            <button type="button" class="graph__btn graph__btn--primary" onclick={() => onFinishEdit(true)}>Готово</button>
            <button type="button" class="graph__btn" onclick={() => onFinishEdit(false)}>Отмена</button>
            <span class="gcard__edithint">Ctrl+Enter — готово · Esc — отмена</span>
          </div>
        {:else if tier === 'lite'}
          {#if n.type==='note'}<div class="gcard__ghost"><span>{n.title}</span>{#if !n.private}<NoteSilhouette text={n.desc||''}/>{/if}</div>{/if}
        {:else if big || withText}
          <div class="gcard__head">
            {@render glyph(n, t)}
            <span class="gcard__title">{n.title}</span>
            {#if n.type === 'quest' && n.status && n.status !== 'active'}
              <span class="gcard__status" title={n.status}>{statusWord(n.status)}</span>
            {:else if n.type === 'step' && n.progress}
              <span class="gcard__prog">{n.progress}</span>
            {/if}
          </div>
          {#if sealed && tier !== 'title' && contentReady}
            <div class="gcard__private"><Icon name="lock" size={16} /><p>Приватная заметка — содержимое скрыто от случайного взгляда.</p><button type="button" class="gcard__show" disabled={revealing} onpointerdown={(e) => e.stopPropagation()} ondblclick={(e) => e.stopPropagation()} onclick={showPrivate}>{revealError ? 'Повторить' : 'Показать'}</button></div>
          {:else if n.type==='note'&& !n.private && (tier==='title' || !contentReady)}<NoteSilhouette text={n.desc||''}/>{/if}
          {#if bodyText && tier !== 'title' && contentReady}
            <div class="gcard__body" style:max-height={n.sh != null ? undefined : `${big ? CARD[n.type].max : CARD.stepBody.max}px`}>
              <MarkdownBody source={bodyText} {labels} host={hostOf} ownValues={!!hostOf} {mapJump} onRef={(k, id) => onOpen?.(k, id)} />
              {#if textLoading}<p role="status">Загрузка полного текста…</p>{/if}
              {#if textError}<button type="button" onpointerdown={e => e.stopPropagation()} onclick={() => textRetry++}>Повторить загрузку полного текста</button>{/if}
            </div>
          {/if}
        {:else}
          {@render glyph(n, t)}
          <span class="gcard__title gcard__title--line">{n.title}</span>
          {#if n.type === 'step' && n.progress}
            <span class="gcard__prog">{n.progress}</span>
          {/if}
        {/if}
        {#if (!square || raster) && tier !== 'lite'}
                      {#if more.length}
            <div class="gcard__more">
              {#each more as m (m.type)}
                <button
                  type="button"
                  class="gcard__dot"
                  style:--dot={TYPES[m.type].color}
                  title="{ENTITY_LABELS[m.type]}: {m.count} не на карте. Клик — показать"
                  aria-label="Показать связи: {ENTITY_LABELS[m.type]}, {m.count}"
                  onpointerdown={(e) => e.stopPropagation()}
                  oncontextmenu={(e) => e.stopPropagation()}
                  onclick={() => onOpenMore(n, m.type)}>{m.count}</button
                >
              {/each}
            </div>
          {/if}
        {/if}
        {#if (!square || raster) && tier !== 'lite' && !editingHere && onResizeStart}
          <button
            type="button"
            class="gcard__grip"
            tabindex="-1"
            title="Потяните, чтобы растянуть; двойной клик — вернуть размер по содержимому"
            aria-label="Растянуть карточку"
            onpointerdown={(e) => onResizeStart(e, n)}
            oncontextmenu={(e) => e.stopPropagation()}
            ondblclick={(e) => {
              e.stopPropagation()
              onResizeReset?.(n)
            }}
          ></button>
        {/if}
        {#if changed}
          <button
            type="button"
            class="gcard__changed"
            title="Есть несохранённые изменения — показать в журнале"
            aria-label="Есть несохранённые изменения"
            onpointerdown={(e) => e.stopPropagation()}
            onclick={() => onShowChanges(n.key)}>!</button
          >
        {/if}
      </div>

{#snippet glyph(n: GraphNodeView, t: { color: string; glyph: string })}
{#if (n.type === 'quest' || n.type === 'note' || n.type === 'questline') && (n.icon || n.icon_url)}
  <span class="gcard__icon" style:--line-color={n.color || t.color}>
    <QuestlineIcon icon={n.icon || 'document'} iconUrl={n.icon_url || null} size="sm" />
  </span>
{:else}
  <span class="gcard__glyph">{n.type === 'step' && done ? '✓' : t.glyph}</span>
{/if}
{/snippet}

<style>
  .gcard__placeholder { white-space: pre-wrap; overflow-wrap: anywhere; opacity: 0.18; }
  .gcard__private { display: flex; flex-direction: column; align-items: flex-start; gap: 0.4rem; color: var(--color-fg-muted, #9a9a9a); } .gcard__private p { margin: 0; }
  .gcard__show { font: inherit; color: inherit; background: transparent; border: 1px solid currentColor; border-radius: 8px; padding: 0.15rem 0.6rem; cursor: pointer; }

 .gcard__ghost{height:100%;display:flex;flex-direction:column;padding:12px;gap:8px;overflow:hidden}.gcard__ghost span{opacity:.55;font-size:14px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}



  .gcard {
    position: absolute;
    left: 0;
    top: 0;
    /* Separate compositor layers let the browser reuse painted text while the map moves. */
    contain: layout style;
    /* The card's own stacking context (z-index is set inline): its badges stay under cards above it. */
    z-index: 0;
    box-sizing: border-box;
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
    padding: 0.55rem 0.7rem;
    border: 2.2px solid var(--c, #888);
    border-radius: 14px;
    /* Card tinted with its type colour, so kind reads at a glance. */
    background: color-mix(in srgb, var(--c-fill, var(--c, #888)) var(--fill-w, 11%), var(--color-bg-raised, #1a1a1a));
    color: var(--color-fg, #e8e8e8);
    font-size: var(--text-sm, 0.875rem);
    cursor: default;
    user-select: text;
  }

  /* A stretched card: the text fills it and scrolls, nothing is cut by the usual height cap. */
  .gcard--sized {
    max-width: none;
  }

  .gcard--sized.gcard--pill {
    white-space: normal;
  }

  .gcard__grip {
    position: absolute;
    right: 2px;
    bottom: 2px;
    width: 16px;
    height: 16px;
    padding: 0;
    border: 0;
    background: linear-gradient(135deg, transparent 50%, color-mix(in srgb, var(--c, #888) 70%, transparent) 50%, transparent 56%, transparent 66%, color-mix(in srgb, var(--c, #888) 70%, transparent) 66%, transparent 72%);
    border-radius: 0 0 12px 0;
    cursor: nwse-resize;
    opacity: 0;
    touch-action: none;
  }

  .gcard:hover .gcard__grip,
  .gcard--sel .gcard__grip,
  .gcard--resizing .gcard__grip,
  .gcard__grip:focus-visible {
    opacity: 1;
  }

  @media (hover: none) {
    .gcard__grip {
      opacity: 0.7;
      width: 22px;
      height: 22px;
    }
  }

  .gcard--pill {
    flex-direction: row;
    align-items: center;
    gap: 0.4rem;
    max-width: 240px;
    padding: 0.3rem 0.8rem;
    border-radius: 999px;
    white-space: nowrap;
  }

  .gcard--step:not(.gcard--pill) {
    border-radius: 10px;
  }

  .gcard--note {
    border-radius: 10px;
  }

  .gcard--attachment,
  .gcard--cluster {
    padding: 0.4rem;
    border-radius: 18px;
    text-align: center;
  }

  .gcard--playing { outline: 2px solid var(--color-accent, #c9a227); outline-offset: 1px; }
  .gcard--current { outline: 2px solid color-mix(in srgb, var(--color-accent, #c9a227) 45%, transparent); outline-offset: 1px; }

  .gcard--root {
    border-width: 3px;
  }

  .gcard--editing {
    cursor: default;
    box-shadow: 0 6px 24px color-mix(in srgb, #000 55%, transparent);
  }

  .gcard__titleedit {
    flex: 1 1 auto;
    min-width: 0;
    padding: 0.15rem 0.35rem;
    border: 1px solid var(--color-border, #333);
    border-radius: 6px;
    background: color-mix(in srgb, var(--c-fill, var(--c, #888)) 14%, var(--color-bg, #121212));
    color: inherit;
    font: inherit;
    font-weight: 600;
  }

  .gcard :global(.mention-field--inside) {
    flex: 1 1 auto;
    min-height: 0;
    height: auto;
    display: flex;
    flex-direction: column;
    border-radius: 6px;
    background: color-mix(in srgb, var(--c-fill, var(--c, #888)) 14%, var(--color-bg, #121212));
  }

  .gcard :global(.gcard__editor),
  .gcard :global(.mention-field--hl .mdh-layer) {
    padding: 0.4rem 0.5rem;
    border-radius: 6px;
    font-family: var(--font-mono, monospace);
    font-size: 0.78rem;
    line-height: 1.45;
  }
  .gcard :global(.gcard__editor) {
    flex: 1 1 auto;
    min-height: 0;
    height: auto;
    resize: none;
    border: 1px solid var(--color-border, #333);
    background: transparent;
    color: var(--color-fg, #e8e8e8);
    user-select: text;
  }

  .gcard__editbar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.4rem;
  }

  .gcard__edithint {
    flex-basis: 100%;
    font-size: 0.65rem;
    color: var(--color-fg-muted, #9a9a9a);
  }

  .gcard--sel {
    background: color-mix(in srgb, var(--c-fill, var(--c, #c9a227)) 24%, var(--color-bg-raised, #1a1a1a));
  }

  .gcard__prog {
    flex: none;
    margin-left: auto;
    padding-left: 0.5rem;
    font-family: var(--font-mono, monospace);
    font-size: 0.7rem;
    color: var(--color-fg-muted, #9a9a9a);
  }

  .gcard--done {
    opacity: 0.6;
  }

  .gcard--orphan {
    border-style: dashed;
  }

  .gcard--picked {
    outline: 3px solid #5aa9ff;
    outline-offset: 2px;
  }

  /* Link mode: what can be picked glows, the rest recedes. */
  .gcard--dim {
    opacity: 0.35;
  }

  .gcard--pickable {
    cursor: crosshair;
    box-shadow: 0 0 0 3px color-mix(in srgb, var(--c, #c9a227) 35%, transparent);
  }

  .gcard--pickable:hover {
    box-shadow: 0 0 0 4px var(--c, #c9a227);
  }

  /* An end of the edge picked in its menu. */
  .gcard--lit {
    opacity: 1;
    box-shadow:
      0 0 0 4px var(--color-accent, #c9a227),
      0 0 0 7px color-mix(in srgb, var(--color-bg, #121212) 80%, transparent),
      0 0 32px 8px color-mix(in srgb, var(--color-accent, #c9a227) 70%, transparent);
  }

  .gcard--anchor {
    box-shadow: 0 0 0 4px var(--c, #c9a227);
  }

  /* New, not yet saved: dotted frame; without an owner the frame turns red. */
  .gcard--draft {
    border-style: dotted;
    box-shadow: 0 0 0 3px color-mix(in srgb, var(--c, #888) 14%, transparent);
  }

  .gcard--ownerless {
    box-shadow: 0 0 0 3px color-mix(in srgb, #e0443e 22%, transparent);
  }

  .gcard__head {
    display: flex;
    align-items: baseline;
    gap: 0.4rem;
    min-width: 0;
  }

  .gcard__icon {
    flex: none;
    display: inline-flex;
    align-self: center;
    color: var(--line-color, var(--c, #888));
  }

  /* Links not on the map yet: a circle per entity type on the card's lower border. */
  .gcard__more {
    position: absolute;
    left: 0;
    right: 0;
    bottom: -11px;
    z-index: 2;
    display: flex;
    justify-content: center;
    gap: 4px;
    pointer-events: none;
  }

  .gcard__dot {
    min-width: 21px;
    height: 21px;
    padding: 0 5px;
    box-sizing: border-box;
    border: 2px solid var(--color-bg, #121212);
    border-radius: 999px;
    background: var(--dot, #888);
    color: #141414;
    font: inherit;
    font-size: 11px;
    font-weight: 700;
    line-height: 17px;
    text-align: center;
    cursor: pointer;
    pointer-events: auto;
    box-shadow: 0 1px 4px color-mix(in srgb, #000 50%, transparent);
  }

  .gcard__dot:hover {
    filter: brightness(1.15);
    transform: scale(1.12);
  }

  /* Unsaved change: a yellow "!" in a circle on the card's top-right corner. */
  .gcard__changed {
    position: absolute;
    top: -9px;
    right: -9px;
    z-index: 2;
    width: 19px;
    height: 19px;
    padding: 0;
    display: grid;
    place-items: center;
    border: 2px solid var(--color-bg, #121212);
    border-radius: 50%;
    background: #f2c230;
    color: #1b1500;
    font: inherit;
    font-size: 11px;
    font-weight: 800;
    line-height: 1;
    cursor: pointer;
    box-shadow: 0 1px 4px color-mix(in srgb, #000 50%, transparent);
  }

  .gcard__changed:hover {
    transform: scale(1.15);
  }

  .gcard__glyph {
    flex: none;
    color: var(--c, #888);
  }

  .gcard__title {
    min-width: 0;
    font-weight: 600;
    overflow-wrap: anywhere;
  }

  .gcard--compact { padding: .25rem .4rem; gap: .15rem; }
  .gcard--compact .gcard__head { flex: 0 1 auto; min-height: 0; overflow: hidden; }
  .gcard--compact .gcard__title { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .gcard--compact .gcard__ghost { box-sizing: border-box; min-height: 0; padding: 0; }
  .gcard__title--line {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-weight: 500;
    font-size: 0.8rem;
  }

  .gcard__status {
    margin-left: auto;
    font-size: 0.65rem;
    color: var(--color-fg-muted, #9a9a9a);
  }

  .gcard__body {
    flex: 1 1 auto;
    min-height: 0;
    overflow: auto;
    font-size: 0.78rem;
    line-height: 1.45;
    color: var(--color-fg-muted, #b5b5b5);
    cursor: auto;
  }

  .gcard__body :global(p) {
    margin: 0 0 0.4em;
  }

  .gcard__body :global(> :last-child) {
    margin-bottom: 0;
  }

  .gcard__body :global(h1),
  .gcard__body :global(h2),
  .gcard__body :global(h3) {
    margin: 0.4em 0 0.25em;
    font-size: 0.9rem;
  }

  .gcard__body :global(pre) {
    overflow: auto;
  }

  .gcard__body :global(img) {
    max-width: 100%;
  }

  .gcard__sq {
    min-height:0;
    height:100%;
    display: grid;
    justify-items: center;
    grid-template-rows: minmax(0,1fr) auto;
    gap: 0.3rem;
    height: 100%;
    min-width: 0;
  }

  .gcard__count {
    font-size: 1.5rem;
    font-weight: 600;
    color: var(--c, #888);
  }

  .gcard__name {
    max-width: 100%;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
    overflow-wrap: anywhere;
    font-size: 0.68rem;
    line-height: 1.2;
  }</style>
