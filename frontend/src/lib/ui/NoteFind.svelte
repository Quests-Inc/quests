<script lang="ts">
  import { searchInNote } from '../js/api.ts'

  /** Search inside the open note with the notes search's rules (word forms, prefix, other layout…):
   * the server says which words matched, this marks them in the rendered text and steps through them.
   * @type {{ noteId: number, container: HTMLElement | null, dep?: string, onClose: () => void }} */
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
  <button type="button" class="btn" disabled={ranges.length === 0} onclick={() => step(-1)} title="Предыдущее (Shift+Enter)" aria-label="Предыдущее">↑</button>
  <button type="button" class="btn" disabled={ranges.length === 0} onclick={() => step(1)} title="Следующее (Enter)" aria-label="Следующее">↓</button>
  <button type="button" class="btn" onclick={onClose} title="Закрыть (Esc)" aria-label="Закрыть поиск">✕</button>
</div>

<style>
  .nf {
    position: sticky;
    top: 0;
    z-index: 5;
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.4rem 0;
    background: var(--bg, var(--surface, Canvas));
  }
  .nf__input {
    flex: 1 1 auto;
    min-width: 0;
  }
  .nf__count {
    min-width: 4.5rem;
    font-size: 0.8rem;
    opacity: 0.7;
    text-align: right;
    white-space: nowrap;
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
