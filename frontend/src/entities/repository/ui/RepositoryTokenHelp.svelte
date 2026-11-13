<script lang="ts">
 let {kind}:{kind:string}=$props()
</script>
<details class="token-help">
 <summary>Как создать токен {kind==='github'?'GitHub':'GitLab'}</summary>
 {#if kind==='github'}
  <ol>
   <li>Откройте <a href="https://github.com/settings/personal-access-tokens/new" target="_blank" rel="noreferrer">создание fine-grained токена</a> в GitHub: Settings → Developer settings → Personal access tokens → Fine-grained tokens.</li>
   <li>Укажите имя, срок действия и Resource owner — аккаунт или организацию, которой принадлежит репозиторий.</li>
   <li>Repository access → Only select repositories: выберите нужный репозиторий. Repository permissions → Contents → Read-only; Metadata → Read-only.</li>
   <li>Generate token, скопируйте значение и вставьте в поле выше. Организация может потребовать одобрения токена.</li>
  </ol>
  <a href="https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens" target="_blank" rel="noreferrer">Документация GitHub</a>
 {:else}
  <ol>
   <li>Откройте <a href="https://gitlab.com/-/user_settings/personal_access_tokens" target="_blank" rel="noreferrer">настройки токенов GitLab</a> → Add new token.</li>
   <li>Укажите имя, срок действия и scope <code>read_api</code>: здесь ветки, коммиты и дерево читаются через API. Для более узкого доступа подойдёт project access token с read_api и ролью Reporter или выше.</li>
   <li>Create token, скопируйте значение и вставьте в поле выше.</li>
  </ol>
  <a href="https://docs.gitlab.com/user/profile/personal_access_tokens/" target="_blank" rel="noreferrer">Документация GitLab</a>
 {/if}
 <p>Для приватного репозитория у аккаунта или проектного токена должен быть доступ к этому репозиторию. Права записи не нужны; push выполняется напрямую на хост.</p>
</details>
<style>
 .token-help{font-family:var(--font-ui);font-size:.8rem;color:var(--color-fg-muted)}summary{cursor:pointer;color:var(--color-accent)}ol{padding-left:1.3rem}li{margin:.4rem 0}p{margin:.6rem 0}
</style>
