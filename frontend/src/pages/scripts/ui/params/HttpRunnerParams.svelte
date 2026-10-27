<script lang="ts">
  /**
   * `http.runner`: the same request as `http.get`, sent by the runner from its own machine. The server's policy does not apply
   * (it can reach a local network); what limits the request is set here: the hosts it may touch, the time and the size.
   */
  type P = {
    host?: string
    scheme?: string
    path?: string
    query?: Record<string, string>
    headers?: Record<string, string>
    allow?: string[]
    timeout_sec?: number
    max_bytes?: number
    no_redirect?: boolean
    auth?: { header?: string; secret?: string; prefix?: string }
  }
  let { params, onChange }: { params: P | undefined; onChange: (p: unknown) => void } = $props()

  const lines = (m?: Record<string, string>) => Object.entries(m ?? {}).map(([k, v]) => `${k}=${v}`).join('\n')

  let host = $state(params?.host ?? '')
  let scheme = $state(params?.scheme ?? 'https')
  let path = $state(params?.path ?? '')
  let query = $state(lines(params?.query))
  let headers = $state(lines(params?.headers))
  let allow = $state((params?.allow ?? []).join(', '))
  let timeout = $state(params?.timeout_sec ? String(params.timeout_sec) : '')
  let maxKb = $state(params?.max_bytes ? String(Math.round(params.max_bytes / 1024)) : '')
  let noRedirect = $state(Boolean(params?.no_redirect))
  let withAuth = $state(Boolean(params?.auth))
  let header = $state(params?.auth?.header ?? 'Authorization')
  let secret = $state(params?.auth?.secret ?? '')
  let prefix = $state(params?.auth?.prefix ?? '')

  const hostBad = $derived(host !== '' && !/^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*(:[0-9]{1,5})?$/i.test(host.trim()))

  function pairs(text: string): Record<string, string> {
    const out: Record<string, string> = {}
    for (const line of text.split('\n')) {
      const at = line.indexOf('=')
      if (at > 0) out[line.slice(0, at).trim()] = line.slice(at + 1).trim()
    }
    return out
  }

  function commit() {
    if (!host.trim() || hostBad) return
    const out: P = { host: host.trim().toLowerCase() }
    if (scheme === 'http') out.scheme = 'http'
    if (path.trim()) out.path = path.trim()
    const q = pairs(query)
    if (Object.keys(q).length) out.query = q
    const h = pairs(headers)
    if (Object.keys(h).length) out.headers = h
    const a = allow.split(/[,\s]+/).filter(Boolean)
    if (a.length) out.allow = a
    if (Number(timeout) > 0) out.timeout_sec = Math.round(Number(timeout))
    if (Number(maxKb) > 0) out.max_bytes = Math.round(Number(maxKb) * 1024)
    if (noRedirect) out.no_redirect = true
    if (withAuth && secret.trim()) out.auth = { header: header.trim(), secret: secret.trim(), ...(prefix ? { prefix } : {}) }
    onChange(out)
  }
</script>

<label class="field">
  <span class="label">Схема</span>
  <select bind:value={scheme} onchange={commit} aria-label="Схема">
    <option value="https">https</option>
    <option value="http">http (без шифрования)</option>
  </select>
</label>
<label class="field">
  <span class="label">Узел (и порт)</span>
  <input type="text" bind:value={host} onchange={commit} placeholder="api.open-meteo.com или 192.168.1.5:8080" aria-label="Узел" />
</label>
{#if hostBad}<p class="insp__err">Только имя узла или адрес (и порт), без схемы и пути.</p>{/if}
<label class="field">
  <span class="label">Путь</span>
  <input type="text" bind:value={path} onchange={commit} placeholder="/v1/forecast или /users/&#123;path&#125;" aria-label="Путь" />
</label>
<label class="field">
  <span class="label">Параметры запроса (по строке: имя=значение)</span>
  <textarea rows="3" bind:value={query} onchange={commit} placeholder="current=temperature_2m" aria-label="Параметры запроса"></textarea>
</label>
<label class="field">
  <span class="label">Заголовки (по строке: имя=значение)</span>
  <textarea rows="2" bind:value={headers} onchange={commit} placeholder="Accept-Language=ru" aria-label="Заголовки"></textarea>
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
  <p class="explain">Секрет берётся у сущности источника (вход «secrets») и уходит на раннер вместе с задачей.</p>
{/if}
<label class="field">
  <span class="label">Разрешённые узлы (через запятую; пусто — без ограничений)</span>
  <input type="text" bind:value={allow} onchange={commit} placeholder="api.example.org, *.example.org" aria-label="Разрешённые узлы" />
</label>
<label class="field">
  <span class="label">Время, с (по умолчанию 20, до 300)</span>
  <input type="number" min="1" max="300" bind:value={timeout} onchange={commit} aria-label="Время, секунд" />
</label>
<label class="field">
  <span class="label">Размер ответа, КБ (по умолчанию 256, до 400)</span>
  <input type="number" min="1" max="400" bind:value={maxKb} onchange={commit} aria-label="Размер ответа, КБ" />
</label>
<label class="check">
  <input type="checkbox" bind:checked={noRedirect} onchange={commit} />
  не ходить по редиректам
</label>
<p class="explain">
  Запрос отправляет раннер со своей машины, политика сервера на него не действует: он достаёт и локальную сеть. Ограничения
  задаёте здесь; список узлов действует и на редиректы. Нужен раннер версии с этим узлом, на старом задача завершится ошибкой.
</p>
