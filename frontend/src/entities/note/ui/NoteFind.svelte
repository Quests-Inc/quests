<script lang="ts">
  import { searchInNote } from '@/shared/api/client.ts'
  import Icon from '@/shared/ui/Icon.svelte'

  /**
   * Search inside the open note with the notes search's rules (word forms, prefix, other layout…):
   * the server says which words matched, this marks them in the rendered text and steps through them.
   */
  let { noteId, container, dep = '', onClose }: { noteId: number, container: HTMLElement | null, dep?: string, onClose: () => void } = $props()

  let q = $state('')
  let ranges = $state(([] as Range[]))
  let cur = $state(0)
  let error = $state('')
  let inputEl = $state((null as HTMLInputElement | null))
  let terms = ([] as string[])
  let seq = 0
  let timer = 0

  const canHighlight = typeof CSS !== 'undefined' && 'highlights' in CSS && typeof Highlight !== 'undefined'

  $effect(() => {
    inputEl?.focus()
    inputEl?.select()
  })

  function clearMarks() {
    if (canHighlight) {
      CSS.highlights.delete('note-find')
      CSS.highlights.delete('note-find-cur')
    }
  }

  function esc(s: string) {
    return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  }

  function collect() {
    ranges = []
    if (!container || terms.length === 0) return
    const re = new RegExp(`(?<![\\p{L}\\p{N}_])(?:${terms.map(esc).join('|')})(?![\\p{L}\\p{N}_])`, 'giu')
    const out = []
    const walker = document.createTreeWalker(container, NodeFilter.SHOW_TEXT)
    for (let n = walker.nextNode(); n; n = walker.nextNode()) {
      const text = n.nodeValue || ''
      re.lastIndex = 0
      for (let m = re.exec(text); m; m = re.exec(text)) {
        const r = document.createRange()
        r.setStart(n, m.index)
        r.setEnd(n, m.index + m[0].length)
        out.push(r)
        if (out.length >= 2000) break
      }
      if (out.length >= 2000) break
    }
    ranges = out
  }

  function paint(scroll: boolean) {
    clearMarks()
    if (ranges.length === 0) return
    if (cur >= ranges.length) cur = 0
    if (canHighlight) {
      CSS.highlights.set('note-find', new Highlight(...ranges))
      CSS.highlights.set('note-find-cur', new Highlight(ranges[cur]))
    }
    if (scroll) ranges[cur].startContainer.parentElement?.scrollIntoView({ block: 'center', behavior: 'smooth' })
  }

  async function run(scroll: boolean) {
    const query = q.trim()
    const mine = ++seq
    if (!query) {
      terms = []
      ranges = []
      error = ''
      clearMarks()
      return
    }
    try {
      const r = await searchInNote(noteId, query)
      if (mine !== seq) return
      error = ''
      terms = r?.terms ?? []
    } catch (e) {
      if (mine !== seq) return
      error = e?.message || String(e)
      terms = []
    }
    collect()
    cur = 0
    paint(scroll)
  }

  // A new query, another note or an edit of the text: search again shortly.
  $effect(() => {
    void q
    void noteId
    void dep
    void container
    clearTimeout(timer)
    timer = window.setTimeout(() => run(true), 250)
    return () => clearTimeout(timer)
  })

  $effect(() => () => {
    seq++
    clearMarks()
  })

  function step(d: number) {
    if (ranges.length === 0) return
    cur = (cur + d + ranges.length) % ranges.length
    paint(true)
  }

  function onKey(e: { key: string; preventDefault: () => void; shiftKey: any }) {
    if (e.key === 'Enter') {
      e.preventDefault()
      step(e.shiftKey ? -1 : 1)
    } else if (e.key === 'Escape') {
      e.preventDefault()
      onClose()
    }
  }
</script>

<div class="nf" role="search">
  <span class="nf__icon" aria-hidden="true"><Icon name="search" /></span>
  <input
    class="nf__input"
    type="search"
    placeholder="Найти в заметке"
    aria-label="Поиск в заметке"
    bind:value={q}
    bind:this={inputEl}
    onkeydown={onKey}
  />
  <span class="nf__count" aria-live="polite">
    {#if error}{error}
    {:else if !container}нет окна просмотра
    {:else if q.trim() && ranges.length === 0}не найдено
    {:else if ranges.length}{cur + 1} / {ranges.length}{/if}
  </span>
  <button type="button" class="nf__btn nf__btn--up" disabled={ranges.length === 0} onclick={() => step(-1)} title="Предыдущее (Shift+Enter)" aria-label="Предыдущее"><Icon name="arrow-up" /></button>
  <button type="button" class="nf__btn nf__btn--down" disabled={ranges.length === 0} onclick={() => step(1)} title="Следующее (Enter)" aria-label="Следующее"><Icon name="arrow-up" /></button>
  <button type="button" class="nf__btn" onclick={onClose} title="Закрыть (Esc)" aria-label="Закрыть поиск"><Icon name="close" /></button>
</div>

<style>
  .nf {
    position: sticky;
    top: 0;
    z-index: 5;
    display: flex;
    align-items: center;
    gap: 0.3rem;
    margin: 0.5rem 0;
    padding: 0.2rem 0.35rem 0.2rem 0.65rem;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-md, 4px);
    background: var(--color-bg-raised, #1a1a1a);
    transition: border-color 0.12s;
  }
  .nf:focus-within {
    border-color: var(--color-accent, #c9a227);
  }
  .nf__icon {
    display: inline-flex;
    width: 1rem;
    height: 1rem;
    color: var(--color-fg-subtle, #6e6e6e);
  }
  .nf__icon :global(svg) {
    width: 100%;
    height: 100%;
  }
  .nf__input {
    flex: 1 1 auto;
    min-width: 0;
    padding: 0.3rem 0.2rem;
    border: 0;
    outline: none;
    background: transparent;
    color: var(--color-fg, #e8e8e8);
    font: inherit;
    font-size: var(--text-sm, 0.875rem);
  }
  .nf__input::placeholder {
    color: var(--color-fg-subtle, #6e6e6e);
  }
  .nf__input::-webkit-search-cancel-button {
    display: none;
  }
  .nf__count {
    min-width: 4.5rem;
    font-family: var(--font-ui, sans-serif);
    font-size: 0.75rem;
    color: var(--color-fg-muted, #9a9a9a);
    text-align: right;
    white-space: nowrap;
  }
  .nf__btn {
    display: grid;
    place-items: center;
    width: 1.7rem;
    height: 1.7rem;
    padding: 0;
    border: 0;
    border-radius: var(--radius-sm, 3px);
    background: transparent;
    color: var(--color-fg-muted, #9a9a9a);
    cursor: pointer;
  }
  .nf__btn :global(svg) {
    width: 0.95rem;
    height: 0.95rem;
  }
  .nf__btn--down :global(svg) {
    transform: rotate(180deg);
  }
  .nf__btn:hover:not(:disabled),
  .nf__btn:focus-visible {
    background: rgba(127, 127, 127, 0.2);
    color: var(--color-fg, #e8e8e8);
  }
  .nf__btn:disabled {
    opacity: 0.35;
    cursor: default;
  }
  :global(::highlight(note-find)) {
    background: rgba(255, 200, 0, 0.35);
    color: inherit;
  }
  :global(::highlight(note-find-cur)) {
    background: rgba(255, 140, 0, 0.7);
    color: inherit;
  }
</style>
