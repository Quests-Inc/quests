<script lang="ts">
  let { hasSelection = true, pending = false, error = '' }: { hasSelection?: boolean; pending?: boolean; error?: string } = $props()
  import SidebarLayout from '@/shared/ui/SidebarLayout.svelte'
  import SidebarTools from '@/shared/ui/SidebarTools.svelte'
  import SearchState from '@/shared/ui/SearchState.svelte'
</script>
<SidebarLayout key="sidebar-regression" {hasSelection}>
  {#snippet sidebar()}
    <aside aria-label="Тестовый список"><SidebarTools label="Поиск в списке" /><div aria-label="Результаты"><SearchState {pending} {error} /></div></aside>
  {/snippet}
  <main><input aria-label="Поиск на холсте" /></main>
</SidebarLayout>
