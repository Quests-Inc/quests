<script>
  import ModalShell from './ModalShell.svelte'
  import ModalHead from './ModalHead.svelte'
  import ServicesSection from './settings/ServicesSection.svelte'
  import AppearanceSection from './settings/AppearanceSection.svelte'
  import HotkeysSection from './settings/HotkeysSection.svelte'
  import WorkspaceSection from './settings/WorkspaceSection.svelte'
  import ArchiveSection from './settings/ArchiveSection.svelte'
  import AccountSection from './settings/AccountSection.svelte'
  import ConnectionsSection from './settings/ConnectionsSection.svelte'
  import { wsx, loadWorkspaceState } from './settings/workspaceState.svelte.js'

  /** @type {{ open?: boolean, onClose: () => void, health?: { api: string, overlay: string, telegram: string, webdav?: string, detail?: any }, liveStatus?: string, username?: string, onLogout?: (() => void) | null, section?: string | null }} */
  let {
    open = false,
    onClose,
    health = { api: 'unknown', overlay: 'unknown', telegram: 'unknown', webdav: 'unknown' },
    liveStatus = 'off',
    username = '',
    onLogout = null,
    section = null,
  } = $props()

  // Sections are shown only when they apply: workspaces and the account need a signed-in account.
  const sections = $derived([
    { id: 'services', label: 'Сервисы' },
    { id: 'appearance', label: 'Внешний вид' },
    { id: 'hotkeys', label: 'Клавиши' },
    ...(onLogout && wsx.enabled
      ? [
          { id: 'workspace', label: 'Пространство' },
          { id: 'archive', label: 'Архив' },
        ]
      : []),
    ...(onLogout ? [{ id: 'connections', label: 'Подключения' }, { id: 'account', label: 'Аккаунт' }] : []),
  ])

  let active = $state('services')
  /** Phone: the section list is a screen of its own; picking one opens it. */
  let pane = $state(/** @type {'nav' | 'content'} */ ('nav'))

  $effect(() => {
    if (!open) return
    active = section && ['services', 'appearance', 'hotkeys'].includes(section) ? section : 'services'
    pane = section ? 'content' : 'nav'
    if (onLogout) void loadWorkspaceState()
  })

  const activeLabel = $derived(sections.find((s) => s.id === active)?.label ?? '')

  function pick(id) {
    active = id
    pane = 'content'
  }
</script>

<ModalShell {open} {onClose} labelledby="settings-modal-title" zIndex={55} maxWidth="48rem">
  <ModalHead id="settings-modal-title" title="Настройки" icon="settings" {onClose} />

  <div class="set" data-pane={pane}>
    <nav class="set__nav" aria-label="Разделы настроек">
      {#each sections as s (s.id)}
        <button
          type="button"
          class="set__item"
          class:set__item--on={active === s.id}
          aria-current={active === s.id ? 'page' : undefined}
          onclick={() => pick(s.id)}
        >
          {s.label}
        </button>
      {/each}
    </nav>

    <div class="set__pane">
      <button type="button" class="btn btn--ghost set__back" onclick={() => (pane = 'nav')}>‹ Разделы</button>
      <h3 class="set__title">{activeLabel}</h3>
      <div class="set__content">
        {#if active === 'services'}
          <ServicesSection {health} {liveStatus} />
        {:else if active === 'appearance'}
          <AppearanceSection />
        {:else if active === 'hotkeys'}
          <HotkeysSection />
        {:else if active === 'workspace'}
          <WorkspaceSection {username} />
        {:else if active === 'archive'}
          <ArchiveSection />
        {:else if active === 'connections' && onLogout}
          <ConnectionsSection />
        {:else if active === 'account' && onLogout}
          <AccountSection {username} {onLogout} />
        {/if}
      </div>
    </div>
  </div>
</ModalShell>

<style>
  .set {
    display: grid;
    grid-template-columns: 11.5rem minmax(0, 1fr);
    /* The head is sticky; the pane scrolls on its own so the section list stays put. */
    height: min(36rem, calc(90vh - 3.6rem));
  }

  .set__nav {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: var(--space-3, 0.75rem) var(--space-2, 0.5rem);
    border-right: 1px solid var(--color-border, #333);
    overflow-y: auto;
  }

  .set__item {
    padding: 0.55rem 0.75rem;
    text-align: left;
    font-family: var(--font-ui);
    font-size: var(--text-sm, 0.875rem);
    color: var(--color-fg-muted, #9a9a9a);
    background: transparent;
    border: 0;
    border-left: 2px solid transparent;
    border-radius: var(--radius-sm, 6px);
    cursor: pointer;
  }

  .set__item:hover {
    color: var(--color-fg);
    background: var(--color-bg-hover, #2a2a2a);
  }

  .set__item--on {
    color: var(--color-accent, #c9a227);
    border-left-color: var(--color-accent, #c9a227);
    background: color-mix(in srgb, var(--color-accent, #c9a227) 10%, transparent);
  }

  .set__pane {
    display: flex;
    flex-direction: column;
    gap: var(--space-3, 0.75rem);
    min-width: 0;
    padding: var(--space-3, 0.75rem) var(--space-4, 1rem) var(--space-4, 1rem);
    overflow-y: auto;
  }

  .set__title {
    margin: 0;
    font-family: var(--font-display, Georgia, serif);
    font-size: var(--text-lg, 1.25rem);
    font-weight: 400;
    color: var(--color-fg);
  }

  .set__content {
    display: grid;
    gap: var(--space-3, 0.75rem);
    align-content: start;
  }

  .set__back {
    display: none;
    align-self: flex-start;
  }

  @media (max-width: 640px) {
    .set {
      grid-template-columns: 1fr;
      height: min(36rem, calc(100dvh - 7rem));
    }
    .set__nav {
      border-right: 0;
      padding: var(--space-2, 0.5rem);
    }
    .set__item {
      padding: 0.8rem 0.75rem;
      border-left: 0;
      border-bottom: 1px solid var(--color-border, #333);
      border-radius: 0;
    }
    .set[data-pane='nav'] .set__pane,
    .set[data-pane='content'] .set__nav {
      display: none;
    }
    .set__back {
      display: inline-flex;
    }
  }
</style>
