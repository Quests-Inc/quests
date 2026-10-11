<script lang="ts">
  import { createTag } from '../js/api.ts'
  import { buildSuggestIndex, suggestTags } from '../js/suggest.ts'

  const MAX = 5

  /** @type {{
   *   catalog?: { id: number, slug: string, label: string, color?: string }[],
   *   value?: { id: number, slug: string, label: string, color?: string }[],
   *   title?: string,
   *   description?: string,
   *   quests?: any[],
   *   enabled?: boolean,
   *   onChange: (tags: { id: number, slug: string, label: string, color?: string }[]) => void,
   *   onCatalog?: (tags: { id: number, slug: string, label: string, color?: string }[]) => void,
   * }} */
  let {
    catalog = [],
    value = [],
    title = '',
    description = '',
    quests = [],
    enabled = true,
    onChange,
    onCatalog = null,
  }: {
    catalog?: { id: number, slug: string, label: string, color?: string }[],
    value?: { id: number, slug: string, label: string, color?: string }[],
    title?: string,
    description?: string,
    quests?: any[],
    enabled?: boolean,
    onChange: (tags: { id: number, slug: string, label: string, color?: string }[]) => void,
    onCatalog?: (tags: { id: number, slug: string, label: string, color?: string }[]) => void,
  } = $props()

  let draft = $state('')
  let busy = $state(false)
  let err = $state('')
  /** Highlighted typed match for arrow-key selection; -1 = none. */
  let active = $state(-1)

  let selected = $derived(Array.isArray(value) ? value : [])
  let exclude = $derived(selected.map((t) => t.slug))

  let index = $derived(enabled ? buildSuggestIndex(quests) : null)
  let hints = $derived(
    enabled && index
      ? suggestTags(index, { title, description }, { excludeSlugs: exclude, limit: 5 })
      : [],
  )

  /** Catalog tags matching what's typed so far: slug/label prefix first, then substring. */
  let matches = $derived.by(() => {
    const q = normalizeSlug(draft)
    const raw = draft.trim().toLowerCase().replace(/ё/g, 'е')
    if (!q && !raw) return []
    const taken = new Set(selected.map((t) => t.id))
    /** @type {{ tag: any, rank: number }[]} */
    const out: { tag: any, rank: number }[] = []
    for (const c of catalog) {
      if (taken.has(c.id)) continue
      const slug = String(c.slug || '').toLowerCase()
      const label = String(c.label || '').toLowerCase().replace(/ё/g, 'е')
      let rank = -1
      if (slug === q || label === raw) rank = 0
      else if (slug.startsWith(q) || label.startsWith(raw)) rank = 1
      else if (slug.includes(q) || label.includes(raw)) rank = 2
      if (rank >= 0) out.push({ tag: c, rank })
    }
    out.sort((a, b) => a.rank - b.rank || a.tag.slug.localeCompare(b.tag.slug))
    return out.slice(0, 8).map((m) => m.tag)
  })

  $effect(() => {
    void draft
    active = -1
  })

  function normalizeSlug(raw: string) {
    return String(raw || '')
      .trim()
      .toLowerCase()
      .replace(/ё/g, 'е')
      .replace(/[\s_]+/g, '-')
      .replace(/[^a-z0-9а-яё-]+/gi, '')
      .replace(/-+/g, '-')
      .replace(/^-|-$/g, '')
  }

  function findInCatalog(token: string) {
    const t = token.trim()
    if (!t) return null
    const slug = normalizeSlug(t)
    const bySlug = catalog.find((c) => c.slug === slug)
    if (bySlug) return bySlug
    const low = t.toLowerCase()
    return catalog.find((c) => String(c.label || '').toLowerCase() === low) || null
  }

  async function resolveToken(token: string) {
    const t = token.trim()
    if (!t) return null
    if (selected.length >= MAX) {
      err = `Не больше ${MAX} тегов`
      return null
    }
    let tag = findInCatalog(t)
    if (!tag) {
      busy = true
      try {
        tag = await createTag({ slug: normalizeSlug(t) || t, label: t })
        if (onCatalog && tag) {
          const next = [...catalog]
          if (!next.some((c) => c.id === tag.id)) next.push(tag)
          onCatalog(next)
        }
      } catch (e) {
        err = e.message || String(e)
        return null
      } finally {
        busy = false
      }
    }
    if (!tag) return null
    if (selected.some((s) => s.id === tag.id)) return null
    return tag
  }

  async function onInput(e: { currentTarget: { value: any } }) {
    err = ''
    const v = e.currentTarget.value
    if (!v.includes(',')) {
      draft = v
      return
    }
    const parts = v.split(',')
    const rest = parts.pop() ?? ''
    let next = [...selected]
    for (const part of parts) {
      if (next.length >= MAX) break
      const tag = await resolveToken(part)
      if (tag && !next.some((s) => s.id === tag.id)) next.push(tag)
    }
    draft = rest
    if (next.length !== selected.length) onChange(next)
  }

  async function onKeydown(e: { key: string; preventDefault: () => void; stopPropagation: () => void }) {
    if ((e.key === 'ArrowDown' || e.key === 'ArrowUp') && matches.length) {
      e.preventDefault()
      const n = matches.length
      active = e.key === 'ArrowDown' ? (active + 1) % n : (active - 1 + n) % n
    } else if (e.key === 'Escape' && active >= 0) {
      e.stopPropagation()
      active = -1
    } else if (e.key === 'Enter' && active >= 0 && matches[active]) {
      e.preventDefault()
      pick(matches[active])
    } else if (e.key === 'Enter') {
      e.preventDefault()
      const tag = await resolveToken(draft)
      if (tag) {
        onChange([...selected, tag])
        draft = ''
      }
    } else if (e.key === 'Backspace' && !draft && selected.length) {
      onChange(selected.slice(0, -1))
    }
  }

  function remove(id: number) {
    onChange(selected.filter((t) => t.id !== id))
  }

  /** Accept a typed match: clears the draft so the next tag can be typed. */
  function pick(tag: { id: any; slug?: string; label?: string; color?: string }) {
    acceptHint(tag)
    draft = ''
    active = -1
  }

  function acceptHint(tag: { id: any; slug?: string; label?: string; color?: string }) {
    if (selected.length >= MAX) {
      err = `Не больше ${MAX} тегов`
      return
    }
    if (selected.some((s) => s.id === tag.id)) return
    onChange([...selected, tag])
  }
