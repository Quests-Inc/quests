<script>
  // Opened from an invitation link (?invite=…). Joins the workspace with the
  // current account, or signs up a new account through the link.
  import { onMount } from 'svelte'
  import { fetchInvite, acceptInvite } from '../js/api.ts'
  import LoginScreen from './LoginScreen.svelte'

  /** @type {{ token: string }} */
  let { token } = $props()

  let info = $state(/** @type {{ workspace: string, authenticated: boolean } | null} */ (null))
  let invalid = $state(false)
  let wantLogin = $state(false)
  let username = $state('')
  let password = $state('')
  let busy = $state(false)
  let error = $state('')

  async function load() {
    try {
      info = await fetchInvite(token)
    } catch {
      invalid = true
    }
  }
  onMount(load)

  function home() {
    location.replace('/')
  }

  async function join(event) {
    event?.preventDefault()
    if (busy) return
    busy = true
    error = ''
    try {
      await acceptInvite(token, info?.authenticated ? {} : { username: username.trim(), password })
      home()
    } catch (err) {
      error = err instanceof Error ? err.message : String(err)
      password = ''
      busy = false
    }
  }

  async function afterLogin() {
    wantLogin = false
    await load()
  }
</script>

{#if wantLogin}
  <LoginScreen onAuthenticated={afterLogin} />
{:else}
  <div class="shell">
    <form class="card" onsubmit={join}>
      <h1>Quests</h1>
      {#if invalid}
        <p class="hint">Ссылка недействительна, уже использована или устарела.</p>
        <button type="button" onclick={home}>На главную</button>
      {:else if !info}
        <p class="hint">Проверяем приглашение…</p>
      {:else if info.authenticated}
        <p class="hint">Вас приглашают в пространство «{info.workspace}».</p>
        {#if error}<p class="error" role="alert">{error}</p>{/if}
        <button type="submit" disabled={busy}>{busy ? 'Вступаем…' : 'Вступить'}</button>
      {:else}
        <p class="hint">Вас приглашают в пространство «{info.workspace}». Создайте аккаунт.</p>
        <label>
          <span>Логин</span>
          <input type="text" autocomplete="username" autocapitalize="none" spellcheck="false" bind:value={username} disabled={busy} />
        </label>
        <label>
          <span>Пароль (не короче 8 символов)</span>
          <input type="password" autocomplete="new-password" bind:value={password} disabled={busy} />
        </label>
        {#if error}<p class="error" role="alert">{error}</p>{/if}
        <button type="submit" disabled={busy || !username.trim() || password.length < 8}>
          {busy ? 'Создаём…' : 'Создать аккаунт и вступить'}
        </button>
        <button type="button" class="link" onclick={() => (wantLogin = true)}>Уже есть аккаунт — войти</button>
      {/if}
    </form>
  </div>
{/if}

<style>
  .shell {
    min-height: 100vh;
    display: grid;
    place-items: center;
    padding: 24px;
    background: var(--color-bg);
  }
  .card {
    width: min(360px, 100%);
    display: flex;
    flex-direction: column;
    gap: 14px;
    padding: 28px;
    background: var(--color-bg-raised);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-lg);
  }
  h1 {
    margin: 0;
    font-family: var(--font-display);
    font-size: 1.6rem;
    color: var(--color-fg);
  }
  .hint {
    margin: -6px 0 4px;
    font-family: var(--font-ui);
    font-size: 0.85rem;
    color: var(--color-fg-muted);
  }
  label {
    display: flex;
    flex-direction: column;
    gap: 6px;
    font-family: var(--font-ui);
    font-size: 0.82rem;
    color: var(--color-fg-muted);
  }
  input {
    padding: 9px 11px;
    font-family: var(--font-body);
    font-size: 0.95rem;
    color: var(--color-fg);
    background: var(--color-bg);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-sm);
  }
  input:focus {
    outline: none;
    border-color: var(--color-accent);
  }
  button {
    padding: 10px;
    font-family: var(--font-ui);
    font-size: 0.9rem;
    color: var(--color-accent-fg);
    background: var(--color-accent);
    border: none;
    border-radius: var(--radius-sm);
    cursor: pointer;
  }
  button:disabled {
    opacity: 0.5;
    cursor: default;
  }
  button.link {
    background: transparent;
    color: var(--color-fg-muted);
    padding: 4px;
  }
  .error {
    margin: 0;
    font-family: var(--font-ui);
    font-size: 0.82rem;
    color: var(--color-danger);
  }
</style>
