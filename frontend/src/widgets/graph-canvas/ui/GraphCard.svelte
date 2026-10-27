<script lang="ts">
  import { CARD, ENTITY_LABELS, SQUARE, TYPES, tinted, type GraphNodeView, type GraphNoteEditing } from '@/entities/graph'
  import { QuestlineIcon } from '@/entities/questline'
  import { statusWord } from '@/shared/lib/questStatus.ts'
  import FileKindIcon from '@/shared/ui/FileKindIcon.svelte'
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
    raster = false,
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
    labels = {},
    onOpenMore,
    onShowChanges,
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
    onStartEdit(n: GraphNodeView): void
    onFinishEdit: (save: boolean) => void
    onOpen?: (kind: string, id: number) => void
    /** Подписи ссылок kind:id → заголовок, те же что в обычном просмотре. */
    labels?: Record<string, string>
    onOpenMore(n: GraphNodeView, type: string): void
    onShowChanges: (key: string) => void
  } = $props()

const autofocusIf = (node: { focus: () => void }, on: unknown) => {
  if (on) node.focus()
}

/** Title of a brand-new note: focused with the whole text selected. */
const focusSelect = (node: HTMLInputElement | HTMLTextAreaElement) => {
  // The card is mounted while the map is still hidden for measuring, and a
  // hidden field cannot take focus — keep trying until it shows.
  let tries = 0
  const grab = () => {
    node.focus()
    node.select()
    if (document.activeElement !== node && node.isConnected && tries++ < 30) requestAnimationFrame(grab)
  }
  grab()
}

  const t = $derived(TYPES[n.type] ?? TYPES.step)
  const square = $derived(n.type === 'attachment' || n.type === 'cluster')
  const big = $derived(n.type === 'quest' || n.type === 'note')
  const withText = $derived(n.type === 'step' && !!n.desc)
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
        class:gcard--pill={!big && !square && !withText}
        style:--c={ownerless ? `color-mix(in srgb, ${t.color} 35%, #e0443e)` : t.color}
        style:--c-fill={tint && !ownerless ? tinted(t.color, tint) : undefined}
        style:--fill-w={tint && !ownerless ? '17%' : undefined}
        style:z-index={editingHere ? 1_000_000 : z}
        style:left="{n.x - n.w / 2}px"
        style:top="{n.y - n.h / 2}px"
        style:width={big ? `${CARD[n.type].w}px` : withText ? `${CARD.stepBody.w}px` : square ? `${SQUARE}px` : undefined}
        style:height={ready && n.h ? `${editingHere ? Math.max(n.h, 340) : n.h}px` : undefined}
        data-gkey={n.key}
        title="{t.label}: {n.title}{orphan ? ' — на неё нет ссылок' : ''}"
        onpointerdown={(e) => {
          onRaise?.(n.key)
          onCardDown(e, n)
        }}
        oncontextmenu={(e) => onCtx(e, n)}
        ondblclick={(e) => {
          if (n.type === 'note' && !(e.target instanceof Element && (e.target as Element).closest('a, button, textarea, input'))) void onStartEdit(n)
        }}
      >
        {#if square}
          <div class="gcard__sq">
            {#if raster}
              <img class="gcard__thumb" src="/api/attachments/{n.id}/image?preview=1" alt="" loading="lazy" draggable="false" />
            {:else if n.type === 'cluster'}
              <span class="gcard__count">{n.title.split(' ')[0]}</span>
            {:else}
              <FileKindIcon att={{ filename: n.title, content_type_detected: n.mime }} size={30} />
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
          <textarea
            class="gcard__editor"
            bind:value={editing.text}
            use:autofocusIf={!editing.fresh}
            placeholder="Markdown"
            spellcheck="false"
            onkeydown={(e) => {
              if (e.key === 'Escape') {
                e.stopPropagation()
                void onFinishEdit(false)
              } else if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
                e.preventDefault()
                void onFinishEdit(true)
              }
            }}
          ></textarea>
          <div class="gcard__editbar">
            <button type="button" class="graph__btn graph__btn--primary" onclick={() => onFinishEdit(true)}>Готово</button>
            <button type="button" class="graph__btn" onclick={() => onFinishEdit(false)}>Отмена</button>
            <span class="gcard__edithint">Ctrl+Enter — готово · Esc — отмена</span>
          </div>
        {:else if tier === 'lite'}
          <!-- frame only: off-screen or too small to read -->
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
          {#if n.desc && tier !== 'title'}
            <div class="gcard__body" style:max-height="{big ? CARD[n.type].max : CARD.stepBody.max}px">
              <MarkdownBody source={n.desc} {labels} onRef={(k, id) => onOpen?.(k, id)} />
            </div>
          {/if}
        {:else}
          {@render glyph(n, t)}
          <span class="gcard__title gcard__title--line">{n.title}</span>
          {#if n.type === 'step' && n.progress}
            <span class="gcard__prog">{n.progress}</span>
          {/if}
        {/if}
        {#if !square && tier !== 'lite'}
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


  .gcard {
    position: absolute;
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

  .gcard__editor {
    flex: 1 1 auto;
    min-height: 0;
    resize: none;
    padding: 0.4rem 0.5rem;
    border: 1px solid var(--color-border, #333);
    border-radius: 6px;
    background: color-mix(in srgb, var(--c-fill, var(--c, #888)) 14%, var(--color-bg, #121212));
    color: var(--color-fg, #e8e8e8);
    font-family: var(--font-mono, monospace);
    font-size: 0.78rem;
    line-height: 1.45;
    user-select: text;
  }

  .gcard__editbar {
    display: flex;
    align-items: center;
    gap: 0.4rem;
  }

  .gcard__edithint {
    margin-left: auto;
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
    display: grid;
    justify-items: center;
    align-content: center;
    gap: 0.3rem;
    height: 100%;
    min-width: 0;
  }

  .gcard__thumb {
    width: 100%;
    height: 56px;
    object-fit: cover;
    border-radius: 10px;
    pointer-events: none;
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
