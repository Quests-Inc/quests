<script>
  import SettingsBlock from './SettingsBlock.svelte'
  import MfaBlock from './MfaBlock.svelte'

  /** @type {{ username?: string, onLogout: () => void }} */
  let { username = '', onLogout } = $props()
</script>

<SettingsBlock title="Аккаунт">
  <div class="row">
    <span class="name">{username || 'вы вошли'}</span>
    <button type="button" class="btn btn--danger-solid" onclick={onLogout}>Выйти</button>
  </div>
</SettingsBlock>

<MfaBlock />

<style>
  .row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
  }
  .name {
    font-size: var(--text-sm, 0.875rem);
    color: var(--color-fg);
  }
</style>
