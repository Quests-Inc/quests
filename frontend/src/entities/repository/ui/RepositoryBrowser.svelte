<script lang="ts">
 import {browseRepository,type Repository,type RepositoryView} from '@/shared/api/repositories.ts'
 import {copyText} from '@/shared/lib/clipboard.ts'
 let {repo}:{repo:Repository}=$props()
 let branch=$state(''),commit=$state(''),path=$state(''),data=$state<RepositoryView|null>(null),error=$state(''),loading=$state(false),revision=$state(0),key=$state(0)
 $effect(()=>{if(key!==repo.id){key=repo.id;branch=repo.branch;commit='';path='';data=null}})
 $effect(()=>{const id=repo.id,b=branch,p=path,c=commit;void revision;if(!b)return;const abort=new AbortController();loading=true;error=''
 void browseRepository(id,b,p,abort.signal,c).then(result=>{if(!abort.signal.aborted)data=result}).catch(e=>{if(!abort.signal.aborted)error=e.message||String(e)}).finally(()=>{if(!abort.signal.aborted)loading=false});return()=>abort.abort()})
 const clone=$derived(data?.clone_url?.startsWith('/')?`${location.origin}${data.clone_url}`:data?.clone_url||repo.url)
</script>
<div class="repo-browser">
 {#if clone}<div class="repo-clone"><code>{clone}</code><button type="button" class="btn btn--ghost" onclick={()=>copyText(clone)}>Копировать</button></div>{/if}
 {#if repo.kind==='local'}<p class="hint">Логин аккаунта Quests и его API-токен для этого пространства. Для push нужны права владельца.</p>{/if}
 <div class="repo-tools">
  <label>Ветка<select aria-label="Ветка репозитория" disabled={loading} value={branch} onchange={e=>{branch=e.currentTarget.value;commit='';path=''}}>{#if !data?.branches.includes(branch)}<option value={branch}>{branch}</option>{/if}{#each data?.branches||[] as name}<option value={name}>{name}</option>{/each}</select></label>
  <label>Коммит<select aria-label="Коммит репозитория" disabled={loading} value={commit} onchange={e=>{commit=e.currentTarget.value;path=''}}><option value="">Последний в ветке</option>{#each data?.commits||[] as item}<option value={item.id}>{item.id.slice(0,8)} · {item.title}</option>{/each}</select></label>
  <button type="button" class="btn btn--ghost" onclick={()=>revision++} disabled={loading}>Обновить</button>
 </div>
 {#if loading}<p role="status" class="hint">Загрузка дерева…</p>{:else if error}<p class="repo-error" role="alert">{error}</p>{:else if data}
  {#if data.limited}<p class="hint">Показана первая страница списка хоста.</p>{/if}
  {#if data.empty}<p class="hint">Пустой репозиторий. Отправьте первый коммит из локального клона.</p>{:else}
   <nav aria-label="Путь в репозитории"><button type="button" onclick={()=>path=''}>Корень</button>{#if path}<span> / {path}</span><button type="button" onclick={()=>path=path.split('/').slice(0,-1).join('/')}>↑ Выше</button>{/if}<code>{data.selected_commit?.slice(0,8)}</code></nav>
   <ul class="repo-tree">{#each data.tree as entry (entry.path)}<li>{#if entry.kind==='directory'}<button type="button" onclick={()=>path=entry.path}><span>▸</span> {entry.name}/</button>{:else}<span class="repo-file"><span aria-hidden="true">·</span> {entry.name}{entry.kind==='symlink'?' ↗':''}</span>{/if}</li>{/each}</ul>
  {/if}
 {/if}
</div>
<style>
 .repo-browser{display:grid;gap:.8rem;min-width:0;font-size:.875rem}.repo-tools{display:flex;gap:.75rem;align-items:end;flex-wrap:wrap}.repo-tools label{display:grid;gap:.35rem;font-size:.75rem;color:var(--color-fg-muted)}.repo-tools select{max-width:100%;min-height:2.2rem;background:var(--color-bg);color:var(--color-fg);border:1px solid var(--color-border);border-radius:var(--radius-md);padding:.3rem .5rem}.repo-tools label:nth-child(2){flex:1;min-width:12rem}.repo-clone{display:flex;align-items:center;gap:.5rem}.repo-clone code{overflow-wrap:anywhere;flex:1;font-size:.8rem}.repo-tree{padding:0;list-style:none;margin:0;border-top:1px solid var(--color-border)}.repo-tree li{padding:.45rem .4rem;border-bottom:1px solid var(--color-border)}.repo-tree button,nav button{padding:0;background:transparent;border:0;color:var(--color-accent);font:inherit;cursor:pointer}.repo-file{color:var(--color-fg-muted)}nav{display:flex;align-items:center;gap:.6rem;flex-wrap:wrap}nav code{margin-left:auto;color:var(--color-fg-muted);font-size:.75rem}.repo-error{color:var(--color-danger)}
</style>
