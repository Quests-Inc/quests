<script lang="ts">
  import type { AttachmentView } from '@/shared/api/knowledge_http.gen.ts'
  import AttachmentPreview from '@/entities/attachment/ui/AttachmentPreview.svelte'
  let { initial }: { initial: AttachmentView } = $props()
  let current = $state(initial)
  export function change(a: AttachmentView) { current = a }
</script>
<AttachmentPreview a={current} />
