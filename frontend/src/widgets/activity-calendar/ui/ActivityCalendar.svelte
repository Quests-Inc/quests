<script lang="ts">
  import { tick } from 'svelte'
  import { slideTab } from '@/app/model/slideTab.ts'
  import { statusColor } from '@/entities/quest'
  import { QuestlineIcon } from '@/entities/questline'
  import Icon from '@/shared/ui/Icon.svelte'
  import OptionPills from '@/shared/ui/OptionPills.svelte'
  import RefText from '@/shared/ui/RefText.svelte'
  import ContextMenu from '@/shared/ui/ContextMenu.svelte'
  import type { EventRead } from '@/shared/api/journal.gen'
  import { dayKey, eventColor, parseDayKey } from '../model/day.ts'
  import {
    buildBlocks,
    buildDayLoad,
    eventsByDayMap,
    questsByDayMap,
  } from '../model/dayLoad.ts'
  import {
    CAL_SCALES,
    type CalScale,
    periodBounds,
    periodKey as makePeriodKey,
    periodLabel,
    shiftPeriodAnchor,
  } from '../model/period.ts'

  let {
    quests = [],
    events = [],
    labels = {},
    focusReq = null,
    onRef,
    onSelectQuest,
    onQuestContextMenu,
    onOpenHistory,
    onCreateEvent,
    onEditEvent,
  }: {
    quests?: any[],
    events?: EventRead[],
    labels?: Record<string, string>,
    /** Jump to this day (period + select + scroll). Bump `n` to re-trigger the same day. */
    focusReq?: { key: string; n: number } | null,
    onRef?: (kind: string, id: number) => void,
    onSelectQuest?: (id: number) => void,
    onQuestContextMenu?: (event: MouseEvent, id: number) => void,
    onOpenHistory?: (dayKey: string) => void,
    onCreateEvent?: (dayKey: string) => void,
    onEditEvent?: (event: EventRead) => void,
  } = $props()

  function openQuestMenu(event: MouseEvent, id: number) {
    event.preventDefault(); event.stopPropagation(); onQuestContextMenu?.(event, id)
  }

  function openDayMenu(event: MouseEvent, key: string) {
    if ((event.target as HTMLElement).closest?.('.cal-card')) return
    event.preventDefault(); event.stopPropagation()
    dayMenu = { open: true, x: event.clientX, y: event.clientY, key }
  }

  function onDayMenu(id: string) {
    const key = dayMenu.key
    dayMenu.open = false
    if (id === 'create') onCreateEvent?.(key)
    if (id === 'history') onOpenHistory?.(key)
  }

  let dayMenuItems = $derived([
    ...(onCreateEvent ? [{ id: 'create', label: 'Создать событие' }] : []),
    ...(onOpenHistory ? [{ id: 'history', label: 'История дня' }] : []),
  ])
  const WEEKDAYS = ['пн', 'вт', 'ср', 'чт', 'пт', 'сб', 'вс']
  const HOUR_TICKS = [0, 6, 12, 18, 24]
  const SCALE_OPTS = CAL_SCALES.map((s) => ({ id: s.id, label: s.label, short: s.short }))

  const initial = periodBounds('month', new Date())
  let scale: CalScale = $state('month')
  let rangeFrom: string = $state(initial.from)
  let rangeTo: string = $state(initial.to)
  let periodDir = $state(1)
  let selectedKey: string | null = $state(null)
  let hoveredId: string | null = $state(null)

  let rootEl: HTMLElement | null = $state(null)
  let dayMenu = $state({ open: false, x: 0, y: 0, key: '' })

  let period = $derived({ from: rangeFrom, to: rangeTo })
  let slideKey = $derived(makePeriodKey(scale, period.from, period.to))
  let navLabel = $derived(periodLabel(scale, period.from, period.to))

  function applyPeriod(next: CalScale, anchor: Date, dir: number) {
    const b = periodBounds(next, anchor)
    if (b.from !== rangeFrom || b.to !== rangeTo || next !== scale) {
      periodDir = dir
      scale = next
      rangeFrom = b.from
      rangeTo = b.to
    }
    if (selectedKey && (selectedKey < b.from || selectedKey > b.to)) {
      selectedKey = null
      hoveredId = null
    }
  }

  function setScale(next: CalScale) {
    if (next === scale) return
    const anchor = selectedKey ? parseDayKey(selectedKey) : new Date()
    const b = periodBounds(next, anchor)
    const dir = b.from > rangeFrom ? 1 : b.from < rangeFrom ? -1 : 1
    applyPeriod(next, anchor, dir)
  }

  function stepPeriod(dir: number) {
    const next = shiftPeriodAnchor(scale, parseDayKey(rangeFrom), dir)
    applyPeriod(scale, next, dir)
  }

  async function goToDay(key: string) {
    const dir = key > rangeTo ? 1 : key < rangeFrom ? -1 : periodDir
    applyPeriod(scale, parseDayKey(key), dir)
    selectedKey = key
    hoveredId = null
    await tick()
    scrollToSelectedDay()
  }

  async function goToday() {
    await goToDay(dayKey(new Date()))
  }

  let lastFocusN = -1
  $effect(() => {
    const req = focusReq
    if (!req || req.n === lastFocusN) return
    lastFocusN = req.n
    void goToDay(req.key)
  })

  let questsByDay = $derived(questsByDayMap(quests))
  let eventsByDay = $derived(eventsByDayMap(events))
  let months = $derived(buildBlocks(scale, period.from, period.to, questsByDay))
  let todayKey = $derived(dayKey(new Date()))
  let dayLoad = $derived(selectedKey ? buildDayLoad(selectedKey, quests, events) : null)

  let hoveredCaption = $derived.by(() => {
    if (!hoveredId || !dayLoad) return ''
    const id = Number(hoveredId.slice(2))
    if (hoveredId.startsWith('q:')) return dayLoad.rows.find((b) => b.id === id)?.caption ?? ''
    if (hoveredId.startsWith('e:')) return dayLoad.eventRows.find((b) => b.id === id)?.caption ?? ''
    return ''
  })

  /** @returns whether day is now selected */
  function selectDay(key: string): boolean {
    const next = selectedKey === key ? null : key
    selectedKey = next
    hoveredId = null
    return next != null
  }

  async function shiftSelectedDay(delta: number) {
    if (!selectedKey) return
    const d = parseDayKey(selectedKey)
    d.setDate(d.getDate() + delta)
    await goToDay(dayKey(d))
  }

  function scrollToSelectedDay() {
    if (!selectedKey || !rootEl) return
    const cell = rootEl.querySelector(`[data-day-key="${selectedKey}"]`)
    cell?.scrollIntoView({ behavior: 'smooth', block: 'center' })
  }

  function scrollToDayPanel() {
    if (!rootEl) return
    const panel = rootEl.querySelector('.day-load')
    panel?.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
  }

  async function onCellClick(key: string, e: MouseEvent) {
    const t = (e.target as HTMLElement)
    if (t.closest?.('.cal-card, .cal-event')) return
    const open = selectDay(key)
    if (open) {
      await tick()
      scrollToDayPanel()
    }
  }
