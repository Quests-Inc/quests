import { mount } from 'svelte'

import '@/app/app.css'
import App from '@/app/App.svelte'

import { installFakeApi } from '../components/fakeApi.ts'
import Review from './Review.svelte'
const appMode = location.pathname.endsWith('/app.html')
const baseApi = appMode ? installFakeApi() : null
const fallbackFetch = window.fetch.bind(window)
if (baseApi) baseApi.data.notes[0].description = '> Пояснение к разделу.\n\nФормула $\\frac{1}{2}$.\n\nТекст quest=1'

const templates = [
  {
    id: 1,
    title: 'Проверить сервисы утром',
    description: 'Проверка состояния устройств и ежедневный отчёт.',
    enabled: true,
    freq: 'daily',
    emit_mode: 'fixed',
    timezone: 'Europe/Moscow',
    steps: [{ id: 1, title: 'Проверить доступность', progress_total: 1, progress_current: 0 }],
    significance: 'common',
  },
  {
    id: 2,
    title: 'Еженедельная ревизия длинного проекта с названием, которое должно переноситься на узком экране',
    enabled: false,
    freq: 'weekly',
    weekdays: '0,2,4',
    emit_mode: 'fixed',
    timezone: 'Europe/Moscow',
    emit_pool_last_outcome: 'error',
    steps: [],
  },
  {
    id: 3,
    title: 'Случайная встреча',
    enabled: true,
    freq: 'daily',
    emit_mode: 'surprise',
    emit_chance: 0.5,
    timezone: 'Europe/Moscow',
    steps: [{ id: 2, title: 'Завершить событие', progress_total: 1, progress_current: 0 }],
  },
]
// Isolated review fixture: no request reaches the user's workspace.
window.fetch = async (input, init) => {
  const url = String(input)
  if (/\/api\/scripts$/.test(url))
    return new Response(
      JSON.stringify([
        { id: 1, title: 'Диагностика: выбор строки', purpose: 'embed', version: 1, subjects: [], description: '' },
      ]),
      { headers: { 'Content-Type': 'application/json' } }
    )
  if (appMode && !/\/templates|\/eval|\/run-stats|\/values/.test(url)) {
    if (/\/api\/(scripts|script-custom-nodes|script-nodes|script-samples|hooks|runs)(?:[/?]|$)/.test(url))
      return new Response('[]', { headers: { 'Content-Type': 'application/json' } })
    return fallbackFetch(input, init)
  }
  let value: unknown = []
  if (url.includes('/auth/state')) value = { auth_required: false, authenticated: true }
  else if (url.includes('/health'))
    value = {
      api: { status: 'ok' },
      components: { db: { status: 'ok' }, overlay: { status: 'ok' }, telegram: { status: 'ok' } },
    }
  else if (url.includes('/workspaces')) value = { current: 1, workspaces: [{ id: 1, name: 'Проверка', role: 'owner' }] }
  else if (url.includes('/attachments')) value = {}
  else if (url.includes('/eval'))
    value = {
      value: JSON.stringify({
        kind: 'markdown-target',
        version: 1,
        text: 'Требуется перезапуск',
        target: { table: 'diagnostics', row: 'restart' },
      }),
      version: 1,
    }
  else if (url.includes('/values')) value = { own: [], inherited: [], effective: [] }
  else if (url.match(/\/templates\/\d+\/streak/)) value = { current: 2, longest: 4 }
  else if (url.includes('/templates')) {
    const id = Number(url.match(/\/templates\/(\d+)/)?.[1])
    const body = init?.body ? JSON.parse(String(init.body)) : {}
    if (init?.method === 'PATCH')
      Object.assign(
        templates.find((t) => t.id === id)!,
        body
      )
    if (init?.method === 'POST') templates.push({ id: 4, ...body })
    value = templates
  } else if (url.includes('/run-stats')) value = {}
  return new Response(JSON.stringify(value), { status: 200, headers: { 'Content-Type': 'application/json' } })
}
mount(appMode ? App : Review, { target: document.getElementById('app')! })
