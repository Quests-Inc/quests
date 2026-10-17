<script lang="ts">
  import { HEAD_H, ROW_H, portColor, type NodePorts, type ScriptNodeView } from '@/entities/script'
  import type { End } from '@/shared/api/scriptgraph.gen.ts'

  /** One node: a coloured head with its title, inputs on the left, outputs on the right. */
  let {
    n,
    ports,
    selected,
    picked,
    problems,
    pendingFrom,
    onDown,
    onPort,
  }: {
    n: ScriptNodeView
    ports: NodePorts | undefined
    selected: boolean
    picked: boolean
    problems: string[]
    pendingFrom: End | null
    onDown: (e: PointerEvent) => void
    onPort: (side: 'in' | 'out', n: ScriptNodeView, port: string, e: PointerEvent | MouseEvent) => void
  } = $props()

  const rows = $derived(Math.max(ports?.inputs.length ?? 0, ports?.outputs.length ?? 0, 1))
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="snode"
  class:snode--sel={selected}
  class:snode--picked={picked}
  class:snode--bad={problems.length > 0 || Boolean(ports?.error)}
  data-gkey={n.key}
  style:--c={n.color}
  style:left="{n.x - n.w / 2}px"
  style:top="{n.y - n.h / 2}px"
  style:width="{n.w}px"
  style:height="{n.h}px"
  onpointerdown={onDown}
>
  <div class="snode__head" style:height="{HEAD_H}px" title={problems.join('\n')}>
    <span class="snode__title">{n.title}</span>
    {#if !n.trusted}<span class="snode__tag" title="Пользовательский код: выполняется только на раннере">раннер</span>{/if}
  </div>
  <div class="snode__body" style:height="{rows * ROW_H}px">
    <ul class="snode__col">
      {#each ports?.inputs ?? [] as p (p.name)}
        <li style:height="{ROW_H}px">
          <button
            type="button"
            class="snode__dot snode__dot--in"
            class:snode__dot--open={pendingFrom !== null}
            style:--c={portColor(p.type)}
            data-in={p.name}
            data-node={n.id}
            title="{p.name}: {p.type}{p.optional ? ' (необязательно)' : ''}{p.multi ? ' · несколько связей' : ''}"
            aria-label="Вход {p.name}"
            onpointerdown={(e) => e.stopPropagation()}
            onclick={(e) => onPort('in', n, p.name, e)}
          ></button>
          <span class="snode__name" class:snode__name--opt={p.optional}>{p.name}</span>
        </li>
      {/each}
    </ul>
    <ul class="snode__col snode__col--out">
      {#each ports?.outputs ?? [] as p (p.name)}
        <li style:height="{ROW_H}px">
          <span class="snode__name">{p.name}</span>
          <button
            type="button"
            class="snode__dot snode__dot--out"
            class:snode__dot--from={pendingFrom?.node === n.id && pendingFrom.port === p.name}
            style:--c={portColor(p.type)}
            data-out={p.name}
            data-node={n.id}
            title="{p.name}: {p.type}"
            aria-label="Выход {p.name}"
            onpointerdown={(e) => onPort('out', n, p.name, e)}
          ></button>
        </li>
      {/each}
    </ul>
  </div>
  {#if ports?.error}<p class="snode__err" title={ports.error}>параметры неверны</p>{/if}
</div>

<style>
  /* The whole card is painted in the node's colour, lighter than the field, and raised by its shadow. */
  .snode {
    --card: color-mix(in srgb, var(--c) 30%, var(--color-bg-raised, #1a1a1a));
    position: absolute;
    box-sizing: border-box;
    border: 1px solid color-mix(in srgb, var(--c) 55%, transparent);
    border-radius: 10px;
    background: var(--card);
    box-shadow:
      inset 0 1px 0 rgba(255, 255, 255, 0.12),
      0 4px 10px rgba(0, 0, 0, 0.5);
    cursor: move;
    font-size: var(--text-xs, 0.78rem);
  }

  .snode--sel {
    border-color: var(--color-accent, #c9a227);
    box-shadow:
      0 0 0 1px var(--color-accent, #c9a227),
      0 4px 10px rgba(0, 0, 0, 0.5);
  }

  .snode--picked {
    outline: 2px solid #5aa9ff;
  }

  .snode--bad {
    border-color: var(--color-danger, #b54a3a);
  }

  .snode__head {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0 0.6rem;
    border-radius: 9px 9px 0 0;
    background: color-mix(in srgb, var(--c) 52%, var(--color-bg-raised, #1a1a1a));
    color: #fff;
    font-weight: 600;
    overflow: hidden;
  }

  .snode__title {
    flex: 1 1 auto;
    min-width: 0;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .snode__tag {
    flex: 0 0 auto;
    padding: 0 0.35rem;
    border-radius: 999px;
    background: rgba(0, 0, 0, 0.3);
    color: #fff;
    font-size: 0.65rem;
    font-weight: 500;
  }

  .snode__body {
    display: flex;
    justify-content: space-between;
  }

  .snode__col {
    margin: 0;
    padding: 0;
    list-style: none;
    min-width: 0;
  }

  .snode__col li {
    display: flex;
    align-items: center;
    gap: 0.35rem;
  }

  .snode__col--out li {
    justify-content: flex-end;
  }

  .snode__name {
    color: var(--color-fg, #ddd);
    white-space: nowrap;
  }

  .snode__name--opt {
    color: var(--color-fg-muted, #9a9a9a);
  }

  /* The dots sit on the node's edge: half outside, so a wire meets the border. */
  .snode__dot {
    flex: 0 0 auto;
    width: 0.8rem;
    height: 0.8rem;
    padding: 0;
    border: 2px solid var(--c);
    border-radius: 50%;
    background: var(--card);
    cursor: crosshair;
  }

  .snode__dot--in {
    margin-left: -0.45rem;
  }

  .snode__dot--out {
    margin-right: -0.45rem;
  }

  .snode__dot:hover,
  .snode__dot--open,
  .snode__dot--from {
    background: var(--c);
  }

  .snode__err {
    margin: 0;
    padding: 0 0.6rem;
    color: var(--color-danger, #b54a3a);
  }
</style>
