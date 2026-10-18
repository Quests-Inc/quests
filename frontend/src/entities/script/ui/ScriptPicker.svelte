<script lang="ts">
  import Icon from '@/shared/ui/Icon.svelte'
  import { toast } from '@/shared/lib/toasts.svelte.ts'
  import { createScript, installStarterNodes } from '@/shared/api/client.ts'
  import type { CustomNode } from '@/shared/api/execution.gen.ts'
  import { customNodes, loadCustomNodes, nodesFor } from '../model/customNodes.svelte.ts'
  import { singleNodeGraph } from '../model/customNode.ts'
  import { purposeOfKind } from '../model/purpose.ts'
  import { loadScripts, scriptsFor, scriptsList } from '../model/scriptsList.svelte.ts'

  /**
   * Which script a step or a note runs. `value` is the script id as text, '' = none. Only the scripts that read this kind of
   * entity are offered; one that is chosen but no longer listed stays visible so it is not lost silently. A new script can be
   * made from a ready node right here (a script that is that node) and is chosen at once; `entityId` (the entity it is made for) goes
   * into the node's `entity_id` field, so the script names its entity itself. `legacy` is a command typed before scripts existed: it is shown, not edited.
   */
  let {
    value = $bindable(''),
    kind,
    onchange,
    label = 'Скрипт',
    legacy = '',
    onClearLegacy,
    entityId,
  }: { value?: string; kind: 'step' | 'note' | 'template' | 'quest' | 'workspace'; onchange?: () => void; label?: string; legacy?: string; onClearLegacy?: () => void; entityId?: number } = $props()

  let menu = $state(false)
  let busy = $state(false)

  $effect(() => {
    void loadScripts()
    void loadCustomNodes()
  })

  let options = $derived(scriptsFor(kind))
  let known = $derived(!value || options.some((s) => String(s.id) === String(value)))
  let fits = $derived(nodesFor(kind))
  let legacyLine = $derived(
    String(legacy || '')
      .trim()
      .split('\n')[0]
  )

  /** The nodes to start from; with none in the workspace yet, the starting library goes in first. */
  async function toggle() {
    menu = !menu
    if (menu && !customNodes.loaded) await loadCustomNodes()
  }

  async function installStarters() {
    busy = true
    try {
      await installStarterNodes()
      await loadCustomNodes(true)
    } catch (e) {
      toast(e instanceof Error ? e.message : String(e), { kind: 'error' })
    } finally {
      busy = false
    }
  }

  async function make(n: CustomNode) {
    busy = true
    try {
      const created = await createScript({ title: n.title, description: n.description, purpose: purposeOfKind(kind), graph: singleNodeGraph(n, entityId) })
      await loadScripts(true)
      value = String(created.id)
      menu = false
      onchange?.()
    } catch (e) {
      toast(e instanceof Error ? e.message : String(e), { kind: 'error' })
    } finally {
      busy = false
    }
  }
</script>

<div class="spick formscope">
  <div class="spick__row">
    <select aria-label={label} bind:value onchange={() => onchange?.()}>
      <option value="">скрипт не выбран</option>
      {#each options as s (s.id)}
        <option value={String(s.id)}>{s.title}{s.needs_runner ? '' : ' · на сервере'}</option>
      {/each}
      {#if !known}
        <option value={String(value)}>скрипт {value} (не найден)</option>
      {/if}
    </select>
    <button type="button" class="btn btn--ghost" aria-expanded={menu} title="Создать скрипт из готовой ноды и выбрать его" onclick={() => void toggle()}>
      <Icon name="add" size={14} /> Из ноды
    </button>
  </div>
  {#if menu}
    <ul class="spick__menu">
      {#each fits as n (n.id)}
        <li>
          <button type="button" disabled={busy} onclick={() => void make(n)}>
            <b>{n.title}</b>
            <small>{n.description}</small>
          </button>
        </li>
      {:else}
        <li class="spick__none">
          Готовых нод для этого нет.
          <button type="button" class="btn btn--link" disabled={busy} onclick={() => void installStarters()}>Поставить стартовые ноды</button>
          или соберите свою во вкладке «Скрипты».
        </li>
      {/each}
    </ul>
  {/if}
  {#if legacyLine && !value}
    <p class="explain spick__legacy">
      Команда без скрипта: <code>{legacyLine}</code>
      {#if onClearLegacy}<button type="button" class="btn btn--link" onclick={onClearLegacy}>убрать</button>{/if}
    </p>
  {/if}
</div>

<style>
  .spick {
    display: grid;
    gap: 0.4rem;
    min-width: 0;
  }
  .spick__row {
    display: flex;
    gap: 0.4rem;
    min-width: 0;
  }
  .spick__row select {
    flex: 1 1 auto;
    min-width: 0;
  }
  .spick__row .btn {
    flex: none;
  }
  .spick__menu {
    margin: 0;
    padding: 0;
    list-style: none;
    display: grid;
    gap: 0.3rem;
  }
  .spick__menu button {
    width: 100%;
    display: grid;
    gap: 0.15rem;
    padding: 0.45rem 0.7rem;
    border: 0;
    border-radius: var(--pill-radius, 12px);
    background: var(--pill-bg, #0f0f0f);
    box-shadow: var(--pill-shadow, none);
    color: inherit;
    font: inherit;
    font-size: var(--text-sm, 0.875rem);
    text-align: left;
    cursor: pointer;
  }
  .spick__menu button:hover:not(:disabled) {
    background: var(--color-bg-hover, #2a2a2a);
  }
  small,
  .spick__none {
    color: var(--color-fg-muted, #9a9a9a);
    font-size: var(--text-xs, 0.75rem);
  }
  .spick__legacy code {
    overflow-wrap: anywhere;
  }
</style>
