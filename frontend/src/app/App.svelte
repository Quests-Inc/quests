<script lang="ts">
  // Quests web UI — orchestrator (state, load, overlays)
  import { CalendarPage } from '@/pages/calendar'
  import { HistoryPage } from '@/pages/history'
  import { JournalOverlays, JournalPage, createJournalDesk } from '@/pages/journal'
  import { RunnersPage } from '@/pages/runners'
  import { ScriptsPage } from '@/pages/scripts'
  import { GraphPage } from '@/pages/graph'
  import { createNavigation, TAB_ORDER } from '@/app/model/navigation.svelte.ts'
  import { buildRefLabels } from '@/app/model/refLabels.ts'
  import {
    attachmentIdFromUrl,
    healthFrom,
    healthOffline,
    noteIdFromUrl,
    questIdFromUrl,
    replaceSearch,
    tabFromUrl,
  } from '@/app/model/route.ts'
  import { onMount, tick, untrack } from 'svelte'
  import { cubicOut } from 'svelte/easing'
  import { tabSwipe } from '@/shared/lib/tabSwipe.ts'
  import {
    listAllAttachments, listCategories, listNotes, listQuestlines, listQuests,
    fetchHealth,
    fetchAuthState,
    logout as apiLogout,
    setUnauthorizedHandler,
  } from '@/shared/api/client.ts'
  import { subscribeQuestEvents } from '@/shared/lib/live.ts'
  import { attachmentCatalog, clearAttachmentIndex, flattenAttachmentIndex, invalidateAttachmentLiveFlags, seedAttachmentIndex, setAttachmentIndex } from '@/entities/attachment'
  import { clearNotes, noteCatalog, setNotes } from '@/entities/note'
  import { clearQuests, questCatalog, setQuests } from '@/entities/quest'
  import { applyTheme, loadSavedTheme } from '@/shared/lib/theme.ts'
  import { installHotkeys, registerHotkeys } from '@/shared/lib/hotkeys.ts'
  import { foldRow, moveRow } from '@/shared/lib/listNav.ts'
  import { ActionAssistantModal } from '@/features/assistant'
  import { TemplatesModal } from '@/features/templates'
  import { SecretsModal } from '@/features/secrets'
  import { refreshTemplates, templateStore } from '@/entities/template'
  import { TagsModal } from '@/features/tags'
  import { SettingsModal } from '@/features/settings'
  import ToastHost from '@/shared/ui/ToastHost.svelte'
  import { JournalHeader } from '@/widgets/journal-header'
  import { NotesPage } from '@/pages/notes'
  import { AttachmentsPage } from '@/pages/attachments'
  import { LoginScreen, InviteScreen, NoWorkspaceScreen } from '@/pages/auth'

  /** Auth gate: null = still checking, true/false = decided. */
  let authed = $state(null)
  /** Invitation link (?invite=…): shown instead of the journal until accepted. */
  let noWorkspace = $state(false)
  const inviteToken = new URLSearchParams(location.search).get('invite')
  /** Whether this instance uses accounts at all (false on an open local one). */
  let authRequired = $state(false)
  let username = $state('')
  /** Cleanup for the live subscription + timers, so logout can tear them down. */
  let stopApp: () => void = null

  let quests = $derived(questCatalog.list)
  let notes = $derived(noteCatalog.list)
  let attachmentIndex = $derived(attachmentCatalog.index)
  let loading = $state(true)
  let error = $state('')
  let liveStatus = $state('off')
  let health: { api: string, overlay: string, telegram: string, webdav: string, db?: string, detail?: Record<string, any> } = $state({
    api: 'unknown',
    overlay: 'unknown',
    telegram: 'unknown',
    webdav: 'unknown',
    db: 'unknown',
  })
  const nav = createNavigation({
    quests: () => quests,
    revealDetail: (tab) => revealDetailOnNarrow(tab),
    revealList: (tab) => revealListOnNarrow(tab),
    openTemplate: (id) => {
      templatesFocusId = id
      templatesOpen = true
    },
  })
  const { setView, swipeTab, openGraph, onGraphCenter, openHistoryDay, clearQuest: clearSelectedQuest, openRef: onJournalRef } = nav
  const selectQuestFromUi = nav.selectQuest
  const selectNoteFromUi = nav.selectNote
  const selectAttachmentFromUi = nav.selectAttachment
  const onGraphOpen = nav.openRef
  /** Sidebar visibility per view that has one — remembered across reloads. */
  let sidebarCollapsed: Record<string, boolean> = $state(loadSidebarCollapsed())

  function loadSidebarCollapsed() {
    try {
      const raw = JSON.parse(localStorage.getItem('quests.sidebarCollapsed') || '{}')
      return { journal: !!raw.journal, notes: !!raw.notes, scripts: !!raw.scripts }
    } catch {
      return { journal: false, notes: false, scripts: false }
    }
  }

  function toggleSidebar(key: string) {
    setSidebarCollapsed(key, !sidebarCollapsed[key])
  }

  function setSidebarCollapsed(key: string, value: boolean) {
    if (sidebarCollapsed[key] === value) return
    sidebarCollapsed = { ...sidebarCollapsed, [key]: value }
    try {
      localStorage.setItem('quests.sidebarCollapsed', JSON.stringify(sidebarCollapsed))
    } catch {
      /* ignore */
    }
  }

  /** Below this, the sidebar/list takes the full screen instead of a side column. */
  const NARROW_SIDEBAR_BREAKPOINT = '(max-width: 480px)'

  function isNarrowViewport() {
    try {
      return window.matchMedia(NARROW_SIDEBAR_BREAKPOINT).matches
    } catch {
      return false
    }
  }

  /** On a full-screen sidebar, picking an item should reveal it, not leave the list covering it. */
  function revealDetailOnNarrow(key: string) {
    if (isNarrowViewport()) setSidebarCollapsed(key, true)
  }

  /** …and going back to the list should bring it back, not leave an empty detail pane. */
  function revealListOnNarrow(key: string) {
    if (isNarrowViewport()) setSidebarCollapsed(key, false)
  }
  /** Bump to refresh the map after live events. */
  let graphNonce = $state(0)
  /** Bump to refresh the history feed after live events. */
  let historyNonce = $state(0)
  let categories = $state([])
  let questlines: { id: number, title: string, category_id?: number | null, color?: string, icon?: string }[] = $state([])

  let templatesOpen = $state(false)
  let secretsOpen = $state(false)
  /** entity the secrets modal opens on (context menu → «Секреты»), or null */
  let secretsOwner = $state((null as { kind: string, id: number } | null))
  /** template the templates modal opens for editing (a template=N link) */
  let templatesFocusId = $state((null as number | null))
  let tagsOpen = $state(false)
  let settingsOpen = $state(false)
  /** Section the settings modal scrolls to when opened (e.g. 'hotkeys'). */
  let settingsSection = $state((null as string | null))
  let assistantOpen = $state(false)
  let nowMs = $state(Date.now())
  let selected = $derived(quests.find((q) => q.id === nav.selectedId) ?? null)
  /** Every card, hidden included — only the Attachments tab shows hidden ones. */
  let allAttachments = $derived(flattenAttachmentIndex(attachmentIndex))
  let refLabels = $derived(buildRefLabels({ notes, quests, questlines, templates: templateStore.list, attachments: allAttachments }))

  async function load({ silent = false } = {}) {
    if (!silent) loading = true
    if (!silent) error = ''
    try {
      const [next, cats, lines, nextNotes, attIndex] = await Promise.all([
        listQuests({}),
        listCategories(),
        listQuestlines(),
        listNotes().catch((): any[] => []),
        listAllAttachments().catch((): any => null),
      ])
      setQuests(next)
      setNotes(Array.isArray(nextNotes) ? nextNotes : [])
      setAttachmentIndex(attIndex && typeof attIndex === 'object' ? attIndex : null)
      categories = Array.isArray(cats) ? cats : []
      questlines = Array.isArray(lines) ? lines : []
      if (attIndex) {
        seedAttachmentIndex(attIndex, {
          questIds: next.map((q) => q.id),
          questlineIds: questlines.map((l) => l.id),
          noteIds: notes.map((n) => n.id),
        })
      }
      const prefer = nav.pendingSelectId ?? nav.selectedId ?? questIdFromUrl()
      if (prefer != null && next.some((q) => q.id === prefer)) {
        nav.selectedId = prefer
        nav.pendingSelectId = null
      } else if (nav.selectedId != null && !next.some((q) => q.id === nav.selectedId)) {
        nav.selectedId = null
      }
      const preferNote = nav.selectedNoteId ?? noteIdFromUrl()
      if (preferNote != null && notes.some((n) => n.id === preferNote)) {
        nav.selectedNoteId = preferNote
      } else if (nav.selectedNoteId != null && !notes.some((n) => n.id === nav.selectedNoteId)) {
        nav.selectedNoteId = null
      }
      const flatAtt = flattenAttachmentIndex(attachmentIndex)
      const preferAtt = nav.selectedAttachmentId ?? attachmentIdFromUrl()
      if (preferAtt != null && flatAtt.some((a) => a.id === preferAtt)) {
        nav.selectedAttachmentId = preferAtt
      } else if (nav.selectedAttachmentId != null && !flatAtt.some((a) => a.id === nav.selectedAttachmentId)) {
        nav.selectedAttachmentId = null
      }
    } catch (e) {
      if (!silent) {
        error = e.message || String(e)
        clearQuests()
        clearNotes()
        clearAttachmentIndex()
        nav.selectedId = null
      }
    } finally {
      if (!silent) loading = false
    }
  }

  function openTemplates() {
    templatesOpen = true
  }
  function openSecrets(owner: { kind: string; id: number } | null = null) {
    secretsOwner = owner
    secretsOpen = true
  }
  function openTags() {
    tagsOpen = true
  }
  function openSettings(opts: { section?: string } | Event | null = null) {
    settingsSection = opts && !(opts instanceof Event) ? (opts.section ?? null) : null
    settingsOpen = true
  }

  const desk = createJournalDesk({
    getQuests: () => quests,
    getQuestlines: () => questlines,
    getSelected: () => selected,
    getSelectedId: () => nav.selectedId,
    setSelectedId: (id) => {
      nav.selectedId = id
    },
    selectQuest: (id, opts) => selectQuestFromUi(id, opts),
    reload: () => load({ silent: true }),
    setError: (message) => {
      error = message
    },
    getRefLabels: () => refLabels,
    openSecrets: (owner) => openSecrets(owner),
    onGraphDirty: () => {
      if (nav.view === 'graph') graphNonce += 1
    },
  })

  // Порядок вкладок задаёт направление слайда: см. model/navigation.svelte.ts
  let graphSeen = $state(false)
  let graphOn = $state(nav.view === 'graph')
  let graphEl = $state((null as HTMLDivElement | null))
  let prevView = nav.view
  const SLIDE_MS = 220
  const SLIDE_EASE = 'cubic-bezier(0.215, 0.61, 0.355, 1)'
  $effect(() => {
    const v = nav.view
    if (v === 'graph') graphSeen = true
    const was = prevView
    prevView = v
    if (v === was) return
    const dir = untrack(() => nav.navDir)
    if (v === 'graph') {
      graphOn = true
      void tick().then(() => {
        graphEl?.animate([{ transform: `translateX(${dir * 100}%)` }, { transform: 'none' }], { duration: SLIDE_MS, easing: SLIDE_EASE })
      })
    } else if (was === 'graph' && graphEl) {
      const anim = graphEl.animate([{ transform: 'none' }, { transform: `translateX(${-dir * 100}%)` }], {
        duration: SLIDE_MS,
        easing: SLIDE_EASE,
      })
      anim.onfinish = () => {
        if (nav.view !== 'graph') graphOn = false
      }
    }
  })

  /** Horizontal slide for tab switches: forward enters from the right, back from the left. */
  function slideTab(node: any, { dir, out = false }: any) {
    const sign = out ? -dir : dir
    return {
      duration: 220,
      easing: cubicOut,
      css: (t: number) => `transform: translateX(${(1 - t) * sign * 100}%)`,
    }
  }

  async function refreshHealth() {
    try {
      const next = healthFrom(await fetchHealth())
      if (health.webdav !== 'unknown' && health.webdav !== next.webdav) invalidateAttachmentLiveFlags()
      health = next
    } catch {
      health = healthOffline()
    }
  }

  /** Boots data loading and live updates. Only runs once authenticated. */
  function startApp() {
    nav.start()
    load()
    refreshHealth()
    const tick = setInterval(() => {
      nowMs = Date.now()
    }, 1000)
    const healthTick = setInterval(refreshHealth, 5000)
    const stop = subscribeQuestEvents(
      (msg) => {
        if (msg?.type === 'hello' && msg.pending_focus != null) {
          selectQuestFromUi(msg.pending_focus)
          return
        }
        if (msg?.type === 'ui_focus_quest' && msg.quest_id != null) {
          selectQuestFromUi(msg.quest_id)
          return
        }
        if (msg?.type === 'quests_changed') {
          if (msg.quest_id != null && msg.source && msg.source !== 'web') {
            desk.noteRemote(msg.quest_id)
          }
          load({ silent: true })
          if (nav.view === 'history') historyNonce += 1
          if (nav.view === 'graph') graphNonce += 1
        }
      },
      {
        onStatus: (s) => (liveStatus = s),
        // The socket missed events while it was down or the tab was hidden.
        onResync: () => {
          void load({ silent: true })
          void refreshTemplates()
          void refreshHealth()
          if (nav.view === 'history') historyNonce += 1
          if (nav.view === 'graph') graphNonce += 1
        },
      },
    )
    return () => {
      clearInterval(tick)
      clearInterval(healthTick)
      stop()
    }
  }

  function onAuthenticated(name: string) {
    username = name
    authed = true
    stopApp = startApp()
  }

  async function doLogout() {
    try {
      await apiLogout()
    } catch {
      /* dropping the session locally is what matters */
    }
    if (stopApp) stopApp()
    stopApp = null
    clearQuests()
    clearNotes()
    clearAttachmentIndex()
    nav.selectedId = null
    nav.selectedNoteId = null
    nav.selectedAttachmentId = null
    authed = false
  }

  onMount(() => {
    void refreshTemplates()
    // Native swipe hosts (Android HubActivity, quest=192) drive tab switches
    // through this instead of a full page reload — keeps the header mounted
    // and lets setView()'s own transition animate the change.
    window.questsNav = {
      setTab: (name: any) => setView(name),
      currentTab: () => nav.view,
    }

    applyTheme(loadSavedTheme())
    // A 401 from any later call means the session lapsed — fall back to login.
    setUnauthorizedHandler(() => {
      if (authed === false) return
      if (stopApp) stopApp()
      stopApp = null
      authed = false
    })

    let cancelled = false
    ;(async () => {
      if (inviteToken) return // the invitation screen owns the page
      let state
      try {
        state = await fetchAuthState()
      } catch {
        // Auth state is unreachable; treat as open so a local instance with a
        // temporarily unhappy API still renders its usual error banner.
        state = { auth_required: false, authenticated: true }
      }
      if (cancelled) return
      authRequired = !!state.auth_required
      if (state.auth_required && !state.authenticated) {
        authed = false
        loading = false
        return
      }
      username = state.username ?? ''
      authed = true
      if (state.auth_required && state.workspace_id === 0) {
        noWorkspace = true
        loading = false
        return
      }
      stopApp = startApp()
    })()

    return () => {
      cancelled = true
      if (stopApp) stopApp()
      stopApp = null
    }
  })

  $effect(() => {
    nav.selectedId
    desk.kbStepId = null
  })

  // Keyboard control (design: note=76). The dispatcher itself stays silent in
  // text fields, modals and context menus, so these only run on the bare page.
  $effect(() => {
    const unregister = registerHotkeys([
      {
        id: 'tabs',
        group: 'Вкладки',
        label: 'Журнал, заметки, вложения, календарь, история, карта',
        keys: '1–6',
        codes: TAB_ORDER.flatMap((_, i) => [`Digit${i + 1}`, `Numpad${i + 1}`]),
        run: (event: { code: string|any[] }) => {
          const i = Number(event.code.slice(-1)) - 1
          if (TAB_ORDER[i] === nav.view) return false
          setView(TAB_ORDER[i])
        },
      },
      {
        id: 'open-quest',
        group: 'Окна',
        label: 'Новый квест',
        keys: 'N',
        codes: ['KeyN'],
        run: () => desk.openCreate(),
      },
      {
        id: 'open-questline',
        group: 'Окна',
        label: 'Новый квестлайн',
        keys: 'L',
        codes: ['KeyL'],
        run: () => desk.openCreateQuestline(),
      },
      {
        id: 'open-templates',
        group: 'Окна',
        label: 'Шаблоны',
        keys: 'T',
        codes: ['KeyT'],
        run: () => openTemplates(),
      },
      {
        id: 'open-assistant',
        group: 'Окна',
        label: 'Ассистент',
        keys: 'A',
        codes: ['KeyA'],
        run: () => (assistantOpen = true),
      },
      {
        id: 'open-settings',
        group: 'Окна',
        label: 'Настройки',
        keys: ',',
        codes: ['Comma'],
        run: () => openSettings(),
      },
      {
        id: 'open-help',
        group: 'Окна',
        label: 'Справка по клавишам',
        keys: '?',
        codes: ['Slash'],
        shift: true,
        run: () => openSettings({ section: 'hotkeys' }),
      },
      {
        id: 'list-down',
        group: 'Список',
        label: 'Следующая строка (квест, группа, заметка)',
        keys: '↓  J',
        codes: ['ArrowDown', 'KeyJ'],
        when: () => ['journal', 'notes'].includes(nav.view),
        run: () => moveRow(1),
      },
      {
        id: 'list-up',
        group: 'Список',
        label: 'Предыдущая строка',
        keys: '↑  K',
        codes: ['ArrowUp', 'KeyK'],
        when: () => ['journal', 'notes'].includes(nav.view),
        run: () => moveRow(-1),
      },
      {
        id: 'list-expand',
        group: 'Список',
        label: 'Развернуть группу',
        keys: '→',
        codes: ['ArrowRight'],
        when: () => ['journal', 'notes'].includes(nav.view),
        run: () => foldRow(true),
      },
      {
        id: 'list-collapse',
        group: 'Список',
        label: 'Свернуть группу',
        keys: '←',
        codes: ['ArrowLeft'],
        when: () => ['journal', 'notes'].includes(nav.view),
        run: () => foldRow(false),
      },
      {
        id: 'step-prev',
        group: 'Шаги открытого квеста',
        label: 'Предыдущий шаг',
        keys: '[',
        codes: ['BracketLeft'],
        when: () => nav.view === 'journal' && !!selected?.steps?.length,
        run: () => desk.kbMoveStep(-1),
      },
      {
        id: 'step-next',
        group: 'Шаги открытого квеста',
        label: 'Следующий шаг',
        keys: ']',
        codes: ['BracketRight'],
        when: () => nav.view === 'journal' && !!selected?.steps?.length,
        run: () => desk.kbMoveStep(1),
      },
      {
        id: 'step-plus',
        group: 'Шаги открытого квеста',
        label: 'Прогресс шага +1',
        keys: '+',
        codes: ['Equal', 'NumpadAdd'],
        shift: 'any',
        when: () => nav.view === 'journal' && !!selected?.steps?.length,
        run: () => desk.kbBumpStep(1),
      },
      {
        id: 'step-minus',
        group: 'Шаги открытого квеста',
        label: 'Прогресс шага −1',
        keys: '−',
        codes: ['Minus', 'NumpadSubtract'],
        when: () => nav.view === 'journal' && !!selected?.steps?.length,
        run: () => desk.kbBumpStep(-1),
      },
      {
        id: 'step-toggle',
        group: 'Шаги открытого квеста',
        label: 'Завершить шаг (повторно — вернуть на единицу назад)',
        keys: 'Пробел',
        codes: ['Space'],
        when: () => nav.view === 'journal' && !!selected?.steps?.length,
        run: () => desk.kbToggleStep(),
      },
      {
        id: 'clear-selection',
        group: 'Список',
        label: 'Снять выделение квестов, затем выбор квеста',
        keys: 'Esc',
        codes: ['Escape'],
        when: () => nav.view === 'journal' && (desk.multi.size > 0 || nav.selectedId != null),
        run: () => (desk.multi.size > 0 ? desk.multi.clear() : clearSelectedQuest()),
      },
    ])
    const detach = installHotkeys()
    return () => {
      detach()
      unregister()
    }
  })
