<script lang="ts">
  /** `http.get`: the host is the owner's, fixed here; what arrives on the path and query inputs fills only the path and the query. */
  type P = { host?: string; path?: string; query?: Record<string, string>; auth?: { header?: string; secret?: string; prefix?: string } }
  let { params, onChange }: { params: P | undefined; onChange: (p: unknown) => void } = $props()

  let host = $state(params?.host ?? '')
  let path = $state(params?.path ?? '')
  let query = $state(Object.entries(params?.query ?? {}).map(([k, v]) => `${k}=${v}`).join('\n'))
  let withAuth = $state(Boolean(params?.auth))
  let header = $state(params?.auth?.header ?? 'Authorization')
  let secret = $state(params?.auth?.secret ?? '')
  let prefix = $state(params?.auth?.prefix ?? '')

  const hostBad = $derived(host !== '' && !/^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*(:[0-9]{1,5})?$/i.test(host.trim()))

  function commit() {
    if (!host.trim() || hostBad) return
    const q: Record<string, string> = {}
    for (const line of query.split('\n')) {
      const at = line.indexOf('=')
      if (at > 0) q[line.slice(0, at).trim()] = line.slice(at + 1).trim()
    }
    const out: P = { host: host.trim().toLowerCase() }
    if (path.trim()) out.path = path.trim()
    if (Object.keys(q).length) out.query = q
    if (withAuth && secret.trim()) out.auth = { header: header.trim(), secret: secret.trim(), ...(prefix ? { prefix } : {}) }
    onChange(out)
  }
</script>

<label class="field">
  <span class="label">Узел</span>
  <input type="text" bind:value={host} onchange={commit} placeholder="api.open-meteo.com" aria-label="Узел" />
</label>
{#if hostBad}<p class="insp__err">Только имя узла (и порт), без схемы и пути.</p>{/if}
<label class="field">
  <span class="label">Путь</span>
  <input type="text" bind:value={path} onchange={commit} placeholder="/v1/forecast или /users/&#123;path&#125;" aria-label="Путь" />
</label>
<label class="field">
  <span class="label">Параметры запроса (по строке: имя=значение; &#123;path&#125;, &#123;path.0&#125;, &#123;path.1&#125; — то, что пришло)</span>
  <textarea rows="3" bind:value={query} onchange={commit} placeholder="lat=&#123;path.0&#125;" aria-label="Параметры запроса"></textarea>
</label>
<label class="check">
  <input type="checkbox" bind:checked={withAuth} onchange={commit} />
  заголовок с секретом
</label>
{#if withAuth}
  <label class="field">
    <span class="label">Заголовок</span>
    <input type="text" bind:value={header} onchange={commit} aria-label="Заголовок" />
  </label>
  <label class="field">
    <span class="label">Имя секрета</span>
    <input type="text" bind:value={secret} onchange={commit} placeholder="API_TOKEN" aria-label="Имя секрета" />
  </label>
  <label class="field">
    <span class="label">Перед секретом</span>
    <input type="text" bind:value={prefix} onchange={commit} placeholder="Bearer " aria-label="Перед секретом" />
  </label>
  <p class="explain">Секрет берётся у сущности источника (вход «secrets»), в ответ и в журнал не попадает.</p>
{/if}
<p class="explain">Узел задаёт владелец скрипта и его должен разрешить администратор сервера. Через вход «path» приходит одна часть пути (`&#123;path&#125;` в пути или в конец), через «query» — значения параметров; сменить узел они не могут.</p>
