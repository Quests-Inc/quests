<script lang="ts">
  /** The code of a Code node: where it runs is the runner; the language only picks an interpreter line when the text has none. */
  let { params, onChange }: { params: { language?: string; text?: string } | undefined; onChange: (p: unknown) => void } = $props()

  let language = $state(params?.language ?? '')
  let text = $state(params?.text ?? '')
  const LANGS = [
    { id: '', label: 'sh' },
    { id: 'bash', label: 'bash' },
    { id: 'python', label: 'python' },
    { id: 'node', label: 'node' },
  ]
  const commit = () => onChange({ language, text })
</script>

<label class="field">
  <span class="label">Язык</span>
  <select bind:value={language} onchange={commit} aria-label="Язык">
    {#each LANGS as l (l.id)}<option value={l.id}>{l.label}</option>{/each}
  </select>
</label>
<label class="field">
  <span class="label">Текст</span>
  <textarea class="mono code" rows="12" spellcheck="false" bind:value={text} onchange={commit} aria-label="Текст кода"></textarea>
</label>
<p class="note">
  Значения со связей приходят в окружение: <code>QUESTS_IN_N&lt;номер&gt;_&lt;ПОРТ&gt;</code> или псевдоним связи. Результат — JSON в stdout.
  Исполняется только на раннере.
</p>

<style>
  .code {
    width: 100%;
    font-size: var(--text-xs, 0.75rem);
    line-height: 1.4;
    resize: vertical;
  }
  .note {
    margin: 0.3rem 0 0;
    font-size: var(--text-xs, 0.75rem);
    color: var(--color-fg-muted, #9a9a9a);
  }
</style>
