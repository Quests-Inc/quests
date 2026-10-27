<script lang="ts">
  import { untrack } from 'svelte'
  import ConfirmModal from '@/shared/ui/ConfirmModal.svelte'
  import Icon from '@/shared/ui/Icon.svelte'
  import { candidates, codeSpecOf, customIdOf, customNodes, loadCustomNodes, loadScripts, PURPOSE_NONE, PURPOSES, scriptsList } from '@/entities/script'
  import { toast } from '@/shared/lib/toasts.svelte.ts'
  import { toastTrashed } from '@/shared/lib/trashUndo.ts'
  import { createScript, deleteScript, getCustomNodes, getScript, getScriptSamples, getScriptUses, updateScript } from '@/shared/api/client.ts'
  import type { CustomNode, ScriptUse } from '@/shared/api/execution.gen.ts'
  import type { Graph, Sample } from '@/shared/api/scriptgraph.gen.ts'
  import { groupScripts } from '../model/groups.ts'
  import { createCanvas } from '../model/canvas.svelte.ts'
  import { createScriptEditor } from '../model/editor.svelte.ts'
  import SidebarLayout from '@/shared/ui/SidebarLayout.svelte'
  import SidebarTools from '@/shared/ui/SidebarTools.svelte'
  import BakeModal from './BakeModal.svelte'
  import TryModal from './TryModal.svelte'
  import { openTarget } from '@/shared/lib/openTarget.ts'
  import CodeNodeModal from './CodeNodeModal.svelte'
  import Inspector from './Inspector.svelte'
  import NodeMetaModal from './NodeMetaModal.svelte'
  import Palette from './Palette.svelte'
  import ProblemList from './ProblemList.svelte'
  import ScriptCanvas from './ScriptCanvas.svelte'
  import ScriptListMenu from './ScriptListMenu.svelte'
  import { createScriptGroup } from '../model/scriptGroup.svelte.ts'
  import SidebarPicked from '@/shared/ui/SidebarPicked.svelte'

  /**
   * The script library (ADR-0003): the workspace's scripts, and an editor for the chosen one: nodes on a field, links from
   * outputs to inputs, the parameters of a node on the right, what the validator finds below. The field is the map's own
   * (its gestures, its minimap); only the cards and the wires are the editor's.
   */
  let { active = true }: { active?: boolean } = $props()


  const ed = createScriptEditor()
  const cv = createCanvas(ed)

  let selectedId = $state<number | null>(null)
  let busy = $state(false)
  let samples = $state<Sample[]>([])
  let scriptMenu = $state<{ open: boolean; x: number; y: number; id: number | null }>({ open: false, x: 0, y: 0, id: null })
  let bakeWhole = $state(false)
  let uses = $state<ScriptUse[]>([])
  let folded = $state<Set<string>>(new Set())
  let search = $state('')
  const shown = $derived.by(() => {
    const q = search.trim().toLowerCase()
    if (!q) return scriptsList.items
    return scriptsList.items.filter((s) => [s.title, s.description ?? '', ...(s.subjects ?? [])].some((t) => t.toLowerCase().includes(q)))
  })
  const groups = $derived(groupScripts(shown, PURPOSES, PURPOSE_NONE))
  function fold(key: string) {
    const next = new Set(folded)
    if (!next.delete(key)) next.add(key)
    folded = next
  }
  // 'leave' asks before dropping unsaved edits for another script; 'delete' before removing this one.
  let ask = $state<{ kind: 'leave'; id: number | null } | { kind: 'delete'; id: number } | null>(null)

  /** What a script's sources read, as the list says it. */
  const SUBJECT_WORDS: Record<string, string> = { step: 'шаг', note: 'заметка', template: 'шаблон', quest: 'квест', workspace: 'пространство' }
  let tryOpen = $state(false)
  /** Code goes to a runner: a node of the code kind, or a saved script the server says needs one. */
  const needsRunner = $derived(
    ed.graph.nodes.some((n) => ed.meta(n.type).kind === 'code') ||
      (!ed.dirty && Boolean(scriptsList.items.find((x) => x.id === selectedId)?.needs_runner))
  )

  /** `?tab=scripts&script=N` (a jump from a hook, a run…) opens that script. */
  function openFromUrl() {
    const id = Number(new URL(location.href).searchParams.get('script'))
    if (Number.isFinite(id) && id > 0 && id !== selectedId) pick(id)
  }

  $effect(() => {
    if (!active) return
    untrack(() => {
      void loadScripts(true)
      void loadCustomNodes(true)
      if (!samples.length) void getScriptSamples().then((x) => (samples = x)).catch(() => {})
      openFromUrl()
    })
    const onRoute = () => untrack(openFromUrl)
    window.addEventListener('quests:route', onRoute)
    return () => window.removeEventListener('quests:route', onRoute)
  })

  async function openScript(id: number) {
    busy = true
    try {
      const full = await getScript(id)
      selectedId = id
      await ed.open(full)
      uses = await getScriptUses(id).catch((): ScriptUse[] => [])
    } catch (e) {
      toast(e instanceof Error ? e.message : String(e), { kind: 'error' })
    } finally {
      busy = false
    }
  }

  const grp = createScriptGroup({
    scripts: () => scriptsList.items,
    selectedId: () => selectedId,
    api: { get: getScript, create: createScript as never, update: updateScript, remove: deleteScript },
    reload: () => loadScripts(true),
    onRemoved: (ids) => {
      if (selectedId != null && ids.includes(selectedId)) {
        selectedId = null
        ed.reset()
      }
    },
  })
  /** Scripts on screen in their order (folded groups are not there). */
  const visibleIds = () => groups.filter((g) => !folded.has(g.key)).flatMap((g) => g.items.map((s) => s.id))

  function pick(id: number) {
    if (id === selectedId) return
    if (ed.dirty) ask = { kind: 'leave', id }
    else void openScript(id)
  }

  /** A new script is an empty field with a default name; nodes and whole graphs are added from the right-click menu. */
  async function createEmpty() {
    busy = true
    try {
      const s = await createScript({ title: 'Новый скрипт', graph: { nodes: [], edges: [] } })
      await loadScripts(true)
      await openScript(s.id)
    } catch (e) {
      toast(e instanceof Error ? e.message : String(e), { kind: 'error' })
    } finally {
      busy = false
    }
  }

  /** Change a script of the list without opening it; the one open and edited takes the change into the editor instead. */
  async function patchScript(id: number, body: { title?: string; purpose?: string }) {
    try {
      if (id === selectedId && ed.dirty) {
        if (body.title !== undefined) ed.title = body.title
        if (body.purpose !== undefined) ed.purpose = body.purpose
        toast('Изменение в редакторе: сохраните скрипт', { kind: 'info' })
        return
      }
      const cur = await getScript(id)
      await updateScript(id, { version: cur.version, ...body })
      await loadScripts(true)
      if (id === selectedId) await openScript(id)
    } catch (e) {
      toast(e instanceof Error ? e.message : String(e), { kind: 'error' })
    }
  }

  /** The script as a custom node (a copy; the script stays): open it, then the bake form takes all its nodes. */
  async function bakeScript(id: number) {
    if (id !== selectedId) {
      if (ed.dirty) {
        toast('Сначала сохраните открытый скрипт', { kind: 'error' })
        return
      }
      await openScript(id)
    }
    bakeWhole = true
    bakeOpen = true
  }

  /** Put a whole graph (a sample, another script) into the open one, nodes unfolded, where the menu was opened. */
  async function insertGraph(kind: 'sample' | 'script', ref: string | number, at: { clientX: number; clientY: number }) {
    try {
      const graph: Graph | undefined = kind === 'sample' ? samples.find((x) => x.id === ref)?.graph : (await getScript(Number(ref))).graph
      if (graph) await cv.insertAtPoint(graph, at)
    } catch (e) {
      toast(e instanceof Error ? e.message : String(e), { kind: 'error' })
    }
  }

  async function save() {
    if (await ed.save()) {
      void loadScripts(true)
      toast('Скрипт сохранён', { kind: 'success' })
    }
  }

  /** Texts that embed the script about to be deleted: their links break while it is in the trash. */
  let askTexts = $state<ScriptUse[]>([])
  function askDelete(id: number) {
    ask = { kind: 'delete', id }
    askTexts = []
    void getScriptUses(id)
      .then((u) => {
        if (ask?.kind === 'delete' && ask.id === id) askTexts = u.filter((x) => x.via === 'text')
      })
      .catch(() => {})
  }
  const deleteMessage = $derived(
    'Скрипт со всеми версиями уйдёт в корзину на 30 дней. Если он подключён к шагу, заметке, шаблону или хуку, сервер откажет и покажет, где.' +
      (askTexts.length
        ? ` Он вставлен в тексты (${askTexts.length}): ${askTexts.map((u) => `«${u.title}»`).join(', ')} — эти вставки перестанут работать, пока скрипт в корзине.`
        : '')
  )

  async function remove(id: number) {
    ask = null
    try {
      await deleteScript(id)
      if (id === selectedId) {
        selectedId = null
        ed.reset()
      }
      await loadScripts(true)
      toastTrashed('script', [id], 'Скрипт в корзине', () => loadScripts(true))
    } catch (e) {
      toast(e instanceof Error ? e.message : String(e), { kind: 'error' })
    }
  }

  /** «нода 3 «Разбор»»: how a problem names its node. */
  function nodeLabel(id: number): string {
    const n = ed.graph.nodes.find((x) => x.id === id)
    return n ? `Нода ${id} «${ed.meta(n.type).title}»:` : `Нода ${id}:`
  }

  function onKey(e: KeyboardEvent) {
    const t = e.target
    const inField = t instanceof Element && t.closest('input, textarea, select, .scv')
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's' && selectedId != null) {
      e.preventDefault()
      void save()
    } else if (e.key === 'Escape' && grp.multi.size > 0 && !inField) grp.multi.clear()
    else if ((e.ctrlKey || e.metaKey) && e.code === 'KeyA' && !e.shiftKey && !inField) {
      e.preventDefault()
      grp.selectAll(visibleIds())
    }
  }

  // Custom nodes (quest=458): a node written as code, or the picked nodes baked into one.
  let codeModal = $state<{ open: boolean; editing: CustomNode | null }>({ open: false, editing: null })
  let metaNode = $state<CustomNode | null>(null)
  let bakeOpen = $state(false)
  function newCode() {
    codeModal.editing = null
    codeModal.open = true
  }
  const bakeCandidates = $derived(bakeOpen ? candidates(ed.graph, bakeWhole ? ed.graph.nodes.map((n) => n.id) : ed.pickedIds()) : [])
  const nodeTitle = (id: number) => {
    const n = ed.graph.nodes.find((x) => x.id === id)
    return n ? `${ed.meta(n.type).title} №${id}` : `№${id}`
  }

  async function refreshNodes() {
    await Promise.all([ed.refreshInfos(), loadCustomNodes(true)])
  }

  async function editCustom(type: string) {
    const id = customIdOf(type)
    if (id == null) return
    try {
      const node = (await getCustomNodes()).find((n) => n.id === id)
      if (!node) throw new Error('нода не найдена')
      if (!codeSpecOf(node)) {
        metaNode = node
        return
      }
      codeModal = { open: true, editing: node }
    } catch (e) {
      toast(e instanceof Error ? e.message : String(e), { kind: 'error' })
    }
  }
