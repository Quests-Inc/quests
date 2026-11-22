// Один бюджет на всё отложенное рисование карты: markdown, адреса картинок, обложки.
// Кадр длиннее бюджета останавливается, остаток переносится на следующий. 8 мс из 16.7
// оставляют браузеру время на layout и paint, чтобы держаться около 60 кадров/с.
// Одна задача, которая сама длиннее бюджета, всё равно дорисовывается — иначе она не сдвинется.
// Порядок внутри очереди: выбранное, целиком на экране, частично, остальное. Мимо экрана — выкинуть.
export const FRAME_BUDGET_MS = 8

type Job = { run: () => void; dropped: boolean; key?: string; seq: number }

const jobs: Job[] = []
let frame = 0
let seq = 0
/** Меньше — раньше. null выкидывает задачу с ключом: карточка не попадает на экран. */
let rankOf: (key: string) => number | null = () => 3

/** Переставляет уже лежащую очередь. На то, что в неё кладут, не влияет. */
export function setFrameRank(rank: (key: string) => number | null) {
  rankOf = rank
}

function rank(job: Job) {
  return job.key != null ? rankOf(job.key) : 3
}

function order() {
  for (let i = jobs.length - 1; i >= 0; i--) {
    const job = jobs[i]
    if (job.dropped || rank(job) == null) jobs.splice(i, 1)
  }
  jobs.sort((a, b) => (rank(a) ?? 3) - (rank(b) ?? 3) || a.seq - b.seq)
}

function pump() {
  frame = 0
  order()
  const started = performance.now()
  let ran = false
  while (jobs.length) {
    if (ran && performance.now() - started >= FRAME_BUDGET_MS) break
    const job = jobs.shift()
    if (!job) continue
    job.run()
    ran = true
  }
  if (jobs.length) frame = requestAnimationFrame(pump)
}

/** Поставить работу в ближайший кадр. key — карточка, по ней очередь расставляет приоритет. */
export function scheduleFrameWork(work: () => void, key?: string) {
  const job: Job = { run: work, dropped: false, key: key || undefined, seq: seq++ }
  jobs.push(job)
  if (!frame) frame = requestAnimationFrame(pump)
  return () => {
    job.dropped = true
  }
}

export function resetFrameWork() {
  for (const job of jobs) job.dropped = true
  jobs.length = 0
  seq = 0
  rankOf = () => 3
  if (frame) cancelAnimationFrame(frame)
  frame = 0
}

export function afterPaint(task: () => void, key?: string) {
  // Сначала кадр с силуэтом, потом сама работа — уже внутри бюджета.
  let cancelled = false
  let cancelJob = () => {}
  const first = requestAnimationFrame(() => {
    if (cancelled) return
    cancelJob = scheduleFrameWork(task, key)
  })
  return () => {
    cancelled = true
    cancelAnimationFrame(first)
    cancelJob()
  }
}
