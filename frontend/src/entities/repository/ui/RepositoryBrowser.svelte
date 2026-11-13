<script lang="ts">
 import {browseRepository,type Repository,type RepositoryView} from '@/shared/api/repositories.ts'
 import {copyText} from '@/shared/lib/clipboard.ts'
 let {repo,initialPath=''}:{repo:Repository;initialPath?:string}=$props()
 let branchDraft=$state('')
 let branch=$state(''),path=$state(''),data=$state<RepositoryView|null>(null),error=$state(''),loading=$state(false),revision=$state(0)
 let key=$state('')
 $effect(()=>{const next=`${repo.id}:${initialPath}`;if(key!==next){key=next;branch=repo.branch;branchDraft=repo.branch;path=initialPath}})
 $effect(()=>{const id=repo.id,b=branch,p=path;void revision;if(!b)return;const abort=new AbortController();loading=true;error='';data=null
 void browseRepository(id,b,p,abort.signal).then(result=>{if(!abort.signal.aborted)data=result}).catch(e=>{if(!abort.signal.aborted)error=e.message||String(e)}).finally(()=>{if(!abort.signal.aborted)loading=false});return()=>abort.abort()})
 const clone=$derived(data?.clone_url?.startsWith('/')?`${location.origin}${data.clone_url}`:data?.clone_url||repo.url)
</script>
<div class="repo-browser">
 <div class="repo-tools">
  <label>Ветка <input aria-label="Ветка репозитория" bind:value={branchDraft} onchange={()=>branch=branchDraft} onkeydown={e=>{if(e.key==='Enter')branch=branchDraft}} list={`repo-branches-${repo.id}`} /></label>
  <datalist id={`repo-branches-${repo.id}`}>{#each data?.branches||[] as name}<option value={name}></option>{/each}</datalist>
  <button type="button" class="btn btn--ghost" onclick={()=>{branch=branchDraft;revision++}}>Обновить</button>
 </div>
 {#if clone}<div class="repo-clone"><code>{clone}</code><button type="button" class="btn btn--ghost" onclick={()=>copyText(clone)}>Копировать адрес клона</button></div>{/if}
 {#if repo.kind==='local'}<p class="hint">git clone: имя пользователя любое, пароль — токен Quests этого пространства. Для push нужны права владельца.</p>{/if}
 {#if loading}<p role="status">Загрузка репозитория…</p>{:else if error}<p class="repo-error" role="alert">{error}</p>{:else if data}
  {#if data.limited}<p class="hint">Хост вернул первую страницу списка. Остальные ветки доступны по имени; полный каталог — в локальном клоне.</p>{/if}
  {#if data.empty}<p class="hint">Пустой репозиторий. Добавьте файлы и отправьте первый коммит.</p>{:else}
   <nav aria-label="Путь в репозитории"><button type="button" onclick={()=>path=''}>Корень</button>{#if path}<span> / {path}</span><button type="button" onclick={()=>path=path.split('/').slice(0,-1).join('/')}>На уровень выше</button>{/if}</nav>
   {#if data.file!==undefined}<pre class="repo-file">{data.file}</pre>{:else if data.binary}<p class="hint">Двоичный файл. Просмотр доступен в локальном клоне.</p>{:else}
    <ul class="repo-tree">{#each data.tree as entry (entry.path)}<li><button type="button" onclick={()=>path=entry.path}><span>{entry.kind==='directory'?'▸':'·'}</span> {entry.name}{entry.kind==='symlink'?' ↗':''}</button></li>{/each}</ul>
   {/if}
   <details><summary>Последние коммиты · {data.commits.length}</summary><ol class="repo-log">{#each data.commits as commit (commit.id)}<li><code>{commit.id.slice(0,8)}</code> <b>{commit.title}</b><span class="hint">{commit.author} · {commit.date}</span></li>{/each}</ol></details>
  {/if}
 {/if}
</div>
<style>
 .repo-browser{display:grid;gap:.7rem;min-width:0}.repo-tools{display:flex;gap:.5rem;align-items:center;flex-wrap:wrap}.repo-tools input{max-width:14rem}.repo-clone{display:flex;align-items:center;gap:.5rem;flex-wrap:wrap}.repo-clone code{overflow-wrap:anywhere}.repo-file{max-height:28rem;overflow:auto;padding:1rem;background:var(--color-bg);white-space:pre;font-size:.8rem}.repo-tree,.repo-log{padding:0;list-style:none}.repo-tree button,nav button{padding:.35rem .5rem;background:transparent;border:0;color:var(--color-fg);cursor:pointer}.repo-tree button:hover,nav button:hover{color:var(--color-accent)}.repo-log li{display:grid;gap:.2rem;padding:.6rem 0;border-bottom:1px solid var(--color-border)}.repo-error{color:var(--color-danger)}
</style>
