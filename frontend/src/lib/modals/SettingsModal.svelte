<script>
  import { THEMES, applyTheme, currentThemeId } from '../js/theme.js'
  import Icon from '../ui/Icon.svelte'
  import ModalShell from './ModalShell.svelte'
  import ModalHead from './ModalHead.svelte'
  import { hotkeyGroups } from '../js/hotkeys.js'
  import { fetchWorkspaces, createWorkspace, switchWorkspace } from '../js/api.js'

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

  let themeId = $state(currentThemeId())

  /** Registered keyboard actions, read when the modal opens (the registry is not reactive). */
  let hotkeys = $state(/** @type {{ title: string, items: { keys: string, label: string }[] }[]} */ ([]))
  let hotkeysEl = $state(/** @type {HTMLElement | null} */ (null))

  $effect(() => {
    if (!open) return
    themeId = currentThemeId()
    hotkeys = hotkeyGroups()
    if (section === 'hotkeys') {
      queueMicrotask(() => hotkeysEl?.scrollIntoView({ block: 'start' }))
    }
  })

  /** Workspaces of the signed-in account; switching reloads the app into the other space. */
  let wsList = $state(/** @type {{ id: number, name: string }[]} */ ([]))
  let wsCurrent = $state(0)
  let wsEnabled = $state(false)
  let wsNewName = $state('')
  let wsBusy = $state(false)
  let wsError = $state('')

  $effect(() => {
    if (!open || !onLogout) return
    fetchWorkspaces()
      .then((r) => {
        wsEnabled = !!r?.enabled
        wsList = r?.workspaces ?? []
        wsCurrent = r?.current ?? 0
      })
      .catch(() => (wsEnabled = false))
  })

  async function pickWorkspace(id) {
    if (wsBusy || id === wsCurrent) return
    wsBusy = true
    wsError = ''
    try {
      await switchWorkspace(id)
      location.reload()
    } catch (e) {
      wsError = e?.message || 'не удалось переключить'
      wsBusy = false
    }
  }

  async function addWorkspace() {
    const name = wsNewName.trim()
    if (!name || wsBusy) return
    wsBusy = true
    wsError = ''
    try {
      const w = await createWorkspace(name)
      await switchWorkspace(w.id)
      location.reload()
    } catch (e) {
      wsError = e?.message || 'не удалось создать'
      wsBusy = false
    }
  }

  function selectTheme(id) {
    themeId = applyTheme(id)
  }

  function statusLabel(s) {
    if (s === 'ok' || s === 'live') return 'онлайн'
    if (s === 'offline' || s === 'off') return 'офлайн'
    if (s === 'disabled') return 'не настроен'
    if (s === 'warn') return 'опережает'
    if (s === 'connecting' || s === 'reconnect') return 'переподключение'
    return 'неизвестно'
  }

  function ageLabel(comp) {
    const row = health?.detail?.components?.[comp]
    if (comp === 'db') {
      return row?.detail || ''
    }
    if (row?.status === 'disabled') {
      if (comp === 'clamav') return 'нет адреса'
      if (comp === 'backup') return 'выключен или ещё не было'
      return 'нет URL'
    }
    const age = row?.age_seconds
    if (age == null) {
      return comp === 'webdav' || comp === 'clamav' || comp === 'backup' ? 'нет опроса' : 'нет heartbeat'
    }
    if (age < 2) return 'только что'
    return `${age} с назад`
  }

  function probeStatus(comp) {
    return health?.detail?.components?.[comp]?.status || health[comp] || 'unknown'
  }

  function chipOf(s) {
    if (s === 'ok' || s === 'offline' || s === 'warn') return s
    return 'unknown'
  }
</script>

