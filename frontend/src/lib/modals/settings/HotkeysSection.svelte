<script>
  import { hotkeyGroups } from '../../js/hotkeys.ts'
  import SettingsBlock from './SettingsBlock.svelte'

  /** Registered keyboard actions, read when the section opens (the registry is not reactive). */
  const hotkeys = hotkeyGroups()
</script>

<SettingsBlock
  title="Клавиши"
  hint="Работают, пока курсор не в поле ввода и не открыто окно. Esc в поле возвращает управление странице. На телефоне не действуют."
>
  {#each hotkeys as g (g.title)}
    <h5 class="keys__group">{g.title}</h5>
    <ul class="keys">
      {#each g.items as it (it.label)}
        <li class="keys__row">
          <kbd class="keys__kbd">{it.keys}</kbd>
          <span class="keys__label">{it.label}</span>
        </li>
      {/each}
    </ul>
  {/each}
</SettingsBlock>

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
</style>
