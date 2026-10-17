<script lang="ts">
  import { tick } from 'svelte'
  import Icon from '@/shared/ui/Icon.svelte'
  import ContextMenu from '@/shared/ui/ContextMenu.svelte'

  let {
    view,
    liveStatus,
    health,
    sidebarCollapsed = false,
    onToggleSidebar = null,
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
    view: 'journal' | 'notes' | 'attachments' | 'calendar' | 'history' | 'graph' | 'runners' | 'scripts',
    liveStatus: string,
    health: { api: string, overlay: string, telegram: string, webdav?: string, db?: string, detail?: Record<string, any> },
    sidebarCollapsed?: boolean,
    onToggleSidebar?: (() => void) | null,
    onViewChange: (v: 'journal' | 'notes' | 'attachments' | 'calendar' | 'history' | 'graph' | 'runners' | 'scripts') => void,
    onOpenSettings: () => void,
    onOpenTemplates: () => void,
    onOpenSecrets: () => void,
    onOpenTags: () => void,
    onOpenTrash: () => void,
    onOpenCreateQuestline: () => void,
    onOpenCreateQuest: () => void,
    onOpenAssistant: () => void,
  } = $props()

  /** Only 'journal' and 'notes' have a collapsible sidebar. */
  let showSidebarToggle = $derived(view === 'journal' || view === 'notes')

  /** Map WS states onto the same chip palette as API/HUD/Bot. */
  let liveChip = $derived.by(() => {
    const s = String(liveStatus || 'off')
    if (s === 'live') return { status: 'ok', title: 'WebSocket: live' }
    if (s === 'connecting' || s === 'reconnect') {
      return { status: 'unknown', title: `WebSocket: ${s}` }
    }
    return { status: 'offline', title: `WebSocket: ${s}` }
  })

  function davTitle(status: string) {
    if (status === 'ok') return 'WebDAV / вложения: онлайн'
    if (status === 'offline') return 'WebDAV / вложения: офлайн'
    return 'WebDAV / вложения: не настроен'
  }

  function dbTitle(status: string) {
    const detail = health?.detail?.components?.db?.detail
    if (status === 'warn') return detail || 'БД: схема новее API — обнови образ'
    if (status === 'ok') return detail || 'БД: схема совпадает с API'
    return 'БД: статус неизвестен'
  }

  let menuOpen = $state(false)
  let menuX = $state(0)
  let menuY = $state(0)

  let showLowActions = $derived(view === 'journal' || view === 'notes')

  let actionsMode = $state(('full' as 'full' | 'icons' | 'icons-core' | 'icons-primary'))

  /** Overflow menu lists only actions hidden behind ⋮ for the current stage. */
  let menuItems = $derived.by(() => {
    const items: { id: string, label: string }[] = []
    if (actionsMode === 'icons-primary') {
      items.push({ id: 'settings', label: 'Настройки' })
    }
    if ((actionsMode === 'icons-core' || actionsMode === 'icons-primary') && showLowActions) {
      items.push(
        { id: 'templates', label: 'Шаблоны' },
        { id: 'secrets', label: 'Секреты' },
        { id: 'tags', label: 'Теги' },
        { id: 'trash', label: 'Корзина' },
        { id: 'questline', label: 'Новый квестлайн' },
      )
    }
    return items
  })

  function openMenu(event: { currentTarget: { getBoundingClientRect: () => any } }) {
    const rect = event.currentTarget.getBoundingClientRect()
    menuX = rect.right
    menuY = rect.bottom + 4
    menuOpen = true
  }

  function onMenuSelect(id: string) {
    if (id === 'assistant') onOpenAssistant()
    else if (id === 'settings') onOpenSettings()
    else if (id === 'templates') onOpenTemplates()
    else if (id === 'secrets') onOpenSecrets()
    else if (id === 'tags') onOpenTags()
    else if (id === 'trash') onOpenTrash()
    else if (id === 'questline') onOpenCreateQuestline()
  }

  /** Progressive collapse: pick the widest actions stage (full → icons →
   * icons-core → icons-primary) that still fits in headerSlots().right. */
  let headerEl = $state(null)
  let leftEl = $state(null)
  let tabsEl = $state(null)
  let measureFullEl = $state(null)
  let measureIconsEl = $state(null)
  let measureCoreEl = $state(null)
  let measurePrimaryEl = $state(null)
  let actionsEl = $state(null)

  const ACTION_STAGES = (['full', 'icons', 'icons-core', 'icons-primary'] as const)

  let showOverflow = $derived(actionsMode === 'icons-core' || actionsMode === 'icons-primary')
  let showSettingsBtn = $derived(
    actionsMode === 'full' || actionsMode === 'icons' || actionsMode === 'icons-core',
  )
  let showLowBtns = $derived((actionsMode === 'full' || actionsMode === 'icons') && showLowActions)

  // Same idea, independently, for the health chips: labeled by default,
  // dots-only only once the brand+health cluster actually doesn't fit.
  let brandEl = $state(null)
  let healthEl = $state(null)
  let healthMeasureEl = $state(null)
  let healthCollapsed = $state(false)

  /** Free width on each side of the tab block.
   *
   * The tabs are centred on the header itself, not on the space between the
   * clusters — so the room left over is not (header - tabs) / 2. With a 324px
   * brand cluster on the left and a 576px action row on the right, splitting
   * the remainder evenly says everything fits while the right-hand side is in
   * fact 19px short, and the labelled buttons slide under the tabs. Measure
   * the actual gaps beside the tab block instead.
   *
   * In portrait the CSS drops the tabs onto their own row; then the two
   * clusters share one line and only have to clear each other — and the
   * health chips, which portrait CSS pulls out of header-left and centers
   * on the header absolutely, so leftEl's width no longer accounts for them.
   * There each side gets whatever lies between its cluster and the header's
   * center, and health's own box is subtracted from the actions' side.
   */
  function headerSlots() {
    const cs = getComputedStyle(headerEl)
    const gap = parseFloat(cs.columnGap) || 12
    const rect = headerEl.getBoundingClientRect()
    const contentLeft = rect.left + (parseFloat(cs.paddingLeft) || 0)
    const contentRight = rect.right - (parseFloat(cs.paddingRight) || 0)

    if (getComputedStyle(tabsEl).position !== 'absolute') {
      const width = contentRight - contentLeft
      if (healthEl && getComputedStyle(healthEl).position === 'absolute') {
        const center = (rect.left + rect.right) / 2
        const health = healthEl.getBoundingClientRect()
        return {
          right: contentRight - health.right - gap,
          // Health is centered, so it needs room for half its width on
          // *both* sides of the center; report the tighter side, doubled.
          health:
            2 *
            Math.min(
              center - (contentLeft + leftEl.offsetWidth) - gap,
              contentRight - (actionsEl?.offsetWidth ?? 0) - center - gap,
            ),
        }
      }
      return {
        left: width - (actionsEl?.offsetWidth ?? 0) - gap,
        right: width - leftEl.offsetWidth - gap,
      }
    }
    const tabs = tabsEl.getBoundingClientRect()
    return {
      left: tabs.left - contentLeft - gap,
      right: contentRight - tabs.right - gap,
    }
  }

  function recomputeCollapse() {
    if (!headerEl || !leftEl || !tabsEl || !measureFullEl) return
    const available = headerSlots().right
    const widths = {
      full: measureFullEl.scrollWidth,
      icons: measureIconsEl?.scrollWidth ?? Infinity,
      'icons-core': measureCoreEl?.scrollWidth ?? Infinity,
      'icons-primary': measurePrimaryEl?.scrollWidth ?? Infinity,
    }
    for (const stage of ACTION_STAGES) {
      if (widths[stage] <= available) {
        actionsMode = stage
        return
      }
    }
    actionsMode = 'icons-primary'
  }

  function recomputeHealthCollapse() {
    if (!headerEl || !leftEl || !tabsEl || !healthMeasureEl) return
    const slots = headerSlots()
    let forHealth
    if (slots.health != null) {
      // Portrait: chips are absolutely centered — budget comes from headerSlots.
      forHealth = slots.health
    } else {
      // In-flow: left slot minus siblings (sidebar toggle, brand, flex gaps).
      // Using brand alone under-counted the toggle and left labeled chips
      // overlapping the action icons (DAV ∩ gear).
      const healthW = healthEl?.offsetWidth ?? 0
      const chrome = Math.max(0, leftEl.offsetWidth - healthW)
      forHealth = slots.left - chrome
    }
    if (!Number.isFinite(forHealth)) forHealth = 0

    const labeledW = healthMeasureEl.scrollWidth
    // Hysteresis: expanding labels frees action budget (portrait absolute
    // health), which then says labels fit again — without slack we
    // oscillate labeled ↔ dots and keep overlapping the gear.
    const EXPAND_SLACK = 48
    if (!healthCollapsed) {
      healthCollapsed = labeledW > forHealth || labeledWouldOverlapActions()
    } else {
      healthCollapsed =
        labeledW + EXPAND_SLACK > forHealth || labeledWouldOverlapActions()
    }
  }

  /** If we painted the full labeled row right now, would it hit actions? */
  function labeledWouldOverlapActions(gap = 8) {
    if (!headerEl || !actionsEl || !healthMeasureEl) return false
    const a = actionsEl.getBoundingClientRect()
    if (a.width <= 0) return false
    const labeledW = healthMeasureEl.scrollWidth
    const healthPos = healthEl ? getComputedStyle(healthEl).position : 'static'
    if (healthPos === 'absolute') {
      const rect = headerEl.getBoundingClientRect()
      const center = (rect.left + rect.right) / 2
      return center + labeledW / 2 + gap > a.left
    }
    if (!leftEl) return false
    const healthW = healthEl?.offsetWidth ?? 0
    const chrome = Math.max(0, leftEl.offsetWidth - healthW)
    const cs = getComputedStyle(headerEl)
    const contentLeft =
      headerEl.getBoundingClientRect().left + (parseFloat(cs.paddingLeft) || 0)
    return contentLeft + chrome + labeledW + gap > a.left
  }

  /** True when health chips and action icons share the same pixels. */
  function healthOverlapsActions(gap = 8) {
    if (!healthEl || !actionsEl) return false
    const h = healthEl.getBoundingClientRect()
    const a = actionsEl.getBoundingClientRect()
    if (h.width <= 0 || a.width <= 0) return false
    return h.right + gap > a.left && a.right + gap > h.left && h.bottom > a.top && a.bottom > h.top
  }

  /** After the slot math, force collapse if the two clusters still collide. */
  function resolveOverlap() {
    if (!healthOverlapsActions()) return false
    if (!healthCollapsed) {
      healthCollapsed = true
      return true
    }
    const idx = ACTION_STAGES.indexOf(actionsMode)
    if (idx >= 0 && idx < ACTION_STAGES.length - 1) {
      actionsMode = ACTION_STAGES[idx + 1]
      return true
    }
    return false
  }

  /** True when the centred tabs cannot fit between the side clusters even at their most collapsed:
   * the tabs then drop onto their own row (like in portrait) instead of painting over the chips and buttons. */
  let stacked = $state(false)
  /** Even on their own row the labelled tabs are wider than the header: icons only, like on a phone. */
  let iconTabs = $state(false)
  let tabsNaturalW = 0

  function recomputeStacked() {
    if (!headerEl || !leftEl || !tabsEl || !actionsEl) return
    // The tabs' own width, whatever row they sit on: the sum of the tab buttons (a stacked row stretches).
    const items = [...tabsEl.children] as HTMLElement[]
    const natural = items.reduce((w, el) => w + el.offsetWidth, 0) + 2.4 * Math.max(0, items.length - 1)
    if (natural > 0 && !iconTabs) tabsNaturalW = natural
    const cs = getComputedStyle(headerEl)
    const gap = parseFloat(cs.columnGap) || 12
    const width = headerEl.clientWidth - (parseFloat(cs.paddingLeft) || 0) - (parseFloat(cs.paddingRight) || 0)
    const healthW = healthEl?.offsetWidth ?? 0
    // Narrowest left cluster: toggle + brand + health reduced to dots (about 3.5rem when it is still labeled).
    // header-left itself stretches across the free space, so its own width says nothing: add up its children.
    const kids = [...leftEl.children].filter((el) => el !== healthEl) as HTMLElement[]
    const chrome = kids.reduce((w, el) => w + el.offsetWidth, 0) + gap * kids.length
    const leftMin = chrome + (healthCollapsed ? healthW : 56)
    const rightMin = measurePrimaryEl?.scrollWidth ?? actionsEl.offsetWidth
    const need = tabsNaturalW + 2 * Math.max(leftMin, rightMin) + 2 * gap
    // Hysteresis so the layout does not flip back and forth around the threshold.
    stacked = stacked ? need + 24 > width : need > width
    // On their own row the tabs need the whole header width; when even that is short, they lose their labels.
    iconTabs = stacked && (iconTabs ? tabsNaturalW + 24 > width : tabsNaturalW > width)
  }

  /** Actions that still collide with the centred tabs after every stage has been tried push the tabs onto their own row. */
  function tabsCollide(gap = 6) {
    if (!tabsEl || !actionsEl || !healthEl || getComputedStyle(tabsEl).position !== 'absolute') return false
    const t = tabsEl.getBoundingClientRect()
    return t.right + gap > actionsEl.getBoundingClientRect().left || t.left - gap < healthEl.getBoundingClientRect().right
  }

  async function recomputeAll() {
    const was = stacked
    recomputeStacked()
    // The collapse maths below reads the tabs' position: let the new layout land first.
    if (stacked !== was) await tick()
    // Health first so a wide labeled row yields budget to actions, then
    // actions pick a stage; finally a geometry check stops residual overlap
    // the slot math missed (toggle chrome, font swap, absolute tabs).
    recomputeHealthCollapse()
    recomputeCollapse()
    if (resolveOverlap()) {
      requestAnimationFrame(() => {
        recomputeCollapse()
        if (resolveOverlap()) requestAnimationFrame(recomputeCollapse)
      })
    }
    requestAnimationFrame(() => {
      if (!stacked && actionsMode === 'icons-primary' && tabsCollide()) stacked = true
    })
  }

  $effect(() => {
    // Re-run whenever the measurer's own content changes shape (e.g. the
    // Шаблоны/Квестлайн buttons appearing only on some views).
    void view
    recomputeAll()
    // First paint measures fallback-font widths: Literata and IBM Plex land
    // afterwards and widen both the tabs and the buttons, which is exactly
    // what this decision depends on. Re-measure once layout has settled and
    // again once the real fonts are in.
    const raf = requestAnimationFrame(recomputeAll)
    document.fonts?.ready.then(recomputeAll)
    const ro = new ResizeObserver(recomputeAll)
    if (headerEl) ro.observe(headerEl)
    if (leftEl) ro.observe(leftEl)
    if (tabsEl) ro.observe(tabsEl)
    if (measureFullEl) ro.observe(measureFullEl)
    if (measureIconsEl) ro.observe(measureIconsEl)
    if (measureCoreEl) ro.observe(measureCoreEl)
    if (measurePrimaryEl) ro.observe(measurePrimaryEl)
    if (brandEl) ro.observe(brandEl)
    if (healthEl) ro.observe(healthEl)
    if (healthMeasureEl) ro.observe(healthMeasureEl)
    if (actionsEl) ro.observe(actionsEl)
    window.addEventListener('resize', recomputeAll)
    return () => {
      cancelAnimationFrame(raf)
      ro.disconnect()
      window.removeEventListener('resize', recomputeAll)
    }
  })
