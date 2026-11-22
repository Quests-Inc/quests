// Один бюджет на всё отложенное рисование карты: markdown, адреса картинок, обложки.
// Кадр длиннее бюджета останавливается, остаток переносится на следующий. 8 мс из 16.7
// оставляют браузеру время на layout и paint, чтобы держаться около 60 кадров/с.
// Одна задача, которая сама длиннее бюджета, всё равно дорисовывается — иначе она не сдвинется.
export const FRAME_BUDGET_MS = 8

type Job = { run: () => void; dropped: boolean }

const jobs: Job[] = []
let frame = 0

function pump() {
  frame = 0
  const started = performance.now()
  let ran = false
  while (jobs.length) {
    if (ran && performance.now() - started >= FRAME_BUDGET_MS) break
    const job = jobs.shift()
    if (!job || job.dropped) continue
    job.run()
    ran = true
  }
  if (jobs.some((job) => !job.dropped)) frame = requestAnimationFrame(pump)
}

/** Поставить работу в ближайший кадр. Возврат снимает её, если она ещё не началась. */
export function scheduleFrameWork(work: () => void) {
  const job: Job = { run: work, dropped: false }
  jobs.push(job)
  if (!frame) frame = requestAnimationFrame(pump)
  return () => {
    job.dropped = true
  }
}

export function resetFrameWork() {
  for (const job of jobs) job.dropped = true
  jobs.length = 0
  if (frame) cancelAnimationFrame(frame)
  frame = 0
}

export function afterPaint(task: () => void) {
  // Сначала кадр с силуэтом, потом сама работа — уже внутри бюджета.
  let cancelled = false
  let cancelJob = () => {}
  const first = requestAnimationFrame(() => {
    if (cancelled) return
    cancelJob = scheduleFrameWork(task)
  })
  return () => {
    cancelled = true
    cancelAnimationFrame(first)
    cancelJob()
  }
}
