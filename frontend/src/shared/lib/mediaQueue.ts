/** A two-wide job queue with a pause. Map photos do not use it. */

type Job = {
  start: (release: () => void) => void
  dropped: boolean
  release: (() => void) | null
}

const jobs: Job[] = []
let active = 0
let paused = false
const MAX = 2

function pump() {
  if (paused) return
  while (active < MAX && jobs.length) {
    const job = jobs.shift()!
    if (job.dropped) continue
    active++
    let released = false
    const release = () => {
      if (released) return
      released = true
      job.release = null
      active--
      pump()
    }
    job.release = release
    job.start(release)
  }
}

export function setMediaPaused(next: boolean) {
  if (paused === next) return
  paused = next
  if (!paused) pump()
}

/** Run `start` when a slot is free. Call the given release when the decode finished. The returned function drops the job. */
export function enqueueMedia(start: (release: () => void) => void) {
  const job: Job = { start, dropped: false, release: null }
  jobs.push(job)
  pump()
  return () => {
    if (job.dropped) return
    job.dropped = true
    const index = jobs.indexOf(job)
    if (index >= 0) jobs.splice(index, 1)
    else job.release?.()
  }
}

export function mediaPaused() {
  return paused
}

export function resetMediaQueue() {
  jobs.splice(0)
  active = 0
  paused = false
}