<ModalShell {open} {onClose} labelledby="settings-modal-title" zIndex={55} maxWidth="26rem">
      <ModalHead id="settings-modal-title" title="Настройки" icon="settings" {onClose} />

      <div class="modal__body">
        <section class="block">
          <h3 class="block__title">Сервисы</h3>
          <p class="block__hint">API отвечает сам; HUD и бот шлют heartbeat. WebDAV и ClamAV сервер опрашивает сам раз в 15 с. БД — сверка alembic при старте.</p>
          <ul class="svc-list">
            <li class="svc" data-status={health.api}>
              <span class="svc__dot" aria-hidden="true"></span>
              <span class="svc__name">API</span>
              <span class="svc__status">{statusLabel(health.api)}</span>
              <span class="svc__meta">HTTP · WS {statusLabel(liveStatus)}</span>
            </li>
            <li class="svc" data-status={chipOf(probeStatus('db'))}>
              <span class="svc__dot" aria-hidden="true"></span>
              <span class="svc__name">БД</span>
              <span class="svc__status">{statusLabel(probeStatus('db'))}</span>
              <span class="svc__meta">{ageLabel('db')}</span>
            </li>
            <li class="svc" data-status={health.overlay}>
              <span class="svc__dot" aria-hidden="true"></span>
              <span class="svc__name">HUD</span>
              <span class="svc__status">{statusLabel(health.overlay)}</span>
              <span class="svc__meta">{ageLabel('overlay')}</span>
            </li>
            <li class="svc" data-status={health.telegram}>
              <span class="svc__dot" aria-hidden="true"></span>
              <span class="svc__name">Telegram</span>
              <span class="svc__status">{statusLabel(health.telegram)}</span>
              <span class="svc__meta">{ageLabel('telegram')}</span>
            </li>
            <li class="svc" data-status={chipOf(probeStatus('webdav'))}>
              <span class="svc__dot" aria-hidden="true"></span>
              <span class="svc__name">WebDAV</span>
              <span class="svc__status">{statusLabel(probeStatus('webdav'))}</span>
              <span class="svc__meta">{ageLabel('webdav')}</span>
            </li>
            <li class="svc" data-status={chipOf(probeStatus('clamav'))}>
              <span class="svc__dot" aria-hidden="true"></span>
              <span class="svc__name">ClamAV</span>
              <span class="svc__status">{statusLabel(probeStatus('clamav'))}</span>
              <span class="svc__meta">{ageLabel('clamav')}</span>
            </li>
            <li class="svc" data-status={chipOf(probeStatus('backup'))}>
              <span class="svc__dot" aria-hidden="true"></span>
              <span class="svc__name">Бэкап</span>
              <span class="svc__status">{statusLabel(probeStatus('backup'))}</span>
              <span class="svc__meta">{ageLabel('backup')}</span>
            </li>
          </ul>
        </section>

        <section class="block">
          <h3 class="block__title">Тема приложения</h3>
          <p class="block__hint">Выбор палитры интерфейса. Сохраняется в этом браузере.</p>
          <ul class="theme-list" role="listbox" aria-label="Тема приложения">
            {#each THEMES as t (t.id)}
              <li>
                <button
                  type="button"
                  class="theme-row"
                  class:theme-row--on={themeId === t.id}
                  role="option"
                  aria-selected={themeId === t.id}
                  onclick={() => selectTheme(t.id)}
                >
                  <span class="theme-row__swatches" aria-hidden="true">
                    {#each t.swatches as hex}
                      <span class="theme-row__swatch" style="background: {hex}"></span>
                    {/each}
                  </span>
                  <span class="theme-row__meta">
                    <span class="theme-row__label">{t.label}</span>
                    <span class="theme-row__scheme">{t.scheme === 'dark' ? 'тёмная' : 'светлая'}</span>
                  </span>
                  {#if themeId === t.id}
                    <Icon name="check" size={14} />
                  {/if}
                </button>
              </li>
            {/each}
          </ul>
        </section>

        <section class="block" bind:this={hotkeysEl}>
          <h3 class="block__title">Клавиши</h3>
          <p class="block__hint">
            Работают, пока курсор не в поле ввода и не открыто окно. Esc в поле возвращает управление странице.
            На телефоне не действуют.
          </p>
          {#each hotkeys as g (g.title)}
            <h4 class="keys__group">{g.title}</h4>
            <ul class="keys">
              {#each g.items as it (it.label)}
                <li class="keys__row">
                  <kbd class="keys__kbd">{it.keys}</kbd>
                  <span class="keys__label">{it.label}</span>
                </li>
              {/each}
            </ul>
          {/each}
        </section>

        {#if onLogout && wsEnabled}
          <section class="block">
            <h3 class="block__title">Пространства</h3>
            <p class="block__hint">
              Отдельные наборы квестов, заметок и шаблонов: данные не пересекаются.
            </p>
            <ul class="ws">
              {#each wsList as w (w.id)}
                <li>
                  <button
                    type="button"
                    class="ws__item"
                    class:ws__item--on={w.id === wsCurrent}
                    disabled={wsBusy}
                    onclick={() => pickWorkspace(w.id)}
                  >
                    {w.name}
                  </button>
                </li>
              {/each}
            </ul>
            <form
              class="ws__new"
              onsubmit={(e) => {
                e.preventDefault()
                addWorkspace()
              }}
            >
              <input
                type="text"
                placeholder="Новое пространство"
                maxlength="80"
                bind:value={wsNewName}
                disabled={wsBusy}
              />
              <button type="submit" disabled={wsBusy || !wsNewName.trim()}>Создать</button>
            </form>
            {#if wsError}<p class="ws__err">{wsError}</p>{/if}
          </section>
        {/if}

        {#if onLogout}
          <section class="block">
            <h3 class="block__title">Аккаунт</h3>
            <div class="account">
              <span class="account__name">{username || 'вы вошли'}</span>
              <button type="button" class="account__logout" onclick={onLogout}>
                Выйти
              </button>
            </div>
          </section>
        {/if}
      </div>
</ModalShell>

<style>
  .keys__group {
    margin: var(--space-3, 0.75rem) 0 var(--space-1, 0.25rem);
    font-size: var(--text-xs, 0.8rem);
    color: var(--color-fg-muted, #9a9a9a);
  }

  .keys {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    gap: 4px;
  }

  .keys__row {
    display: flex;
    align-items: baseline;
    gap: 10px;
  }

  .keys__kbd {
    flex: none;
    min-width: 3.4rem;
    text-align: center;
    padding: 1px 6px;
    border: 1px solid var(--color-border, #444);
    border-radius: 4px;
    font-family: var(--font-ui);
    font-size: 0.8rem;
    color: var(--color-fg);
  }

  .keys__label {
    font-size: 0.85rem;
    color: var(--color-fg);
  }

  .ws {
    list-style: none;
    margin: 0 0 var(--space-3, 0.75rem);
    padding: 0;
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }

  .ws__item {
    padding: 6px 12px;
    font-family: var(--font-ui);
    font-size: 0.85rem;
    color: var(--color-fg);
    background: transparent;
    border: 1px solid var(--color-border);
    border-radius: var(--radius-sm);
    cursor: pointer;
  }

  .ws__item--on {
    border-color: var(--color-accent, var(--color-fg));
    cursor: default;
  }

  .ws__new {
    display: flex;
    gap: 8px;
  }

  .ws__new input {
    flex: 1;
    min-width: 0;
  }

  .ws__err {
    margin: 6px 0 0;
    font-size: 0.8rem;
    color: var(--color-danger);
  }

  .account {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
  }

  .account__name {
    font-family: var(--font-ui);
    font-size: 0.85rem;
    color: var(--color-fg);
  }

  .account__logout {
    padding: 6px 12px;
    font-family: var(--font-ui);
    font-size: 0.82rem;
    color: var(--color-danger);
    background: transparent;
    border: 1px solid var(--color-border);
    border-radius: var(--radius-sm);
    cursor: pointer;
  }

  .account__logout:hover {
    border-color: var(--color-danger);
  }

  /* Roomier than a form: these are separate blocks, not fields. */
  :global(.modal) .modal__body {
    gap: var(--space-5, 1.5rem);
  }

  .block__title {
    margin: 0 0 var(--space-1, 0.25rem);
    font-size: var(--text-sm, 0.875rem);
    letter-spacing: 0.04em;
    text-transform: uppercase;
    color: var(--color-fg-muted, #9a9a9a);
  }

  .block__hint {
    margin: 0 0 var(--space-3, 0.75rem);
    font-family: var(--font-body, Georgia, serif);
    font-size: var(--text-sm, 0.875rem);
    color: var(--color-fg-subtle, #6e6e6e);
  }

  .svc-list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    gap: 0.35rem;
  }

  .svc {
    display: grid;
    grid-template-columns: auto 1fr auto;
    grid-template-areas:
      "dot name status"
      "dot meta meta";
    column-gap: 0.55rem;
    row-gap: 0.1rem;
    align-items: center;
    padding: 0.55rem 0.7rem;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-lg, 12px);
    background: color-mix(in srgb, var(--color-bg, #121212) 70%, transparent);
  }

  .svc__dot {
    grid-area: dot;
    width: 0.5rem;
    height: 0.5rem;
    border-radius: 50%;
    background: currentColor;
  }

  .svc__name {
    grid-area: name;
    font-weight: 600;
    font-size: var(--text-sm, 0.875rem);
  }

  .svc__status {
    grid-area: status;
    font-family: var(--font-mono, monospace);
    font-size: var(--text-xs, 0.75rem);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }

  .svc__meta {
    grid-area: meta;
    font-size: var(--text-xs, 0.75rem);
    color: var(--color-fg-muted, #9a9a9a);
  }

  .svc[data-status='ok'] {
    color: var(--color-success, #7a9e3a);
  }

  .svc[data-status='offline'] {
    color: var(--color-danger, #b54a3a);
  }

  .svc[data-status='warn'] {
    color: var(--color-warning, #c47a20);
  }

  .svc[data-status='unknown'] {
    color: var(--color-fg-subtle, #6e6e6e);
  }

  .svc[data-status='ok'] .svc__name,
  .svc[data-status='offline'] .svc__name,
  .svc[data-status='warn'] .svc__name,
  .svc[data-status='unknown'] .svc__name {
    color: var(--color-fg, #e8e8e8);
  }

  .theme-list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    gap: 0.35rem;
  }

  .theme-row {
    width: 100%;
    display: grid;
    grid-template-columns: auto 1fr auto;
    align-items: center;
    gap: 0.75rem;
    padding: 0.55rem 0.7rem;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-lg, 12px);
    background: color-mix(in srgb, var(--color-bg, #121212) 70%, transparent);
    color: var(--color-fg, #e8e8e8);
    font: inherit;
    text-align: left;
    cursor: pointer;
  }

  .theme-row:hover {
    background: var(--color-bg-hover, #2a2a2a);
  }

  .theme-row--on {
    border-color: color-mix(in srgb, var(--color-accent, #c9a227) 55%, var(--color-border, #333));
    background: color-mix(in srgb, var(--color-accent, #c9a227) 14%, var(--color-bg, #121212));
  }

  .theme-row__swatches {
    display: inline-flex;
    gap: 0;
    border-radius: 999px;
    overflow: hidden;
    border: 1px solid var(--color-border, #333);
  }

  .theme-row__swatch {
    width: 0.7rem;
    height: 1.15rem;
  }

  .theme-row__meta {
    display: grid;
    gap: 0.1rem;
    min-width: 0;
  }

  .theme-row__label {
    font-weight: 600;
    font-size: var(--text-sm, 0.875rem);
  }

  .theme-row__scheme {
    font-size: var(--text-xs, 0.75rem);
    color: var(--color-fg-muted, #9a9a9a);
  }

</style>
