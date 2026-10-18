<script lang="ts">
  import { untrack } from 'svelte'
  import ConfirmModal from '@/shared/ui/ConfirmModal.svelte'
  import Icon from '@/shared/ui/Icon.svelte'
  import { candidates, codeSpecOf, customIdOf, customNodes, loadCustomNodes, loadScripts, PURPOSE_NONE, PURPOSES, purposeOfKind, scriptsList, singleNodeGraph } from '@/entities/script'
  import { toast } from '@/shared/lib/toasts.svelte.ts'
  import { createScript, deleteScript, getCustomNodes, getScript, getScriptUses, installStarterNodes } from '@/shared/api/client.ts'
  import type { CustomNode, ScriptUse } from '@/shared/api/execution.gen.ts'
  import { groupScripts } from '../model/groups.ts'
  import { createCanvas } from '../model/canvas.svelte.ts'
  import { createScriptEditor } from '../model/editor.svelte.ts'
  import BakeModal from './BakeModal.svelte'
  import CodeNodeModal from './CodeNodeModal.svelte'
  import Inspector from './Inspector.svelte'
  import NodeMetaModal from './NodeMetaModal.svelte'
  import Palette from './Palette.svelte'
  import ProblemList from './ProblemList.svelte'
  import ScriptCanvas from './ScriptCanvas.svelte'

  /**
   * The script library (ADR-0003): the workspace's scripts, and an editor for the chosen one: nodes on a field, links from
   * outputs to inputs, the parameters of a node on the right, what the validator finds below. The field is the map's own
   * (its gestures, its minimap); only the cards and the wires are the editor's.
   */
  let { active = true, sidebarCollapsed = false }: { active?: boolean; sidebarCollapsed?: boolean } = $props()

  const USE_KINDS: Record<string, string> = { step: 'шаг', note: 'заметка', template: 'шаблон', template_step: 'шаг шаблона' }

  const ed = createScriptEditor()
  const cv = createCanvas(ed)

  let selectedId = $state<number | null>(null)
  let busy = $state(false)
  let menu = $state(false)
  let uses = $state<ScriptUse[]>([])
  let folded = $state<Set<string>>(new Set())
  const readyNodes = $derived(customNodes.items.filter((n) => !n.problem))
  const groups = $derived(groupScripts(scriptsList.items, PURPOSES, PURPOSE_NONE))
  function fold(key: string) {
    const next = new Set(folded)
    if (!next.delete(key)) next.add(key)
    folded = next
  }
  // 'leave' asks before dropping unsaved edits for another script; 'delete' before removing this one.
  let ask = $state<{ kind: 'leave'; id: number | null } | { kind: 'delete' } | null>(null)

  $effect(() => {
    if (!active) return
    untrack(() => {
      void loadScripts(true)
      void loadCustomNodes(true)
    })
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

  function pick(id: number) {
    if (id === selectedId) return
    if (ed.dirty) ask = { kind: 'leave', id }
    else void openScript(id)
  }

  /** A new script is made of one ready node (its sources and sinks come with it); the starting library can be installed first. */
  async function createFrom(node: CustomNode) {
    menu = false
    busy = true
    try {
      const subject = node.subjects?.[0] ?? ''
      const s = await createScript({
        title: node.title,
        description: node.description,
        purpose: purposeOfKind(subject),
        graph: singleNodeGraph(node),
      })
      await loadScripts(true)
      await openScript(s.id)
    } catch (e) {
      toast(e instanceof Error ? e.message : String(e), { kind: 'error' })
    } finally {
      busy = false
    }
  }

  async function installStarters() {
    busy = true
    try {
      await installStarterNodes()
      await loadCustomNodes(true)
      await ed.refreshInfos()
    } catch (e) {
      toast(e instanceof Error ? e.message : String(e), { kind: 'error' })
    } finally {
      busy = false
    }
  }

  async function save() {
    if (await ed.save()) {
      void loadScripts(true)
      toast('Скрипт сохранён', { kind: 'success' })
    }
  }

  async function remove() {
    ask = null
    if (selectedId == null) return
    try {
      await deleteScript(selectedId)
      selectedId = null
      ed.reset()
      await loadScripts(true)
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
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's' && selectedId != null) {
      e.preventDefault()
      void save()
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
  const bakeCandidates = $derived(bakeOpen ? candidates(ed.graph, ed.pickedIds()) : [])
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

<div class="scripts" class:scripts--collapsed={sidebarCollapsed}>
  <aside class="scripts__list" aria-label="Скрипты">
    <div class="scripts__tools">
      <button type="button" class="btn scripts__new" aria-expanded={menu} onclick={() => (menu = !menu)}>
        <Icon name="add" /> Новый скрипт
      </button>
    </div>
    {#if menu}
      <ul class="scripts__samples">
        {#each readyNodes as n (n.id)}
          <li>
            <button type="button" onclick={() => void createFrom(n)}>
              <b>{n.title}</b>
              <small>{n.description}</small>
            </button>
          </li>
        {:else}
          <li class="scripts__none-nodes">Готовых нод нет.</li>
        {/each}
        {#if !readyNodes.length}
          <li><button type="button" disabled={busy} onclick={() => void installStarters()}><b>Поставить стартовые ноды</b><small>Прежние образцы: шаг, заметка, шаблон, хук.</small></button></li>
        {/if}
      </ul>
    {/if}
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
                  <button type="button" class="scripts__item" class:scripts__item--on={s.id === selectedId} onclick={() => pick(s.id)}>
                    <span>{s.title}</span>
                    <small>{(s.subjects ?? []).join(', ')} · {s.needs_runner ? 'раннер' : 'сервер'} · v{s.version}</small>
                  </button>
                </li>
              {/each}
            </ul>
          {/if}
        </section>
      {:else}
        <p class="scripts__none">Скриптов пока нет. Начните с готовой ноды.</p>
      {/each}
    </div>
  </aside>

  <section class="scripts__main formscope">
    {#if selectedId == null}
      <div class="scripts__empty">
        <h2>Скрипты</h2>
        <p class="explain">
          Скрипт — это схема из нод: источник читает шаг или заметку, нода кода выполняется на раннере, разбор проверяет ответ,
          приёмник записывает результат. Выберите скрипт слева или начните с готовой ноды.
        </p>
        <div class="scripts__cards">
          {#each readyNodes as n (n.id)}
            <button type="button" class="scripts__card" onclick={() => void createFrom(n)}>
              <b>{n.title}</b>
              <small>{n.description}</small>
            </button>
          {:else}
            <button type="button" class="scripts__card" disabled={busy} onclick={() => void installStarters()}>
              <b>Поставить стартовые ноды</b>
              <small>Готовые ноды из прежних образцов: шаг, заметка, шаблон, хук.</small>
            </button>
          {/each}
        </div>
      </div>
    {:else}
      <div class="scripts__bar">
        <input type="text" class="scripts__title" bind:value={ed.title} aria-label="Название скрипта" />
        <span class="scripts__ver">v{ed.version}{ed.dirty ? ' · не сохранён' : ''}</span>
        <div class="detail__actions">
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
          <button type="button" class="btn btn--icon btn--danger" title="Удалить" aria-label="Удалить" onclick={() => (ask = { kind: 'delete' })}>
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
          <ScriptCanvas {ed} {cv} onNewCode={newCode} onBake={() => (bakeOpen = true)} />
        </div>
        <Inspector {ed} onEditCustom={(t) => void editCustom(t)} />
      </div>
      {#if uses.length}
        <p class="scripts__uses">
          Используется: {uses.map((u) => `${USE_KINDS[u.subject_type] ?? u.subject_type} «${u.title}»`).join(', ')}
        </p>
      {/if}
    {/if}
  </section>
</div>

<CodeNodeModal
  open={codeModal.open}
  editing={codeModal.editing}
  onClose={() => (codeModal.open = false)}
  onSaved={() => void refreshNodes()}
/>
<NodeMetaModal open={metaNode !== null} node={metaNode} onClose={() => (metaNode = null)} onSaved={() => void refreshNodes()} />
<BakeModal
  open={bakeOpen}
  count={ed.picked.size}
  candidates={bakeCandidates}
  {nodeTitle}
  onClose={() => (bakeOpen = false)}
  nodes={customNodes.items}
  onBake={async (title, chosen, replaceId) => {
    await ed.bake(title, chosen, replaceId)
    void loadCustomNodes(true)
  }}
/>

<ConfirmModal
  open={ask?.kind === 'delete'}
  title="Удалить скрипт?"
  message="Скрипт и все его версии будут удалены. Если он где-то подключён, сервер откажет и покажет, где."
  onCancel={() => (ask = null)}
  onConfirm={() => void remove()}
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
  /* The same frame as the journal and the notes: a raised list on the left (it folds from the header), the work on the right. */
  .scripts {
    flex: 1 1 auto;
    min-height: 0;
    display: grid;
    grid-template-columns: minmax(14rem, var(--sidebar-width, 16.5rem)) 1fr;
    overflow: hidden;
  }
  .scripts--collapsed {
    grid-template-columns: 0 1fr;
  }
  .scripts--collapsed .scripts__list {
    border-right: 0;
  }
  .scripts__list {
    display: flex;
    flex-direction: column;
    min-height: 0;
    overflow: hidden;
    border-right: 1px solid var(--color-border, #333);
    background: var(--color-bg-raised, #1a1a1a);
  }
  .scripts__tools {
    flex: none;
    padding: 0.55rem 0.6rem;
    border-bottom: 1px solid var(--color-border, #333);
  }
  .scripts__new {
    width: 100%;
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
  /* A group of scripts made from one sample: a title bar in the sample's colour and the scripts under it on a rail of that colour. */
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
  .scripts__samples {
    display: grid;
    gap: 0.3rem;
    padding: 0.5rem 0.6rem !important;
    border-bottom: 1px solid var(--color-border, #333);
  }
  .scripts__samples button,
  .scripts__card {
    width: 100%;
    display: grid;
    gap: 0.15rem;
    padding: 0.5rem 0.7rem;
    border: 0;
    border-radius: var(--pill-radius, 12px);
    background: var(--pill-bg, #0f0f0f);
    box-shadow: var(--pill-shadow, none);
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  .scripts__samples button:hover,
  .scripts__card:hover {
    background: var(--color-bg-hover, #2a2a2a);
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
  .scripts__item--on,
  .scripts__item--on:hover {
    border-left-color: var(--color-accent, #c9a227);
    background: color-mix(in srgb, var(--color-accent, #c9a227) 14%, var(--color-bg-raised, #1a1a1a));
  }
  .scripts__none {
    padding: 1rem;
    color: var(--color-fg-muted, #9a9a9a);
    font-size: var(--text-sm, 0.875rem);
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
  .scripts__cards {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(15rem, 1fr));
    gap: 0.6rem;
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
  .scripts__uses {
    margin: 0;
    font-size: var(--text-xs, 0.78rem);
    color: var(--color-fg-muted, #9a9a9a);
  }
  @media (max-width: 960px) {
    .scripts__main {
      overflow: auto;
    }
    .scripts__work {
      flex-direction: column;
    }
  }
  @media (max-width: 720px) {
    .scripts {
      grid-template-columns: minmax(7rem, 30%) 1fr;
    }
    .scripts--collapsed {
      grid-template-columns: 0 1fr;
    }
  }
</style>
