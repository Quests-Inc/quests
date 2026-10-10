import { test } from 'bun:test'
import assert from 'node:assert/strict'

import { buildQuestPdfDocument } from '../src/lib/js/questPdfDocument.js'

test('a quest becomes meta, steps, attachments and dates blocks', () => {
  const doc = buildQuestPdfDocument(
    {
      id: 7,
      title: 'Выкатить',
      status: 'active',
      description: 'описание note=3',
      steps: [
        { title: 'Один', progress_current: 1, progress_total: 1, description: '- пункт' },
        { title: 'Два', progress_current: 0, progress_total: 2 },
      ],
      created_at: '2026-10-10T10:00:00Z',
    },
    {
      labels: { 'note:3': 'Релизы' },
      tzLabel: 'UTC+3',
      attachments: [{ filename: 'a.png' }, { id: 9 }],
    },
    {
      significanceLabel: () => 'эпическое',
      periodBadge: () => null,
      quantifiedProgress: () => '1/2',
    }
  )
  assert.equal(doc.title, 'Выкатить')
  assert.equal(doc.labels['note:3'], 'Релизы')
  assert.equal(doc.meta[0].text, 'active')
  assert.match(doc.meta[0].color, /^#[0-9a-f]{6}$/i)
  assert.deepEqual(
    doc.meta.slice(1).map((c) => c.text),
    ['1/2']
  )
  const steps = doc.blocks.find((b) => b.type === 'steps')
  assert.deepEqual(
    steps.steps.map((s) => [s.title, s.progress, s.done]),
    [
      ['Один', '1/1', true],
      ['Два', '0/2', false],
    ]
  )
  assert.equal(steps.steps[0].markdown, '- пункт')
  assert.deepEqual(doc.blocks.find((b) => b.type === 'list').items, ['a.png', '#9'])
  const facts = doc.blocks.find((b) => b.type === 'facts').facts
  assert.equal(facts.length, 1)
  assert.match(facts[0].label, /Создан \(UTC\+3\)/)
})