</script>

{#if inviteToken}
  <InviteScreen token={inviteToken} />
{:else if noWorkspace}
  <NoWorkspaceScreen />
{:else if authed === false}
  <LoginScreen {onAuthenticated} />
{:else if authed === null}
  <div class="boot-gate"></div>
{:else}
<div class="journal">
  <JournalHeader
    view={nav.view}
    {liveStatus}
    {health}
    sidebarCollapsed={sidebarCollapsed[nav.view === 'notes' || nav.view === 'scripts' ? nav.view : 'journal']}
    onToggleSidebar={() => toggleSidebar(nav.view === 'notes' || nav.view === 'scripts' ? nav.view : 'journal')}
    onViewChange={(v) => setView(v)}
    onOpenSettings={openSettings}
    onOpenTemplates={openTemplates}
    onOpenSecrets={() => openSecrets()}
    onOpenTags={openTags}
    onOpenTrash={() => openSettings({ section: 'trash' })}
    onOpenCreateQuestline={desk.openCreateQuestline}
    onOpenCreateQuest={() => desk.openCreate()}
    onOpenAssistant={() => (assistantOpen = true)}
  />

  {#if error}
    <p class="banner-error" role="alert">{error}</p>
  {/if}

  <ToastHost />

  <div class="journal__stage" use:tabSwipe={swipeTab}>
  {#key nav.view}
  <div
    class="journal__viewport"
    in:slideTab={{ dir: nav.navDir }}
    out:slideTab={{ dir: nav.navDir, out: true }}
  >
  {#if nav.view === 'calendar'}
    <CalendarPage
      labels={refLabels}
      onRef={onJournalRef}
      onSelectQuest={selectQuestFromUi}
      onOpenHistory={openHistoryDay}
      onQuestContextMenu={(event, id) => desk.menu.openQuest(event, { id })}
    />
  {:else if nav.view === 'notes'}
    <div class="journal__notes">
      <NotesPage
        {questlines}
        selectedId={nav.selectedNoteId}
        labels={refLabels}
        sidebarCollapsed={sidebarCollapsed.notes}
        onSelect={(id) => selectNoteFromUi(id)}
        onChanged={() => load({ silent: true })}
        onRef={onJournalRef}
        onOpenSecrets={(id) => openSecrets({ kind: 'note', id })}
      />
    </div>
  {:else if nav.view === 'graph'}
    <!-- drawn below, outside the keyed viewport -->
  {:else if nav.view === 'runners'}
    <div class="journal__runners">
      <RunnersPage active={nav.view === 'runners'} onOpenQuest={(id) => selectQuestFromUi(id)} />
    </div>
  {:else if nav.view === 'scripts'}
    <div class="journal__scripts"><ScriptsPage active={nav.view === 'scripts'} sidebarCollapsed={sidebarCollapsed.scripts} /></div>
  {:else if nav.view === 'history'}
    <div class="journal__history">
      <HistoryPage
        active={nav.view === 'history'}
        nonce={historyNonce}
        {categories}
        {questlines}
        onOpen={onJournalRef}
      />
    </div>
  {:else if nav.view === 'attachments'}
    <div class="journal__attachments">
      <AttachmentsPage
        {questlines}
        selectedId={nav.selectedAttachmentId}
        onSelect={(id) => selectAttachmentFromUi(id)}
        onOpenOwner={onJournalRef}
        onChanged={() => load({ silent: true })}
      />
    </div>
  {:else}
    <JournalPage
      {loading}
      {categories}
      {questlines}
      selectedId={nav.selectedId}
      {selected}
      sidebarCollapsed={sidebarCollapsed.journal}
      {nowMs}
      labels={refLabels}
      kbStepId={desk.kbStepId}
      statusBusy={desk.statusBusy}
      deleting={desk.del.questBusy}
      deleteAnimActive={desk.del.animActive}
      stepBusyId={desk.stepBusyId}
      stepEditId={desk.stepEditId}
      stepEditValue={desk.stepEditValue}
      onSelect={(id) => selectQuestFromUi(id)}
      pickedIds={desk.multi.picked}
      onRowClick={(e, id, order) => desk.multi.click(e, id, order)}
      onClearPicked={() => desk.multi.clear()}
      onTogglePin={desk.togglePin}
      onQuestContextMenu={desk.menu.openQuest}
      onLineContextMenu={desk.menu.openLine}
      onStepContextMenu={desk.menu.openStep}
      onOpenGraph={openGraph}
      onToggleCompleted={desk.toggleCompleted}
      onOpenEdit={(q) => desk.openEdit(q)}
      onRequestDelete={desk.requestDeleteSelected}
      onBumpStep={desk.bumpStep}
      onBeginEditStep={desk.beginEditStep}
      onStepEditKeydown={desk.onStepEditKeydown}
      onStepEditBlur={desk.onStepEditBlur}
      onStepEditInput={(v) => (desk.stepEditValue = v)}
      onDeleteAnimDone={desk.del.animDone}
      consumeRemoteEdit={desk.consumeRemoteEdit}
      onRef={onJournalRef}
    />
  {/if}
  </div>
  {/key}
  <!-- The map stays mounted once opened (just hidden), so coming back is instant and keeps its nav.view. -->
  {#if graphSeen}
    <div class="journal__viewport journal__viewport--keep" bind:this={graphEl} style:display={graphOn ? undefined : 'none'}>
    <div class="journal__graph">
      <GraphPage
        active={nav.view === 'graph'}
        rootType={nav.graphRoot.type}
        rootId={nav.graphRoot.id}
        nonce={graphNonce}
        {questlines}
        onCenter={onGraphCenter}
        onOpen={onGraphOpen}
      />
    </div>
    </div>
  {/if}

  </div>
</div>

<JournalOverlays {desk} {questlines} {selected} />

<TemplatesModal
  open={templatesOpen}
  focusId={templatesFocusId}
  onClose={() => {
    templatesOpen = false
    templatesFocusId = null
  }}
  onChanged={() => {
    void load({ silent: true })
    void refreshTemplates()
  }}
  onOpenSecrets={(id) => openSecrets({ kind: 'template', id })}
/>

<SecretsModal
  open={secretsOpen}
  owner={secretsOwner}
  {questlines}
  labels={refLabels}
  onClose={() => (secretsOpen = false)}
/>

<TagsModal open={tagsOpen} onClose={() => (tagsOpen = false)} />

<SettingsModal
  open={settingsOpen}
  onClose={() => (settingsOpen = false)}
  {health}
  {liveStatus}
  {username}
  section={settingsSection}
  onLogout={authRequired ? doLogout : null}
/>

<ActionAssistantModal
  open={assistantOpen}
  {questlines}
  onClose={() => (assistantOpen = false)}
  onApplied={() => load({ silent: true })}
/>
{/if}

<style>
  /* Blank canvas while the auth probe is in flight — avoids flashing either
     the login card or the journal before we know which one is right. */
  .boot-gate {
    min-height: 100vh;
    background: var(--color-bg);
  }
</style>
