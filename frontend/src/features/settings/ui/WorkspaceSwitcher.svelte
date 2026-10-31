<script lang="ts">
  import { onDestroy, onMount, tick } from 'svelte'
  import WorkspaceModal from './WorkspaceModal.svelte'
  import Icon from '@/shared/ui/Icon.svelte'
  import { wsx, loadWorkspaceList, pickWorkspace } from '../model/workspaceState.svelte.ts'

  let { onOpenSettings = () => {}, canManage = true }: { onOpenSettings?: () => void; canManage?: boolean } = $props()
  let modalOpen = $state(false)
  let editing = $state<{ id: number; name: string; color?: string; icon?: string } | null>(null)
  function edit(workspace: typeof editing, event?: MouseEvent) {
    event?.preventDefault()
    clearHoverTimers()
    open = false
    editing = workspace
    modalOpen = true
  }
  function settings() { clearHoverTimers(); open = false; onOpenSettings() }
  onMount(() => { if (!wsx.loaded) void loadWorkspaceList() })
  const current = $derived(wsx.list.find((w: { id: number }) => w.id === wsx.current))
  let open = $state(false)
  let error = $state('')
  let root = $state<HTMLDivElement>()
  let trigger = $state<HTMLButtonElement>()

  let hoverTimer: ReturnType<typeof setTimeout> | undefined
  let leaveTimer: ReturnType<typeof setTimeout> | undefined
  function clearHoverTimers() {
    clearTimeout(hoverTimer)
    clearTimeout(leaveTimer)
  }
  onDestroy(clearHoverTimers)
  function enter(event: PointerEvent) {
    if (event.pointerType !== 'mouse') return
    clearHoverTimers()
    if (!open && !modalOpen && !wsx.busy) hoverTimer = setTimeout(() => {
      if (!wsx.busy) open = true
    }, 500)
  }
  function leave(event: PointerEvent) {
    if (event.pointerType !== 'mouse') return
    clearHoverTimers()
    leaveTimer = setTimeout(() => { open = false }, 220)
  }

  async function toggle() {
    clearHoverTimers()
    if (!wsx.loaded) await loadWorkspaceList()
    open = !open
  }
  async function choose(id: number) {
    clearHoverTimers()
    if (id === wsx.current) { open = false; trigger?.focus(); return }
    error = ''
    await pickWorkspace(id)
    error = wsx.error
    if (error) { open = true; await tick(); trigger?.focus() }
    else open = false
  }
  function outside(event: PointerEvent) {
    if (root && event.target instanceof Node && !root.contains(event.target)) { clearHoverTimers(); open = false }
  }
  async function keydown(event: KeyboardEvent) {
    clearHoverTimers()
    if (event.key === 'Escape') {
      if (open) { event.preventDefault(); event.stopPropagation(); open = false; trigger?.focus() }
      return
    }
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
    event.preventDefault()
    const wasOpen = open
    if (!open) { open = true; await tick() }
    const items = [...(root?.querySelectorAll<HTMLButtonElement>('[role=menuitemradio]:not(:disabled), [role=menuitem]:not(:disabled), .workspace-edit:not(:disabled)') ?? [])]
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
<div class="workspace-switcher" bind:this={root} style:--workspace-color={current?.color || '#9a9a9a'} onkeydown={keydown} onpointerenter={enter} onpointerleave={leave} role="presentation" onfocusout={(event) => { if (!root?.contains(event.relatedTarget as Node)) open = false }}>
  <button class="workspace-trigger" bind:this={trigger} type="button" aria-label={`Пространство: ${current?.name || 'загрузка'}`} aria-haspopup="menu" aria-expanded={open} title="Переключить пространство" disabled={wsx.busy} onclick={toggle} oncontextmenu={(event) => { event.preventDefault(); if (canManage && current?.role === 'owner') edit(current, event) }}>
    <span class="workspace-icon"><Icon name={current?.icon || 'layers'} size={16} /></span>
    <span class="workspace-name">{current?.name || 'Пространство…'}</span>
    <span class="workspace-arrow"><Icon name="chevron-down" size={11} /></span>
  </button>
  {#if open}
    <div class="workspace-menu" role="menu" aria-label="Пространства">
      {#each wsx.list as workspace (workspace.id)}
        <div class="workspace-entry" class:workspace-entry--current={workspace.id === wsx.current}>
          <button type="button" class="workspace-option" role="menuitemradio" aria-checked={workspace.id === wsx.current} disabled={wsx.busy} onclick={() => choose(workspace.id)} oncontextmenu={(event) => { event.preventDefault(); if (canManage && workspace.role === 'owner') edit(workspace, event) }}>
            <span class="workspace-icon" style:color={workspace.color || '#9a9a9a'}><Icon name={workspace.icon || 'layers'} size={16} /></span>
            <span class="workspace-name">{workspace.name}</span>
            {#if workspace.role}<span class="workspace-role">{workspace.role === 'owner' ? 'владелец' : 'участник'}</span>{/if}
          </button>
          {#if canManage && workspace.role === 'owner'}
            <button type="button" class="workspace-edit" title="Редактировать пространство" aria-label={`Редактировать пространство ${workspace.name}`} disabled={wsx.busy} onclick={(event) => edit(workspace, event)}><Icon name="edit" size={14} /></button>
          {/if}
        </div>
      {:else}
        <button type="button" class="workspace-option" onclick={() => loadWorkspaceList()}>Загрузить пространства</button>
      {/each}
      <div class="workspace-menu__actions">
        <button type="button" class="workspace-option" role="menuitem" disabled={wsx.busy || !canManage} title={canManage ? undefined : 'Создание пространства доступно после входа в аккаунт'} onclick={() => edit(null)}><Icon name="add" size={15} />Добавить пространство</button>
        <button type="button" class="workspace-option" role="menuitem" onclick={settings}><Icon name="settings" size={15} />Настройки пространств</button>
      </div>
      {#if error}<p class="workspace-error" role="alert">{error}</p>{/if}
    </div>
  {/if}
</div>

<WorkspaceModal open={modalOpen} workspace={editing} onClose={() => (modalOpen = false)} />

<style>
  .workspace-switcher { position: relative; min-width: 0; max-width: 13rem; margin-left: 0.65rem; }
  .workspace-trigger, .workspace-option { display: flex; align-items: center; gap: 0.5rem; font: inherit; font-size: var(--text-sm); cursor: pointer; color: var(--color-fg); }
  .workspace-trigger { width: 100%; min-height: 2rem; padding: 0.35rem 0.55rem; border: 1px solid color-mix(in srgb, var(--workspace-color) 28%, var(--color-border)); border-radius: var(--radius-md); background: color-mix(in srgb, var(--workspace-color) 7%, var(--color-bg-raised)); font-family: var(--font-display); line-height: 1; }
  .workspace-trigger:hover, .workspace-trigger[aria-expanded=true] { border-color: var(--workspace-color); background: color-mix(in srgb, var(--workspace-color) 12%, var(--color-bg-raised)); }
  .workspace-icon, .workspace-arrow { display: flex; flex-shrink: 0; }
  .workspace-icon { color: var(--workspace-color); }
  .workspace-arrow { color: var(--color-fg-muted); margin-left: auto; }
  .workspace-trigger .workspace-name { position: relative; top: -1px; }
  .workspace-name { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; text-align: left; }
  .workspace-menu { position: absolute; top: calc(100% + 0.45rem); left: 0; z-index: 50; width: max-content; min-width: 13rem; max-width: min(22rem, calc(100vw - 3rem)); max-height: 65vh; overflow-y: auto; padding: 0.3rem; border: 1px solid var(--color-border); border-radius: var(--radius-md); background: var(--color-bg-raised); box-shadow: 0 12px 28px -8px #0009; }
  .workspace-entry { display: flex; align-items: center; border-radius: var(--radius-sm); }
  .workspace-entry--current { background: color-mix(in srgb, var(--color-accent) 8%, transparent); }
  .workspace-entry:hover, .workspace-entry:focus-within { background: var(--color-bg-muted); }
  .workspace-entry .workspace-option { flex: 1; min-width: 0; }
  .workspace-edit { display: grid; place-items: center; flex-shrink: 0; width: 2rem; height: 2rem; border: 0; background: transparent; color: var(--color-fg-muted); cursor: pointer; border-radius: var(--radius-sm); }
  .workspace-edit:hover { color: var(--color-fg); }
  .workspace-menu__actions { border-top: 1px solid var(--color-border); margin-top: 0.3rem; padding-top: 0.3rem; }
  .workspace-option { width: 100%; min-height: 2.25rem; border: 0; border-radius: var(--radius-sm); background: transparent; padding: 0.45rem 0.55rem; }
  .workspace-menu__actions .workspace-option:hover, .workspace-menu__actions .workspace-option:focus-visible { background: var(--color-bg-muted); }
  .workspace-role { color: var(--color-fg-subtle); margin-left: auto; padding-left: 0.7rem; font-size: var(--text-xs); white-space: nowrap; }
  button:focus-visible { outline: 1px solid var(--color-accent); outline-offset: 2px; }
  button:disabled { opacity: 0.5; cursor: wait; }
  .workspace-error { padding: 0.4rem 0.55rem; color: var(--color-danger); font-size: var(--text-sm); max-width: 18rem; }
  @media (max-width: 600px) { .workspace-switcher { max-width: 38vw; margin-left: 0.4rem; } }
</style>
