<script lang="ts">
 import {listRepositories,createRepository,linkRepository,unlinkRepository,updateRepository,type Repository,type RepositoryLink} from '@/shared/api/repositories.ts'
 import ContextMenu from '@/shared/ui/ContextMenu.svelte'
 import {copyText} from '@/shared/lib/clipboard.ts'
 import SidebarLayout from '@/shared/ui/SidebarLayout.svelte'
 import SidebarTools from '@/shared/ui/SidebarTools.svelte'
 import Icon from '@/shared/ui/Icon.svelte'
 import {sidebars,SIDEBAR_ONE_PANE} from '@/shared/lib/sidebar.svelte.ts'
 import {replaceSearch} from '@/shared/lib/urlSearch.ts'
 import {plural} from '@/shared/lib/duration.ts'
 import {untrack,onMount} from 'svelte'
 import RepositoryTokenHelp from './RepositoryTokenHelp.svelte'
 import RepositoryBrowser from './RepositoryBrowser.svelte'
 import ModalShell from '@/shared/ui/ModalShell.svelte'
 import ModalHead from '@/shared/ui/ModalHead.svelte'
 import ModalFoot from '@/shared/ui/ModalFoot.svelte'
 let {owner}:{owner?:RepositoryLink}=$props()
 let repos=$state<Repository[]>([]),error=$state(''),busy=$state(false),loaded=$state(false),expanded=$state(false),creating=$state(false),selected=$state<number|null>(untrack(()=>{const id=Number(new URL(location.href).searchParams.get('repo'));return Number.isFinite(id)&&id>0?id:null}))
 let search=$state(''),attaching=$state(false),editingId=$state<number|null>(null),slug=$state(''),originalSlug=$state('')
 let title=$state(''),kind=$state('local'),url=$state(''),branch=$state('main'),token=$state(''),attachId=$state(''),quota=$state(0),tokenSaved=$state(false),removeToken=$state(false)
 let refreshNo=$state(0)
 const shown=$derived(owner?repos.filter(repo=>repo.links.some(l=>l.kind===owner.kind&&l.id===owner.id)):repos.filter(repo=>{const q=search.trim().toLowerCase().replace(/^(?:repository|repo)=/,'');return !q||String(repo.id)===q||repo.slug?.includes(q)||repo.title.toLowerCase().includes(q)||repo.url.toLowerCase().includes(q)}))
 $effect(()=>{if(!owner){const id=selected;untrack(()=>replaceSearch(id?{repo:id}:{tab:'repositories'}))}})
 const available=$derived(repos.filter(repo=>!shown.some(x=>x.id===repo.id)))
 $effect(()=>{void owner?.id;void owner?.kind;void refreshNo;const abort={active:true};loaded=false
 void listRepositories().then(result=>{if(abort.active){repos=result.items??[];quota=result.quota_bytes??0;loaded=true}}).catch(e=>{if(abort.active)error=e.message||String(e)});return()=>{abort.active=false}})
 async function create(event:SubmitEvent){event.preventDefault();busy=true;error='';try{if(editingId!==null){await updateRepository(editingId,{title,slug,branch,...(kind!=='local'?{url,...(removeToken?{token:''}:token.trim()?{token:token.trim()}:{})}:{})})}else{await createRepository({title,slug,kind,url,branch,token,owner})};creating=false;title='';token='';expanded=true;refreshNo++}catch(e){error=e.message||String(e)}finally{busy=false}}
 async function attach(){if(!owner||!attachId)return;busy=true;error='';try{await linkRepository(Number(attachId),owner);refreshNo++;attachId='';attaching=false;expanded=true}catch(e){error=e.message||String(e)}finally{busy=false}}
 async function detach(repo:Repository){if(!owner)return;busy=true;try{await unlinkRepository(repo.id,owner);refreshNo++}catch(e){error=e.message||String(e)}finally{busy=false}}

 function openCreate(){tokenSaved=false;removeToken=false;editingId=null;title='';slug='';kind='local';url='';branch='main';creating=true;token='';error=''}
 function openEdit(repo:Repository){tokenSaved=repo.has_token;removeToken=false;editingId=repo.id;title=repo.title;slug=repo.slug??'';originalSlug=slug;kind=repo.kind;url=repo.url;branch=repo.branch;token='';error='';creating=true}
 function selectRepo(id:number){selected=id;if(window.matchMedia(SIDEBAR_ONE_PANE).matches)sidebars.setCollapsed('repositories',true);replaceSearch({repo:id})}
 let menu=$state<{repo:Repository;x:number;y:number}|null>(null)
 function context(event:MouseEvent,repo:Repository){event.preventDefault();menu={repo,x:event.clientX,y:event.clientY}}
 async function menuAction(action:string){const repo=menu?.repo;if(!repo)return;try{if(action==='edit')openEdit(repo);else if(action==='ref')await copyText(`repo=${repo.id}`);else if(action==='link')await copyText(new URL(`?repo=${repo.id}`,location.href).href);else if(action==='source'){const source=repo.clone_url||repo.url;if(!source)throw new Error('Адрес репозитория недоступен');await copyText(new URL(source,location.origin).href)}}catch(e){error=e.message||String(e)}}
 onMount(()=>{const open=(e:Event)=>{if(!owner)selectRepo((e as CustomEvent<number>).detail)};window.addEventListener('quests:repository',open);return()=>window.removeEventListener('quests:repository',open)})
 const activeRepo=$derived(repos.find(repo=>repo.id===selected)??null)
