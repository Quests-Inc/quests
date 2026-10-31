<script lang="ts">
  import { onMount, tick } from 'svelte'
  import Icon from '@/shared/ui/Icon.svelte'
  import { wsx, loadWorkspaceList, pickWorkspace } from '../model/workspaceState.svelte.ts'

  onMount(() => { if (!wsx.loaded) void loadWorkspaceList() })
  const current = $derived(wsx.list.find((w: { id: number }) => w.id === wsx.current))
  let open = $state(false)
  let error = $state('')
  let root = $state<HTMLDivElement>()
  let trigger = $state<HTMLButtonElement>()

  async function toggle() {
    if (!wsx.loaded) await loadWorkspaceList()
    open = !open
  }
  async function choose(id: number) {
    if (id === wsx.current) { open = false; trigger?.focus(); return }
    error = ''
    await pickWorkspace(id)
    error = wsx.error
    if (error) { open = true; await tick(); trigger?.focus() }
    else open = false
  }
  function outside(event: PointerEvent) {
    if (root && event.target instanceof Node && !root.contains(event.target)) open = false
  }
  async function keydown(event: KeyboardEvent) {
    if (event.key === 'Escape') {
      if (open) { event.preventDefault(); event.stopPropagation(); open = false; trigger?.focus() }
      return
    }
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
    event.preventDefault()
    const wasOpen = open
    if (!open) { open = true; await tick() }
    const items = [...(root?.querySelectorAll<HTMLButtonElement>('[role=menuitemradio]') ?? [])]
    if (!items.length) return
    const index = items.indexOf(document.activeElement as HTMLButtonElement)
    const last = items.length - 1
    const next = event.key === 'Home' ? 0 : event.key === 'End' ? last
      : !wasOpen ? Math.max(0, items.findIndex((item) => item.getAttribute('aria-checked') === 'true'))
      : event.key === 'ArrowDown' ? (index + 1) % items.length : (index <= 0 ? last : index - 1)
    items[next]?.focus()
  }
</script>

<svelte:window onpointerdown={outside} />
<div class="workspace-switcher" bind:this={root} style:--workspace-color={current?.color || '#9a9a9a'} onkeydown={keydown} role="presentation" onfocusout={(event) => { if (!root?.contains(event.relatedTarget as Node)) open = false }}>
  <button class="workspace-trigger" bind:this={trigger} type="button" aria-label={`Пространство: ${current?.name || 'загрузка'}`} aria-haspopup="menu" aria-expanded={open} title="Переключить пространство" disabled={wsx.busy} onclick={toggle}>
    <span class="workspace-icon"><Icon name={current?.icon || 'layers'} size={16} /></span>
    <span class="workspace-name">{current?.name || 'Пространство…'}</span>
    <span class="workspace-arrow"><Icon name="chevron-down" size={11} /></span>
  </button>
  {#if open}
    <div class="workspace-menu" role="menu" aria-label="Пространства">
      <div class="workspace-menu__title">Пространства</div>
      {#each wsx.list as workspace (workspace.id)}
        <button type="button" class="workspace-option" role="menuitemradio" aria-checked={workspace.id === wsx.current} disabled={wsx.busy} onclick={() => choose(workspace.id)}>
          <span class="workspace-icon" style:color={workspace.color || '#9a9a9a'}><Icon name={workspace.icon || 'layers'} size={16} /></span>
          <span class="workspace-name">{workspace.name}</span>
          {#if workspace.id === wsx.current}<span class="workspace-check"><Icon name="checkmark" size={14} /></span>{/if}
        </button>
      {:else}
        <button type="button" class="workspace-option" onclick={() => loadWorkspaceList()}>Загрузить пространства</button>
      {/each}
      {#if error}<p class="workspace-error" role="alert">{error}</p>{/if}
    </div>
  {/if}
</div>

<style>
  .workspace-switcher { position: relative; min-width: 0; max-width: 13rem; margin-left: 0.65rem; }
  .workspace-trigger, .workspace-option { display: flex; align-items: center; gap: 0.5rem; font: inherit; font-size: var(--text-sm); cursor: pointer; color: var(--color-fg); }
  .workspace-trigger { width: 100%; min-height: 2rem; padding: 0.35rem 0.55rem; border: 1px solid color-mix(in srgb, var(--workspace-color) 28%, var(--color-border)); border-radius: var(--radius-md); background: color-mix(in srgb, var(--workspace-color) 7%, var(--color-bg-raised)); font-family: var(--font-display); }
  .workspace-trigger:hover, .workspace-trigger[aria-expanded=true] { border-color: var(--workspace-color); background: color-mix(in srgb, var(--workspace-color) 12%, var(--color-bg-raised)); }
  .workspace-icon, .workspace-arrow, .workspace-check { display: flex; flex-shrink: 0; }
  .workspace-icon { color: var(--workspace-color); }
  .workspace-arrow { color: var(--color-fg-muted); margin-left: auto; }
  .workspace-name { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; text-align: left; }
  .workspace-menu { position: absolute; top: calc(100% + 0.45rem); left: 0; z-index: 50; width: max-content; min-width: 13rem; max-width: min(22rem, calc(100vw - 3rem)); max-height: 65vh; overflow-y: auto; padding: 0.3rem; border: 1px solid var(--color-border); border-radius: var(--radius-md); background: var(--color-bg-raised); box-shadow: 0 12px 28px -8px #0009; }
  .workspace-menu__title { padding: 0.4rem 0.55rem 0.5rem; font-size: var(--text-xs); color: var(--color-fg-subtle); }
  .workspace-option { width: 100%; min-height: 2.25rem; border: 0; border-radius: var(--radius-sm); background: transparent; padding: 0.45rem 0.55rem; }
  .workspace-option:hover, .workspace-option:focus-visible { background: var(--color-bg-muted); }
  .workspace-option[aria-checked=true] { background: color-mix(in srgb, var(--color-accent) 8%, transparent); }
  .workspace-check { color: var(--color-accent); margin-left: auto; }
  button:focus-visible { outline: 1px solid var(--color-accent); outline-offset: 2px; }
  button:disabled { opacity: 0.5; cursor: wait; }
  .workspace-error { padding: 0.4rem 0.55rem; color: var(--color-danger); font-size: var(--text-sm); max-width: 18rem; }
  @media (max-width: 600px) { .workspace-switcher { max-width: 38vw; margin-left: 0.4rem; } }
</style>
