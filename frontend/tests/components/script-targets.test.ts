import { afterEach, expect, mock, test } from 'bun:test'
const evaluate = mock(async () => '')
mock.module('@/shared/api/scripts.ts', () => ({ evalScriptText: evaluate }))
import { scriptEmbed } from '@/shared/lib/scriptEmbed.ts'
const source =
  'script=1{choice}\n\n<!-- table-id: diagnostic -->\n| Code | Meaning |\n|---|---|\n| <!-- row-id: ok -->0 | Норма |\n| <!-- row-id: fail -->1 | Сбой |'
const envelope = (row: string) =>
  JSON.stringify({ kind: 'markdown-target', version: 1, text: row, target: { table: 'diagnostic', row } })
const cleanups: (() => void)[] = []
afterEach(() => {
  for (const f of cleanups.splice(0)) f()
  evaluate.mockReset()
})
const settle = () => new Promise((r) => setTimeout(r, 30))
function setup(text = source) {
  const root = document.createElement('div')
  root.innerHTML = `<p><a href="?script=1&amp;_=choice${text.includes('?auto') ? '&amp;auto=1' : ''}">choice</a></p><table data-md-table="diagnostic"><tbody><tr data-md-row="ok"><td>Норма</td></tr><tr data-md-row="fail"><td>Сбой</td></tr></tbody></table>`
  document.body.append(root)
  let scrolls = 0
  for (const row of root.querySelectorAll('tr'))
    (row as HTMLElement).scrollIntoView = () => {
      scrolls++
    }
  const action = scriptEmbed(root)
  cleanups.push(() => {
    action.destroy()
    root.remove()
  })
  return { root, scrolls: () => scrolls, action }
}
test('explicit result highlights stable row, jump does not rerun, rerun replaces previous highlight', async () => {
  evaluate.mockResolvedValueOnce(envelope('fail')).mockResolvedValueOnce(envelope('ok'))
  const { root, scrolls } = setup()
  ;(root.querySelector('a') as HTMLElement).click()
  await settle()
  expect(root.querySelector('tr.md-target--selected')?.textContent).toContain('Сбой')
  expect(scrolls()).toBe(1)
  ;(root.querySelector('[data-target-link]') as HTMLElement).click()
  await settle()
  expect(evaluate).toHaveBeenCalledTimes(1)
  expect(scrolls()).toBe(2)
  ;(root.querySelector('a') as HTMLElement).click()
  await settle()
  expect(root.querySelectorAll('tr.md-target--selected')).toHaveLength(1)
  expect(root.querySelector('tr.md-target--selected')?.textContent).toContain('Норма')
})
test('automatic result does not scroll; missing row keeps text and gives a useful label', async () => {
  evaluate.mockResolvedValueOnce(envelope('gone'))
  const { root, scrolls } = setup(source.replace('script=1{choice}', 'script=1{choice}?auto'))
  await settle()
  expect(scrolls()).toBe(0)
  expect(root.textContent).toContain('gone')
  expect(root.textContent).toContain('Строка недоступна')
})
test('ordinary text stays compatible and an async result after destroy is discarded', async () => {
  evaluate.mockResolvedValueOnce('12')
  const { root, action } = setup()
  ;(root.querySelector('a') as HTMLElement).click()
  await settle()
  expect(root.querySelector('a')?.textContent).toBe('choice = 12')
  let done: (s: string) => void = () => {}
  evaluate.mockImplementationOnce(
    () =>
      new Promise<string>((r) => {
        done = r
      })
  )
  ;(root.querySelector('a') as HTMLElement).click()
  action.destroy()
  done(envelope('fail'))
  await settle()
  expect(root.querySelector('.md-target--selected')).toBeNull()
})

test('changing host clears results even when the same HTML nodes are reused', async () => {
  evaluate.mockResolvedValueOnce(envelope('fail'))
  const { root, action } = setup()
  root.dataset.valueHost = 'note:1'
  action.update({})
  ;(root.querySelector('a') as HTMLElement).click()
  await settle()
  expect(root.querySelector('.md-target--selected')).not.toBeNull()
  root.dataset.valueHost = 'note:2'
  await settle()
  expect(root.querySelector('.md-target--selected')).toBeNull()
  expect(root.querySelector('[data-target-link]')).toBeNull()
  expect(root.querySelector('a')?.textContent).toBe('choice')
})

test('encounter and diagnostics tables keep independent results in the same document', async () => {
  evaluate
    .mockResolvedValueOnce(
      JSON.stringify({ kind: 'markdown-target', version: 1, text: 'Волки', target: { table: 'night', row: 'wolves' } })
    )
    .mockResolvedValueOnce(envelope('fail'))
  const { root } = setup()
  root.insertAdjacentHTML(
    'afterbegin',
    '<p><a href="?script=2&amp;_=night">Ночь</a></p><table data-md-table="night"><tbody><tr data-md-row="wolves"><td>Волки у лагеря</td></tr></tbody></table>'
  )
  await settle()
  ;(root.querySelector('a[href^="?script=2"]') as HTMLElement).click()
  await settle()
  ;(root.querySelector('a[href^="?script=1"]') as HTMLElement).click()
  await settle()
  expect(root.querySelectorAll('.md-target--selected')).toHaveLength(2)
  expect(root.querySelector('table[data-md-table="night"] .md-target--selected')?.textContent).toContain('Волки')
  expect(root.querySelector('table[data-md-table="diagnostic"] .md-target--selected')?.textContent).toContain('Сбой')
})
