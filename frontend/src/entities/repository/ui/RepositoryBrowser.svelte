<script lang="ts">
 import {browseRepository,type Repository,type RepositoryView} from '@/shared/api/repositories.ts'
 import {ago} from '@/shared/lib/time.ts'
 import FileKindIcon from '@/shared/ui/FileKindIcon.svelte'
 import Icon from '@/shared/ui/Icon.svelte'
 import {copyText} from '@/shared/lib/clipboard.ts'
 let {repo}:{repo:Repository}=$props()
 let branch=$state(''),commit=$state(''),path=$state(''),data=$state<RepositoryView|null>(null),error=$state(''),loading=$state(false),revision=$state(0),key=$state('')
 $effect(()=>{const next=`${repo.id}:${repo.url}:${repo.branch}`;if(key!==next){key=next;branch=repo.branch;commit='';path='';data=null}})
 $effect(()=>{const id=repo.id,b=branch,p=path,c=commit;void revision;if(!b)return;const abort=new AbortController();loading=true;error=''
 void browseRepository(id,b,p,abort.signal,c).then(result=>{if(!abort.signal.aborted)data=result}).catch(e=>{if(!abort.signal.aborted)error=e.message||String(e)}).finally(()=>{if(!abort.signal.aborted)loading=false});return()=>abort.abort()})
 const entries=$derived([...(data?.tree??[])].sort((a,b)=>Number(b.kind==='directory')-Number(a.kind==='directory')||a.name.localeCompare(b.name)))
 const clone=$derived(data?.clone_url?.startsWith('/')?`${location.origin}${data.clone_url}`:data?.clone_url||repo.url)
 const prefixCommand=$derived(clone?`git config --global url."${clone.slice(0,clone.lastIndexOf('/')+1)}".insteadOf "q:"`:'')
</script>
<div class="repo-browser">
 {#if clone}<div class="repo-clone"><code>{clone}</code><button type="button" class="btn btn--ghost" onclick={()=>copyText(clone)}>Копировать</button></div>{/if}
 {#if repo.kind==='local' && clone.includes('/git/')}<details class="repo-prefix"><summary>Короткие адреса q:</summary><p class="hint">Один раз выполните на своём компьютере для этого пространства:</p><div class="repo-clone"><code>{prefixCommand}</code><button type="button" class="btn btn--ghost" onclick={()=>copyText(prefixCommand)}>Копировать настройку</button></div><code>git clone q:{repo.slug}.git</code></details>{/if}
 {#if repo.kind==='local'}<p class="hint">Логин аккаунта Quests и его API-токен для этого пространства. Для push нужны права владельца.</p>{/if}
 <div class="repo-tools">
  <label>Ветка<select aria-label="Ветка репозитория" disabled={loading} value={branch} onchange={e=>{branch=e.currentTarget.value;commit='';path=''}}>{#if !data?.branches.includes(branch)}<option value={branch}>{branch}</option>{/if}{#each data?.branches||[] as name}<option value={name}>{name}</option>{/each}</select></label>
  <label>Коммит<select aria-label="Коммит репозитория" disabled={loading} value={commit} onchange={e=>{commit=e.currentTarget.value;path=''}}><option value="">Последний в ветке</option>{#each data?.commits||[] as item}<option value={item.id}>{item.id.slice(0,8)} · {item.title}</option>{/each}</select></label>
  <button type="button" class="btn btn--ghost" onclick={()=>revision++} disabled={loading}>Обновить</button>
 </div>
 {#if loading}<p role="status" class="hint">Загрузка дерева…</p>{:else if error}<p class="repo-error" role="alert">{error}</p>{:else if data}
  {#if data.limited}<p class="hint">Часть списка или истории недоступна.</p>{/if}
  {#if data.empty}<p class="hint">Пустой репозиторий. Отправьте первый коммит из локального клона.</p>{:else}
   <nav aria-label="Путь в репозитории"><button type="button" onclick={()=>path=''}>Корень</button>{#if path}<span> / {path}</span><button type="button" onclick={()=>path=path.split('/').slice(0,-1).join('/')}>↑ Выше</button>{/if}<code>{data.selected_commit?.slice(0,8)}</code></nav>
   <ul class="repo-tree">{#each entries as entry (entry.path)}<li>
    <span class="repo-entry">{#if entry.kind==='directory'}<button type="button" onclick={()=>path=entry.path}><Icon name="folder" size={17}/><span>{entry.name}/</span></button>{:else}<span class="repo-file"><FileKindIcon att={{filename:entry.name}} size={17}/><span>{entry.name}{entry.kind==='symlink'?' ↗':''}</span></span>{/if}</span>
    <span class="repo-message" title={entry.last_commit?.title??'История недоступна'}>{entry.last_commit?.title??'—'}</span>
    {#if entry.last_commit}<time datetime={entry.last_commit.date} title={`${entry.last_commit.author} · ${new Date(entry.last_commit.date).toLocaleString()}`}>{ago(entry.last_commit.date)}</time>{:else}<span class="repo-date">—</span>{/if}
   </li>{/each}</ul>
  {/if}
 {/if}
</div>
<style>
 .repo-browser{font-family:var(--font-ui);display:grid;gap:.8rem;min-width:0;font-size:.875rem}.repo-tools{display:flex;gap:.75rem;align-items:end;flex-wrap:wrap}.repo-tools label{display:grid;gap:.35rem;font-size:.75rem;color:var(--color-fg-muted)}.repo-tools select{max-width:100%;min-height:2.2rem;background:var(--color-bg);color:var(--color-fg);border:1px solid var(--color-border);border-radius:var(--radius-md);padding:.3rem .5rem}.repo-tools label:nth-child(2){flex:1;min-width:12rem}.repo-clone{display:flex;align-items:center;gap:.5rem}.repo-clone code{overflow-wrap:anywhere;flex:1;font-size:.8rem}.repo-tree{padding:0;list-style:none;margin:0;border-top:1px solid var(--color-border)}.repo-tree li{display:grid;grid-template-columns:minmax(8rem,1fr) minmax(0,1.5fr) auto;align-items:center;gap:1rem;padding:.45rem .4rem;border-bottom:1px solid var(--color-border)}.repo-tree button,nav button{padding:0;background:transparent;border:0;color:var(--color-accent);font:inherit;cursor:pointer}.repo-entry{min-width:0}.repo-entry button,.repo-file{display:flex;align-items:center;gap:.5rem;min-width:0}.repo-entry span span,.repo-entry button span{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.repo-file{color:var(--color-fg)}.repo-message{overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--color-fg-muted)}time,.repo-date{font-size:.75rem;color:var(--color-fg-muted);white-space:nowrap}@media(max-width:640px){.repo-tree li{grid-template-columns:minmax(0,1fr) auto;gap:.4rem}.repo-message{grid-column:1;grid-row:2;font-size:.75rem}.repo-tree time,.repo-date{grid-column:2;grid-row:1 / span 2}}nav{display:flex;align-items:center;gap:.6rem;flex-wrap:wrap}nav code{margin-left:auto;color:var(--color-fg-muted);font-size:.75rem}.repo-error{color:var(--color-danger)}
</style>
