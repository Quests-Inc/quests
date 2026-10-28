<script lang="ts">
  import Icon from '@/shared/ui/Icon.svelte'
  import ContextMenu from '@/shared/ui/ContextMenu.svelte'
  import HealthMeter from './HealthMeter.svelte'

  type View = 'journal' | 'notes' | 'attachments' | 'calendar' | 'history' | 'graph' | 'runners' | 'scripts' | 'hooks' | 'runs' | 'templates' | 'trash'

  let {
    view,
    liveStatus,
    health,
    onViewChange,
    onOpenSettings,
    onOpenTemplates,
    onOpenSecrets,
    onOpenTags,
    onOpenTrash,
    onOpenCreateQuestline,
    onOpenCreateQuest,
    onOpenAssistant,
  }: {
    view: View,
    liveStatus: string,
    health: { api: string, overlay: string, telegram: string, webdav?: string, db?: string, detail?: Record<string, any> },
    onViewChange: (v: View) => void,
    onOpenSettings: () => void,
    onOpenTemplates: () => void,
    onOpenSecrets: () => void,
    onOpenTags: () => void,
    onOpenTrash: () => void,
    onOpenCreateQuestline: () => void,
    onOpenCreateQuest: () => void,
    onOpenAssistant: () => void,
  } = $props()

  type Tab = { view: View, label: string, icon: string }
  type Group = { id: string, label: string, icon: string, tabs: Tab[] }

  const T: Record<View, Tab> = {
    journal: { view: 'journal', label: 'Журнал', icon: 'scroll' },
    notes: { view: 'notes', label: 'Заметки', icon: 'document' },
    graph: { view: 'graph', label: 'Карта', icon: 'map' },
    calendar: { view: 'calendar', label: 'Календарь', icon: 'calendar' },
    attachments: { view: 'attachments', label: 'Вложения', icon: 'attachment' },
    history: { view: 'history', label: 'История', icon: 'scroll' },
    runners: { view: 'runners', label: 'Раннеры', icon: 'layers' },
    scripts: { view: 'scripts', label: 'Скрипты', icon: 'edit' },
    hooks: { view: 'hooks', label: 'Хуки', icon: 'flag' },
    templates: { view: 'templates', label: 'Шаблоны', icon: 'repeat' },
    trash: { view: 'trash', label: 'Корзина', icon: 'delete' },
    runs: { view: 'runs', label: 'Запуски', icon: 'list' },
  }

  const MAIN: Tab[] = [T.journal, T.notes, T.graph, T.calendar]
  const GROUPS: Group[] = [
    { id: 'more', label: 'Данные', icon: 'folder', tabs: [T.attachments, T.history, T.trash] },
    { id: 'auto', label: 'Автоматика', icon: 'repeat', tabs: [T.templates, T.scripts, T.hooks, T.runs, T.runners] },
  ]

  /** A group button wears the name of its open section, so you can tell where you are. */
  const groupTab = (g: Group) => g.tabs.find((t) => t.view === view) ?? null

  /** The group whose list is open. It opens on hover (or tap / ArrowDown) and closes a moment after the pointer leaves. */
  let drop = $state<string | null>(null)
  let closeTimer: ReturnType<typeof setTimeout> | undefined
  let headerEl = $state<HTMLElement | null>(null)

  const open = (id: string) => {
    clearTimeout(closeTimer)
    drop = id
  }
  const closeSoon = () => {
    clearTimeout(closeTimer)
    closeTimer = setTimeout((): void => {
      drop = null
    }, 220)
  }
  const pick = (v: View) => {
    drop = null
    onViewChange(v)
  }

  /** Keeps the list inside the window: shifted when it would stick out on either side (the tabs sit mid-screen on a phone). */
  function fit(node: HTMLElement) {
    const r = node.getBoundingClientRect()
    const right = r.right - (window.innerWidth - 8)
    const left = 8 - r.left
    if (right > 0) node.style.setProperty('--drop-shift', `${-right}px`)
    else if (left > 0) node.style.setProperty('--drop-shift', `${left}px`)
  }

  function onGroupKey(e: KeyboardEvent, g: Group) {
    const root = (e.currentTarget as HTMLElement).closest('.view-group')
    const items = [...(root?.querySelectorAll<HTMLElement>('[role=menuitem]') ?? [])]
    if (e.key === 'Escape' && drop) {
      drop = null
      root?.querySelector<HTMLElement>('.view-tab')?.focus()
    } else if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault()
      if (drop !== g.id) {
        open(g.id)
        queueMicrotask(() => root?.querySelector<HTMLElement>('[role=menuitem]')?.focus())
        return
      }
      const i = items.indexOf(document.activeElement as HTMLElement)
      const next = e.key === 'ArrowDown' ? (i + 1) % items.length : (i <= 0 ? items.length : i) - 1
      items[next]?.focus()
    }
  }

  function onWindowDown(e: PointerEvent) {
    if (drop && !(e.target as Element).closest('.view-group')) drop = null
  }

  let moreOpen = $state(false)
  let moreX = $state(0)
  let moreY = $state(0)

  /** The right-hand actions in priority order. All are drawn as icons while the row has room; what does not fit goes, from the end, under «⋯». */
  type Action = { id: string, label: string, aria?: string, icon: string, accent?: boolean, run: () => void }
  const ACTIONS: Action[] = [
    { id: 'quest', label: 'Новый квест', icon: 'add', accent: true, run: () => onOpenCreateQuest() },
    { id: 'assistant', label: 'Команда', aria: 'Командная строка журнала', icon: 'terminal', run: () => onOpenAssistant() },
    { id: 'secrets', label: 'Окружение', icon: 'key', run: () => onOpenSecrets() },
    { id: 'tags', label: 'Теги', icon: 'tag', run: () => onOpenTags() },
    { id: 'questline', label: 'Новый квестлайн', icon: 'layers', run: () => onOpenCreateQuestline() },
    { id: 'settings', label: 'Настройки', icon: 'settings', run: () => onOpenSettings() },
  ]

  /** One icon button is this wide (rem) plus the gap; measured against the room the right column really has. */
  const BTN_REM = 2
  const GAP_REM = 0.1
  let room = $state(Infinity)
  const shown = $derived.by(() => {
    const rem = parseFloat(getComputedStyle(document.documentElement).fontSize) || 16
    const fits = Math.floor((room / rem + GAP_REM) / (BTN_REM + GAP_REM))
    if (fits >= ACTIONS.length) return ACTIONS.length
    return Math.max(0, fits - 1)
  })
  const inline = $derived(ACTIONS.slice(0, shown))
  const folded = $derived(ACTIONS.slice(shown))

  /** The room for the icons is what the header has left after the health and, when they share its row, the tabs: measured from the header, not from the icons' own column, so the column cannot lock itself small. */
  function watchRoom(node: HTMLElement) {
    const measure = (): void => {
      const cs = getComputedStyle(node)
      const gap = parseFloat(cs.columnGap) || 0
      const inner = node.clientWidth - parseFloat(cs.paddingLeft) - parseFloat(cs.paddingRight)
      const left = node.querySelector('.header-left')!.getBoundingClientRect()
      const tabs = node.querySelector('.view-tabs')!.getBoundingClientRect()
      const sameRow = tabs.top < left.bottom
      // No layout (a bare DOM): nothing to measure against, so draw everything.
      if (!(inner > 0)) return void (room = Infinity)
      room = inner - left.width - gap - (sameRow ? tabs.width + gap : 0)
    }
    const ro = new ResizeObserver(measure)
    ro.observe(node)
    node.querySelectorAll('.header-left, .view-tabs').forEach((el) => ro.observe(el))
    measure()
    return { destroy: () => ro.disconnect() }
  }

  function openMore(event: { currentTarget: HTMLElement }) {
    const rect = event.currentTarget.getBoundingClientRect()
    moreX = rect.right
    moreY = rect.bottom + 4
    moreOpen = true
  }
