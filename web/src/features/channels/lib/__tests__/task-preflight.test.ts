/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { createInstance } from 'i18next'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import type { TaskChannelDiagnostic } from '../../types'
import { handleTestChannel } from '../channel-actions'
import {
  createChannelTestCSV,
  isFailedChannelTest,
} from '../channel-test-export'

vi.mock('sonner', () => ({
  toast: { info: vi.fn(), success: vi.fn(), error: vi.fn() },
}))
afterEach(() => vi.restoreAllMocks())
const i18n = createInstance()
await i18n.init({ lng: 'en', resources: { en: { translation: {} } } })
for (const outcome of ['partial', 'untested'] as const) {
  test(`${outcome} is neutral, skips GET and preserves export evidence and failed selection`, async () => {
    const diagnostic: TaskChannelDiagnostic = {
      kind: 'task_plugin',
      mode: 'preflight',
      status: 'preflight_partial',
      outcome,
      generation: 1,
      diagnostic_duration_ms: 0,
      plugin: 'video-plugin',
      model: 'video',
      mapped_model: 'video',
      connectivity_tested: false,
      live_generation_tested: false,
      checks: [
        {
          check: 'pricing',
          status: 'pass',
          billing_source: 'plugin_expression',
        },
      ],
    }
    vi.spyOn(api, 'post').mockResolvedValue({
      data: { success: true, data: diagnostic },
    })
    const get = vi
      .spyOn(api, 'get')
      .mockRejectedValue(new Error('unexpected legacy test'))
    const complete = vi.fn()
    await handleTestChannel(99, { testModel: 'video', silent: true }, complete)
    expect(get).not.toHaveBeenCalled()
    expect(complete).toHaveBeenCalledWith(
      undefined,
      undefined,
      undefined,
      undefined,
      diagnostic
    )
    const result = { status: outcome, diagnostic } as const
    expect(isFailedChannelTest(result)).toBe(false)
    const csv = createChannelTestCSV(
      {
        channelId: 99,
        channelName: 'vendor',
        models: ['video'],
        results: { video: result },
      },
      i18n.t
    )
    expect(csv).toContain('"preflight"')
    expect(csv).toContain(`"${outcome}"`)
    expect(csv).toContain('"false","false"')
    expect(csv).toContain('plugin_expression')
  })
}
test('concrete pricing failure belongs to failed set even when API success is true', async () => {
  const diagnostic: TaskChannelDiagnostic = {
    kind: 'task_plugin',
    mode: 'preflight',
    generation: 1,
    diagnostic_duration_ms: 0,
    model: 'video',
    mapped_model: 'video',
    connectivity_tested: false,
    live_generation_tested: false,
    status: 'pricing_not_ready',
    outcome: 'fail',
    checks: [
      { check: 'pricing', status: 'fail', reason: 'PLUGIN_PRICE_NOT_FOUND' },
    ],
  }
  vi.spyOn(api, 'post').mockResolvedValue({
    data: { success: true, data: diagnostic },
  })
  const complete = vi.fn()
  await handleTestChannel(99, { silent: true }, complete)
  expect(complete.mock.calls[0][0]).toBe(false)
  expect(isFailedChannelTest({ status: 'error', diagnostic })).toBe(true)
})
test('ordinary classification continues the unchanged synchronous GET contract', async () => {
  vi.spyOn(api, 'post').mockResolvedValue({
    data: {
      success: true,
      data: {
        kind: 'ordinary',
        outcome: 'untested',
        status: 'ordinary_channel_test_required',
      },
    },
  })
  const get = vi
    .spyOn(api, 'get')
    .mockResolvedValue({ data: { success: true, time: 0.2 } })
  const complete = vi.fn()
  await handleTestChannel(99, { testModel: 'chat', silent: true }, complete)
  expect(get).toHaveBeenCalledWith(
    '/api/channel/test/99',
    expect.objectContaining({ params: { model: 'chat' } })
  )
  expect(complete).toHaveBeenCalledWith(true, 200)
})

test('unavailable diagnostic is untested and cannot trigger failed-model actions or legacy test', async () => {
  vi.spyOn(api, 'post').mockRejectedValue(new Error('diagnostic unavailable'))
  const get = vi
    .spyOn(api, 'get')
    .mockRejectedValue(new Error('unexpected legacy test'))
  const complete = vi.fn()
  await handleTestChannel(99, { testModel: 'video', silent: true }, complete)
  const [legacySuccess, responseTime, , , diagnostic] = complete.mock.calls[0]
  expect(legacySuccess).toBeUndefined()
  expect(responseTime).toBeUndefined()
  expect(diagnostic.outcome).toBe('untested')
  expect(isFailedChannelTest({ status: 'untested', diagnostic })).toBe(false)
  expect(get).not.toHaveBeenCalled()
})

