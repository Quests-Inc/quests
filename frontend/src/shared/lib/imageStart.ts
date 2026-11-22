/**
 * Назначает адрес картинки в общем бюджете кадра. Слот — только это назначение:
 * скачивание и декод идут сами и слот не держат. Остановка камеры очередь не сбрасывает.
 */
import { resetFrameWork, scheduleFrameWork } from './frameTasks.ts'

/** Поставить назначение адреса в кадр. Возвращённая функция снимает его, если оно ещё не вышло. */
export function scheduleImage(run: () => void, key?: string) {
  return scheduleFrameWork(run, key)
}

export function resetImageStarts() {
  resetFrameWork()
}
