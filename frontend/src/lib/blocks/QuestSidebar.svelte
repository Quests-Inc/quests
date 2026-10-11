<script lang="ts">
  import { tick } from 'svelte'
  import Icon from '../ui/Icon.svelte'
  import RefText from '../ui/RefText.svelte'
  import QuestlineIcon from '../ui/QuestlineIcon.svelte'
  import FlagHints from '../ui/FlagHints.svelte'
  import { helpLine } from '../js/searchFlags.ts'
  import {
    isQuestInactive,
    periodBadge,
    questTimer,
    quantifiedProgress,
    significanceLabel,
    statusColor,
  } from '../js/questFormat.ts'

  /** @type {{
   *   loading: boolean,
   *   quests: any[],
   *   matchedQuests: any[],
   *   listedQuests: any[],
   *   byCategory: any[],
   *   frozenQuests: any[],
   *   frozenOpen: boolean,
   *   selectedId: number | null,
   *   searchQuery: string,
   *   showAllQuests: boolean,
   *   onShowAll?: (on: boolean) => void,
   *   querySpec: any,
   *   parsedSearch: any,
   *   categoryOpen: Record<string, boolean>,
   *   lineOpen: Record<string, boolean>,
   *   nowMs: number,
   *   onSelect: (id: number) => void,
   *   onTogglePin: (quest: any, event: Event) => void,
   *   onQuestContextMenu: (event: MouseEvent, quest: any) => void,
   *   onLineContextMenu: (event: MouseEvent, line: any) => void,
   *   onToggleCategory: (key: string) => void,
   *   onToggleLine: (catKey: string, lineKey: string) => void,
   *   onToggleFrozen: () => void,
   *   categories: any[],
   *   scopeCategoryId: number | null,
   *   scopeQuestlineId: number | null,
   *   scopeQuestlineOptions: any[],
   *   onScopeCategory: (id: number | null) => void,
   *   onScopeQuestline: (id: number | null) => void,
   *   labels?: Record<string, string>,
   *   onRef?: (kind: string, id: number) => void,
   * }} */
  let {
    loading,
    quests,
    matchedQuests,
    listedQuests,
    byCategory,
    frozenQuests,
    frozenOpen,
    selectedId,
    searchQuery = $bindable(''),
    showAllQuests = false,
    onShowAll,
    querySpec,
    parsedSearch,
    categoryOpen,
    lineOpen,
    nowMs,
    onSelect,
    onTogglePin,
    onQuestContextMenu,
    onLineContextMenu,
    onToggleCategory,
    onToggleLine,
    onToggleFrozen,
    categories,
    scopeCategoryId,
    scopeQuestlineId,
    scopeQuestlineOptions,
    onScopeCategory,
    onScopeQuestline,
    labels = {},
    onRef,
  }: {
    loading: boolean,
    quests: any[],
    matchedQuests: any[],
    listedQuests: any[],
    byCategory: any[],
    frozenQuests: any[],
    frozenOpen: boolean,
    selectedId: number | null,
    searchQuery: string,
    showAllQuests: boolean,
    onShowAll?: (on: boolean) => void,
    querySpec: any,
    parsedSearch: any,
    categoryOpen: Record<string, boolean>,
    lineOpen: Record<string, boolean>,
    nowMs: number,
    onSelect: (id: number) => void,
    onTogglePin: (quest: any, event: Event) => void,
    onQuestContextMenu: (event: MouseEvent, quest: any) => void,
    onLineContextMenu: (event: MouseEvent, line: any) => void,
    onToggleCategory: (key: string) => void,
    onToggleLine: (catKey: string, lineKey: string) => void,
    onToggleFrozen: () => void,
    categories: any[],
    scopeCategoryId: number | null,
    scopeQuestlineId: number | null,
    scopeQuestlineOptions: any[],
    onScopeCategory: (id: number | null) => void,
    onScopeQuestline: (id: number | null) => void,
    labels?: Record<string, string>,
    onRef?: (kind: string, id: number) => void,
  } = $props()

  function onScopeCategoryChange(event: { currentTarget: { value: any } }) {
    const v = event.currentTarget.value
    onScopeCategory(v === '' ? null : Number(v))
  }
  function onScopeQuestlineChange(event: { currentTarget: { value: any } }) {
    const v = event.currentTarget.value
    onScopeQuestline(v === '' ? null : Number(v))
  }

  /** @type {HTMLElement | null} */
  let rootEl: HTMLElement | null = $state(null)

  /** The nearest ancestor that actually scrolls. */
  function scrollParent(el: HTMLElement) {
    for (let p = el.parentElement; p; p = p.parentElement) {
      const oy = getComputedStyle(p).overflowY
      if ((oy === 'auto' || oy === 'scroll') && p.scrollHeight > p.clientHeight) return p
    }
    return null
  }

  // A quest picked from elsewhere (a link, a hotkey, the map) may sit far down
  // a long list: bring its row into view, centred, unless it is already visible.
  let scrolledFor = (null as number | null)
  $effect(() => {
    const id = selectedId
    // The list may not be there yet when the quest is chosen: re-check as it fills.
    void byCategory
    void listedQuests
    void lineOpen
    void categoryOpen
    if (id == null || !rootEl || scrolledFor === id) return
    void tick().then(() => {
      const row = rootEl?.querySelector('.quest-row--active')
      if (!row) return
      scrolledFor = id
      const box = scrollParent((row as HTMLElement))
      if (!box) return
      const a = row.getBoundingClientRect()
      const b = box.getBoundingClientRect()
      if (a.top < b.top || a.bottom > b.bottom) row.scrollIntoView({ block: 'center' })
    })
  })

  function isCategoryOpen(key: string|number) {
    return categoryOpen[key] !== false
  }

  function isLineOpen(catKey: any, lineKey: any) {
    return lineOpen[`${catKey}:${lineKey}`] !== false
  }
