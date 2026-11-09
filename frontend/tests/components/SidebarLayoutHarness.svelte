<script lang="ts">
  let { hasSelection = true }: { hasSelection?: boolean } = $props()
  import SidebarLayout from '@/shared/ui/SidebarLayout.svelte'
</script>
<SidebarLayout key="sidebar-regression" {hasSelection}>
  {#snippet sidebar()}
    <aside aria-label="Тестовый список"><input aria-label="Поиск в списке" /><button type="button">Создать</button></aside>
  {/snippet}
  <main><input aria-label="Поиск на холсте" /></main>
</SidebarLayout>
