<script lang="ts">
 import { createEntitySearch } from '@/shared/lib/entitySearch.svelte.ts'
 let q = $state('')
 const search = createEntitySearch(() => q ? {q,types:['note']} : null,{delay:0,all:true})
</script>
<input aria-label="Запрос" bind:value={q}/>
<output>{search.pending ? 'ожидание' : search.error || search.hits?.map(h=>h.id).join(',') || 'пусто'}</output>
