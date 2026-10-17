<script lang="ts">
  import { untrack } from 'svelte'
  import ConfirmModal from '@/shared/ui/ConfirmModal.svelte'
  import { loadScripts, scriptsList } from '@/entities/script'
  import { toast } from '@/shared/lib/toasts.svelte.ts'
  import { createScript, deleteScript, getScript, getScriptSamples, getScriptUses } from '@/shared/api/client.ts'
  import type { ScriptUse } from '@/shared/api/execution.gen.ts'
  import type { Sample } from '@/shared/api/scriptgraph.gen.ts'
  import { createCanvas } from '../model/canvas.svelte.ts'
  import { createScriptEditor } from '../model/editor.svelte.ts'
  import Inspector from './Inspector.svelte'
  import Palette from './Palette.svelte'
  import ProblemList from './ProblemList.svelte'
  import ScriptCanvas from './ScriptCanvas.svelte'

  /**
   * The script library (ADR-0003): the workspace's scripts, and an editor for the chosen one: nodes on a field, links from
   * outputs to inputs, the parameters of a node on the right, what the validator finds below. The field is the map's own
   * (its gestures, its minimap); only the cards and the wires are the editor's.
   */
  let { active = true }: { active?: boolean } = $props()

  const ed = createScriptEditor()
  const cv = createCanvas(ed)

  let samples = $state<Sample[]>([])
  let selectedId = $state<number | null>(null)
  let busy = $state(false)
  let menu = $state(false)
  let uses = $state<ScriptUse[]>([])
  // 'leave' asks before dropping unsaved edits for another script; 'delete' before removing this one.
  let ask = $state<{ kind: 'leave'; id: number | null } | { kind: 'delete' } | null>(null)

  $effect(() => {
    if (!active) return
    untrack(() => {
      void loadScripts(true)
      if (!samples.length) void getScriptSamples().then((s) => (samples = s)).catch(() => {})
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

  async function createFrom(sample: Sample) {
    menu = false
    busy = true
    try {
      const s = await createScript({ title: sample.title, description: sample.description, graph: sample.graph })
      await loadScripts(true)
      await openScript(s.id)
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

  function onKey(e: KeyboardEvent) {
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's' && selectedId != null) {
      e.preventDefault()
      void save()
    }
  }
</script>

<svelte:window onkeydown={active ? onKey : undefined} />

<div class="scripts">
  <aside class="scripts__list" aria-label="Скрипты">
    <div class="scripts__new">
      <button type="button" class="btn btn--primary" aria-expanded={menu} onclick={() => (menu = !menu)}>+ Новый скрипт</button>
      {#if menu}
        <ul class="scripts__samples">
          {#each samples as s (s.id)}
            <li>
              <button type="button" onclick={() => void createFrom(s)}>
                <b>{s.title}</b>
                <small>{s.description}</small>
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
    <ul>
      {#each scriptsList.items as s (s.id)}
        <li>
          <button type="button" class="scripts__item" class:scripts__item--on={s.id === selectedId} onclick={() => pick(s.id)}>
            <span>{s.title}</span>
            <small>{(s.subjects ?? []).join(', ')} · {s.needs_runner ? 'раннер' : 'сервер'} · v{s.version}</small>
          </button>
        </li>
      {:else}
        <li class="scripts__none">Скриптов пока нет. Начните с образца.</li>
      {/each}
    </ul>
  </aside>

  <section class="scripts__main">
    {#if selectedId == null}
      <div class="scripts__empty">
        <h2>Скрипты</h2>
        <p>
          Скрипт — это схема из нод: источник читает шаг или заметку, нода кода выполняется на раннере, разбор проверяет ответ,
          приёмник записывает результат. Выберите скрипт слева или начните с образца.
        </p>
        <div class="scripts__cards">
          {#each samples as s (s.id)}
            <button type="button" class="scripts__card" onclick={() => void createFrom(s)}>
              <b>{s.title}</b>
              <small>{s.description}</small>
            </button>
          {/each}
        </div>
      </div>
    {:else}
      <div class="scripts__bar">
        <input class="scripts__title" bind:value={ed.title} aria-label="Название скрипта" />
        <span class="scripts__ver">v{ed.version}{ed.dirty ? ' · не сохранён' : ''}</span>
        <button type="button" class="btn btn--primary" disabled={!ed.dirty || ed.saving || busy} onclick={() => void save()}>
          {ed.saving ? 'Сохранение…' : 'Сохранить'}
        </button>
        <button type="button" class="btn btn--danger-solid" onclick={() => (ask = { kind: 'delete' })}>Удалить</button>
      </div>
      <input class="scripts__desc" bind:value={ed.description} placeholder="Описание (необязательно)" aria-label="Описание" />
      {#if ed.error}<p class="scripts__err">{ed.error}</p>{/if}
      <div class="scripts__work">
        <Palette {ed} {cv} />
        <ScriptCanvas {ed} {cv} />
        <Inspector {ed} />
      </div>
      <ProblemList problems={ed.problems} checking={ed.checking} onPick={(n) => ed.select(n)} />
      {#if uses.length}
        <p class="scripts__uses">
          Используется: {uses.map((u) => `${u.subject_type === 'step' ? 'шаг' : 'заметка'} «${u.title}»`).join(', ')}
        </p>
      {/if}
    {/if}
  </section>
</div>

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
  .scripts {
    flex: 1 1 auto;
    min-height: 0;
    display: flex;
    gap: 1rem;
    padding: 1rem 1.25rem 1.25rem;
    overflow: hidden;
  }
  .scripts__list {
    flex: 0 0 15rem;
    min-height: 0;
    overflow: auto;
    display: grid;
    align-content: start;
    gap: 0.6rem;
  }
  .scripts__list ul {
    margin: 0;
    padding: 0;
    list-style: none;
    display: grid;
    gap: 0.3rem;
  }
  .scripts__samples {
    margin-top: 0.4rem !important;
  }
  .scripts__samples button,
  .scripts__card {
    width: 100%;
    display: grid;
    gap: 0.15rem;
    padding: 0.5rem 0.7rem;
    border: 1px solid var(--color-border, #333);
    border-radius: 10px;
    background: var(--pill-bg, #151515);
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  .scripts__samples button:hover,
  .scripts__card:hover {
    border-color: var(--color-accent, #c9a227);
  }
  small {
    color: var(--color-fg-muted, #9a9a9a);
    font-size: var(--text-xs, 0.75rem);
  }
  .scripts__item {
    width: 100%;
    display: grid;
    gap: 0.1rem;
    padding: 0.45rem 0.7rem;
    border: 1px solid transparent;
    border-radius: 10px;
    background: transparent;
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  .scripts__item:hover {
    background: var(--pill-bg, #151515);
  }
  .scripts__item--on {
    border-color: var(--color-accent, #c9a227);
    background: var(--pill-bg, #151515);
  }
  .scripts__none {
    color: var(--color-fg-muted, #9a9a9a);
    font-size: var(--text-sm, 0.875rem);
  }
  .scripts__main {
    flex: 1 1 auto;
    min-width: 0;
    min-height: 0;
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }
  .scripts__empty {
    max-width: 52rem;
  }
  .scripts__empty p {
    color: var(--color-fg-muted, #9a9a9a);
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
    flex-wrap: wrap;
  }
  .scripts__title {
    flex: 1 1 14rem;
    min-width: 0;
    font-weight: 600;
  }
  .scripts__ver {
    color: var(--color-fg-muted, #9a9a9a);
    font-size: var(--text-xs, 0.78rem);
  }
  .scripts__desc {
    width: 100%;
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
  .scripts__uses {
    margin: 0;
    font-size: var(--text-xs, 0.78rem);
    color: var(--color-fg-muted, #9a9a9a);
  }
  @media (max-width: 960px) {
    .scripts {
      flex-direction: column;
      overflow: auto;
    }
    .scripts__list {
      flex-basis: auto;
    }
    .scripts__work {
      flex-direction: column;
    }
  }
</style>
