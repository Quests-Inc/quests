<script lang="ts">
  import { bindRefRegistries } from '@/app/model/refRegistry.svelte.ts'

  let { notes, quests, questlines, labels }: { notes: any[]; quests: any[]; questlines: any[]; labels: Record<string, string> } = $props()
  bindRefRegistries(() => ({ notes, quests, questlines, labels }))
</script>