test('ordinary pricing diagnostics do not override the legacy tester user pricing policy', async () => {
  vi.spyOn(api, 'post').mockResolvedValue({
    data: {
      success: true,
      data: {
        kind: 'ordinary',
        outcome: 'fail',
        status: 'pricing_not_ready',
        checks: [],
      },
    },
  })
  const get = vi
    .spyOn(api, 'get')
    .mockResolvedValue({ data: { success: true, time: 0.2 } })
  const complete = vi.fn()
  await handleTestChannel(99, { testModel: 'ordinary', silent: true }, complete)
  expect(get).toHaveBeenCalled()
  expect(complete).toHaveBeenCalledWith(true, 200)
})

test('connectivity CSV preserves catalog evidence and neutral model access or rate limits', () => {
  const diagnostic: TaskChannelDiagnostic = {
    kind: 'task_plugin',
    mode: 'connectivity',
    status: 'model_access_not_confirmed',
    outcome: 'partial',
    model: 'alias',
    mapped_model: 'mapped',
    generation: 1,
    diagnostic_duration_ms: 0,
    connectivity_tested: true,
    live_generation_tested: false,
    connectivity_status: 'connectivity_pass',
    connectivity_latency_ms: 12,
    credential_identity: 'key_index:2',
    model_access: 'not_confirmed',
    catalog_model: 'mapped',
    checks: [{ check: 'connectivity', status: 'pass', evidence: 'catalog' }],
  }
  expect(isFailedChannelTest({ status: 'partial', diagnostic })).toBe(false)
  const csv = createChannelTestCSV(
    {
      channelId: 99,
      channelName: 'vendor',
      models: ['alias'],
      results: { alias: { status: 'partial', diagnostic } },
    },
    i18n.t
  )
  expect(csv).toContain('"connectivity"')
  expect(csv).toContain('"true","false"')
  expect(csv).toContain(
    '"connectivity_pass","12","key_index:2","not_confirmed","mapped","catalog"'
  )
  diagnostic.connectivity_status = 'upstream_rate_limited'
  expect(isFailedChannelTest({ status: 'partial', diagnostic })).toBe(false)
  diagnostic.connectivity_status = 'connectivity_unavailable'
  diagnostic.connectivity_tested = false
  expect(isFailedChannelTest({ status: 'untested', diagnostic })).toBe(false)
})

test('CSV preserves exact diagnostic reason and canonical endpoint context', () => {
  const diagnostic: TaskChannelDiagnostic = {
    kind: 'task_plugin',
    mode: 'preflight',
    status: 'pricing_not_ready',
    outcome: 'fail',
    model: 'alias',
    mapped_model: 'canonical',
    endpoint: '/v1/images/generations',
    protocol: 'openai_image',
    generation: 1,
    diagnostic_duration_ms: 0,
    connectivity_tested: false,
    live_generation_tested: false,
    checks: [
      {
        check: 'provider_contract',
        status: 'fail',
        reason_code: 'MISSING_AUTHORITATIVE_OUTPUT_DIMENSIONS',
        evidence: 'catalog hash exact',
        message: 'Dimensions missing',
      },
    ],
  }
  const csv = createChannelTestCSV(
    {
      channelId: 1,
      channelName: 'DFLOP',
      models: ['alias'],
      results: { alias: { status: 'error', diagnostic } },
    },
    i18n.t
  )
  expect(csv).toContain('"endpoint","protocol","canonical_model"')
  expect(csv).toContain('MISSING_AUTHORITATIVE_OUTPUT_DIMENSIONS')
  expect(csv).toContain('catalog hash exact')
  expect(csv).toContain('/v1/images/generations')
})

test('ambiguous runtime timeout preserves evidence and never retries the paid tester', async () => {
  vi.spyOn(api, 'post').mockResolvedValue({
    data: {
      success: true,
      data: {
        kind: 'ordinary',
        outcome: 'untested',
        status: 'ordinary_channel_test_required',
      },
    },
  })
  const diagnostic: TaskChannelDiagnostic = {
    kind: 'ordinary',
    mode: 'runtime',
    model: 'grok',
    mapped_model: 'grok',
    generation: 1,
    diagnostic_duration_ms: 0,
    status: 'runtime_failure_unclassified',
    outcome: 'partial',
    connectivity_tested: false,
    live_generation_tested: false,
    checks: [
      {
        check: 'runtime_evidence',
        status: 'not_tested',
        reason_code: 'INSUFFICIENT_EVIDENCE',
        message: 'HTTP 524',
      },
    ],
  }
  const get = vi
    .spyOn(api, 'get')
    .mockResolvedValue({
      data: { success: false, message: 'HTTP 524', diagnostic },
    })
  const complete = vi.fn()
  await handleTestChannel(1, { testModel: 'grok', silent: true }, complete)
  expect(get).toHaveBeenCalledTimes(1)
  expect(complete).toHaveBeenCalledWith(
    undefined,
    undefined,
    'HTTP 524',
    undefined,
    diagnostic
  )
  expect(isFailedChannelTest({ status: 'partial', diagnostic })).toBe(false)
})