</script>

<svelte:window onpointerdown={onWindowDown} />

<header class="journal__header" bind:this={headerEl} use:watchRoom>
  <div class="header-left">
    <HealthMeter {health} {liveStatus} />
  </div>

  <div class="view-tabs" role="tablist" aria-label="Раздел">
    {#each MAIN as t (t.view)}
      <button
        type="button"
        class="view-tab"
        class:view-tab--on={view === t.view}
        role="tab"
        aria-selected={view === t.view}
        title={t.label}
        onclick={() => onViewChange(t.view)}
      >
        <Icon name={t.icon} size={15} />
        <span class="view-tab__label">{t.label}</span>
      </button>
    {/each}
    {#each GROUPS as g (g.id)}
      {@const cur = groupTab(g)}
      <div
        class="view-group"
        role="presentation"
        onpointerenter={(e) => e.pointerType === 'mouse' && open(g.id)}
        onpointerleave={(e) => e.pointerType === 'mouse' && closeSoon()}
        onfocusin={() => clearTimeout(closeTimer)}
        onkeydown={(e) => onGroupKey(e, g)}
      >
        <button
          type="button"
          class="view-tab view-tab--group"
          class:view-tab--on={cur != null}
          class:view-tab--open={drop === g.id}
          role="tab"
          aria-selected={cur != null}
          aria-haspopup="menu"
          aria-expanded={drop === g.id}
          title={cur ? `${g.label} · ${cur.label}` : g.label}
          onclick={() => open(g.id)}
        >
          <Icon name={cur ? cur.icon : g.icon} size={15} />
          <span class="view-tab__label">{cur ? cur.label : g.label}</span>
          <Icon name="chevron-down" size={11} />
        </button>
        {#if drop === g.id}
          <div class="view-drop" role="menu" aria-label={g.label} use:fit>
            {#each g.tabs as t (t.view)}
              <button type="button" role="menuitem" class="view-drop__item" class:view-drop__item--on={view === t.view} onclick={() => pick(t.view)}>
                {t.label}
              </button>
            {/each}
          </div>
        {/if}
      </div>
    {/each}
  </div>

  <div class="header-right">
    <div class="header-actions">
      {#each inline as a (a.id)}
        <button type="button" class="btn btn--icon-only" class:btn--accent={a.accent} title={a.label} aria-label={a.aria ?? a.label} onclick={a.run}>
          <Icon name={a.icon} />
        </button>
      {/each}
      {#if folded.length}
        <button type="button" class="btn btn--icon-only btn--overflow" title="Ещё" onclick={openMore} aria-haspopup="menu" aria-expanded={moreOpen} aria-label="Ещё действия">
          <Icon name="more" />
        </button>
      {/if}
    </div>
  </div>
</header>

<ContextMenu
  open={moreOpen}
  x={moreX}
  y={moreY}
  items={folded.map((a) => ({ id: a.id, label: a.label }))}
  onSelect={(id) => folded.find((a) => a.id === id)?.run()}
  onClose={() => (moreOpen = false)}
/>

<style>
  /* The list hangs from the header: same fill, the header's bottom line breaks above it, side and bottom borders, no top border. */
  .view-group {
    position: relative;
    display: flex;
    --drop-gap: var(--space-3, 0.75rem);
  }
  .view-drop {
    position: absolute;
    top: calc(100% + var(--drop-gap));
    left: 50%;
    z-index: 30;
    min-width: max(100%, 9rem);
    margin-top: -1px;
    /* Centred under its tab, the labels centred in it. */
    transform: translateX(calc(-50% + var(--drop-shift, 0px)));
    padding: 0.25rem 0 0.35rem;
    border: 1px solid var(--color-border, #333);
    border-top: 0;
    border-radius: 0 0 var(--radius-md, 4px) var(--radius-md, 4px);
    background: var(--color-bg-raised, #1a1a1a);
    box-shadow: 0 10px 18px -8px rgba(0, 0, 0, 0.5);
  }
  .view-drop__item {
    display: block;
    width: 100%;
    padding: 0.45rem 1.2rem;
    border: 0;
    background: transparent;
    color: var(--color-fg-muted, #9a9a9a);
    font-family: var(--font-display, Georgia, serif);
    font-size: var(--text-sm, 0.875rem);
    letter-spacing: 0.02em;
    text-align: center;
    white-space: nowrap;
    cursor: pointer;
  }
  .view-drop__item:hover,
  .view-drop__item:focus-visible {
    background: color-mix(in srgb, var(--color-fg, #e8e8e8) 7%, transparent);
    color: var(--color-fg, #e8e8e8);
    outline: none;
  }
  .view-drop__item--on {
    color: var(--color-accent, #c9a227);
  }
  @media (orientation: portrait) {
    .view-group {
      --drop-gap: 0.375rem;
    }
  }
</style>
