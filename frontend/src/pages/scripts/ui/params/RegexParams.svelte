<script lang="ts">
  /** A Regex node (RE2): match → yes/no, groups → g0…g9 (text of the whole match and of each group). */
  type P = { mode?: string; pattern?: string }
  let { params, onChange }: { params: P | undefined; onChange: (p: unknown) => void } = $props()

  let mode = $state(params?.mode ?? 'match')
  let pattern = $state(params?.pattern ?? '')
  let sample = $state('')

  const MODES = [
    { id: 'match', label: 'совпало или нет' },
    { id: 'groups', label: 'достать группы' },
  ]

  const commit = () => onChange({ mode, pattern })

  /** Quick check on a sample text; JS regexp is close to RE2 for the usual patterns, the server decides. */
  const probe = $derived.by(() => {
    if (!pattern || !sample) return ''
    try {
      const re = new RegExp(pattern)
      const m = re.exec(sample)
      if (!m) return 'нет совпадения'
      return mode === 'groups' ? m.map((g, i) => `g${i}=${g ?? ''}`).join('  ') : 'совпало'
    } catch {
      return 'шаблон не разбирается'
    }
  })
</script>

<label class="field">
  <span class="label">Режим</span>
  <select bind:value={mode} onchange={commit} aria-label="Режим">
    {#each MODES as m (m.id)}<option value={m.id}>{m.label}</option>{/each}
  </select>
</label>
<label class="field">
  <span class="label">Шаблон (RE2)</span>
  <input type="text" bind:value={pattern} onchange={commit} placeholder={'(\\d+)к(\\d+)'} spellcheck="false" aria-label="Шаблон" />
</label>
<label class="field">
  <span class="label">Проверить на тексте</span>
  <input type="text" bind:value={sample} placeholder="5+3к8" spellcheck="false" aria-label="Проверочный текст" />
</label>
{#if probe}<p class="explain">{probe}</p>{/if}
{#if mode === 'groups'}
  <p class="explain">Выходы g0 (всё совпадение), g1…g9 — группы шаблона. Нет совпадения — ветка не выполняется.</p>
{:else}
  <p class="explain">Текст уходит на «yes» при совпадении, иначе на «no».</p>
{/if}
