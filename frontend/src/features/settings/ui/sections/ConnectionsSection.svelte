<script lang="ts">
  import { onMount } from 'svelte'
  import SettingsBlock from './SettingsBlock.svelte'
  import { fetchTokens, createToken, revokeToken } from '@/shared/api/client.ts'
  import { currentWorkspaceName } from '../../model/workspaceState.svelte.ts'

  let tokens = $state(([] as { id: number, name: string, created_at: string, last_used_at: string | null }[]))
  let name = $state('')
  let busy = $state(false)
  let error = $state('')
  /** The freshly minted secret: shown once, then gone. */
  let fresh = $state((null as { name: string, token: string } | null))
  let copied = $state('')

  const PRESETS = ['HUD', 'MCP', 'Android', 'CLI', 'Бот']

  async function load() {
    try {
      tokens = (await fetchTokens())?.tokens ?? []
    } catch (e) {
      error = e?.message || 'не удалось загрузить токены'
    }
  }
  onMount(load)

  async function create() {
    const n = name.trim()
    if (!n || busy) return
    busy = true
    error = ''
    try {
      const r = await createToken(n)
      fresh = { name: r.name, token: r.token }
      name = ''
      await load()
    } catch (e) {
      error = e?.message || 'не удалось создать токен'
    } finally {
      busy = false
    }
  }

  async function revoke(t: { name: any; id: number }) {
    if (!confirm(`Отозвать токен «${t.name}»? Клиент с ним перестанет работать.`)) return
    error = ''
    try {
      await revokeToken(t.id)
      tokens = tokens.filter((x) => x.id !== t.id)
    } catch (e) {
      error = e?.message || 'не удалось отозвать'
    }
  }

  async function copy(text: string, key: string) {
    try {
      await navigator.clipboard.writeText(text)
      copied = key
      setTimeout(() => (copied = ''), 1800)
    } catch {
      /* the field is selectable — copy by hand */
    }
  }

  function when(iso: string | null | undefined) {
    if (!iso) return 'ни разу'
    const d = new Date(iso.endsWith('Z') ? iso : iso.replace(' ', 'T') + 'Z')
    return Number.isNaN(d.getTime()) ? iso : d.toLocaleString('ru-RU', { dateStyle: 'short', timeStyle: 'short' })
  }

  const envSnippet = $derived(fresh ? `QUESTS_API=${location.origin}\nQUESTS_API_TOKEN=${fresh.token}` : '')
</script>

<SettingsBlock
  title="Новый токен"
  hint={`Токен даёт клиенту доступ к вашему аккаунту в пространстве «${currentWorkspaceName() || '…'}». Одно название — одно устройство или программа: так проще отзывать.`}
