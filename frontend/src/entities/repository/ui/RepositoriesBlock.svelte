<script lang="ts">
 import {listRepositories,createRepository,linkRepository,unlinkRepository,updateRepository,type Repository,type RepositoryLink} from '@/shared/api/repositories.ts'
 import PageToolbar from '@/shared/ui/PageToolbar.svelte'
 import {plural} from '@/shared/lib/duration.ts'
 import {untrack} from 'svelte'
 import RepositoryBrowser from './RepositoryBrowser.svelte'
 import ModalShell from '@/shared/ui/ModalShell.svelte'
 import ModalHead from '@/shared/ui/ModalHead.svelte'
 import ModalFoot from '@/shared/ui/ModalFoot.svelte'
 let {owner}:{owner?:RepositoryLink}=$props()
 let repos=$state<Repository[]>([]),error=$state(''),busy=$state(false),loaded=$state(false),expanded=$state(false),creating=$state(false),selected=$state<number|null>(untrack(()=>Number(new URL(location.href).searchParams.get('repo'))||null))
 let search=$state('')
 let title=$state(''),kind=$state('local'),url=$state(''),branch=$state('main'),token=$state(''),attachId=$state(''),quota=$state(0),tokenFor=$state<number|null>(null)
 let refreshNo=$state(0)
 const shown=$derived(owner?repos.filter(repo=>repo.links.some(l=>l.kind===owner.kind&&l.id===owner.id)):repos.filter(repo=>{const q=search.trim().toLowerCase().replace(/^repository=/,'');return !q||String(repo.id)===q||repo.title.toLowerCase().includes(q)||repo.url.toLowerCase().includes(q)}))
 const available=$derived(repos.filter(repo=>!shown.some(x=>x.id===repo.id)))
 $effect(()=>{void owner?.id;void owner?.kind;void refreshNo;const abort={active:true};loaded=false
 void listRepositories().then(result=>{if(abort.active){repos=result.items??[];quota=result.quota_bytes??0;loaded=true}}).catch(e=>{if(abort.active)error=e.message||String(e)});return()=>{abort.active=false}})
 async function create(event:SubmitEvent){event.preventDefault();busy=true;error='';try{await createRepository({title,kind,url,branch,token,owner});creating=false;title='';token='';refreshNo++}catch(e){error=e.message||String(e)}finally{busy=false}}
 async function attach(){if(!owner||!attachId)return;busy=true;error='';try{await linkRepository(Number(attachId),owner);refreshNo++;attachId=''}catch(e){error=e.message||String(e)}finally{busy=false}}
 async function detach(repo:Repository){if(!owner)return;busy=true;try{await unlinkRepository(repo.id,owner);refreshNo++}catch(e){error=e.message||String(e)}finally{busy=false}}
 async function saveToken(){if(tokenFor===null)return;busy=true;error='';try{await updateRepository(tokenFor,{token});tokenFor=null;token='';refreshNo++}catch(e){error=e.message||String(e)}finally{busy=false}}
 function selectRepo(id:number){selected=id;const next=new URL(location.href);next.searchParams.set('repo',String(id));history.replaceState(history.state,'',next)}
 const activeRepo=$derived(repos.find(repo=>repo.id===selected)??null)
