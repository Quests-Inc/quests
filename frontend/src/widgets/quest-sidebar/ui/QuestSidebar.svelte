<script lang="ts">
  import { tick } from 'svelte'
  import Icon from '@/shared/ui/Icon.svelte'
  import RefText from '@/shared/ui/RefText.svelte'
  import { QuestlineIcon } from '@/entities/questline'
  import ScopePicker from '@/shared/ui/ScopePicker.svelte'
  import FlagHints from '@/shared/ui/FlagHints.svelte'
  import { helpLine } from '@/shared/lib/searchFlags.ts'
  import { statusWord } from '@/shared/lib/questStatus.ts'
  import { isListSelectAll } from '@/shared/lib/listKeys.ts'
  import SidebarTools from '@/shared/ui/SidebarTools.svelte'
  import SidebarPicked from '@/shared/ui/SidebarPicked.svelte'
  import {
    isQuestInactive,
    periodBadge,
    questTimer,
    quantifiedProgress,
    significanceLabel,
    statusColor,
  } from '@/entities/quest'

  let {
    loading,
    quests,
    matchedQuests,
    listedQuests,
    byCategory,
    frozenQuests,
    frozenOpen,
    failedInboxQuests = [],
    failedOpen = true,
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
    pickedIds = new Set<number>(),
    onRowClick,
    onClearPicked,
    onSelectAll,
    onTogglePin,
    onQuestContextMenu,
    onLineContextMenu,
    onToggleCategory,
    onToggleLine,
    onToggleFrozen,
    onToggleFailed,
    categories,
    scopeCategoryId,
    scopeQuestlineId,
    scopeQuestlinesFor,
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
    failedInboxQuests?: any[],
    failedOpen?: boolean,
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
    /** Quests picked with Ctrl/Shift-click. */
    pickedIds?: ReadonlySet<number>,
    /** Ctrl/Shift-click goes here first: true = taken into the selection, the row is not opened. */
    onRowClick?: (event: MouseEvent, id: number, order: number[]) => boolean,
    onClearPicked?: () => void,
    onSelectAll?: (ids: number[]) => void,
    onTogglePin: (quest: any, event: Event) => void,
    onQuestContextMenu: (event: MouseEvent, quest: any) => void,
    onLineContextMenu: (event: MouseEvent, line: any) => void,
    onToggleCategory: (key: string) => void,
    onToggleLine: (catKey: string, lineKey: string) => void,
    onToggleFrozen: () => void,
    onToggleFailed?: () => void,
    categories: any[],
    scopeCategoryId: number | 'none' | null,
    scopeQuestlineId: number | null,
    scopeQuestlinesFor: (cat: number | 'none') => { id: number; title: string }[],
    onScopeCategory: (id: number | 'none' | null) => void,
    onScopeQuestline: (id: number | null) => void,
    labels?: Record<string, string>,
    onRef?: (kind: string, id: number) => void,
  } = $props()

  /** The picker chooses a category and, optionally, one of its questlines in one go. */
  function pickScope(category: number | 'none' | null, questline: number | null) {
    if (category !== scopeCategoryId) onScopeCategory(category)
    if (questline !== scopeQuestlineId || category !== scopeCategoryId) onScopeQuestline(questline)
  }

  let rootEl: HTMLElement | null = $state(null)

  /** Quest ids in the order the rows are on screen now (folded groups are not there). */
  function visibleOrder() {
    const ids = [...(rootEl?.querySelectorAll<HTMLElement>('.quest-row[data-qid]') ?? [])].map((el) => Number(el.dataset.qid))
    return [...new Set(ids)]
  }

  function onKey(e: KeyboardEvent) {
    if (!isListSelectAll(e)) return
    e.preventDefault()
    onSelectAll?.(visibleOrder())
  }

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

<svelte:window onkeydown={onKey} />

<aside class="sidebar" data-nav-root bind:this={rootEl}>
  <SidebarTools
    bind:value={searchQuery}
    label="Поиск по названию, разделу, квестлайну, описанию, шагам"
    hint={`Фильтры: ${helpLine(querySpec)}; -key:value — исключить`}
  >
    <FlagHints value={searchQuery} spec={querySpec} parsed={parsedSearch} onPick={(v) => (searchQuery = v)} />
    <div class="sidebar__scope">
      <div class="sidebar__scope-pick">
        <ScopePicker
          categories={categories}
          category={scopeCategoryId}
          questline={scopeQuestlineId}
          linesFor={scopeQuestlinesFor}
          onPick={pickScope}
        />
      </div>
      <button
        type="button"
        class="sidebar__showall"
        class:sidebar__showall--on={showAllQuests}
        aria-pressed={showAllQuests}
        aria-label="Показывать завершённые"
        title={showAllQuests ? 'Показаны и завершённые — клик: только открытые' : 'Только открытые — клик: показать и завершённые'}
        onclick={() => onShowAll?.(!showAllQuests)}
      >
        <Icon name={showAllQuests ? 'eye' : 'eye-off'} size={16} />
      </button>
    </div>
  </SidebarTools>
  {#snippet questRow(q: any)}
    {@const rowTimer = questTimer(q, nowMs)}
    {@const frac = quantifiedProgress(q)}
    <button
      type="button"
      data-nav
      class="quest-row"
      class:quest-row--active={q.id === selectedId}
      class:quest-row--picked={pickedIds.has(q.id)}
      data-qid={q.id}
      class:quest-row--pinned={q.pinned}
      class:quest-row--inactive={isQuestInactive(q) && q.status !== 'frozen' && !(q.status === 'failed' && q.failure_inbox)}
      style:--tag-n={q.tags?.length || 0}
      onclick={(e) => {
        if (onRowClick?.(e, q.id, visibleOrder())) return
        onSelect(q.id)
      }}
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
              <span class="status" style:color={statusColor(q.status)} title={q.status}>{statusWord(q.status)}</span>
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
    {#if failedInboxQuests.length > 0}
      <div class="sidebar__failed">
        <button
          type="button"
          data-nav
          class="sidebar__failed-toggle"
          aria-expanded={failedOpen}
          onclick={onToggleFailed}
        >
          <span class="sidebar__failed-label">Провалено</span>
          <span class="sidebar__failed-hint">{failedInboxQuests.length}</span>
          <span class="sidebar__failed-chevron" aria-hidden="true">
            <Icon name={failedOpen ? 'chevron-down' : 'chevron-right'} size={12} />
          </span>
        </button>
        {#if failedOpen}
          <div class="sidebar__failed-body">
            {#each failedInboxQuests as q (q.id)}
              {@render questRow(q)}
            {/each}
          </div>
        {/if}
      </div>
    {/if}
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
      <p class="empty">Нет активных — покажите завершённые кнопкой с глазом рядом с разделом</p>
    {:else}
      {#snippet categoryBody(g: any)}
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
  <SidebarPicked count={pickedIds.size} onClear={() => onClearPicked?.()} />
</aside>
