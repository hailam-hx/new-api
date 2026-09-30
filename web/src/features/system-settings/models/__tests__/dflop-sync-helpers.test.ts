import { describe, expect, it } from 'vitest'

import { canApplyPreview, selectableModels } from '../dflop-sync-helpers'

describe('DFLOP pricing selection', () => {
  it('excludes blocked and unsupported models from ordinary apply', () => {
    expect(selectableModels([
      { model_id: 'safe', status: 'SUPPORTED_AUTO', action: 'UPDATE' },
      { model_id: 'drift', status: 'MANUAL_DRIFT', action: 'UPDATE' },
      { model_id: 'video', status: 'UNSUPPORTED_MAPPING', action: 'SKIP' },
    ])).toEqual(['safe'])
  })

  it('disables apply when the preview has expired', () => {
    expect(canApplyPreview(100, 700, ['safe'])).toBe(false)
    expect(canApplyPreview(100, 699, ['safe'])).toBe(true)
    expect(canApplyPreview(100, 101, [])).toBe(false)
  })
})