</script>

<svelte:window onkeydown={active ? onKey : undefined} />

<SidebarLayout key="scripts" class="scripts">
  {#snippet sidebar()}
  <aside class="scripts__list" aria-label="Скрипты">
    <SidebarTools bind:value={search} label="Поиск по скриптам" hint="По названию, описанию и видам сущностей" createLabel="Новый скрипт" createBusy={busy} onCreate={() => void createEmpty()} />
    <div class="scripts__items">
      {#each groups as g (g.key)}
        <section class="sgroup" style:--c={g.color}>
          <button type="button" class="sgroup__head" aria-expanded={!folded.has(g.key)} onclick={() => fold(g.key)}>
            <Icon name={folded.has(g.key) ? 'chevron-right' : 'chevron-down'} size={14} />
            <span class="sgroup__title">{g.title}</span>
            <span class="sgroup__count">{g.items.length}</span>
          </button>
          {#if !folded.has(g.key)}
            <ul>
              {#each g.items as s (s.id)}
                <li>
                  <button
                    type="button"
                    class="scripts__item"
                    class:scripts__item--on={s.id === selectedId}
                    class:scripts__item--picked={grp.multi.has(s.id)}
                    onclick={(e) => !grp.click(e, s.id, visibleIds()) && pick(s.id)}
                    oncontextmenu={(e) => {
                      e.preventDefault()
                      if (grp.multi.targets(s.id).length < 2) grp.multi.clear()
                      scriptMenu = { open: true, x: e.clientX, y: e.clientY, id: s.id }
                    }}
                  >
                    <span>{s.title}</span>
                    <small>{[(s.subjects ?? []).map((x) => SUBJECT_WORDS[x] ?? x).join(', '), s.needs_runner ? 'на раннере' : 'на сервере', `v${s.version}`].filter(Boolean).join(' · ')}</small>
                  </button>
                </li>
              {/each}
            </ul>
          {/if}
        </section>
      {:else}
        <p class="empty">{search.trim() ? 'Ничего не найдено' : 'Скриптов пока нет. «+» создаёт пустой.'}</p>
      {/each}
    </div>
    <SidebarPicked count={grp.multi.size} onClear={() => grp.multi.clear()} />
  </aside>
  {/snippet}

  <section class="scripts__main formscope">
    {#if selectedId == null}
      <div class="scripts__empty">
        <h2>Скрипты</h2>
        <p class="explain">
          Скрипт — это схема из нод: источник читает шаг или заметку, нода кода выполняется на раннере, разбор проверяет ответ,
          приёмник записывает результат. Выберите скрипт слева или создайте новый: пустое поле, куда ноды и целые графы добавляются правым кликом.
        </p>
      </div>
    {:else}
      <div class="scripts__bar">
        <input type="text" class="scripts__title" bind:value={ed.title} aria-label="Название скрипта" />
        <span class="scripts__ver">v{ed.version}{ed.dirty ? ' · не сохранён' : ''}</span>
        <div class="detail__actions">
          <button
            type="button"
            class="btn btn--icon"
            title="Запустить на… — проверить скрипт на шаге, заметке, событии или аргументах"
            aria-label="Запустить на…"
            disabled={!ed.graph.nodes.length || busy}
            onclick={() => (tryOpen = true)}
          >
            <Icon name="target" />
          </button>
          <button
            type="button"
            class="btn btn--icon"
            title="Запуски этого скрипта"
            aria-label="Запуски этого скрипта"
            onclick={() => selectedId != null && openTarget({ kind: 'runs', script: selectedId })}
          >
            <Icon name="list" />
          </button>
          <button
            type="button"
            class="btn btn--icon"
            disabled={!ed.dirty || ed.saving || busy}
            title={ed.saving ? 'Сохранение…' : 'Сохранить (Ctrl+S)'}
            aria-label="Сохранить"
            onclick={() => void save()}
          >
            {#if ed.saving}…{:else}<Icon name="save" />{/if}
          </button>
          <button type="button" class="btn btn--icon btn--danger" title="Удалить" aria-label="Удалить" onclick={() => selectedId != null && askDelete(selectedId)}>
            <Icon name="delete" />
          </button>
        </div>
      </div>
      <div class="scripts__meta">
        <input type="text" class="scripts__desc" bind:value={ed.description} placeholder="Описание (необязательно)" aria-label="Описание" />
        <select bind:value={ed.purpose} aria-label="Для чего скрипт" title="Для чего скрипт: по этому он попадает в списки выбора и в группы слева">
          <option value="">общий</option>
          {#each PURPOSES as p (p.id)}<option value={p.id}>{p.title}</option>{/each}
        </select>
      </div>
      {#if ed.error}<p class="scripts__err">{ed.error}</p>{/if}
      <div class="scripts__work">
        <Palette {ed} {cv} onNewCode={newCode} />
        <div class="scripts__center">
          <ProblemList problems={ed.problems} label={nodeLabel} onPick={(n) => ed.select(n)} />
          <ScriptCanvas
            {ed}
            {cv}
            onNewCode={newCode}
            onBake={() => {
              bakeWhole = false
              bakeOpen = true
            }}
            onEditCustom={(t) => void editCustom(t)}
            samples={samples.map((x) => ({ id: x.id, title: x.title }))}
            scripts={scriptsList.items.filter((x) => x.id !== selectedId).map((x) => ({ id: x.id, title: x.title }))}
            onInsert={(k, id, at) => void insertGraph(k, id, at)}
          />
        </div>
        <Inspector {ed} {uses} onEditCustom={(t) => void editCustom(t)} />
      </div>

    {/if}
  </section>
</SidebarLayout>

<TryModal
  open={tryOpen}
  scriptId={selectedId}
  graph={ed.graph}
  dirty={ed.dirty}
  subjects={ed.subjects}
  {needsRunner}
  onClose={() => (tryOpen = false)}
/>
<CodeNodeModal
  open={codeModal.open}
  editing={codeModal.editing}
  onClose={() => (codeModal.open = false)}
  onSaved={() => void refreshNodes()}
/>
<NodeMetaModal open={metaNode !== null} node={metaNode} onClose={() => (metaNode = null)} onSaved={() => void refreshNodes()} />
<ScriptListMenu
  menu={scriptMenu}
  scripts={scriptsList.items}
  onClose={() => (scriptMenu.open = false)}
  onRename={(id, title) => patchScript(id, { title })}
  onPurpose={(id, purpose) => void patchScript(id, { purpose })}
  onBake={(id) => void bakeScript(id)}
  onDelete={(id) => askDelete(id)}
  onCopy={(id) => void grp.copyRef(id)}
  onDuplicate={(id) => void grp.duplicate(id).then((n) => n != null && openScript(n))}
  group={scriptMenu.id != null ? grp.multi.targets(scriptMenu.id) : []}
  groupItems={grp.items(grp.multi.list)}
  onGroup={(action, ids) => grp.run(action, ids)}
/>
<ConfirmModal
  open={grp.ask.length > 0}
  title="Удалить скрипты?"
  message={`Выбранные скрипты (${grp.ask.length}) со всеми версиями уйдут в корзину на 30 дней. Подключённые сервер не отдаст; вставки в тексты перестанут работать, пока скрипт в корзине.`}
  busy={grp.busy}
  onCancel={grp.cancelDelete}
  onConfirm={grp.confirmDelete}
/>
<BakeModal
  open={bakeOpen}
  count={bakeWhole ? ed.graph.nodes.length : ed.picked.size}
  candidates={bakeCandidates}
  {nodeTitle}
  onClose={() => (bakeOpen = false)}
  nodes={customNodes.items}
  onBake={async (title, chosen, replaceId) => {
    await ed.bake(title, chosen, replaceId, bakeWhole)
    void loadCustomNodes(true)
  }}
/>

<ConfirmModal
  open={ask?.kind === 'delete'}
  title="Удалить скрипт?"
  message={deleteMessage}
  onCancel={() => (ask = null)}
  onConfirm={() => ask?.kind === 'delete' && void remove(ask.id)}
/>
<ConfirmModal
  open={ask?.kind === 'leave'}
  title="Есть несохранённые изменения"
  message="Перейти к другому скрипту без сохранения? Правки пропадут."
  confirmLabel="Перейти"
  onCancel={() => (ask = null)}
  onConfirm={() => {
    const id = ask?.kind === 'leave' ? ask.id : null
    ask = null
    if (id != null) void openScript(id)
  }}
/>

<style>
  .scripts__list {
    display: flex;
    flex-direction: column;
    min-height: 0;
    overflow: hidden;
    border-right: 1px solid var(--color-border, #333);
    background: var(--color-bg-raised, #1a1a1a);
  }
  .scripts__list ul {
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .scripts__items {
    flex: 1 1 auto;
    min-height: 0;
    overflow-y: auto;
    padding-bottom: 0.5rem;
  }
  /* A group of scripts of one purpose: a title bar in its colour and the scripts under it on a rail of that colour. */
  .sgroup {
    margin-top: 0.6rem;
  }
  .sgroup__head {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    width: 100%;
    padding: 0.3rem 0.6rem;
    border: 0;
    border-left: 3px solid var(--c);
    background: color-mix(in srgb, var(--c) 12%, var(--color-bg-raised, #1a1a1a));
    color: inherit;
    font-family: var(--font-ui, sans-serif);
    font-size: var(--text-xs, 0.75rem);
    font-weight: 600;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    text-align: left;
    cursor: pointer;
  }
  .sgroup__head:hover {
    background: color-mix(in srgb, var(--c) 20%, var(--color-bg-raised, #1a1a1a));
  }
  .sgroup__title {
    flex: 1 1 auto;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .sgroup__count {
    color: var(--color-fg-muted, #9a9a9a);
    font-weight: 400;
  }
  .sgroup ul {
    margin-left: 0.55rem !important;
    border-left: 1px solid color-mix(in srgb, var(--c) 45%, transparent);
  }
  small {
    color: var(--color-fg-muted, #9a9a9a);
    font-size: var(--text-xs, 0.75rem);
  }
  .scripts__item {
    width: 100%;
    display: grid;
    gap: 0.1rem;
    padding: 0.4rem 0.6rem 0.4rem 0.55rem;
    border: 0;
    border-left: 2px solid transparent;
    background: transparent;
    color: inherit;
    font-family: var(--font-body, Georgia, serif);
    font-size: var(--text-sm, 0.875rem);
    text-align: left;
    cursor: pointer;
  }
  .scripts__item:hover {
    background: color-mix(in srgb, var(--color-fg, #e8e8e8) 4%, transparent);
  }
  .scripts__item--picked,
  .scripts__item--picked,
  .scripts__item--on,
  .scripts__item--on:hover {
    border-left-color: var(--color-accent, #c9a227);
    background: color-mix(in srgb, var(--color-accent, #c9a227) 14%, var(--color-bg-raised, #1a1a1a));
  }
  .scripts__main {
    min-width: 0;
    min-height: 0;
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
    padding: 1rem 1.25rem 1.25rem;
    overflow: hidden;
  }
  .scripts__empty {
    max-width: 52rem;
    display: grid;
    gap: 0.8rem;
  }
  .scripts__empty h2 {
    margin: 0;
  }
  .scripts__bar {
    display: flex;
    align-items: center;
    gap: 0.6rem;
  }
  .scripts__meta {
    display: flex;
    gap: 0.6rem;
  }
  .scripts__meta input {
    flex: 1 1 auto;
    width: auto;
    min-width: 0;
  }
  .scripts__meta select {
    flex: 0 0 11rem;
    width: 11rem;
  }
  .scripts__title {
    flex: 1 1 14rem;
    min-width: 0;
    font-weight: 600;
  }
  .scripts__ver {
    flex: none;
    color: var(--color-fg-muted, #9a9a9a);
    font-size: var(--text-xs, 0.78rem);
  }
  .scripts__err {
    margin: 0;
    color: var(--color-danger, #b54a3a);
    font-size: var(--text-sm, 0.875rem);
  }
  .scripts__work {
    flex: 1 1 auto;
    min-height: 18rem;
    display: flex;
    gap: 0.7rem;
  }
  .scripts__center {
    flex: 1 1 auto;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }
  @media (max-width: 960px) {
    .scripts__main {
      overflow: auto;
    }
    .scripts__work {
      flex-direction: column;
    }
  }
</style>
