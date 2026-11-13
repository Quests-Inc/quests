<script lang="ts">
 import GraphImage from '@/widgets/graph-canvas/ui/GraphImage.svelte'
 let pixels=$state(100)
</script>
<input aria-label="Размер на экране" type="number" bind:value={pixels}/>
<GraphImage id={42} {pixels}/>