</script>

{#snippet eventChip(event: EventRead, extra: string)}
  <button
    type="button"
    class="cal-event {extra}"
    style:--event-color={eventColor(event.color)}
    title={event.title}
    onclick={() => onEditEvent?.(event)}
  >
    <span class="cal-event__title">{event.title}</span>
    <span class="cal-event__icon">
      <QuestlineIcon icon={event.icon || 'calendar'} iconUrl={event.icon_url || null} size="sm" />
    </span>
  </button>
{/snippet}

<div class="activity-cal" aria-label="Календарь задач" bind:this={rootEl}>
  <header class="activity-cal__head">
    <h2 class="activity-cal__title">Календарь</h2>
  </header>
  <div class="activity-cal__toolbar">
    <div class="activity-cal__scale">
      <OptionPills
        options={SCALE_OPTS}
        value={scale}
        compact
        label="Масштаб календаря"
        onToggle={(id) => setScale(id as CalScale)}
      />
    </div>
    <div class="activity-cal__period-nav" role="group" aria-label="Листание периода">
      <button
        type="button"
        class="btn day-load__icon-btn"
        aria-label="Предыдущий период"
        onclick={() => stepPeriod(-1)}
      >
        ‹
      </button>
      <span class="activity-cal__period-label">{navLabel}</span>
      <button
        type="button"
        class="btn day-load__icon-btn"
        aria-label="Следующий период"
        onclick={() => stepPeriod(1)}
      >
        ›
      </button>
    </div>
    <button
      type="button"
      class="btn activity-cal__today"
      onclick={goToday}
      title="Сегодня"
      aria-label="Сегодня"
    >
      <span class="activity-cal__today-text">Сегодня</span>
      <span class="activity-cal__today-icon" aria-hidden="true">
        <Icon name="calendar" size={16} />
      </span>
    </button>
  </div>

  <div class="activity-cal__stage">
    {#key slideKey}
      <div
        class="activity-cal__slide"
        in:slideTab={{ dir: periodDir }}
        out:slideTab={{ dir: periodDir, out: true }}
      >
        <div class="activity-cal__months">
          {#each months as month (month.key)}
            <section class="activity-cal__month">
              <h3 class="activity-cal__month-label">{month.label}</h3>
              <div class="activity-cal__weekdays" aria-hidden="true">
                {#each WEEKDAYS as w}
                  <span>{w}</span>
                {/each}
              </div>
              <div class="activity-cal__grid" role="grid" aria-label={month.label}>
                {#each month.cells as cell}
                  {#if cell}
                    <div
                      class="activity-cal__cell"
                      class:activity-cal__cell--today={cell.key === todayKey}
                      class:activity-cal__cell--selected={cell.key === selectedKey}
                      role="gridcell"
                      tabindex="0"
                      data-day-key={cell.key}
                      aria-label="{cell.key}: {cell.items.length} задач"
                      aria-selected={cell.key === selectedKey}
                      onclick={(e) => onCellClick(cell.key, e)}
                      oncontextmenu={(e) => openDayMenu(e, cell.key)}
                      onkeydown={async (e) => {
                        if (e.key === 'Enter' || e.key === ' ') {
                          e.preventDefault()
                          const open = selectDay(cell.key)
                          if (open) {
                            await tick()
                            scrollToDayPanel()
                          }
                        }
                      }}
                    >
                      <div class="activity-cal__cell-head">
                        <span class="activity-cal__daynum">{cell.day}</span>
                        {#if cell.items.length > 0}
                          {@const doneN = cell.items.filter((i) => i.done).length}
                          <span class="activity-cal__count">
                            {#if doneN > 0}
                              <span class="activity-cal__done" title="Выполнено: {doneN}">
                                <Icon name="check" size={10} />{doneN}
                              </span>
                            {/if}
                            {cell.items.length}
                          </span>
                        {/if}
                      </div>
                      <div class="activity-cal__cards">
                        {#each eventsByDay[cell.key] || [] as event (event.id)}
                          {@render eventChip(event, '')}
                        {/each}
                        {#if (eventsByDay[cell.key]?.length ?? 0) > 0 && cell.items.length > 0}
                          <div class="activity-cal__split" role="separator"></div>
                        {/if}
                        {#each cell.items as item (item.id)}
                          <button
                            type="button"
                            class="cal-card"
                            class:cal-card--done={item.done}
                            data-sig={item.significance}
                            data-status={item.status}
                            title={item.title}
                            onclick={() => onSelectQuest?.(item.id)}
                            oncontextmenu={(e) => openQuestMenu(e, item.id)}
                          >
                            <span
                              class="cal-card__status"
                              style:background={statusColor(item.status)}
                              aria-hidden="true"
                            ></span>
                            <RefText class="cal-card__title" source={item.title} {labels} {onRef} />
                            <span class="cal-card__time">{item.timeLabel}</span>
                          </button>
                        {/each}
                      </div>
                    </div>
                  {:else}
                    <div class="activity-cal__cell activity-cal__cell--empty" aria-hidden="true"></div>
                  {/if}
                {/each}
              </div>
            </section>
          {/each}
        </div>
      </div>
    {/key}
  </div>

  {#if dayLoad}
    <section class="day-load" aria-label="Загрузка дня">
      <header class="day-load__head">
        <div class="day-load__nav">
          <button
            type="button"
            class="btn day-load__icon-btn"
            aria-label="Предыдущий день"
            onclick={() => shiftSelectedDay(-1)}
          >
            ‹
          </button>
          <h3 class="day-load__title">{dayLoad.title}</h3>
          <button
            type="button"
            class="btn day-load__icon-btn"
            aria-label="Следующий день"
            onclick={() => shiftSelectedDay(1)}
          >
            ›
          </button>
        </div>
        <div class="day-load__actions">
          {#if selectedKey && onOpenHistory}
            <button
              type="button"
              class="btn day-load__icon-btn"
              onclick={() => onOpenHistory?.(selectedKey ?? '')}
              title="История дня"
              aria-label="История дня"
            >
              <Icon name="scroll" size={16} />
            </button>
          {/if}
          <button
            type="button"
            class="btn day-load__icon-btn"
            onclick={scrollToSelectedDay}
            title="К дню в сетке"
            aria-label="К дню в сетке"
          >
            <Icon name="arrow-up" size={16} />
          </button>
          <button
            type="button"
            class="btn day-load__icon-btn"
            aria-label="Закрыть"
            onclick={() => {
              selectedKey = null
              hoveredId = null
            }}
          >
            <Icon name="close" size={14} />
          </button>
        </div>
      </header>

      <div class="day-load__body">
        {#if selectedKey && (eventsByDay[selectedKey] || []).length}
          <div class="cal-events">
            <h4 class="cal-events__title">События</h4>
            <ul class="cal-events__list">
              {#each eventsByDay[selectedKey] as event (event.id)}
                <li>
                  {@render eventChip(event, 'cal-events__item')}
                </li>
              {/each}
            </ul>
          </div>
        {/if}
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div class="day-board" aria-label="Доска дня" onmouseleave={() => (hoveredId = null)}>
          <div class="day-board__axis" aria-hidden="true">
            <div class="day-board__gutter"></div>
            <div class="day-tl__hours">
              {#each HOUR_TICKS as h}
                <span style:left="{(h / 24) * 100}%">{String(h).padStart(2, '0')}</span>
              {/each}
            </div>
          </div>

          {#if dayLoad.eventRows.length === 0 && dayLoad.rows.length === 0}
            <p class="day-tl__empty">Нет задач с окном и нет событий в этот день</p>
          {:else}
            <div
              class="day-board__rows"
              role="list"
              aria-label="{dayLoad.eventRows.length} событий, {dayLoad.rows.length} квестов"
            >
              {#each dayLoad.eventRows as row (row.id)}
                <div
                  class="day-board__row"
                  class:day-board__row--hover={hoveredId === `e:${row.id}`}
                  role="listitem"
                  onmouseenter={() => (hoveredId = `e:${row.id}`)}
                >
                  <button
                    type="button"
                    class="day-board__label"
                    title={row.tip}
                    onclick={() => onEditEvent?.(row.event)}
                  >
                    <span class="day-board__icon" style:color={row.color} aria-hidden="true">
                      <QuestlineIcon icon={row.icon} iconUrl={row.iconUrl} size="sm" />
                    </span>
                    <span class="day-board__title">{row.title}</span>
                  </button>
                  <div class="day-board__track">
                    {#each HOUR_TICKS as h}
                      <div class="day-tl__gridline" style:left="{(h / 24) * 100}%"></div>
                    {/each}
                    <button
                      type="button"
                      class="day-tl__bar day-board__bar day-tl__bar--event"
                      class:day-tl__bar--point={row.point}
                      class:day-tl__bar--from-prev={row.fromPrev}
                      class:day-tl__bar--to-next={row.toNext}
                      class:day-tl__bar--hover={hoveredId === `e:${row.id}`}
                      style:--event-color={row.color}
                      style:left="{row.left * 100}%"
                      style:width="{Math.max(row.width * 100, row.point ? 0.35 : 0.4)}%"
                      title={row.tip}
                      onclick={() => onEditEvent?.(row.event)}
                    ></button>
                  </div>
                </div>
              {/each}
              {#each dayLoad.rows as row (row.id)}
                <div
                  class="day-board__row"
                  class:day-board__row--hover={hoveredId === `q:${row.id}`}
                  role="listitem"
                  onmouseenter={() => (hoveredId = `q:${row.id}`)}
                >
                  <button
                    type="button"
                    class="day-board__label"
                    title={row.tip}
                    onclick={() => onSelectQuest?.(row.id)}
                    oncontextmenu={(e) => openQuestMenu(e, row.id)}
                  >
                    <span
                      class="day-board__status"
                      style:background={statusColor(row.status)}
                      aria-hidden="true"
                    ></span>
                    <RefText class="day-board__title" source={row.title} {labels} {onRef} />
                  </button>
                  <div class="day-board__track">
                    {#each HOUR_TICKS as h}
                      <div class="day-tl__gridline" style:left="{(h / 24) * 100}%"></div>
                    {/each}
                    <button
                      type="button"
                      class="day-tl__bar day-board__bar"
                      class:day-tl__bar--point={row.point}
                      class:day-tl__bar--from-prev={row.fromPrev}
                      class:day-tl__bar--to-next={row.toNext}
                      class:day-tl__bar--hover={hoveredId === `q:${row.id}`}
                      data-sig={row.significance}
                      data-status={row.status}
                      style:left="{row.left * 100}%"
                      style:width="{Math.max(row.width * 100, row.point ? 0.35 : 0.4)}%"
                      title={row.tip}
                      onclick={() => onSelectQuest?.(row.id)}
                      oncontextmenu={(e) => openQuestMenu(e, row.id)}
                    ></button>
                    {#if row.marker != null}
                      <span
                        class="day-tl__mark"
                        class:day-tl__mark--hover={hoveredId === `q:${row.id}`}
                        data-sig={row.significance}
                        data-kind={row.markKind}
                        style:left="{row.marker * 100}%"
                        title={row.tip}
                        aria-hidden="true"
                      ></span>
                    {/if}
                  </div>
                </div>
              {/each}
            </div>
          {/if}

          <div class="day-tl__caption" aria-live="polite">
            {#if hoveredCaption}
              {hoveredCaption}
            {:else}
              <span class="day-tl__caption-hint">наведите на строку или событие</span>
            {/if}
          </div>
        </div>

        {#if dayLoad.noWindow.length > 0}
          <div class="day-nowin">
            <h4 class="day-events__title">Без окна (выданы сегодня)</h4>
            <ul class="day-nowin__list">
              {#each dayLoad.noWindow as q (q.id)}
                <li
                  class="day-nowin__item"
                  class:day-nowin__item--hover={hoveredId === `q:${q.id}`}
                  onmouseenter={() => (hoveredId = `q:${q.id}`)}
                  onmouseleave={() => (hoveredId = null)}
                >
                  <time class="day-events__time">{q.timeLabel}</time>
                  <button
                    type="button"
                    class="day-events__quest"
                    onclick={() => onSelectQuest?.(q.id)}
                    oncontextmenu={(e) => openQuestMenu(e, q.id)}
                  >
                    <RefText source={q.title} {labels} {onRef} />
                  </button>
                </li>
              {/each}
            </ul>
          </div>
        {/if}

        <div class="day-events">
          <h4 class="day-events__title">История дня</h4>
          {#if dayLoad.events.length === 0}
            <p class="day-events__empty">Нет выдач и смен статуса</p>
          {:else}
            <ul class="day-events__list">
              {#each dayLoad.events as ev (ev.kind + '-' + ev.quest.id + '-' + ev.at.getTime())}
                <li
                  class="day-events__item"
                  class:day-events__item--hover={hoveredId === `q:${Number(ev.quest.id)}`}
                  onmouseenter={() => (hoveredId = `q:${Number(ev.quest.id)}`)}
                  onmouseleave={() => (hoveredId = null)}
                >
                  <time class="day-events__time">{ev.timeLabel}</time>
                  <span
                    class="day-events__kind"
                    data-kind={ev.kind}
                    style:--kind-color={statusColor(
                      ev.kind === 'issued' ? 'active' : ev.kind,
                    )}
                  >
                    {ev.label}
                  </span>
                  <button
                    type="button"
                    class="day-events__quest"
                    onclick={() => onSelectQuest?.(Number(ev.quest.id))}
                    oncontextmenu={(e) => openQuestMenu(e, Number(ev.quest.id))}
                  >
                    <RefText source={ev.quest.title || '?'} {labels} {onRef} />
                  </button>
                </li>
              {/each}
            </ul>
          {/if}
        </div>
      </div>
    </section>
  {/if}
</div>

<ContextMenu
  open={dayMenu.open}
  x={dayMenu.x}
  y={dayMenu.y}
  items={dayMenuItems}
  onSelect={onDayMenu}
  onClose={() => (dayMenu.open = false)}
/>
