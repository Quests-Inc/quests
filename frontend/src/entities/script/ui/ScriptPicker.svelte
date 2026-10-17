<script lang="ts">
  import { loadScripts, scriptsFor, scriptsList } from '../model/scriptsList.svelte.ts'

  /**
   * Which script a step or a note runs instead of a command. `value` is the script id as text, '' = none. Only the scripts
   * that read this kind of entity are offered; one that is chosen but no longer listed stays visible so it is not lost silently.
   */
  let {
    value = $bindable(''),
    kind,
    onchange,
    label = 'Скрипт',
  }: { value?: string; kind: 'step' | 'note'; onchange?: () => void; label?: string } = $props()

  $effect(() => {
    void loadScripts()
  })

  let options = $derived(scriptsFor(kind))
  let known = $derived(!value || options.some((s) => String(s.id) === String(value)))
</script>

<select class="spick" aria-label={label} bind:value onchange={() => onchange?.()}>
  <option value="">без скрипта (команда)</option>
  {#each options as s (s.id)}
    <option value={String(s.id)}>{s.title}{s.needs_runner ? '' : ' · на сервере'}</option>
  {/each}
  {#if !known}
    <option value={String(value)}>скрипт {value} (не найден)</option>
  {/if}
</select>
{#if scriptsList.loaded && !options.length}
  <p class="spick__hint">Скриптов для этого пока нет: создайте во вкладке «Скрипты».</p>
{/if}

<style>
  .spick {
    min-width: 0;
    max-width: 100%;
  }
  .spick__hint {
    margin: 0.2rem 0 0;
    font-size: var(--text-xs, 0.75rem);
    color: var(--color-fg-muted);
  }
</style>