</script>

<div class="tag-field">
  <div class="tag-field__chips">
    {#each selected as t (t.id)}
      <button
        type="button"
        class="tag-chip"
        style:--tag={t.color || '#9a9a9a'}
        onclick={() => remove(t.id)}
        title="Убрать {t.slug}"
      >
        {t.slug}
        <span aria-hidden="true">×</span>
      </button>
    {/each}
    <input
      class="tag-field__input"
      type="text"
      placeholder={selected.length ? 'ещё тег…' : 'slug, через запятую'}
      bind:value={draft}
      oninput={onInput}
      onkeydown={onKeydown}
      disabled={busy || selected.length >= MAX}
      autocomplete="off"
    />
  </div>
  {#if err}
    <p class="tag-field__err">{err}</p>
  {/if}
  {#if matches.length}
    <div class="tag-field__hints" role="listbox" aria-label="Подходящие теги">
      {#each matches as m, i (m.id)}
        <button
          type="button"
          class="tag-hint tag-hint--match"
          class:tag-hint--active={i === active}
          style:--tag={m.color || '#9a9a9a'}
          role="option"
          aria-selected={i === active}
          onmousedown={(e) => e.preventDefault()}
          onclick={() => pick(m)}
        >
          {m.slug}
        </button>
      {/each}
    </div>
  {:else if hints.length}
    <div class="tag-field__hints" role="list">
      {#each hints as h (h.tag.id)}
        <button
          type="button"
          class="tag-hint"
          style:--tag={h.tag.color || '#9a9a9a'}
          role="listitem"
          onclick={() => acceptHint(h.tag)}
        >
          {h.tag.slug}
        </button>
      {/each}
    </div>
  {/if}
</div>

<style>
  .tag-field {
    display: grid;
    gap: 0.35rem;
  }

  .tag-field__chips {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.3rem;
    min-height: 2rem;
    padding: 0.25rem 0.4rem;
    border: 1px solid var(--color-border, #333);
    border-radius: 4px;
    background: var(--color-bg, #121212);
  }

  .tag-chip {
    display: inline-flex;
    align-items: center;
    gap: 0.25rem;
    padding: 0.1rem 0.4rem;
    border: 1px solid color-mix(in srgb, var(--tag, #9a9a9a) 45%, var(--color-border, #333));
    border-radius: 3px;
    background: transparent;
    color: var(--color-fg-muted, #9a9a9a);
    font-family: var(--font-mono, monospace);
    font-size: 0.7rem;
    cursor: pointer;
  }

  .tag-chip:hover {
    color: var(--color-fg, #e8e8e8);
  }

  .tag-field__input {
    flex: 1 1 6rem;
    min-width: 5rem;
    border: 0;
    background: transparent;
    color: var(--color-fg, #e8e8e8);
    font-family: var(--font-mono, monospace);
    font-size: 0.8rem;
    outline: none;
  }

  .tag-field__err {
    margin: 0;
    font-size: 0.75rem;
    color: var(--color-danger, #b54a3a);
  }

  .tag-field__hints {
    display: flex;
    flex-wrap: wrap;
    gap: 0.3rem;
  }

  .tag-hint {
    padding: 0.12rem 0.45rem;
    border: 1px dashed color-mix(in srgb, var(--tag, #9a9a9a) 40%, var(--color-border, #333));
    border-radius: 3px;
    background: transparent;
    color: var(--color-fg-muted, #9a9a9a);
    font-family: var(--font-mono, monospace);
    font-size: 0.68rem;
    cursor: pointer;
  }

  .tag-hint--match {
    border-style: solid;
  }

  .tag-hint--active {
    color: var(--color-fg, #e8e8e8);
    background: color-mix(in srgb, var(--tag, #9a9a9a) 22%, transparent);
  }

  .tag-hint:hover {
    color: var(--color-fg, #e8e8e8);
    border-style: solid;
  }
</style>
