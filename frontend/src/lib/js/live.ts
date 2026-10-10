/**
 * Live updates via WebSocket. Reconnects with backoff.
 * onEvent receives parsed server messages ({ type, revision, action, quest_id }).
 *
 * The server keeps no event backlog, so anything that happened while the
 * socket was dead (sleep, network switch, background tab) is simply lost.
 * onResync is the catch-up: it fires after every RE-connect and when the tab
 * comes back to the foreground after a while, and the caller should refetch.
 */

// Server sends an app-level {type:'ping'} every 25s (see wsPingInterval in
// go/internal/httpapi/events.go) regardless of real traffic — so silence
// this long means the socket is dead, not just quiet.
const STALE_AFTER_MS = 70_000
const WATCHDOG_INTERVAL_MS = 10_000
// A CONNECTING socket that never fires open/error/close (seen on mobile
// right after a refresh, mid network handover) can sit there well past any
// sane connect time — the OS-level TCP timeout it's actually waiting on
// runs much longer than a user will. Give a connect attempt this long,
// then abandon it and retry ourselves.
const CONNECT_TIMEOUT_MS = 8_000
// A tab hidden for less than this has not missed anything worth a refetch.
const RESYNC_AFTER_HIDDEN_MS = 10_000
// Reconnect, visibility and online events often arrive together.
const RESYNC_MIN_GAP_MS = 2_000

export function subscribeQuestEvents(onEvent, { onStatus, onResync } = {}) {
  let stopped = false
  let socket = null
  let attempt = 0
  let timer = null
  let watchdog = null
  let lastMessageAt = 0
  let everOpened = false
  let hiddenAt = 0
  let lastResyncAt = 0

  const wsUrl = () => {
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
    return `${proto}//${location.host}/ws`
  }

  const setStatus = (status) => {
    if (onStatus) onStatus(status)
  }

  const resync = () => {
    const now = Date.now()
    if (stopped || !onResync || now - lastResyncAt < RESYNC_MIN_GAP_MS) return
    lastResyncAt = now
    onResync()
  }

  const schedule = () => {
    if (stopped) return
    const delay = Math.min(8000, 500 * 2 ** attempt)
    attempt += 1
    timer = setTimeout(connect, delay)
  }

  const connect = () => {
    if (stopped) return
    setStatus('connecting')
    const ws = new WebSocket(wsUrl())
    socket = ws

    const connectTimeout = setTimeout(() => {
      if (socket !== ws || ws.readyState !== WebSocket.CONNECTING) return
      try {
        ws.close()
      } catch {
        /* ignore */
      }
    }, CONNECT_TIMEOUT_MS)

    socket.addEventListener('open', () => {
      clearTimeout(connectTimeout)
      attempt = 0
      lastMessageAt = Date.now()
      setStatus('live')
      // Events from the gap are gone for good; a first connect needs nothing
      // (the app has just loaded), every later one does.
      if (everOpened) resync()
      everOpened = true
    })

    socket.addEventListener('message', (ev) => {
      lastMessageAt = Date.now()
      try {
        const data = JSON.parse(ev.data)
        if (data?.type === 'ping') {
          try {
            socket?.send('pong')
          } catch {
            /* ignore */
          }
          return
        }
        onEvent(data)
      } catch {
        /* ignore malformed */
      }
    })

    socket.addEventListener('close', () => {
      clearTimeout(connectTimeout)
      setStatus('reconnect')
      schedule()
    })

    socket.addEventListener('error', () => {
      socket?.close()
    })
  }

  /** readyState can stay OPEN forever on a half-open ("zombie") connection —
   * e.g. after a laptop sleep/wake or a network switch that never delivers
   * a close frame. The server-side ping is our only reliable heartbeat:
   * if nothing (not even a ping) arrived recently, the socket is dead —
   * force-close it so the existing reconnect logic takes over. */
  const checkStale = () => {
    if (stopped || !socket || socket.readyState !== WebSocket.OPEN) return
    if (Date.now() - lastMessageAt > STALE_AFTER_MS) {
      try {
        socket.close()
      } catch {
        /* ignore */
      }
    }
  }

  const onVisibility = () => {
    if (document.visibilityState === 'hidden') {
      hiddenAt = Date.now()
      return
    }
    if (stopped) return
    checkStale()
    if (hiddenAt && Date.now() - hiddenAt > RESYNC_AFTER_HIDDEN_MS) resync()
    hiddenAt = 0
  }

  // Network is back: don't wait for the backoff timer or the 70s watchdog.
  const onOnline = () => {
    if (stopped) return
    if (!socket || socket.readyState === WebSocket.CLOSED) {
      if (timer) clearTimeout(timer)
      attempt = 0
      connect()
    }
    resync()
  }

  const hardClose = () => {
    stopped = true
    if (timer) clearTimeout(timer)
    if (watchdog) clearInterval(watchdog)
    document.removeEventListener('visibilitychange', onVisibility)
    window.removeEventListener('online', onOnline)
    try {
      socket?.close()
    } catch {
      /* ignore */
    }
    socket = null
    setStatus('off')
  }

  // Drop the socket on unload so the server doesn't keep a zombie client
  // (zombies made the overlay think a tab was still open → no new tab, no UI).
  const onPageHide = (ev) => {
    if (ev && ev.persisted) {
      try {
        socket?.close()
      } catch {
        /* ignore */
      }
      return
    }
    hardClose()
  }
  window.addEventListener('pagehide', onPageHide)
  window.addEventListener('beforeunload', onPageHide)

  document.addEventListener('visibilitychange', onVisibility)
  window.addEventListener('online', onOnline)

  connect()
  watchdog = setInterval(checkStale, WATCHDOG_INTERVAL_MS)

  return () => {
    window.removeEventListener('pagehide', onPageHide)
    window.removeEventListener('beforeunload', onPageHide)
    hardClose()
  }
}
