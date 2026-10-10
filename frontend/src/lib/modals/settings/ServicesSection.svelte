<script>
  import SettingsBlock from './SettingsBlock.svelte'

  /** @type {{ health: { api: string, overlay: string, telegram: string, webdav?: string, detail?: any }, liveStatus?: string }} */
  let { health, liveStatus = 'off' } = $props()

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

  const rows = $derived([
    { name: 'API', status: health.api, label: statusLabel(health.api), meta: `HTTP · WS ${statusLabel(liveStatus)}` },
    { name: 'БД', status: chipOf(probeStatus('db')), label: statusLabel(probeStatus('db')), meta: ageLabel('db') },
    { name: 'HUD', status: health.overlay, label: statusLabel(health.overlay), meta: ageLabel('overlay') },
    { name: 'Telegram', status: health.telegram, label: statusLabel(health.telegram), meta: ageLabel('telegram') },
    { name: 'WebDAV', status: chipOf(probeStatus('webdav')), label: statusLabel(probeStatus('webdav')), meta: ageLabel('webdav') },
    { name: 'ClamAV', status: chipOf(probeStatus('clamav')), label: statusLabel(probeStatus('clamav')), meta: ageLabel('clamav') },
    { name: 'Бэкап', status: chipOf(probeStatus('backup')), label: statusLabel(probeStatus('backup')), meta: ageLabel('backup') },
  ])
</script>

<SettingsBlock
  title="Сервисы"
  hint="API отвечает сам; HUD и бот шлют heartbeat. WebDAV и ClamAV сервер опрашивает сам раз в 15 с. БД — сверка версии схемы при старте."
>
  <ul class="svc-list">
    {#each rows as r (r.name)}
      <li class="svc" data-status={r.status}>
        <span class="svc__dot" aria-hidden="true"></span>
        <span class="svc__name">{r.name}</span>
        <span class="svc__status">{r.label}</span>
        <span class="svc__meta">{r.meta}</span>
      </li>
    {/each}
  </ul>
</SettingsBlock>

<style>
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
  .svc[data-status='unknown'] .svc__name {
    color: var(--color-fg, #e8e8e8);
  }
</style>
