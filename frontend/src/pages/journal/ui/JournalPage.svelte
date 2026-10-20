<script lang="ts">
  import { questCatalog, groupQuestsByCategory, inScope, listOpenOrAll, questSpec, scopeQuestlineChoices, type CategoryScope } from '@/entities/quest'
  import { noteCatalog } from '@/entities/note'
  import { QuestDetail } from '@/widgets/quest-detail'
  import { QuestSidebar } from '@/widgets/quest-sidebar'
  import SidebarLayout from '@/shared/ui/SidebarLayout.svelte'
  import { questMatchesQuery } from '@/shared/lib/search.ts'
  import { parseQuery, passes, setToken, tokenValues } from '@/shared/lib/searchFlags.ts'

  let {
    loading,
    categories = [],
    questlines = [],
    selectedId = null,
    selected = null,
    nowMs,
    labels = {},
    kbStepId = null,
    statusBusy = false,
    deleting = false,
    deleteAnimActive = false,
    stepBusyId = null,
    stepEditId = null,
    stepEditValue = '',
    onSelect,
    pickedIds = new Set<number>(),
    onRowClick,
    onClearPicked,
    onSelectAll,
    onTogglePin,
    onQuestContextMenu,
    onLineContextMenu,
    onStepContextMenu,
    onOpenGraph,
    onToggleCompleted,
    onOpenEdit,
    onRequestDelete,
    onBumpStep,
    onBeginEditStep,
    onStepEditKeydown,
    onStepEditBlur,
    onStepEditInput,
    onDeleteAnimDone,
    consumeRemoteEdit,
    onRef,
  }: {
    loading: boolean
    categories?: { id: number; slug?: string; label: string; sort_order?: number; color?: string }[]
    questlines?: { id: number; title: string; category_id?: number | null; color?: string; icon?: string }[]
    selectedId?: number | null
    selected?: { id: number; questline_id?: number | null; steps?: unknown[] } | null
    nowMs: number
    labels?: Record<string, string>
    kbStepId?: number | null
    statusBusy?: boolean
    deleting?: boolean
    deleteAnimActive?: boolean
    stepBusyId?: number | null
    stepEditId?: number | null
    stepEditValue?: string
    onSelect: (id: number) => void
    pickedIds?: ReadonlySet<number>
    onRowClick?: (event: MouseEvent, id: number, order: number[]) => boolean
    onClearPicked?: () => void
    onSelectAll?: (ids: number[]) => void
    onTogglePin: (quest: { id: number }, event: Event) => void
    onQuestContextMenu: (event: MouseEvent, quest: { id: number }) => void
    onLineContextMenu: (event: MouseEvent, line: { id: number }) => void
    onStepContextMenu: (event: MouseEvent, step: { id: number }) => void
    onOpenGraph: (type: string, id: number) => void
    onToggleCompleted: (quest?: { id: number }) => void
    onOpenEdit: (quest?: { id: number }) => void
    onRequestDelete: (quest?: { id: number }) => void
    onBumpStep: (step: { id: number }, delta: number, questId?: number | null) => void
    onBeginEditStep: (step: { id: number }, questId?: number | null) => void
    onStepEditKeydown: (event: KeyboardEvent, step: { id: number }) => void
    onStepEditBlur: (step: { id: number }) => void
    onStepEditInput: (value: string) => void
    onDeleteAnimDone: () => void
    consumeRemoteEdit: (questId: number | null) => boolean
    onRef?: (kind: string, id: number) => void
  } = $props()

  let quests = $derived(questCatalog.list)
  let notes = $derived(noteCatalog.list)
  let searchQuery = $state('')
  let categoryOpen = $state({} as Record<string, boolean>)
  let lineOpen = $state({} as Record<string, boolean>)
  let frozenOpen = $state(false)
  let failedOpen = $state(true)

  let questQuerySpec = $derived(questSpec({ categories }))
  let parsedQuestSearch = $derived(parseQuery(searchQuery, questQuerySpec))
  let showAllQuests = $derived(tokenValues(parsedQuestSearch, 'status').includes('all'))
  let scopeCategoryId = $derived.by(() => {
    const slug = tokenValues(parsedQuestSearch, 'cat')[0]
    if (slug === 'none') return 'none' as const
    return slug ? (categories.find((c) => c.slug === slug)?.id ?? null) : null
  })
  let scopeQuestlineId = $derived.by(() => {
    const v = tokenValues(parsedQuestSearch, 'line')[0]
    if (!v) return null
    const byId = questlines.find((l) => String(l.id) === v)
    if (byId) return byId.id
    return questlines.find((l) => (l.title || '').replace(/\s+/g, '_').toLowerCase() === v.toLowerCase())?.id ?? null
  })
  const scopeQuestlinesFor = (cat: CategoryScope) => scopeQuestlineChoices({ scopeCategoryId: cat, questlines, quests, showAll: showAllQuests })
  let matchedQuests = $derived(quests.filter((q) => questMatchesQuery(q, parsedQuestSearch.text) && passes(parsedQuestSearch, questQuerySpec, q)))
  let listedQuests = $derived(listOpenOrAll(matchedQuests, { showAll: showAllQuests, statusTokens: tokenValues(parsedQuestSearch, 'status') }))
  let scopedQuests = $derived(inScope(listedQuests, { categoryId: scopeCategoryId, questlineId: scopeQuestlineId }))
  let byCategory = $derived(groupQuestsByCategory(scopedQuests, categories, questlines))
  let frozenQuests = $derived(inScope(matchedQuests.filter((q) => q.status === 'frozen'), { categoryId: scopeCategoryId, questlineId: scopeQuestlineId }))
  let failedInboxQuests = $derived(
    inScope(
      matchedQuests.filter((q) => q.status === 'failed' && q.failure_inbox),
      { categoryId: scopeCategoryId, questlineId: scopeQuestlineId },
    ),
  )

  function setShowAll(on: boolean) {
    const rest = tokenValues(parsedQuestSearch, 'status').filter((v) => v !== 'all')
    searchQuery = setToken(searchQuery, 'status', on ? [...rest, 'all'] : rest)
  }
  function setScopeCategory(id: CategoryScope) {
    const slug = id == null ? null : id === 'none' ? 'none' : categories.find((c) => c.id === id)?.slug
    searchQuery = setToken(setToken(searchQuery, 'line', []), 'cat', slug ? [slug] : [])
  }
  function setScopeQuestline(id: number | null) {
    searchQuery = setToken(searchQuery, 'line', id == null ? [] : [String(id)])
  }
  function isCategoryOpen(key: string | number) {
    return categoryOpen[key] !== false
  }
  function toggleCategory(key: string) {
    categoryOpen = { ...categoryOpen, [key]: !isCategoryOpen(key) }
  }
  function isLineOpen(catKey: string, lineKey: string) {
    const id = `${catKey}:${lineKey}`
    return lineOpen[id] !== false
  }
  function toggleLine(catKey: string, lineKey: string) {
    const id = `${catKey}:${lineKey}`
    lineOpen = { ...lineOpen, [id]: !isLineOpen(catKey, lineKey) }
  }