</script>
{#if owner}
 <section class="repositories repositories--linked" aria-label="Привязанные репозитории">
  {#if shown.length}<button type="button" class="block__label repo-heading" aria-expanded={expanded} onclick={()=>expanded=!expanded}>Репозитории ({shown.length}) <Icon name={expanded?'chevron-down':'chevron-right'} size={12}/></button>{:else}<span class="block__label repo-heading">Репозитории</span>{/if}
  {#if error}<p role="alert" class="repo-error">{error}</p>{/if}
  {#if expanded && shown.length}<ul class="repo-links">{#each shown as repo (repo.id)}<li class="repo-link">
   <a href={`?repo=${repo.id}`}><Icon name="folder" size={16}/><span>{repo.title}</span></a><small>{repo.kind==='local'?'Свой Git':repo.kind} · {repo.branch}</small>
   <button type="button" class="repo-unlink" disabled={busy} onclick={()=>detach(repo)} title="Отвязать репозиторий" aria-label={`Отвязать ${repo.title}`}><Icon name="close" size={14}/></button>
  </li>{/each}</ul>{/if}
  <div class="repo-attach-line"><button type="button" class="repo-text-action" aria-expanded={attaching} onclick={()=>attaching=!attaching}>Привязать репозиторий</button><span> или </span><button type="button" class="repo-text-action" onclick={openCreate}>создать новый</button></div>
  {#if attaching}<div class="repo-attach"><select aria-label="Привязать существующий репозиторий" bind:value={attachId}><option value="">Существующий репозиторий…</option>{#each available as repo}<option value={repo.id}>{repo.title}</option>{/each}</select><button type="button" class="btn btn--ghost" disabled={busy||!attachId} onclick={attach}>Привязать</button>{#if !available.length}<span class="hint">Нет непривязанных репозиториев.</span>{/if}</div>{/if}
 </section>
{:else}
 <SidebarLayout key="repositories" hasSelection={Boolean(activeRepo)}>
  {#snippet sidebar()}<aside class="repo-sidebar" aria-label="Репозитории пространства">
   <SidebarTools bind:value={search} label="Поиск репозиториев" placeholder="Название, адрес или ID" createLabel="Новый репозиторий" onCreate={openCreate}>
    <div class="repo-summary"><span>{shown.length} из {repos.length}</span><span>Git: {Math.round(repos.reduce((sum,r)=>sum+r.bytes,0)/1048576)} / {Math.round(quota/1048576)} МБ</span></div>
   </SidebarTools>
   <div class="repo-list">{#if error}<p role="alert" class="repo-empty repo-error">{error}</p>{:else if !loaded}<p class="repo-empty hint" role="status">Загрузка репозиториев…</p>{:else if !shown.length}<p class="repo-empty hint">Репозитории не найдены.</p>{/if}
    {#each shown as repo (repo.id)}<button type="button" class="repo-row" class:repo-row--on={selected===repo.id} aria-current={selected===repo.id?'true':undefined} onclick={()=>selectRepo(repo.id)} oncontextmenu={e=>context(e,repo)}><span>{repo.title}</span><small>{repo.kind==='local'?'Свой Git':repo.kind} · {repo.links.length} {plural(repo.links.length,['связь','связи','связей'])}</small></button>{/each}
   </div>
  </aside>{/snippet}
  <section class="repo-detail" aria-label="Репозиторий">
   {#if activeRepo}<header class="repo-detail__head"><h1 oncontextmenu={e=>context(e,activeRepo)}>{activeRepo.title}</h1><button type="button" class="repo-settings" title="Редактировать репозиторий" aria-label="Редактировать репозиторий" onclick={()=>openEdit(activeRepo)}><Icon name="edit" size={18}/></button></header><div class="repo-detail__body">{#key activeRepo}<RepositoryBrowser repo={activeRepo}/>{/key}</div>{:else}<p class="repo-empty hint">Выберите репозиторий для просмотра дерева.</p>{/if}
  </section>
 </SidebarLayout>
{/if}
<ContextMenu open={Boolean(menu)} x={menu?.x??0} y={menu?.y??0} items={[{id:'ref',label:`Копировать repo=${menu?.repo.id??''}`},{id:'edit',label:'Изменить'},{id:'source',label:'Копировать источник'},{id:'link',label:'Копировать ссылку'}]} onSelect={menuAction} onClose={()=>menu=null}/>
<ModalShell open={creating} onClose={()=>{if(!busy)creating=false}} closeDisabled={busy} labelledby="repo-create-title" maxWidth="38rem"><ModalHead id="repo-create-title" title={editingId===null?"Новый репозиторий":"Настройки репозитория"} onClose={()=>creating=false} closeDisabled={busy}/><form class="modal__form" onsubmit={create}>
 <label class="field"><span class="label">Название</span><input required bind:value={title} maxlength="200"/></label>
 <label class="field"><span class="label">Slug репозитория</span><input aria-label="Slug репозитория" required={editingId!==null} pattern="[a-z0-9]+(-[a-z0-9]+)*" maxlength="63" placeholder="Автоматически: repo-ID" bind:value={slug}/></label>
 <p class="hint">Уникален внутри пространства. Строчные латинские буквы, цифры и дефисы. Изменение названия не меняет slug.</p>
 {#if editingId!==null && slug!==originalSlug}<p class="hint">Git-адрес изменится. Обновите origin в локальном клоне.</p>{/if}
 <label class="field"><span class="label">Хранение</span><select bind:value={kind} disabled={editingId!==null}><option value="local">Свой Git в Quests</option><option value="github">GitHub</option><option value="gitlab">GitLab</option></select></label>
 {#if kind!=='local'}<label class="field"><span class="label">Адрес репозитория</span><input type="url" required placeholder={`https://${kind}.com/owner/project`} bind:value={url}/></label><label class="field"><span class="label">{editingId===null?'Токен доступа · необязательно':tokenSaved?'Заменить токен доступа':'Токен доступа'}</span><input aria-label="Токен доступа" type="password" autocomplete="new-password" disabled={removeToken} placeholder={tokenSaved?'Сохранён · пусто — оставить текущий':'Не задан'} bind:value={token}/></label>
 {#if editingId!==null && tokenSaved}<label class="hint"><input type="checkbox" bind:checked={removeToken}/> Удалить сохранённый токен</label>{/if}
 <p class="hint">{tokenSaved?'Сохранённый токен не показывается. Пустое поле оставит его без изменений.':'Для публичного репозитория токен необязателен; для приватного нужен токен с доступом к нему.'} Токен хранится зашифрованным. Дерево и история доступны участникам пространства.</p>
 <RepositoryTokenHelp {kind}/>
 {/if}
 <label class="field"><span class="label">Ветка</span><input required bind:value={branch}/></label>
 {#if error}<p role="alert" class="repo-error">{error}</p>{/if}
 <ModalFoot onCancel={()=>creating=false} submitLabel={editingId===null?"Создать":"Сохранить"} {busy}/>
</form></ModalShell>
<style>
 .repo-settings{display:inline-flex;align-items:center;justify-content:center;flex:none;width:2.25rem;height:2.25rem;padding:0;border:0;border-radius:var(--radius-md);background:transparent;color:var(--color-fg-muted);cursor:pointer}.repo-settings:hover{color:var(--color-accent)}.repo-settings:focus-visible{outline:2px solid var(--color-accent);outline-offset:2px}

 .repositories--linked{display:grid;gap:.75rem;min-width:0;margin-bottom:1.5rem}
 .repo-heading{display:flex;align-items:center;gap:.3rem;width:fit-content;margin:0;padding:0;border:0;background:transparent;line-height:1}
 button.repo-heading{cursor:pointer}button.repo-heading:hover{color:var(--color-fg-muted)}
 .repo-links{display:grid;gap:.5rem;list-style:none;margin:0;padding:0}
 .repo-link{display:flex;align-items:center;gap:.5rem;padding:.5rem .75rem;border-radius:var(--pill-radius,12px);background:var(--pill-bg,#0f0f0f);box-shadow:var(--pill-shadow,none);min-width:0;flex-wrap:wrap}
 .repo-link a{display:flex;align-items:center;gap:.35rem;min-width:0;flex:1;font-weight:600}.repo-link a span{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
 .repo-link small{color:var(--color-fg-muted);font-family:var(--font-ui);font-size:.75rem}
 .repo-unlink{display:flex;padding:.25rem;background:transparent;border:0;color:var(--color-fg-muted);cursor:pointer}
 .repo-text-action{padding:0;background:transparent;border:0;color:var(--color-accent);font:inherit;text-decoration:underline;text-underline-offset:3px;cursor:pointer}
 .repo-attach-line{display:flex;align-items:baseline;gap:.25rem;flex-wrap:wrap;color:var(--color-fg-muted)}.repo-attach{display:flex;gap:.5rem;align-items:center;flex-wrap:wrap}.repo-attach select{min-width:0;max-width:100%;flex:1}
 .repo-sidebar{display:flex;flex-direction:column;min-height:0;overflow:hidden;font-family:var(--font-ui)}
 .repo-summary{display:flex;justify-content:space-between;gap:.5rem;flex-wrap:wrap;font-size:.75rem;color:var(--color-fg-muted)}
 .repo-list{overflow:auto;min-height:0;flex:1}.repo-row{display:block;width:100%;text-align:left;border:0;border-bottom:1px solid var(--color-border);background:transparent;color:var(--color-fg);padding:.7rem .75rem;cursor:pointer;font:inherit;overflow-wrap:anywhere}
 .repo-row:hover{background:color-mix(in srgb,var(--color-accent) 5%,transparent)}.repo-row--on{background:color-mix(in srgb,var(--color-accent) 10%,transparent);color:var(--color-accent)}
 .repo-row small{display:block;margin-top:.3rem;color:var(--color-fg-muted);font-size:.75rem}
 .repo-detail{min-width:0;min-height:0;overflow:auto}.repo-detail__head{display:flex;justify-content:space-between;align-items:center;gap:.75rem;padding:1.5rem 2rem 1rem;border-bottom:1px solid var(--color-border)}.repo-detail h1{margin:0;font-size:1.5rem;overflow-wrap:anywhere}.repo-detail__body{padding:1.5rem 2rem}.repo-empty{padding:1rem;margin:0}.repo-error{color:var(--color-danger)}
 @media(max-width:45rem){.repo-detail__head,.repo-detail__body{padding:1rem}}
</style>
