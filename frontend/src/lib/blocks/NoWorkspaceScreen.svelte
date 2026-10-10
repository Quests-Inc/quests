<script>
  // Signed in but a member of no workspace (e.g. removed from the last one).
  import { createWorkspace, switchWorkspace, logout } from '../js/api.ts'

  let name = $state('')
  let busy = $state(false)
  let error = $state('')

  async function create(event) {
    event.preventDefault()
    if (busy || !name.trim()) return
    busy = true
    error = ''
    try {
      const w = await createWorkspace(name.trim())
      await switchWorkspace(w.id)
      location.reload()
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
      busy = false
    }
  }

  async function out() {
    try {
      await logout()
    } finally {
      location.reload()
    }
  }
</script>

<div class="shell">
  <form class="card" onsubmit={create}>
    <h1>Quests</h1>
    <p class="hint">Вы не состоите ни в одном пространстве. Создайте своё или откройте ссылку-приглашение.</p>
    <input type="text" placeholder="Название пространства" maxlength="80" bind:value={name} disabled={busy} />
    {#if error}<p class="error" role="alert">{error}</p>{/if}
    <button type="submit" disabled={busy || !name.trim()}>Создать пространство</button>
    <button type="button" class="link" onclick={out}>Выйти</button>
  </form>
</div>

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
  input {
    padding: 9px 11px;
    font-family: var(--font-body);
    font-size: 0.95rem;
    color: var(--color-fg);
    background: var(--color-bg);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-sm);
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
