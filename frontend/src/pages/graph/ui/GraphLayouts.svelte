<script lang="ts">
  import Icon from '@/shared/ui/Icon.svelte'
  import ConfirmModal from '@/shared/ui/ConfirmModal.svelte'
  import SidebarTools from '@/shared/ui/SidebarTools.svelte'
  import type { createGraphLayouts } from '../model/layouts.svelte.ts'
  let {layouts}: {layouts:ReturnType<typeof createGraphLayouts>}=$props()
  let title=$state('')
  let renaming=$state<number|string|null>(null), renameText=$state('')
  let removing=$state<{id:number|string;title:string}|null>(null)
  const matched=$derived(layouts.items.filter(i=>i.title.toLocaleLowerCase().includes(title.trim().toLocaleLowerCase())))
  const active=$derived(layouts.items.find(i=>i.id===layouts.current))
  const outside=$derived(active&&!matched.some(i=>i.id===active.id)?active:null)
  $effect(()=>{if(!layouts.loaded)void layouts.load()})
  async function create(e?:SubmitEvent){e?.preventDefault();await layouts.newEmpty(title);if(!layouts.error)title=''}
  async function rename(){const id=renaming;renaming=null;if(id!=null&&renameText.trim())await layouts.rename(id,renameText)}
</script>
<aside class="glay" aria-label="Расклады карты">
  <form onsubmit={create}><SidebarTools bind:value={title} placeholder="Название / фильтр раскладов" label="Название и фильтр раскладов" createLabel="Создать пустой расклад" createBusy={layouts.busy||!layouts.loaded} onCreate={()=>void create()} /></form>
  <p class="glay__status" role="status">{active?.temporary?'Временный — пропадёт после перезагрузки':layouts.saving?'Сохраняется…':'Изменения сохраняются автоматически'}</p>
  {#if layouts.error}<p role="alert">{layouts.error}</p>{/if}
  {#if outside}<div class="glay__outside"><small>Открыт · вне фильтра</small>{@render row(outside)}</div>{/if}
  {#if !layouts.loaded}<p>Загрузка…</p>{:else}
    <div class="glay__list">{#each matched as l (l.id)}{@render row(l)}{:else}<p>Ничего не найдено</p>{/each}</div>
  {/if}
</aside>
{#snippet row(l: (typeof layouts.items)[number])}
  <div class="glay__item" class:glay__item--on={l.id===layouts.current}>
    {#if renaming===l.id}<input aria-label="Новое название расклада" bind:value={renameText} onblur={()=>void rename()} onkeydown={e=>{if(e.key==='Enter')void rename();if(e.key==='Escape')renaming=null}} />
    {:else}
      <button type="button" class="glay__open" disabled={layouts.busy} onclick={()=>void layouts.open(l.id)}><span>{l.title}</span><small>{l.temporary?'Временный':'Сохранённый'}</small></button>
      <div class="glay__actions">
        {#if l.id===layouts.current&&l.temporary}<button type="button" class="btn btn--icon" aria-label="Сохранить временный расклад" title="Сохранить временный расклад" disabled={!layouts.canSave} onclick={()=>void layouts.saveNew(l.title)}><Icon name="save" size={14}/></button>{/if}
        <button type="button" class="btn btn--icon" aria-label="Переименовать" disabled={layouts.busy} onclick={()=>{renaming=l.id;renameText=l.title}}><Icon name="edit" size={14}/></button>
        <button type="button" class="btn btn--icon" aria-label="Удалить расклад" disabled={layouts.busy} onclick={()=>removing={id:l.id,title:l.title}}><Icon name="delete" size={14}/></button>
      </div>
    {/if}
  </div>
{/snippet}
{#if removing}<ConfirmModal title="Удалить расклад?" message={`Расклад «${removing.title}» будет удалён. Сущности и вложения останутся.`} onConfirm={async()=>{if(removing)await layouts.remove(removing.id);removing=null}} onCancel={()=>removing=null}/>{/if}
<style>
.glay{display:flex;flex-direction:column;min-height:0;overflow:auto}.glay__status{font:var(--text-xs) var(--font-ui);padding:0 .6rem;color:var(--color-fg-muted)}.glay__item{display:flex;align-items:center;border-left:3px solid transparent}.glay__item--on{border-left-color:var(--color-accent);background:color-mix(in srgb,var(--color-accent) 12%,transparent)}.glay__open{flex:1;min-width:0;display:flex;flex-direction:column;align-items:flex-start;border:0;background:transparent;color:inherit;padding:.6rem;cursor:pointer}.glay__open span{max-width:100%;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.glay__open small{opacity:.55}.glay__actions{display:flex}.glay__actions :global(.btn){border:0;background:transparent;padding:.25rem;min-width:1.5rem}.glay__outside{border-bottom:1px solid var(--color-border);padding-bottom:.4rem}.glay__outside>small{padding:.5rem;opacity:.6}
</style>
