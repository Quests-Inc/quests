<script lang="ts">
  /**
   * Hours + minutes, both optional (empty = unset). Values stay as the raw
   * input strings/numbers; callers turn them into seconds (js/duration.js).
   * @type {{ hours?: string | number, minutes?: string | number, disabled?: boolean, label?: string }}
   */
  let {
    hours = $bindable(''),
    minutes = $bindable(''),
    disabled = false,
    label = 'Длительность',
  }: { hours?: string | number, minutes?: string | number, disabled?: boolean, label?: string } = $props()
</script>

<span class="dur" role="group" aria-label={label}>
  <input type="number" min="0" bind:value={hours} {disabled} aria-label="{label}: часы" />
  <span class="dur__unit">ч</span>
  <input type="number" min="0" max="59" bind:value={minutes} {disabled} aria-label="{label}: минуты" />
  <span class="dur__unit">мин</span>
</span>
