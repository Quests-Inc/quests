<script>
  import { THEMES, applyTheme, currentThemeId } from '../../js/theme.ts'
  import Icon from '../../ui/Icon.svelte'
  import SettingsBlock from './SettingsBlock.svelte'

  let themeId = $state(currentThemeId())

  function selectTheme(id) {
    themeId = applyTheme(id)
  }
</script>

<SettingsBlock title="Тема приложения" hint="Выбор палитры интерфейса. Сохраняется в этом браузере.">
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
</SettingsBlock>

<style>
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
