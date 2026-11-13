<script lang="ts">
  import { outlinePath } from '../model/silhouette.ts'
  let {text}: {text:string}=$props()
  const path=$derived(outlinePath(text))
</script>
<svg class="note-outline" viewBox="0 0 100 100" preserveAspectRatio="none" aria-hidden="true"><path d={path}/></svg>
<style>.note-outline{width:100%;height:100%;min-height:24px;opacity:.16;pointer-events:none;overflow:hidden}.note-outline path{fill:currentColor}</style>
