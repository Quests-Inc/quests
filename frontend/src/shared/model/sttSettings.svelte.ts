import { getStt, saveStt } from '@/shared/api/stt.ts'

type SttSettings = { baseUrl: string; model: string; apiKey: string; hasKey: boolean; enabled: boolean }

const legacyKey = 'quests.stt'

function readLegacy(): { baseUrl: string; model: string; apiKey: string } {
  try {
    const raw = JSON.parse(localStorage.getItem(legacyKey) || '{}') as Partial<SttSettings>
    return {
      baseUrl: typeof raw.baseUrl === 'string' ? raw.baseUrl : '',
      model: typeof raw.model === 'string' && raw.model ? raw.model : 'whisper-1',
      apiKey: typeof raw.apiKey === 'string' ? raw.apiKey : '',
    }
  } catch {
    return { baseUrl: '', model: 'whisper-1', apiKey: '' }
  }
}

let current = $state<SttSettings>({ baseUrl: '', model: 'whisper-1', apiKey: '', hasKey: false, enabled: false })
let ready = false
let keyDirty = false
let saveTimer = 0

async function push() {
  const body: { base_url: string; model: string; api_key?: string } = {
    base_url: current.baseUrl,
    model: current.model || 'whisper-1',
  }
  if (keyDirty) body.api_key = current.apiKey
  const data = await saveStt(body)
  current.hasKey = !!data?.has_key
  current.enabled = !!data?.enabled
  if (keyDirty) {
    keyDirty = false
    current.apiKey = ''
  }
  try {
    localStorage.removeItem(legacyKey)
  } catch {
    /* The server copy is the one that counts. */
  }
}

function schedule() {
  if (!ready) return
  window.clearTimeout(saveTimer)
  saveTimer = window.setTimeout(() => {
    void push().catch(() => {
      /* The next edit tries again; a failed save leaves the fields as typed. */
    })
  }, 400)
}

async function pull() {
  try {
    const data = await getStt()
    if (data) {
      current.baseUrl = data.base_url || ''
      current.model = data.model || 'whisper-1'
      current.hasKey = !!data.has_key
      current.enabled = !!data.enabled
    }
  } catch {
    /* Offline: recording still saves, transcription stays off. */
  }
  if (!current.baseUrl) {
    const old = readLegacy()
    if (old.baseUrl) {
      current.baseUrl = old.baseUrl
      current.model = old.model
      current.apiKey = old.apiKey
      current.enabled = true
      keyDirty = old.apiKey !== ''
      ready = true
      try {
        await push()
      } catch {
        /* Kept in the form; the user can save again from settings. */
      }
      return
    }
  }
  ready = true
}

void pull()

export const sttSettings = {
  get baseUrl() {
    return current.baseUrl
  },
  set baseUrl(value: string) {
    current.baseUrl = value
    current.enabled = value.trim() !== ''
    schedule()
  },
  get model() {
    return current.model
  },
  set model(value: string) {
    current.model = value
    schedule()
  },
  get apiKey() {
    return current.apiKey
  },
  set apiKey(value: string) {
    current.apiKey = value
    keyDirty = true
    schedule()
  },
  get enabled() {
    return current.enabled
  },
  get hasKey() {
    return current.hasKey
  },
}
