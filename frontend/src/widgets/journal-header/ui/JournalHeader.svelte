<script lang="ts">
  import Icon from '@/shared/ui/Icon.svelte'
  import ContextMenu from '@/shared/ui/ContextMenu.svelte'
  import HealthMeter from './HealthMeter.svelte'

  type View = 'journal' | 'notes' | 'attachments' | 'calendar' | 'history' | 'graph' | 'runners' | 'scripts' | 'hooks'

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
  }

  const MAIN: Tab[] = [T.journal, T.notes, T.graph, T.calendar]
  const GROUPS: Group[] = [
    { id: 'more', label: 'Ещё', icon: 'folder', tabs: [T.attachments, T.history] },
    { id: 'auto', label: 'Автоматика', icon: 'repeat', tabs: [T.runners, T.scripts, T.hooks] },
  ]

  /** A group button wears the name of its open section, so you can tell where you are. */
  const groupTab = (g: Group) => g.tabs.find((t) => t.view === view) ?? null

  let groupMenu = $state<{ open: boolean, x: number, y: number, group: Group | null }>({ open: false, x: 0, y: 0, group: null })

  function openGroup(event: { currentTarget: HTMLElement }, g: Group) {
    const rect = event.currentTarget.getBoundingClientRect()
    groupMenu = { open: true, x: rect.left + rect.width / 2, y: rect.bottom + 4, group: g }
  }

  let groupItems = $derived((groupMenu.group?.tabs ?? []).map((t) => ({ id: t.view, label: t.label, checked: t.view === view })))

  let moreOpen = $state(false)
  let moreX = $state(0)
  let moreY = $state(0)

  const MORE_ITEMS = [
    { id: 'settings', label: 'Настройки' },
    { id: 'templates', label: 'Шаблоны' },
    { id: 'secrets', label: 'Секреты' },
    { id: 'tags', label: 'Теги' },
    { id: 'trash', label: 'Корзина' },
    { id: 'questline', label: 'Новый квестлайн' },
  ]

  function openMore(event: { currentTarget: HTMLElement }) {
    const rect = event.currentTarget.getBoundingClientRect()
    moreX = rect.right
    moreY = rect.bottom + 4
    moreOpen = true
  }

  const MORE_ACTIONS: Record<string, () => void> = {
    settings: () => onOpenSettings(),
    templates: () => onOpenTemplates(),
    secrets: () => onOpenSecrets(),
    tags: () => onOpenTags(),
    trash: () => onOpenTrash(),
    questline: () => onOpenCreateQuestline(),
  }
</script>

<header class="journal__header">
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
      <button
        type="button"
        class="view-tab view-tab--group"
        class:view-tab--on={cur != null}
        role="tab"
        aria-selected={cur != null}
        aria-haspopup="menu"
        aria-expanded={groupMenu.open && groupMenu.group?.id === g.id}
        title={cur ? `${g.label} · ${cur.label}` : g.label}
        onclick={(e) => openGroup(e, g)}
      >
        <Icon name={cur ? cur.icon : g.icon} size={15} />
        <span class="view-tab__label">{cur ? cur.label : g.label}</span>
        <Icon name="chevron-down" size={11} />
      </button>
    {/each}
  </div>

  <div class="header-actions">
    <button type="button" class="btn" title="Команда" onclick={onOpenAssistant} aria-label="Командная строка журнала">
      <Icon name="terminal" />
      <span class="btn__text">Команда</span>
    </button>
    <button type="button" class="btn btn--accent" title="Новый квест" onclick={onOpenCreateQuest} aria-label="Новый квест">
      <Icon name="add" />
      <span class="btn__text">Новый квест</span>
    </button>
    <button
      type="button"
      class="btn btn--overflow"
      title="Ещё"
      onclick={openMore}
      aria-haspopup="menu"
      aria-expanded={moreOpen}
      aria-label="Ещё действия"
    >
      <Icon name="more" />
    </button>
  </div>
</header>

<ContextMenu
  open={groupMenu.open}
  x={groupMenu.x}
  y={groupMenu.y}
  center
  items={groupItems}
  onSelect={(id) => onViewChange(id as View)}
  onClose={() => (groupMenu.open = false)}
/>
<ContextMenu
  open={moreOpen}
  x={moreX}
  y={moreY}
  items={MORE_ITEMS}
  onSelect={(id) => MORE_ACTIONS[id]?.()}
  onClose={() => (moreOpen = false)}
/>
