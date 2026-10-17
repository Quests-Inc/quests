<script lang="ts">
  import { TYPES } from '../model/constants.ts'

  /** Миникарта: все узлы мелкими прямоугольниками и рамка видимой области; клик и перетаскивание двигают вид. */
  let {
    mini,
    onGo,
  }: {
    mini: { w: number; h: number; x0: number; y0: number; sc: number; nodes: { key: string; type: string; x: number; y: number; w: number; h: number; draft?: boolean; color?: string }[]; view: { x: number; y: number; w: number; h: number } }
    onGo: (e: { currentTarget: HTMLElement; clientX: number; clientY: number }) => void
  } = $props()

  let dragging = false
</script>

    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div
      class="gmini"
      style:width="{mini.w}px"
      style:height="{mini.h}px"
      title="Миникарта: клик или перетаскивание — переместить вид"
      onpointerdown={(e) => {
        e.stopPropagation()
        dragging = true
        e.currentTarget.setPointerCapture(e.pointerId)
        onGo(e)
      }}
      onpointermove={(e) => dragging && onGo(e)}
      onpointerup={() => (dragging = false)}
      onpointercancel={() => (dragging = false)}
      onwheel={(e) => e.stopPropagation()}
      oncontextmenu={(e) => {
        e.preventDefault()
        e.stopPropagation()
      }}
    >
      <svg width={mini.w} height={mini.h} aria-hidden="true">
        {#each mini.nodes as n (n.key)}
          <rect
            x={(n.x - n.w / 2 - mini.x0) * mini.sc}
            y={(n.y - n.h / 2 - mini.y0) * mini.sc}
            width={Math.max(2, n.w * mini.sc)}
            height={Math.max(2, n.h * mini.sc)}
            rx="1"
            fill={n.color ?? TYPES[n.type]?.color ?? '#888'}
            opacity={n.draft ? 1 : 0.7}
          />
        {/each}
        <rect
          class="gmini__view"
          x={(mini.view.x - mini.x0) * mini.sc}
          y={(mini.view.y - mini.y0) * mini.sc}
          width={mini.view.w * mini.sc}
          height={mini.view.h * mini.sc}
        />
      </svg>
    </div>

<style>


  /* Map tools: flat square icon buttons in the bottom-right corner. */
  .gmini {
    position: absolute;
    z-index: 6;
    left: 0.6rem;
    top: 0.6rem;
    box-shadow: 0 0 0 1px var(--color-border, #333);
    border-radius: 3px;
    background: color-mix(in srgb, var(--pill-bg, #0f0f0f) 85%, transparent);
    cursor: pointer;
    touch-action: none;
  }

  .gmini svg {
    display: block;
  }

  .gmini__view {
    fill: rgba(201, 162, 39, 0.05);
    stroke: var(--color-accent, #c9a227);
    stroke-width: 1.2;
  }</style>
