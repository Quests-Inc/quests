import { expect, test } from 'bun:test'

import { searchAfterWorkspaceSwitch } from '../../src/shared/lib/urlSearch.ts'

test('workspace switch retains only the section, never entity ids or entity-based filters', () => {
  expect(searchAfterWorkspaceSwitch('?note=78')).toEqual({ tab: 'notes' })
  expect(searchAfterWorkspaceSwitch('?attachment=12')).toEqual({ tab: 'attachments' })
  expect(searchAfterWorkspaceSwitch('?quest=507')).toEqual({})
  expect(searchAfterWorkspaceSwitch('?tab=graph&gt=note&gid=78&layout=3')).toEqual({ tab: 'graph' })
  expect(searchAfterWorkspaceSwitch('?tab=scripts&script=7&q=note%3D78')).toEqual({ tab: 'scripts' })
  expect(searchAfterWorkspaceSwitch('?tab=journal&quest=1')).toEqual({})
  expect(searchAfterWorkspaceSwitch('?tab=invalid&note=78')).toEqual({ tab: 'notes' })
  expect(searchAfterWorkspaceSwitch('?tab=invalid')).toEqual({})
})
