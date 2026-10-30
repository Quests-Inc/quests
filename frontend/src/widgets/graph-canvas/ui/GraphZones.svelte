<script lang="ts">
  import { readZones, type GraphZone } from '@/entities/graph'
  let { zones, cam, drawing = false, worldAt, onChange, onFinish }: {
    zones: GraphZone[]; cam: { x: number; y: number; k: number }; drawing?: boolean
    worldAt: (e: { clientX: number; clientY: number }) => { x: number; y: number }
    onChange: (zones: GraphZone[]) => void; onFinish: () => void
  } = $props()
  let selected = $state<string | null>(null)
  let drag = $state<{ kind: 'draw' | 'move' | 'resize'; start: { x: number; y: number }; original: GraphZone; pointer: number } | null>(null)
  let preview = $state<GraphZone | null>(null)
  const shown = $derived(preview ? [...zones.filter((z) => z.id !== preview!.id), preview] : zones)
  const current = $derived(zones.find((z) => z.id === selected))

  function start(e: PointerEvent, zone?: GraphZone, kind: 'draw' | 'move' | 'resize' = 'draw') {
    if (e.button !== 0 || (kind === 'draw' && !drawing)) return
    e.preventDefault(); e.stopPropagation()
    const point = worldAt(e)
    const original = zone ? { ...zone } : { id: crypto.randomUUID(), title: 'Новая зона', color: '#6a8fbf', x: point.x, y: point.y, w: 0, h: 0 }
    selected = original.id
    drag = { kind, start: point, original, pointer: e.pointerId }
    preview = { ...original }
    ;(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId)
  }
  function move(e: PointerEvent) {
    if (!drag || e.pointerId !== drag.pointer) return
    e.stopPropagation()
    const point = worldAt(e), dx = point.x - drag.start.x, dy = point.y - drag.start.y
    const z = drag.original
    preview = drag.kind === 'draw' ? { ...z, x: Math.min(z.x, point.x), y: Math.min(z.y, point.y), w: Math.abs(dx), h: Math.abs(dy) }
      : drag.kind === 'move' ? { ...z, x: z.x + dx, y: z.y + dy }
      : { ...z, w: Math.max(40, z.w + dx), h: Math.max(40, z.h + dy) }
  }
  function finish(e: PointerEvent) {
    if (!drag || e.pointerId !== drag.pointer) return
    e.stopPropagation()
    if (e.type !== 'pointercancel' && preview && preview.w >= 40 && preview.h >= 40) onChange(readZones([...zones.filter((z) => z.id !== preview!.id), preview]))
    drag = null; preview = null
    onFinish()
  }
  function change(patch: Partial<GraphZone>) {
    if (!current) return
    const next = readZones([{ ...current, ...patch }])[0]
    if (next) onChange(zones.map((z) => z.id === current.id ? next : z))
  }
  function onKey(e: KeyboardEvent) {
    if (e.key !== 'Escape' || (!drawing && !selected && !drag)) return
    drag = null; preview = null; selected = null; onFinish()
  }
</script>

<svelte:window onkeydown={onKey} />
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="zones" class:zones--drawing={drawing} onpointerdown={(e) => start(e)} onpointermove={move} onpointerup={finish} onpointercancel={finish} oncontextmenu={(e) => { if (drawing) { e.preventDefault(); e.stopPropagation(); onFinish() } }}>
  <div class="zones__world" style:transform="translate({cam.x}px, {cam.y}px) scale({cam.k})">
    {#each shown as z (z.id)}
      <div class="zone" class:zone--selected={selected === z.id} style:left="{z.x}px" style:top="{z.y}px" style:width="{z.w}px" style:height="{z.h}px" style:--zone-color={z.color}>
        <button type="button" class="zone__title" aria-label={`Зона: ${z.title}`} title="Потяните, чтобы переместить зону" onpointerdown={(e) => start(e, z, 'move')} onclick={() => selected = z.id}>{z.title}</button>
        <button type="button" class="zone__resize" aria-label={`Изменить размер зоны: ${z.title}`} onpointerdown={(e) => start(e, z, 'resize')}>◢</button>
      </div>
    {/each}
  </div>
</div>
{#if drawing}
  <p class="zones__hint">Нарисуйте область на карте · Esc или ПКМ — отмена</p>
{/if}
{#if current && !drawing}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="zones__editor" role="group" aria-label="Настройки зоны" onpointerdown={(e) => e.stopPropagation()} oncontextmenu={(e) => e.stopPropagation()}>
    <input aria-label="Название зоны" maxlength="120" value={current.title} onchange={(e) => change({ title: e.currentTarget.value })} />
    <input type="color" aria-label="Цвет зоны" value={current.color} onchange={(e) => change({ color: e.currentTarget.value })} />
    <label>Ш <input type="number" min="40" max="100000" aria-label="Ширина зоны" value={Math.round(current.w)} onchange={(e) => change({ w: Number(e.currentTarget.value) })} /></label>
    <label>В <input type="number" min="40" max="100000" aria-label="Высота зоны" value={Math.round(current.h)} onchange={(e) => change({ h: Number(e.currentTarget.value) })} /></label>
    <button type="button" class="graph__btn" onclick={() => { onChange(zones.filter((z) => z.id !== selected)); selected = null }}>Удалить зону</button>
    <button type="button" class="graph__btn" onclick={() => selected = null}>Готово</button>
  </div>
{/if}

<style>
  .zones { position: absolute; inset: 0; pointer-events: none; }
  .zones--drawing { z-index: 12; pointer-events: auto; cursor: crosshair; }
  .zones__world { position: absolute; left: 0; top: 0; width: 0; height: 0; transform-origin: 0 0; }
  .zone { position: absolute; box-sizing: border-box; border: 2px dashed var(--zone-color); border-radius: 12px; background: color-mix(in srgb, var(--zone-color) 8%, transparent); }
  .zone--selected { border-style: solid; }
  .zone__title { position: absolute; left: 10px; top: -16px; max-width: calc(100% - 20px); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; padding: 4px 9px; border: 1px solid var(--zone-color); border-radius: 6px; background: var(--color-bg-raised, #1a1a1a); color: var(--zone-color); font: inherit; cursor: move; pointer-events: auto; touch-action: none; }
  .zone__resize { position: absolute; right: 0; bottom: 0; border: 0; background: transparent; color: var(--zone-color); cursor: nwse-resize; pointer-events: auto; touch-action: none; }
  .zones--drawing .zone__title, .zones--drawing .zone__resize { pointer-events: none; }
  .zones__hint { position: absolute; z-index: 13; top: 8px; left: 50%; transform: translateX(-50%); margin: 0; padding: 8px 12px; border-radius: 6px; background: var(--color-bg-raised, #1a1a1a); pointer-events: none; font-size: var(--text-sm); }
  .zones__editor { position: absolute; z-index: 14; left: 10px; right: 10px; bottom: 52px; display: flex; flex-wrap: wrap; align-items: center; gap: 8px; padding: 10px; background: var(--color-bg-raised, #1a1a1a); border: 1px solid var(--color-border, #333); border-radius: 8px; font-size: var(--text-xs); }
  .zones__editor input { min-width: 0; padding: 4px; background: transparent; color: inherit; border: 1px solid var(--color-border, #333); border-radius: 4px; }
  .zones__editor input[type=number] { width: 5rem; }
  .zones__editor input[type=color] { width: 32px; height: 28px; padding: 1px; }
</style>
