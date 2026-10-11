<script lang="ts">
  import { tick } from 'svelte'
  import { parseApiDate } from '../js/time.ts'
  import { QUEST_STATUS_LABELS } from '../js/api.ts'
  import { statusColor } from '../js/questFormat.ts'
  import Icon from '../ui/Icon.svelte'
  import DateInput from '../ui/DateInput.svelte'
  import RefText from '../ui/RefText.svelte'

  /** @type {{
   *   quests?: any[],
   *   labels?: Record<string, string>,
   *   onRef?: (kind: string, id: number) => void,
   *   onSelectQuest?: (id: number) => void,
   *   onQuestContextMenu?: (event: MouseEvent, id: number) => void,
   *   onOpenHistory?: (dayKey: string) => void,
   * }} */
  let { quests = [], labels = {}, onRef, onSelectQuest, onQuestContextMenu, onOpenHistory }: {
    quests?: any[],
    labels?: Record<string, string>,
    onRef?: (kind: string, id: number) => void,
    onSelectQuest?: (id: number) => void,
    onQuestContextMenu?: (event: MouseEvent, id: number) => void,
    onOpenHistory?: (dayKey: string) => void,
  } = $props()

  /** @param {MouseEvent} event @param {number} id */
  function openQuestMenu(event: MouseEvent, id: number) {
    event.preventDefault()
    event.stopPropagation()
    onQuestContextMenu?.(event, id)
  }

  const WEEKDAYS = ['пн', 'вт', 'ср', 'чт', 'пт', 'сб', 'вс']
  const HOUR_TICKS = [0, 6, 12, 18, 24]

  /** @type {string} YYYY-MM-DD — same picker as quest modal (`type=date`) */
  let rangeFrom: string = $state(monthStartKey(new Date()))
  /** @type {string} YYYY-MM-DD */
  let rangeTo: string = $state(monthEndKey(new Date()))
  /** @type {string | null} YYYY-MM-DD */
  let selectedKey: string | null = $state(null)
  /** @type {number | null} hovered quest id (timeline ↔ event list) */
  let hoveredId: number | null = $state(null)

  /** @type {HTMLElement | null} */
  let rootEl: HTMLElement | null = $state(null)

  function monthKey(d: Date) {
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`
  }

  function dayKey(d: Date) {
    const y = d.getFullYear()
    const m = String(d.getMonth() + 1).padStart(2, '0')
    const day = String(d.getDate()).padStart(2, '0')
    return `${y}-${m}-${day}`
  }

  function monthStartKey(d = new Date()) {
    return dayKey(new Date(d.getFullYear(), d.getMonth(), 1))
  }

  function monthEndKey(d = new Date()) {
    return dayKey(new Date(d.getFullYear(), d.getMonth() + 1, 0))
  }

  /** @param {string} key */
  function parseDayKey(key: string) {
    const [y, m, d] = key.split('-').map(Number)
    return new Date(y, m - 1, d)
  }

  /** @param {string} key */
  function dayBounds(key: string) {
    const start = parseDayKey(key)
    start.setHours(0, 0, 0, 0)
    const end = new Date(start)
    end.setDate(end.getDate() + 1)
    return { start, end }
  }

  function formatTime(d: Date) {
    return d.toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' })
  }

  function formatDayTitle(key: string) {
    return parseDayKey(key).toLocaleDateString('ru-RU', {
      weekday: 'long',
      day: 'numeric',
      month: 'long',
      year: 'numeric',
    })
  }

  function sigKey(q: { significance: any }) {
    const s = String(q?.significance || 'common').toLowerCase()
    if (['insignificant', 'common', 'uncommon', 'epic', 'legendary'].includes(s)) return s
    return 'common'
  }

  /**
   * Urgent window [deadline − duration, deadline]. Point if no duration.
   * @param {any} q
   * @returns {{ start: Date, end: Date } | null}
   */
  function questWindow(q: any): { start: Date, end: Date } | null {
    const deadline = parseApiDate(q?.deadline_at)
    if (!deadline) return null
    const dur = Number(q.duration_seconds) || 0
    if (dur > 0) {
      return { start: new Date(deadline.getTime() - dur * 1000), end: deadline }
    }
    return { start: deadline, end: deadline }
  }

  /**
   * Planned urgent window, clipped to actual finish when completed/failed.
   * @param {any} q
   * @returns {{ start: Date, end: Date, finish: Date | null, markKind: 'completed' | 'failed' | null } | null}
   */
  function actualWindow(q: any): { start: Date, end: Date, finish: Date | null, markKind: 'completed' | 'failed' | null } | null {
    const planned = questWindow(q)
    if (!planned) return null
    const status = String(q.status || '')
    /** @type {Date | null} */
    let finish: Date | null = null
    /** @type {'completed' | 'failed' | null} */
    let markKind: 'completed' | 'failed' | null = null
    if (status === 'completed') {
      finish = parseApiDate(q.completed_at)
      if (finish) markKind = 'completed'
    } else if (status === 'failed') {
      finish = parseApiDate(q.updated_at)
      if (finish) markKind = 'failed'
    }
    if (!finish) {
      return { start: planned.start, end: planned.end, finish: null, markKind: null }
    }
    // Fact load: from window open to finish (shorten or extend past deadline).
    let start = planned.start
    let end = finish
    if (end.getTime() < start.getTime()) {
      start = finish
      end = finish
    }
    return { start, end, finish, markKind }
  }

  /**
   * @param {{ start: Date, end: Date }} win
   * @param {Date} dayStart
   * @param {Date} dayEnd
   */
  function clipToDay(win: { start: Date, end: Date }, dayStart: Date, dayEnd: Date) {
    const s = Math.max(win.start.getTime(), dayStart.getTime())
    const e = Math.min(win.end.getTime(), dayEnd.getTime())
    if (e < s) return null
    if (e === s) return { startMs: s, endMs: s, point: true }
    return { startMs: s, endMs: e, point: false }
  }

  /** Normalize from ≤ to. */
  let period = $derived.by(() => {
    let from = rangeFrom
    let to = rangeTo
    if (from > to) {
      const t = from
      from = to
      to = t
    }
    return { from, to }
  })

  /**
   * Cards on the month grid: deadline day, else created day.
   * @type {Record<string, { id: number, title: string, status: string, significance: string, timeMs: number, timeLabel: string, done: boolean }[]>}
   */
  let questsByDay: Record<string, { id: number, title: string, status: string, significance: string, timeMs: number, timeLabel: string, done: boolean }[]> = $derived.by(() => {
    /** @type {Record<string, any[]>} */
    const map: Record<string, any[]> = {}
    for (const q of quests) {
      const raw = q?.deadline_at || q?.created_at
      const d = parseApiDate(raw)
      if (!d || q?.id == null) continue
      const key = dayKey(d)
      const status = String(q.status || 'active')
      const entry = {
        id: Number(q.id),
        title: String(q.title || '?'),
        status,
        significance: sigKey(q),
        timeMs: d.getTime(),
        timeLabel: formatTime(d),
        done: status === 'completed',
      }
      if (!map[key]) map[key] = []
      map[key].push(entry)
    }
    for (const key of Object.keys(map)) {
      map[key].sort((a, b) => a.timeMs - b.timeMs || a.id - b.id)
    }
    return map
  })

  /**
   * @param {number} year
   * @param {number} month 0-based
   */
  function buildMonth(year: number, month: number) {
    const first = new Date(year, month, 1)
    const daysInMonth = new Date(year, month + 1, 0).getDate()
    const startPad = (first.getDay() + 6) % 7
    /** @type {({ key: string, day: number, items: typeof questsByDay[string] } | null)[]} */
    const cells: ({ key: string, day: number, items: typeof questsByDay[string] } | null)[] = []
    for (let i = 0; i < startPad; i++) cells.push(null)
    for (let day = 1; day <= daysInMonth; day++) {
      const d = new Date(year, month, day)
      const key = dayKey(d)
      cells.push({ key, day, items: questsByDay[key] || [] })
    }
    while (cells.length % 7 !== 0) cells.push(null)
    const label = first.toLocaleDateString('ru-RU', {
      month: 'long',
      year: 'numeric',
    })
    return { key: monthKey(first), label, cells }
  }

  let months = $derived.by(() => {
    void questsByDay
    const fromD = parseDayKey(period.from)
    const toD = parseDayKey(period.to)
    const [fy, fm] = [fromD.getFullYear(), fromD.getMonth()]
    const [ty, tm] = [toD.getFullYear(), toD.getMonth()]
    /** @type {ReturnType<typeof buildMonth>[]} */
    const out: ReturnType<typeof buildMonth>[] = []
    let y = fy
    let m = fm
    while (y < ty || (y === ty && m <= tm)) {
      out.push(buildMonth(y, m))
      m += 1
      if (m > 11) {
        m = 0
        y += 1
      }
    }
    return out
  })

  let todayKey = $derived(dayKey(new Date()))

  /**
   * Day board: one row per quest with a deadline window on this day,
   * a concurrent-count load chart, no-window creates, and the event log.
   */
  let dayLoad = $derived.by(() => {
    if (!selectedKey) return null
    const { start: dayStart, end: dayEnd } = dayBounds(selectedKey)
    const dayMs = dayEnd.getTime() - dayStart.getTime()

    /** @type {Set<number>} */
    const onBoard: Set<number> = new Set()
    /** @type {any[]} */
    const rows: any[] = []
    for (const q of quests) {
      const win = actualWindow(q)
      if (!win || q?.id == null) continue
      const clipped = clipToDay(win, dayStart, dayEnd)
      if (!clipped) continue

      const status = String(q.status || 'active')
      const fromPrev = win.start < dayStart
      const toNext = win.end >= dayEnd
      const endToday = win.end >= dayStart && win.end < dayEnd

      /** @type {number | null} */
      let markMs: number | null = null
      /** @type {'deadline' | 'completed' | 'failed' | null} */
      let markKind: 'deadline' | 'completed' | 'failed' | null = null
      if (win.finish && win.finish >= dayStart && win.finish < dayEnd) {
        markMs = win.finish.getTime()
        markKind = win.markKind
      } else if (!win.finish && endToday) {
        markMs = win.end.getTime()
        markKind = 'deadline'
      }

      const left = (clipped.startMs - dayStart.getTime()) / dayMs
      const rawW = (clipped.endMs - clipped.startMs) / dayMs
      const width = clipped.point ? 0 : Math.max(rawW, 0.004)
      const id = Number(q.id)
      onBoard.add(id)

      rows.push({
        id,
        title: String(q.title || '?'),
        status,
        significance: sigKey(q),
        left,
        width,
        startMs: clipped.startMs,
        endMs: clipped.endMs,
        marker: markMs != null ? (markMs - dayStart.getTime()) / dayMs : null,
        markKind,
        point: clipped.point,
        fromPrev,
        toNext,
        caption: buildBarCaption(q, win, fromPrev, endToday, markKind),
        tip: buildBarTip(q, win, fromPrev, endToday),
      })
    }

    rows.sort(
      (a, b) =>
        a.left - b.left || b.width - a.width || a.id - b.id,
    )

    // Concurrent count at midpoints of LOAD_BUCKETS slots across the day.
    const LOAD_BUCKETS = 48
    /** @type {number[]} */
    const load: number[] = new Array(LOAD_BUCKETS).fill(0)
    for (let i = 0; i < LOAD_BUCKETS; i++) {
      const t0 = dayStart.getTime() + (dayMs * i) / LOAD_BUCKETS
      const t1 = dayStart.getTime() + (dayMs * (i + 1)) / LOAD_BUCKETS
      const mid = (t0 + t1) / 2
      let n = 0
      for (const row of rows) {
        if (row.point) {
          if (row.startMs >= t0 && row.startMs < t1) n += 1
        } else if (row.startMs <= mid && row.endMs > mid) {
          n += 1
        }
      }
      load[i] = n
    }
    const loadMax = Math.max(1, ...load)

    /** Created today, no board row (load undefined — keep off the strip). */
    /** @type {any[]} */
    const noWindow: any[] = []
    for (const q of quests) {
      if (q?.id == null) continue
      const id = Number(q.id)
      if (onBoard.has(id)) continue
      const created = parseApiDate(q.created_at)
      if (!created || created < dayStart || created >= dayEnd) continue
      noWindow.push({
        id,
        title: String(q.title || '?'),
        status: String(q.status || 'active'),
        significance: sigKey(q),
        timeLabel: formatTime(created),
      })
    }
    noWindow.sort((a, b) => a.timeLabel.localeCompare(b.timeLabel) || a.id - b.id)

    /** @type {any[]} */
    const events: any[] = []
    for (const q of quests) {
      if (q?.id == null) continue
      const created = parseApiDate(q.created_at)
      if (created && created >= dayStart && created < dayEnd) {
        events.push({
          kind: 'issued',
          label: 'выдан',
          at: created,
          timeLabel: formatTime(created),
          quest: q,
        })
      }
      const completed = parseApiDate(q.completed_at)
      if (completed && completed >= dayStart && completed < dayEnd) {
        events.push({
          kind: 'completed',
          label: QUEST_STATUS_LABELS.completed ?? 'выполнен',
          at: completed,
          timeLabel: formatTime(completed),
          quest: q,
        })
      }
      const updated = parseApiDate(q.updated_at)
      const st = String(q.status || '')
      if (
        updated &&
        updated >= dayStart &&
        updated < dayEnd &&
        (st === 'failed' || st === 'expired' || st === 'archived')
      ) {
        const sameCreate =
          created && Math.abs(created.getTime() - updated.getTime()) < 2000
        if (!sameCreate) {
          events.push({
            kind: st,
            label: QUEST_STATUS_LABELS[st] ?? st,
            at: updated,
            timeLabel: formatTime(updated),
            quest: q,
          })
        }
      }
    }
    events.sort(
      (a, b) => a.at.getTime() - b.at.getTime() || Number(a.quest.id) - Number(b.quest.id),
    )

    return {
      key: selectedKey,
      title: formatDayTitle(selectedKey),
      rows,
      load,
      loadMax,
      noWindow,
      events,
    }
  })

  let hoveredRow = $derived(
    dayLoad?.rows.find((b) => b.id === hoveredId) ?? null,
  )

  /**
   * @param {any} q
   * @param {{ start: Date, end: Date, finish: Date | null }} win
   * @param {boolean} fromPrev
   * @param {boolean} endToday
   * @param {string | null} markKind
   */
  function buildBarCaption(q: any, win: { start: Date, end: Date, finish: Date | null }, fromPrev: boolean, endToday: boolean, markKind: string | null) {
    const title = String(q.title || '?')
    const endLabel = formatTime(win.end)
    const span = fromPrev
      ? `${formatTime(win.start)} (${dayKey(win.start)}) → ${endLabel}${endToday ? '' : ` (${dayKey(win.end)})`}`
      : `${formatTime(win.start)} → ${endLabel}${endToday ? '' : ` (${dayKey(win.end)})`}`
    if (markKind === 'completed') return `${title} · выполнен · ${span}`
    if (markKind === 'failed') return `${title} · провален · ${span}`
    return `${title} · ${span}`
  }

  /**
   * @param {any} q
   * @param {{ start: Date, end: Date }} win
   * @param {boolean} fromPrev
   * @param {boolean} endToday
   */
  function buildBarTip(q: any, win: { start: Date, end: Date }, fromPrev: boolean, endToday: boolean) {
    const parts = [String(q.title || '?')]
    if (fromPrev) parts.push(`с ${formatTime(win.start)} (${dayKey(win.start)})`)
    else parts.push(`с ${formatTime(win.start)}`)
    parts.push(`до ${formatTime(win.end)}${endToday ? '' : ` (${dayKey(win.end)})`}`)
    return parts.join(' · ')
  }

  /** @param {string} key @returns {boolean} whether day is now selected */
  function selectDay(key: string): boolean {
    const next = selectedKey === key ? null : key
    selectedKey = next
    hoveredId = null
    return next != null
  }

  /** @param {number} delta */
  async function shiftSelectedDay(delta: number) {
    if (!selectedKey) return
    const d = parseDayKey(selectedKey)
    d.setDate(d.getDate() + delta)
    const key = dayKey(d)
    selectedKey = key
    hoveredId = null
    if (key < rangeFrom) rangeFrom = key
    if (key > rangeTo) rangeTo = key
    await tick()
    scrollToSelectedDay()
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

  /** @param {string} key @param {MouseEvent} e */
  async function onCellClick(key: string, e: MouseEvent) {
    const t = (e.target as HTMLElement)
    if (t.closest?.('.cal-card')) return
    const open = selectDay(key)
    if (open) {
      await tick()
      scrollToDayPanel()
    }
  }
</script>

<div class="activity-cal" aria-label="Календарь задач" bind:this={rootEl}>
  <header class="activity-cal__head">
    <div class="activity-cal__head-row">
      <h2 class="activity-cal__title">Календарь</h2>
      {#if selectedKey}
        <button
          type="button"
          class="btn activity-cal__jump"
          onclick={scrollToSelectedDay}
          title="Прокрутить к выбранному дню"
        >
          К выбранному дню
        </button>
      {/if}
    </div>
    <div class="activity-cal__period" role="group" aria-label="Период просмотра">
      <label class="activity-cal__period-field">
        <span>с</span>
        <DateInput bind:value={rangeFrom} label="Период с" />
      </label>
      <label class="activity-cal__period-field">
        <span>по</span>
        <DateInput bind:value={rangeTo} label="Период по" />
      </label>
    </div>
  </header>

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
              title="События дня в истории"
              aria-label="События дня в истории"
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
        <div class="day-board" aria-label="Доска дня" onmouseleave={() => (hoveredId = null)}>
          <div class="day-board__axis" aria-hidden="true">
            <div class="day-board__gutter"></div>
            <div class="day-tl__hours">
              {#each HOUR_TICKS as h}
                <span style:left="{(h / 24) * 100}%">{String(h).padStart(2, '0')}</span>
              {/each}
            </div>
          </div>

          <div class="day-board__load" aria-label="Загруженность">
            <div class="day-board__gutter day-board__gutter--muted">нагрузка</div>
            <div
              class="day-load-chart"
              role="img"
              aria-label="Одновременных окон: пик {dayLoad.loadMax}"
            >
              {#each HOUR_TICKS as h}
                <div class="day-tl__gridline" style:left="{(h / 24) * 100}%"></div>
              {/each}
              <div class="day-load-chart__bars" aria-hidden="true">
                {#each dayLoad.load as n, i (i)}
                  <span
                    class="day-load-chart__col"
                    style:height="{n === 0 ? 0 : Math.max(8, (n / dayLoad.loadMax) * 100)}%"
                    title={String(n)}
                  ></span>
                {/each}
              </div>
            </div>
          </div>

          {#if dayLoad.rows.length === 0}
            <p class="day-tl__empty">Нет задач с окном/дедлайном в этот день</p>
          {:else}
            <div
              class="day-board__rows"
              role="list"
              aria-label="{dayLoad.rows.length} квестов"
            >
              {#each dayLoad.rows as row (row.id)}
                <div
                  class="day-board__row"
                  class:day-board__row--hover={hoveredId === row.id}
                  role="listitem"
                  onmouseenter={() => (hoveredId = row.id)}
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
                      class:day-tl__bar--hover={hoveredId === row.id}
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
                        class:day-tl__mark--hover={hoveredId === row.id}
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
            {#if hoveredRow}
              {hoveredRow.caption}
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
                  class:day-nowin__item--hover={hoveredId === q.id}
                  onmouseenter={() => (hoveredId = q.id)}
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
          <h4 class="day-events__title">События дня</h4>
          {#if dayLoad.events.length === 0}
            <p class="day-events__empty">Нет выдач и смен статуса</p>
          {:else}
            <ul class="day-events__list">
              {#each dayLoad.events as ev (ev.kind + '-' + ev.quest.id + '-' + ev.at.getTime())}
                <li
                  class="day-events__item"
                  class:day-events__item--hover={hoveredId === Number(ev.quest.id)}
                  onmouseenter={() => (hoveredId = Number(ev.quest.id))}
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