</script>

<header class="journal__header" class:journal__header--stacked={stacked} class:journal__header--icon-tabs={iconTabs} bind:this={headerEl}>
  <div class="header-left" bind:this={leftEl}>
    {#if showSidebarToggle}
      <button
        type="button"
        class="btn btn--ghost btn--icon sidebar-toggle"
        onclick={onToggleSidebar}
        title={sidebarCollapsed ? 'Показать боковую панель' : 'Скрыть боковую панель'}
        aria-label={sidebarCollapsed ? 'Показать боковую панель' : 'Скрыть боковую панель'}
        aria-pressed={sidebarCollapsed}
      >
        <Icon name="sidebar" size={16} />
      </button>
    {/if}
    <!-- No section label here: tabbed views show theirs via the tab
         highlight — a header copy just repeated it and, on a phone, collided
         with the health chips. -->
    <div class="brand" bind:this={brandEl}></div>

    {#snippet healthChips()}
      <span
        class="health__chip"
        data-status={liveChip.status}
        title={liveChip.title}
        aria-label={liveChip.title}
      >
        <span class="health__dot" aria-hidden="true"></span>
        {#if !healthCollapsed}<span class="health__label">Live</span>{/if}
      </span>
      <span class="health__chip" data-status={health.api} title="API" aria-label="API">
        <span class="health__dot" aria-hidden="true"></span>
        {#if !healthCollapsed}<span class="health__label">API</span>{/if}
      </span>
      <span
        class="health__chip"
        data-status={health.db || 'unknown'}
        title={dbTitle(health.db)}
        aria-label={dbTitle(health.db)}
      >
        <span class="health__dot" aria-hidden="true"></span>
        {#if !healthCollapsed}<span class="health__label">DB</span>{/if}
      </span>
      <span class="health__chip" data-status={health.overlay} title="HUD / оверлей" aria-label="HUD / оверлей">
        <span class="health__dot" aria-hidden="true"></span>
        {#if !healthCollapsed}<span class="health__label">HUD</span>{/if}
      </span>
      <span class="health__chip" data-status={health.telegram} title="Telegram-бот" aria-label="Telegram-бот">
        <span class="health__dot" aria-hidden="true"></span>
        {#if !healthCollapsed}<span class="health__label">Bot</span>{/if}
      </span>
      <span
        class="health__chip"
        data-status={health.webdav || 'unknown'}
        title={davTitle(health.webdav)}
        aria-label={davTitle(health.webdav)}
      >
        <span class="health__dot" aria-hidden="true"></span>
        {#if !healthCollapsed}<span class="health__label">DAV</span>{/if}
      </span>
    {/snippet}

    <div
      class="health"
      class:health--collapsed={healthCollapsed}
      bind:this={healthEl}
      role="status"
      aria-label="Состояние сервисов"
    >
      {@render healthChips()}
    </div>
  </div>

  <!-- Off-screen twin of the labeled health row, always at natural width,
       used only to decide whether the labels fit. -->
  <div class="health health--measure" bind:this={healthMeasureEl} aria-hidden="true" inert>
    <span class="health__chip">
      <span class="health__dot" aria-hidden="true"></span>
      <span class="health__label">Live</span>
    </span>
    <span class="health__chip">
      <span class="health__dot" aria-hidden="true"></span>
      <span class="health__label">API</span>
    </span>
    <span class="health__chip">
      <span class="health__dot" aria-hidden="true"></span>
      <span class="health__label">DB</span>
    </span>
    <span class="health__chip">
      <span class="health__dot" aria-hidden="true"></span>
      <span class="health__label">HUD</span>
    </span>
    <span class="health__chip">
      <span class="health__dot" aria-hidden="true"></span>
      <span class="health__label">Bot</span>
    </span>
    <span class="health__chip">
      <span class="health__dot" aria-hidden="true"></span>
      <span class="health__label">DAV</span>
    </span>
  </div>
  <div class="view-tabs" role="tablist" aria-label="Раздел" bind:this={tabsEl}>
    <button
      type="button"
      class="view-tab"
      class:view-tab--on={view === 'journal'}
      role="tab"
      aria-selected={view === 'journal'}
      title="Журнал"
      onclick={() => onViewChange('journal')}
    >
      <Icon name="scroll" size={15} />
      <span class="view-tab__label">Журнал</span>
    </button>
    <button
      type="button"
      class="view-tab"
      class:view-tab--on={view === 'notes'}
      role="tab"
      aria-selected={view === 'notes'}
      title="Заметки"
      onclick={() => onViewChange('notes')}
    >
      <Icon name="document" size={15} />
      <span class="view-tab__label">Заметки</span>
    </button>
    <button
      type="button"
      class="view-tab"
      class:view-tab--on={view === 'attachments'}
      role="tab"
      aria-selected={view === 'attachments'}
      title="Вложения"
      onclick={() => onViewChange('attachments')}
    >
      <Icon name="attachment" size={15} />
      <span class="view-tab__label">Вложения</span>
    </button>
    <button
      type="button"
      class="view-tab"
      class:view-tab--on={view === 'calendar'}
      role="tab"
      aria-selected={view === 'calendar'}
      title="Календарь"
      onclick={() => onViewChange('calendar')}
    >
      <Icon name="calendar" size={15} />
      <span class="view-tab__label">Календарь</span>
    </button>
    <button
      type="button"
      class="view-tab"
      class:view-tab--on={view === 'history'}
      role="tab"
      aria-selected={view === 'history'}
      title="История"
      onclick={() => onViewChange('history')}
    >
      <Icon name="scroll" size={15} />
      <span class="view-tab__label">История</span>
    </button>
    <button
      type="button"
      class="view-tab"
      class:view-tab--on={view === 'graph'}
      role="tab"
      aria-selected={view === 'graph'}
      title="Карта связей"
      onclick={() => onViewChange('graph')}
    >
      <Icon name="map" size={15} />
      <span class="view-tab__label">Карта</span>
    </button>
    <button
      type="button"
      class="view-tab"
      class:view-tab--on={view === 'runners'}
      role="tab"
      aria-selected={view === 'runners'}
      title="Раннеры: устройства, на которых выполняются автопроверки"
      onclick={() => onViewChange('runners')}
    >
      <Icon name="layers" size={15} />
      <span class="view-tab__label">Раннеры</span>
    </button>
    <button
      type="button"
      class="view-tab"
      class:view-tab--on={view === 'scripts'}
      role="tab"
      aria-selected={view === 'scripts'}
      title="Скрипты: схемы проверок из нод"
      onclick={() => onViewChange('scripts')}
    >
      <Icon name="edit" size={15} />
      <span class="view-tab__label">Скрипты</span>
    </button>
  </div>
  {#snippet fullActions()}
    <button type="button" class="btn" title="Команда" aria-label="Командная строка журнала">
      <Icon name="terminal" />
      <span class="btn__text">Команда</span>
    </button>
    <button type="button" class="btn" title="Настройки" aria-label="Настройки">
      <Icon name="settings" />
      <span class="btn__text">Настройки</span>
    </button>
    {#if view === 'journal' || view === 'notes'}
      <button type="button" class="btn" title="Шаблоны" aria-label="Шаблоны периодики">
        <Icon name="repeat" />
        <span class="btn__text">Шаблоны</span>
      </button>
      <button type="button" class="btn" title="Секреты" aria-label="Секреты">
        <Icon name="key" />
        <span class="btn__text">Секреты</span>
      </button>
      <button type="button" class="btn" title="Теги" aria-label="Теги">
        <Icon name="pin" />
        <span class="btn__text">Теги</span>
      </button>
      <button type="button" class="btn" title="Корзина" aria-label="Корзина">
        <Icon name="delete" />
        <span class="btn__text">Корзина</span>
      </button>
      <button type="button" class="btn" title="Квестлайн" aria-label="Новый квестлайн">
        <Icon name="flag" />
        <span class="btn__text">Квестлайн</span>
      </button>
    {/if}
    <button type="button" class="btn btn--accent" title="Новый квест" aria-label="Новый квест">
      <Icon name="add" />
      <span class="btn__text">Новый квест</span>
    </button>
  {/snippet}

  <div
    class="header-actions"
    class:header-actions--icons={actionsMode !== 'full'}
    bind:this={actionsEl}
  >
    <button
      type="button"
      class="btn"
      title="Команда"
      onclick={onOpenAssistant}
      aria-label="Командная строка журнала"
    >
      <Icon name="terminal" />
      <span class="btn__text">Команда</span>
    </button>
    {#if showSettingsBtn}
      <button type="button" class="btn" title="Настройки" onclick={onOpenSettings} aria-label="Настройки">
        <Icon name="settings" />
        <span class="btn__text">Настройки</span>
      </button>
    {/if}
    {#if showLowBtns}
      <button
        type="button"
        class="btn"
        title="Шаблоны"
        onclick={onOpenTemplates}
        aria-label="Шаблоны периодики"
      >
        <Icon name="repeat" />
        <span class="btn__text">Шаблоны</span>
      </button>
      <button type="button" class="btn" title="Секреты" onclick={onOpenSecrets} aria-label="Секреты">
        <Icon name="key" />
        <span class="btn__text">Секреты</span>
      </button>
      <button type="button" class="btn" title="Теги" onclick={onOpenTags} aria-label="Теги">
        <Icon name="pin" />
        <span class="btn__text">Теги</span>
      </button>
      <button type="button" class="btn" title="Корзина" onclick={onOpenTrash} aria-label="Корзина">
        <Icon name="delete" />
        <span class="btn__text">Корзина</span>
      </button>
      <button
        type="button"
        class="btn"
        title="Квестлайн"
        onclick={onOpenCreateQuestline}
        aria-label="Новый квестлайн"
      >
        <Icon name="flag" />
        <span class="btn__text">Квестлайн</span>
      </button>
    {/if}
    {#if showOverflow && (actionsMode === 'icons-primary' || showLowActions)}
      <button
        type="button"
        class="btn btn--overflow"
        title="Ещё"
        onclick={openMenu}
        aria-haspopup="menu"
        aria-expanded={menuOpen}
        aria-label="Ещё действия"
      >
        <Icon name="more" />
      </button>
    {/if}
    <button
      type="button"
      class="btn btn--accent"
      title="Новый квест"
      onclick={onOpenCreateQuest}
      aria-label="Новый квест"
    >
      <Icon name="add" />
      <span class="btn__text">Новый квест</span>
    </button>
  </div>

  <!-- Off-screen twins: one row per collapse stage, measured against headerSlots().right -->
  <div class="header-actions header-actions--measure" bind:this={measureFullEl} aria-hidden="true" inert>
    {@render fullActions()}
  </div>
  <div
    class="header-actions header-actions--measure header-actions--icons"
    bind:this={measureIconsEl}
    aria-hidden="true"
    inert
  >
    {@render fullActions()}
  </div>
  <div
    class="header-actions header-actions--measure header-actions--icons"
    bind:this={measureCoreEl}
    aria-hidden="true"
    inert
  >
    <button type="button" class="btn" tabindex="-1"><Icon name="terminal" /><span class="btn__text">Команда</span></button>
    <button type="button" class="btn" tabindex="-1"><Icon name="settings" /><span class="btn__text">Настройки</span></button>
    {#if showLowActions}
      <button type="button" class="btn btn--overflow" tabindex="-1"><Icon name="more" /></button>
    {/if}
    <button type="button" class="btn btn--accent" tabindex="-1"><Icon name="add" /><span class="btn__text">Новый квест</span></button>
  </div>
  <div
    class="header-actions header-actions--measure header-actions--icons"
    bind:this={measurePrimaryEl}
    aria-hidden="true"
    inert
  >
    <button type="button" class="btn" tabindex="-1"><Icon name="terminal" /><span class="btn__text">Команда</span></button>
    <button type="button" class="btn btn--overflow" tabindex="-1"><Icon name="more" /></button>
    <button type="button" class="btn btn--accent" tabindex="-1"><Icon name="add" /><span class="btn__text">Новый квест</span></button>
  </div>
</header>

<ContextMenu
  open={menuOpen}
  x={menuX}
  y={menuY}
  items={menuItems}
  onSelect={onMenuSelect}
  onClose={() => (menuOpen = false)}
/>