</script>

<SidebarLayout key="journal" class="journal__body">
  {#snippet sidebar()}
  <QuestSidebar
    {loading}
    {quests}
    {matchedQuests}
    {listedQuests}
    {byCategory}
    {frozenQuests}
    {frozenOpen}
    onToggleFrozen={() => (frozenOpen = !frozenOpen)}
    {failedInboxQuests}
    {failedOpen}
    onToggleFailed={() => (failedOpen = !failedOpen)}
    {categories}
    {scopeCategoryId}
    {scopeQuestlineId}
    {scopeQuestlinesFor}
    onScopeCategory={setScopeCategory}
    onScopeQuestline={setScopeQuestline}
    bind:searchQuery
    {showAllQuests}
    onShowAll={setShowAll}
    querySpec={questQuerySpec}
    parsedSearch={parsedQuestSearch}
    {selectedId}
    {categoryOpen}
    {lineOpen}
    {nowMs}
    {onSelect}
    {pickedIds}
    {onRowClick}
    {onClearPicked}
    {onSelectAll}
    {onTogglePin}
    {onQuestContextMenu}
    {onLineContextMenu}
    onToggleCategory={toggleCategory}
    onToggleLine={toggleLine}
    {labels}
    {onRef}
  />
  {/snippet}
  <QuestDetail
    {onOpenGraph}
    {selected}
    {kbStepId}
    {quests}
    {showAllQuests}
    {nowMs}
    {statusBusy}
    {deleting}
    {deleteAnimActive}
    {consumeRemoteEdit}
    {onDeleteAnimDone}
    {stepBusyId}
    {stepEditId}
    {stepEditValue}
    {onToggleCompleted}
    {onOpenEdit}
    {onRequestDelete}
    {onBumpStep}
    {onBeginEditStep}
    {onStepEditKeydown}
    {onStepEditBlur}
    {onStepEditInput}
    onQuestTitleContextMenu={onQuestContextMenu}
    onLineHeadContextMenu={(e) => {
      const line = questlines.find((l) => l.id === selected?.questline_id)
      if (line) onLineContextMenu(e, line)
    }}
    {onStepContextMenu}
    onSelectQuest={onSelect}
    {labels}
    {onRef}
    {notes}
    {questlines}
  />
</SidebarLayout>
