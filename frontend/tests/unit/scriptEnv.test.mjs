import { expect, test } from 'bun:test'

import { EnvName } from '../../src/pages/scripts/model/env.ts'

test('имя переменной для значения, пришедшего по связи в код: псевдоним или номер ноды и порт', () => {
  const from = { node: 2, port: 'title' }
  expect(EnvName({ from })).toBe('QUESTS_IN_N2_TITLE')
  expect(EnvName({ from, alias: 'NAME' })).toBe('QUESTS_IN_NAME')
  // одно и то же имя порта из разных нод не пересекается
  expect(EnvName({ from: { node: 3, port: 'title' } })).not.toBe(EnvName({ from }))
})