>
  <form
    class="row"
    onsubmit={(e) => {
      e.preventDefault()
      create()
    }}
  >
    <input type="text" class="grow" placeholder="Например: HUD на ноутбуке" maxlength="64" bind:value={name} disabled={busy} />
    <button type="submit" class="btn btn--accent" disabled={busy || !name.trim()}>Создать</button>
  </form>
  <div class="presets">
    {#each PRESETS as p (p)}
      <button type="button" class="btn btn--ghost preset" onclick={() => (name = p)}>{p}</button>
    {/each}
  </div>
  {#if error}<p class="err" role="alert">{error}</p>{/if}
</SettingsBlock>

{#if fresh}
  <SettingsBlock title={`Токен «${fresh.name}» — скопируйте сейчас`} hint="Он показывается один раз: на сервере хранится только его отпечаток. Потеряли — отзовите и создайте новый.">
    <div class="row">
      <input type="text" class="grow mono" readonly value={fresh.token} onfocus={(e) => e.currentTarget.select()} aria-label="Токен" />
      <button type="button" class="btn btn--accent" onclick={() => copy(fresh.token, 'token')}>
        {copied === 'token' ? 'Скопировано' : 'Копировать'}
      </button>
    </div>
    <div class="row">
      <pre class="snippet">{envSnippet}</pre>
      <button type="button" class="btn" onclick={() => copy(envSnippet, 'env')}>{copied === 'env' ? 'Скопировано' : 'Копировать как env'}</button>
    </div>
    <button type="button" class="btn btn--ghost done" onclick={() => (fresh = null)}>Я сохранил, скрыть</button>
  </SettingsBlock>
{/if}

<SettingsBlock title="Мои токены в этом пространстве">
  {#if tokens.length === 0}
    <p class="note">Токенов пока нет.</p>
  {:else}
    <ul class="tokens">
      {#each tokens as t (t.id)}
        <li class="tokens__row">
          <span class="tokens__text">
            <span class="tokens__name">{t.name}</span>
            <span class="tokens__meta">создан {when(t.created_at)} · использован {when(t.last_used_at)}</span>
          </span>
          <button type="button" class="btn btn--danger tokens__x" onclick={() => revoke(t)}>Отозвать</button>
        </li>
      {/each}
    </ul>
  {/if}
</SettingsBlock>

<SettingsBlock title="Как подключить">
  <ul class="how">
    <li><b>HUD (оверлей)</b> — на ПК: переменные <code>QUESTS_API</code> и <code>QUESTS_API_TOKEN</code> (см. выше) перед запуском.</li>
    <li><b>MCP / агенты</b> — те же две переменные в окружении <code>quests-mcp</code>.</li>
    <li><b>Android-HUD</b> — в настройках приложения: адрес сервера и токен.</li>
    <li><b>CLI</b> — <code>QUESTS_API</code> и <code>QUESTS_API_TOKEN</code>, затем <code>quests list</code>.</li>
    <li><b>Telegram-бот</b> — необязательный личный клиент, см. <code>docs/telegram-bot.md</code>.</li>
  </ul>
  <p class="note">Токен привязан к этому пространству: клиент работает только в нём. Чтобы подключить другое пространство — переключитесь в него и создайте токен там.</p>
</SettingsBlock>

<style>
  .row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.5rem;
  }
  .row .btn {
    white-space: nowrap;
  }
  .grow {
    flex: 1 1 12rem;
    min-width: 0;
  }
  .mono {
    font-family: var(--font-mono, monospace);
    font-size: 0.8rem;
  }
  .presets {
    display: flex;
    flex-wrap: wrap;
    gap: 0.25rem;
  }
  .preset {
    height: 1.7rem;
    padding: 0 0.6rem;
    border: 1px solid var(--color-border, #333);
  }
  .snippet {
    flex: 1 1 14rem;
    margin: 0;
    padding: 0.5rem 0.65rem;
    overflow-x: auto;
    font-family: var(--font-mono, monospace);
    font-size: 0.75rem;
    color: var(--color-fg);
    background: var(--color-bg, #121212);
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-sm, 6px);
  }
  .done {
    justify-self: start;
  }
  .tokens {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
  }
  /* Name over meta on the left, the action pinned to the right edge — rows keep one shape
     whatever the text length (a flex-wrap row let the button drop to its own line). */
  .tokens__row {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    align-items: center;
    gap: 0.75rem;
    padding: 0.5rem 0;
    border-top: 1px solid var(--color-border, #333);
  }
  .tokens__row:first-child {
    border-top: 0;
    padding-top: 0;
  }
  .tokens__text {
    display: grid;
    gap: 0.1rem;
    min-width: 0;
  }
  .tokens__name {
    font-size: var(--text-sm, 0.875rem);
    color: var(--color-fg);
    overflow-wrap: anywhere;
  }
  .tokens__meta {
    font-size: 0.75rem;
    color: var(--color-fg-muted, #9a9a9a);
  }
  .tokens__x {
    white-space: nowrap;
  }
  .how {
    margin: 0;
    padding-left: 1.1rem;
    display: grid;
    gap: 0.3rem;
    font-size: 0.82rem;
    line-height: 1.4;
    color: var(--color-fg);
  }
  .how code {
    font-family: var(--font-mono, monospace);
    font-size: 0.78rem;
  }
  .note {
    margin: 0;
    font-size: 0.8rem;
    line-height: 1.4;
    color: var(--color-fg-muted, #9a9a9a);
  }
  .err {
    margin: 0;
    font-size: 0.82rem;
    color: var(--color-danger, #b54a3a);
  }
</style>
