/**
 * Назначает адрес картинки по два за кадр. Слот — только это назначение:
 * скачивание и декод идут сами и слот не держат. Остановка камеры очередь не сбрасывает и не выплёскивает.
 */

type Job = { run: () => void; dropped: boolean }

const pending: Job[] = []
let scheduled = false
const PER_FRAME = 2

function pump() {
  scheduled = false
  let started = 0
  while (started < PER_FRAME && pending.length) {
    const job = pending.shift()!
    if (job.dropped) continue
    started++
    job.run()
  }
  if (pending.some((job) => !job.dropped)) {
    scheduled = true
    requestAnimationFrame(pump)
  } else pending.length = 0
}

/** Поставить назначение адреса в кадр. Возвращённая функция снимает его, если оно ещё не вышло. */
export function scheduleImage(run: () => void) {
  const job: Job = { run, dropped: false }
  pending.push(job)
  if (!scheduled) {
    scheduled = true
    requestAnimationFrame(pump)
  }
  return () => {
    job.dropped = true
  }
}

export function resetImageStarts() {
  pending.length = 0
  scheduled = false
}