</script>
<section class="repositories" class:repositories--linked={Boolean(owner)}>
 {#if owner}<button type="button" class="repo-heading" aria-expanded={!owner||expanded} onclick={()=>expanded=!expanded}>Репозитории {#if loaded}· {shown.length}{/if}</button>{/if}
 {#if !owner||expanded}
  {#if error}<p role="alert" class="repo-error">{error}</p>{/if}
  {#if !owner}<PageToolbar><input class="search" type="search" aria-label="Поиск репозиториев" placeholder="Название, адрес или ID" bind:value={search}/><button type="button" class="btn btn--ghost" onclick={()=>{creating=true;token='';error=''}}>+ Репозиторий</button></PageToolbar><span class="hint">{shown.length} из {repos.length} · свой Git: {Math.round(repos.reduce((sum,r)=>sum+r.bytes,0)/1048576)} / {Math.round(quota/1048576)} МБ</span>
  {:else}<div class="repo-actions"><button type="button" class="btn btn--ghost" onclick={()=>{creating=true;token='';error=''}}>+ Репозиторий</button></div>{/if}
  {#if owner && available.length}<div class="repo-attach"><select aria-label="Привязать существующий репозиторий" bind:value={attachId}><option value="">Существующий репозиторий…</option>{#each available as repo}<option value={repo.id}>{repo.title}</option>{/each}</select><button type="button" class="btn btn--ghost" disabled={busy||!attachId} onclick={attach}>Привязать</button></div>{/if}
  {#if loaded && !shown.length}<p class="hint">{owner?'Нет привязанных репозиториев.':'Репозитории не найдены.'}</p>{/if}
  <div class="repo-workspace" class:repo-workspace--linked={Boolean(owner)}>
   <div class="repo-list" aria-label="Репозитории пространства">
    {#each shown as repo (repo.id)}
     <div class="repo-row" class:repo-row--on={selected===repo.id&&!owner}>
      {#if owner}<a class="repo-name" href={`?tab=repositories&repo=${repo.id}`}>{repo.title}</a>{:else}<button type="button" class="repo-name" onclick={()=>selectRepo(repo.id)}>{repo.title}<small>{repo.kind==='local'?'Свой Git':repo.kind} · {repo.links.length} {plural(repo.links.length,['связь','связи','связей'])}</small></button>{/if}
      {#if owner}<button type="button" class="repo-unlink" disabled={busy} onclick={()=>detach(repo)} title="Отвязать репозиторий" aria-label={`Отвязать ${repo.title}`}>×</button>{/if}
     </div>
    {/each}
   </div>
   {#if !owner}<section class="repo-detail" aria-label="Репозиторий">
    {#if activeRepo}<header class="repo-detail__head"><h2>{activeRepo.title}</h2>{#if activeRepo.kind!=='local'}<button type="button" class="btn btn--ghost" onclick={()=>{tokenFor=activeRepo.id;token=''}}>Секрет {activeRepo.has_token?'задан':'не задан'}</button>{/if}</header><RepositoryBrowser repo={activeRepo}/>{:else}<p class="hint">Выберите репозиторий для просмотра дерева.</p>{/if}
   </section>{/if}
  </div>
 {/if}
</section>
<ModalShell open={creating} onClose={()=>{if(!busy)creating=false}} closeDisabled={busy} labelledby="repo-create-title" maxWidth="38rem"><ModalHead id="repo-create-title" title="Новый репозиторий" onClose={()=>creating=false} closeDisabled={busy}/><form class="modal__form" onsubmit={create}>
 <label class="field"><span class="label">Название</span><input required bind:value={title} maxlength="200"/></label>
 <label class="field"><span class="label">Хранение</span><select bind:value={kind}><option value="local">Свой Git в Quests</option><option value="github">GitHub</option><option value="gitlab">GitLab</option></select></label>
 {#if kind!=='local'}<label class="field"><span class="label">Адрес репозитория</span><input type="url" required placeholder={`https://${kind}.com/owner/project`} bind:value={url}/></label><label class="field"><span class="label">Токен только для чтения · необязательно</span><input type="password" autocomplete="off" bind:value={token}/></label><p class="hint">Push идёт напрямую на внешний хост. Quests читает журнал и дерево через API. Исходящий доступ на сервере должен быть разрешён.</p>{/if}
 <label class="field"><span class="label">Ветка</span><input required bind:value={branch}/></label>
 {#if error}<p role="alert" class="repo-error">{error}</p>{/if}
 <ModalFoot onCancel={()=>creating=false} submitLabel="Создать" {busy}/>
</form></ModalShell>
<ModalShell open={tokenFor!==null} onClose={()=>tokenFor=null} closeDisabled={busy} labelledby="repo-token-title"><ModalHead id="repo-token-title" title="Секрет доступа к репозиторию" onClose={()=>tokenFor=null} closeDisabled={busy}/><div class="modal__body"><label class="field"><span class="label">Новый токен · пусто — удалить</span><input type="password" autocomplete="off" bind:value={token}/></label><p class="hint">Сохранённое значение не показывается и не передаётся раннерам.</p>{#if error}<p role="alert" class="repo-error">{error}</p>{/if}<ModalFoot onCancel={()=>tokenFor=null} onSubmit={saveToken} submitLabel="Сохранить" {busy}/></div></ModalShell>
<style>
 .repositories{font-family:var(--font-ui);display:grid;gap:.7rem;padding:.5rem 0;min-width:0}.repo-heading{font-weight:600;text-align:left;color:var(--color-fg-muted);border:0;background:transparent;padding:.2rem 0;cursor:pointer;font-size:.85rem}.repo-actions,.repo-attach{display:flex;gap:.5rem;align-items:center;flex-wrap:wrap}.repo-workspace{display:grid;grid-template-columns:minmax(12rem,17rem) minmax(0,1fr);border-top:1px solid var(--color-border);min-height:20rem}.repo-workspace--linked{display:block;min-height:0;border:0}.repo-list{min-width:0}.repo-row{display:flex;align-items:center;border-bottom:1px solid var(--color-border);padding:.6rem .4rem;gap:.4rem}.repo-row--on{background:color-mix(in srgb,var(--color-accent) 10%,transparent)}.repo-name{display:block;flex:1;min-width:0;text-align:left;overflow-wrap:anywhere;border:0;background:transparent;color:var(--color-accent);font:inherit;cursor:pointer;text-decoration:none;padding:0}.repo-name small{display:block;margin-top:.3rem;color:var(--color-fg-muted);font-size:.75rem}.repo-detail{min-width:0;padding:1rem;border-left:1px solid var(--color-border)}.repo-detail__head{display:flex;justify-content:space-between;align-items:center;gap:.5rem;margin-bottom:1rem}.repo-detail h2{font-size:1.1rem;margin:0}.repo-unlink{background:transparent;border:0;color:var(--color-fg-muted);cursor:pointer;font-size:1.1rem}.repo-error{color:var(--color-danger)}.repositories--linked{gap:.3rem}.repositories--linked .repo-actions .btn{background:transparent;border-color:transparent;padding:.25rem}.repositories--linked .repo-row{padding:.3rem 0}@media(max-width:45rem){.repo-workspace{grid-template-columns:minmax(0,1fr)}.repo-detail{border-left:0;border-top:1px solid var(--color-border)}}
</style>
