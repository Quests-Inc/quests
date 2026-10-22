<script lang="ts">
  import { ValueTable } from '@/entities/value'
  import MarkdownBody from '@/shared/ui/MarkdownBody.svelte'

  let {
    stepId,
    source = '',
    show = false,
    class: className = '',
    labels = {},
    onRef,
    typing = false,
  }: {
    stepId: number
    source?: string
    show?: boolean
    class?: string
    labels?: Record<string, string>
    onRef?: (kind: string, id: number) => void
    typing?: boolean
  } = $props()

  let values = $state({} as Record<string, string>)
</script>

<div class="step__values">
  <ValueTable variant="hidden" owner={{ kind: 'step', id: stepId }} onEffective={(text) => (values = text)} />
</div>
{#if show}
  <MarkdownBody class={className} {source} {labels} {onRef} {typing} {values} host={{ kind: 'step', id: stepId }} />
{/if}
