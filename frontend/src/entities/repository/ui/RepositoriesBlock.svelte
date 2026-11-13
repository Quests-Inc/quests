<script lang="ts">
 import {listRepositories,createRepository,linkRepository,unlinkRepository,updateRepository,type Repository,type RepositoryLink} from '@/shared/api/repositories.ts'
 import RepositoryBrowser from './RepositoryBrowser.svelte'
 import ModalShell from '@/shared/ui/ModalShell.svelte'
 import ModalHead from '@/shared/ui/ModalHead.svelte'
 import ModalFoot from '@/shared/ui/ModalFoot.svelte'
 let {owner,compact=false}:{owner?:RepositoryLink;compact?:boolean}=$props()
 let repos=$state<Repository[]>([]),error=$state(''),busy=$state(false),loaded=$state(false),expanded=$state(false),creating=$state(false),selected=$state<number|null>(null)
 let search=$state('')
 let title=$state(''),kind=$state('local'),url=$state(''),branch=$state('main'),token=$state(''),attachId=$state(''),linkPath=$state(''),quota=$state(0),tokenFor=$state<number|null>(null)
 let refreshNo=$state(0)
 const shown=$derived(owner?repos.filter(repo=>repo.links.some(l=>l.kind===owner.kind&&l.id===owner.id)):repos.filter(repo=>{const q=search.trim().toLowerCase().replace(/^repository=/,'');return !q||String(repo.id)===q||repo.title.toLowerCase().includes(q)||repo.url.toLowerCase().includes(q)}))
 const available=$derived(repos.filter(repo=>!shown.some(x=>x.id===repo.id)))
 $effect(()=>{void owner?.id;void owner?.kind;void refreshNo;if(compact&&!expanded)return;const abort={active:true};loaded=false
 void listRepositories().then(result=>{if(abort.active){repos=result.items??[];quota=result.quota_bytes??0;loaded=true}}).catch(e=>{if(abort.active)error=e.message||String(e)});return()=>{abort.active=false}})
 async function create(event:SubmitEvent){event.preventDefault();busy=true;error='';try{await createRepository({title,kind,url,branch,token,owner});creating=false;title='';token='';refreshNo++}catch(e){error=e.message||String(e)}finally{busy=false}}
 async function attach(){if(!owner||!attachId)return;busy=true;error='';try{await linkRepository(Number(attachId),{...owner,path:linkPath});refreshNo++;attachId='';linkPath=''}catch(e){error=e.message||String(e)}finally{busy=false}}
 async function detach(repo:Repository){if(!owner)return;busy=true;try{await unlinkRepository(repo.id,owner);refreshNo++}catch(e){error=e.message||String(e)}finally{busy=false}}
 async function saveToken(){if(tokenFor===null)return;busy=true;error='';try{await updateRepository(tokenFor,{token});tokenFor=null;token='';refreshNo++}catch(e){error=e.message||String(e)}finally{busy=false}}
 const pathFor=(repo:Repository)=>owner?repo.links.find(l=>l.kind===owner.kind&&l.id===owner.id)?.path||'':''
</script>
<section class="repositories" class:repositories--compact={compact}>
 <button type="button" class="repo-heading" aria-expanded={!owner||expanded} onclick={()=>expanded=!expanded}>Репозитории {#if loaded}· {shown.length}{/if}</button>
 {#if !owner||expanded}
  {#if error}<p role="alert" class="repo-error">{error}</p>{/if}
  {#if !owner}<input type="search" aria-label="Поиск репозиториев" placeholder="Название, адрес или ID" bind:value={search}/>{/if}
  <div class="repo-actions"><button type="button" class="btn btn--ghost" onclick={()=>{creating=true;token='';error=''}}>+ Репозиторий</button>{#if !owner}<span class="hint">{Math.round(repos.reduce((sum,r)=>sum+r.bytes,0)/1048576)} / {Math.round(quota/1048576)} МБ · свой Git</span>{/if}</div>
  {#if owner && available.length}<div class="repo-attach"><select aria-label="Привязать существующий репозиторий" bind:value={attachId}><option value="">Существующий репозиторий…</option>{#each available as repo}<option value={repo.id}>{repo.title}</option>{/each}</select>{#if owner.kind==='step'}<input aria-label="Путь шага в репозитории" placeholder="Путь к файлу или папке" bind:value={linkPath}/>{/if}<button type="button" class="btn btn--ghost" disabled={busy||!attachId} onclick={attach}>Привязать</button></div>{/if}
  {#if loaded && !shown.length}<p class="hint">Репозитории пока не привязаны.</p>{/if}
  {#each shown as repo (repo.id)}<article class="repo-card"><header><button type="button" class="repo-name" onclick={()=>selected=selected===repo.id?null:repo.id}>{repo.title}</button><span class="hint">{repo.kind==='local'?'Свой Git':repo.kind} {#if pathFor(repo)}· {pathFor(repo)}{/if}</span>{#if owner}<button type="button" class="btn btn--ghost" disabled={busy} onclick={()=>detach(repo)}>Отвязать</button>{/if}{#if repo.kind!=='local'}<button type="button" class="btn btn--ghost" onclick={()=>{tokenFor=repo.id;token=''}}>Секрет {repo.has_token?'задан':'не задан'}</button>{/if}</header>{#if owner?.kind==='step'}<label>Путь шага <input aria-label="Путь шага в репозитории" value={pathFor(repo)} onchange={async e=>{error='';try{await linkRepository(repo.id,{...owner!,path:e.currentTarget.value});refreshNo++}catch(err){error=err.message||String(err)}}}/></label>{/if}{#if selected===repo.id}<RepositoryBrowser {repo} initialPath={pathFor(repo)}/>{/if}</article>{/each}
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
 .repositories{font-family:var(--font-ui);display:grid;gap:.6rem;padding:.7rem 0;min-width:0}.repo-heading{font-weight:600;text-align:left;color:var(--color-fg);border:0;background:transparent;padding:.25rem 0;cursor:pointer}.repo-actions,.repo-attach{display:flex;gap:.5rem;align-items:center;flex-wrap:wrap}.repo-card{padding:.7rem;border:1px solid var(--color-border);border-radius:var(--radius-md);display:grid;gap:.8rem;min-width:0}.repo-card header{display:flex;gap:.6rem;align-items:center;flex-wrap:wrap}.repo-name{flex:1;text-align:left;min-width:0;overflow-wrap:anywhere;border:0;background:transparent;color:var(--color-accent);font-weight:600;cursor:pointer}.repo-error{color:var(--color-danger)}.repositories--compact{font-size:.8rem;padding:.3rem 0}
</style>