</script>

<aside class="sidebar" data-nav-root bind:this={rootEl}>
  <div class="sidebar__tools">
    <input
      class="search"
      type="search"
      placeholder="Поиск…"
      bind:value={searchQuery}
      aria-label="Поиск по названию, разделу, квестлайну, описанию, шагам"
      title={`Фильтры: ${helpLine(querySpec)}; -key:value — исключить`}
    />
    <FlagHints value={searchQuery} spec={querySpec} parsed={parsedSearch} onPick={(v) => (searchQuery = v)} />
    <label class="sidebar__filter">
      <input type="checkbox" checked={showAllQuests} onchange={(e) => onShowAll?.(e.currentTarget.checked)} />
      <span>Показывать завершённые</span>
    </label>
    <div class="sidebar__scope">
      <select
        class="sidebar__scope-select"
        value={scopeCategoryId ?? ''}
        onchange={onScopeCategoryChange}
        aria-label="Сузить до раздела"
      >
        <option value="">Все разделы</option>
        {#each categories as cat (cat.id)}
          <option value={cat.id}>{cat.label}</option>
        {/each}
      </select>
      {#if scopeCategoryId != null}
        <select
          class="sidebar__scope-select"
          value={scopeQuestlineId ?? ''}
          onchange={onScopeQuestlineChange}
          aria-label="Сузить до квестлайна"
        >
          <option value="">Весь раздел</option>
          {#each scopeQuestlineOptions as line (line.id)}
            <option value={line.id}>{line.title}</option>
          {/each}
        </select>
      {/if}
    </div>
  </div>
  {#snippet questRow(q)}
    {@const rowTimer = questTimer(q, nowMs)}
    {@const frac = quantifiedProgress(q)}
    <button
      type="button"
      data-nav
      class="quest-row"
      class:quest-row--active={q.id === selectedId}
      class:quest-row--pinned={q.pinned}
      class:quest-row--inactive={isQuestInactive(q) && q.status !== 'frozen'}
      style:--tag-n={q.tags?.length || 0}
      onclick={() => onSelect(q.id)}
      oncontextmenu={(e) => onQuestContextMenu(e, q)}
    >
      <span class="quest-row__top">
        <RefText class="quest-row__title" source={q.title} {labels} {onRef} />
        <span
          class="pin-btn"
          class:pin-btn--on={q.pinned}
          class:pin-btn--off={q.overlay_hidden}
          role="button"
          tabindex="0"
          title={q.pinned ? 'Всегда в оверлее — клик: никогда' : q.overlay_hidden ? 'Никогда в оверлее — клик: по умолчанию' : 'Оверлей по умолчанию — клик: всегда'}
          aria-label={q.pinned ? 'Всегда в оверлее' : q.overlay_hidden ? 'Никогда в оверлее' : 'Оверлей по умолчанию'}
          onclick={(e) => onTogglePin(q, e)}
          onkeydown={(e) => {
            if (e.key === 'Enter' || e.key === ' ') onTogglePin(q, e)
          }}
        >
          <Icon name={q.pinned ? 'pin-filled' : q.overlay_hidden ? 'pin-off' : 'pin'} size={14} />
        </span>
      </span>
      {#if q.tags?.length}
        <span class="quest-row__tags" aria-hidden="true">
          {#each q.tags as t (t.id)}
            <span class="quest-row__tag" style:--tag={t.color || '#9a9a9a'}>{t.label || t.slug}</span>
          {/each}
        </span>
      {/if}
      {#if q.significance && q.significance !== 'common'}
        <span class="quest-row__sig" data-sig={q.significance}>{significanceLabel(q)}</span>
      {/if}
      {#if q.status !== 'active' || periodBadge(q) || rowTimer || frac}
        <span class="quest-row__meta">
          <span class="quest-row__meta-left">
            {#if q.status !== 'active'}
              <span class="status" style:color={statusColor(q.status)}>{q.status}</span>
            {/if}
            {#if periodBadge(q)}
              <span class="period-badge" title="Периодический инстанс">{periodBadge(q)}</span>
            {/if}
            {#if rowTimer}
              <span class="row-timer" data-tone={rowTimer.tone}>{rowTimer.label}</span>
            {/if}
          </span>
          {#if frac}
            <span class="progress">{frac}</span>
          {/if}
        </span>
      {/if}
    </button>
  {/snippet}

  <div class="sidebar__list" aria-label="Список квестов">
    {#if frozenQuests.length > 0}
      <div class="sidebar__frozen">
        <button
          type="button"
          data-nav
          class="sidebar__frozen-toggle"
          aria-expanded={frozenOpen}
          onclick={onToggleFrozen}
        >
          <span class="sidebar__frozen-label">Заморожено</span>
          <span class="sidebar__frozen-hint">{frozenQuests.length}</span>
          <span class="sidebar__frozen-chevron" aria-hidden="true">
            <Icon name={frozenOpen ? 'chevron-down' : 'chevron-right'} size={12} />
          </span>
        </button>
        {#if frozenOpen}
          <div class="sidebar__frozen-body">
            {#each frozenQuests as q (q.id)}
              {@render questRow(q)}
            {/each}
          </div>
        {/if}
      </div>
    {/if}
    {#if loading}
      <p class="empty">Загрузка…</p>
    {:else if quests.length === 0}
      <p class="empty">Квестов нет</p>
    {:else if matchedQuests.length === 0}
      <p class="empty">Ничего не найдено</p>
    {:else if listedQuests.length === 0}
      <p class="empty">Нет активных — включи «Показывать завершённые»</p>
    {:else}
      {#snippet categoryBody(g)}
        {#if g.lines.length === 0}
          {#each g.alone as q (q.id)}
            {@render questRow(q)}
          {/each}
        {:else}
          {#each g.lines as line (line.key)}
            <div class="quest-line" style="--line-color: {line.color || '#9a9a9a'}">
              <button
                type="button"
                data-nav
                class="quest-line__toggle"
                aria-expanded={isLineOpen(g.key, line.key)}
                onclick={() => onToggleLine(g.key, line.key)}
                oncontextmenu={(e) => onLineContextMenu(e, line)}
              >
                <span class="quest-line__icon" aria-hidden="true">
                  <QuestlineIcon
                    icon={line.icon || 'document'}
                    iconUrl={line.icon_url || null}
                    size="sm"
                  />
                </span>
                <span class="quest-line__label">{line.title}</span>
                <span class="quest-line__hint">{line.quests.length}</span>
                <span class="quest-line__chevron" aria-hidden="true">
                  <Icon
                    name={isLineOpen(g.key, line.key) ? 'chevron-down' : 'chevron-right'}
                    size={12}
                  />
                </span>
              </button>
              {#if isLineOpen(g.key, line.key)}
                <div class="quest-line__body">
                  {#each line.quests as q (q.id)}
                    {@render questRow(q)}
                  {/each}
                </div>
              {/if}
            </div>
          {/each}
          {#if g.alone.length > 0}
            <div class="quest-line quest-line--alone">
              <div class="quest-line__alone-label">Без квестлайна</div>
              <div class="quest-line__body">
                {#each g.alone as q (q.id)}
                  {@render questRow(q)}
                {/each}
              </div>
            </div>
          {/if}
        {/if}
      {/snippet}

      {#each byCategory as g (g.key)}
        <div
          class="quest-subgroup"
          class:quest-subgroup--plain={g.key === 'none'}
          style={g.color ? `--cat-color: ${g.color}` : undefined}
        >
          <button
            type="button"
            data-nav
            class="quest-subgroup__toggle"
            aria-expanded={isCategoryOpen(g.key)}
            onclick={() => onToggleCategory(g.key)}
          >
            <span class="quest-subgroup__swatch" aria-hidden="true"></span>
            <span class="quest-subgroup__label">{g.label}</span>
            <span class="quest-subgroup__hint">{g.questCount}</span>
            <span class="quest-subgroup__chevron" aria-hidden="true">
              <Icon
                name={isCategoryOpen(g.key) ? 'chevron-down' : 'chevron-right'}
                size={12}
              />
            </span>
          </button>
          {#if isCategoryOpen(g.key)}
            <div class="quest-subgroup__body">
              {@render categoryBody(g)}
            </div>
          {/if}
        </div>
      {/each}
    {/if}
  </div>
</aside>
