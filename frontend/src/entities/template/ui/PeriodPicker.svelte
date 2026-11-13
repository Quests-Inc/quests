<script lang="ts">
 import OptionPills from '@/shared/ui/OptionPills.svelte'
 import { WEEKDAY_LABELS } from '@/shared/api/templateConsts.ts'
 let {freq=$bindable('daily'),weekdays=$bindable(new Set<number>()),monthdays=$bindable(new Set<number>()),cycleLength=$bindable(4),cycleStart=$bindable(''),cycleDays=$bindable(new Set<number>())}: {
  freq:string; weekdays:Set<number>; monthdays:Set<number>; cycleLength:number; cycleStart:string; cycleDays:Set<number>
 }=$props()
  const FREQ_OPTIONS = [
    { id: 'daily', label: 'День' },
    { id: 'weekly', label: 'Неделя' },
    { id: 'monthly', label: 'Месяц' },
    { id: 'cycle', label: 'Свой промежуток' },
  ]
 const DAY_OPTIONS=WEEKDAY_LABELS.map(d=>({id:String(d.id),label:d.label}))
 let weekdayIds=$derived(new Set([...weekdays].map(String)))
  function toggleDay(idStr: any) {
    const id = Number(idStr)
    const next = new Set(weekdays)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    if (next.size === 0) next.add(id)
    weekdays = next
  }

</script>
        <div class="inline-line">
          <OptionPills options={FREQ_OPTIONS} bind:value={freq} label="Период" compact wrap />
          {#if freq === 'weekly'}
            <OptionPills options={DAY_OPTIONS} selected={weekdayIds} onToggle={toggleDay} label="Дни недели" compact />
          {/if}
        </div>
        {#if freq === 'monthly'}
          <div class="period-days" role="group" aria-label="Числа месяца">
            {#each Array.from({length:31},(_,i)=>i+1) as day}
              <button type="button" class="period-day" class:period-day--on={monthdays.has(day)} aria-pressed={monthdays.has(day)} onclick={()=>{const next=new Set(monthdays);next.has(day)?next.delete(day):next.add(day);monthdays=next}}>{day}</button>
            {/each}
          </div>
          <p class="hint">Выберите числа месяца. Если даты нет в месяце, выпуск пропускается.</p>
        {:else if freq === 'cycle'}
          <div class="field-row">
            <label class="field"><span class="label">Дней в цикле</span><input type="number" min="1" max="366" step="1" required bind:value={cycleLength} /></label>
            <label class="field"><span class="label">Начальная дата · день 1</span><input type="date" required bind:value={cycleStart} /></label>
          </div>
          <div class="period-days" role="group" aria-label="Дни цикла для выпуска">
            {#each Array.from({length:Math.min(366,Math.max(0,Number(cycleLength)||0))},(_,i)=>i+1) as day}
              <button type="button" class="period-day" class:period-day--on={cycleDays.has(day)} aria-pressed={cycleDays.has(day)} onclick={()=>{const next=new Set(cycleDays);next.has(day)?next.delete(day):next.add(day);cycleDays=next}}>{day}</button>
            {/each}
          </div>
          <p class="hint">Выберите дни выпуска. Цикл повторяется с начальной даты. Для 2/2: 4 дня, выпуск в дни 1 и 2.</p>
        {/if}

<style>
  .period-days { display:grid; grid-template-columns:repeat(7,minmax(0,1fr)); max-width:24rem; gap:0.35rem; max-height:15rem; overflow:auto; padding:0.15rem; }
  .period-day { padding:0.5rem 0; border:1px solid var(--color-border); border-radius:var(--radius-md); background:transparent; color:var(--color-fg-muted); }
  .period-day--on { border-color:var(--color-accent); background:color-mix(in srgb,var(--color-accent) 15%,transparent); color:var(--color-accent); }
</style>
