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
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, expect, test, vi } from 'vitest'

import { createChannelTestCSV } from '../../lib/channel-test-export'
import { ChannelTestExport } from '../dialogs/channel-test-export'

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  fallbackLng: 'en',
  resources: { en: { translation: {} } },
})

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

test('exports every model with complete errors and distinguishes unknown cost from zero', () => {
  const models = Array.from({ length: 215 }, (_, index) => `model-${index}`)
  const csv = createChannelTestCSV(
    {
      channelId: 1,
      channelName: 'DFLOP',
      models,
      results: {
        'model-0': {
          status: 'success',
          responseTime: 0,
          completedAt: 1790827200000,
          endpointType: 'anthropic',
          stream: false,
        },
        'model-214': {
          status: 'error',
          errorCode: 'upstream_error',
          error: 'Bad response, body: "failure"\nFull detail',
        },
      },
    },
    i18n.t
  )
  expect(csv.startsWith('\uFEFF')).toBe(true)
  expect(csv).toContain('"model-214","Failed"')
  expect(csv).toContain('"Bad response, body: ""failure""\nFull detail"')
  expect(csv).toContain('"model-1","Not tested"')
  expect(csv).toContain('"0"')
  expect(csv).not.toContain('"Unknown"')
  expect(csv).toContain('2026-10-01T04:00:00.000Z')
  expect(csv).toContain('"anthropic","false"')
})

test('exports model names and upstream errors as text rather than spreadsheet formulas', () => {
  const csv = createChannelTestCSV(
    {
      channelId: 1,
      channelName: '=malicious',
      models: ['@model'],
      results: {
        '@model': {
          status: 'error',
          error: '  =HYPERLINK("https://example.invalid")',
        },
      },
    },
    i18n.t
  )
  expect(csv).toContain('"\'@model"')
  expect(csv).toContain('"\'=malicious"')
  expect(csv).toContain('"\'  =HYPERLINK')
})

test('downloads CSV without a network request and disables export during testing', () => {
  const createObjectURL = vi.fn((_blob: Blob) => 'blob:test-export')
  const revokeObjectURL = vi.fn()
  vi.stubGlobal(
    'URL',
    Object.assign(class extends URL {}, { createObjectURL, revokeObjectURL })
  )
  const click = vi
    .spyOn(HTMLAnchorElement.prototype, 'click')
    .mockImplementation(() => {})
  const props = {
    channelId: 1,
    channelName: 'DFLOP',
    models: ['model-a'],
    results: {},
    disabled: false,
  }
  const view = render(
    <I18nextProvider i18n={i18n}>
      <ChannelTestExport {...props} />
    </I18nextProvider>
  )
  fireEvent.click(screen.getByRole('button', { name: 'Export CSV' }))
  expect(click).toHaveBeenCalledOnce()
  expect(createObjectURL.mock.calls[0][0]).toBeInstanceOf(Blob)
  expect(revokeObjectURL).toHaveBeenCalledWith('blob:test-export')
  view.rerender(
    <I18nextProvider i18n={i18n}>
      <ChannelTestExport {...props} disabled />
    </I18nextProvider>
  )
  expect(screen.getByRole('button', { name: 'Export CSV' })).toBeDisabled()
})

test('exports persisted verification layers, exact evidence, and blank unknown costs without secrets', () => {
  const csv = createChannelTestCSV(
    {
      channelId: 1,
      channelName: 'DFLOP',
      models: ['model-a', 'model-b'],
      results: {},
      verification: {
        run: {
          id: 7,
          channel_id: 1,
          catalog_hash: 'catalog-hash',
          started_at: 1790827200,
          status: 'PARTIAL',
          source: 'DFLOP',
        },
        items: [
          {
            model: 'model-a',
            protocol: 'openai_image',
            operation: 'generate',
            mode: 'text',
            fixture_id: 'image-v1',
            endpoint: '/v1/images/generations',
            config_status: 'PASS',
            connectivity_status: 'PASS',
            request_status: 'NOT_TESTED',
            generation_status: 'NOT_TESTED',
            parser_status: 'NOT_TESTED',
            billing_status: 'BLOCKED',
            ledger_status: 'NOT_TESTED',
            status: 'PARTIAL',
            reason_code: 'READY_FOR_PAID_AUTHORIZATION',
            pricing_snapshot_hash: 'pricing-hash',
            billing_expr_hash: 'expr-hash',
            idempotency_key_hash: 'idempotency-hash',
            request_id: 'req-1',
            task_id: 'task-1',
            trace_id: 'trace-1',
            terminal_status: 'succeeded',
            normalized_usage_json: '{"image_count":1,"api_key":"usage-secret"}',
            provider_usage_json:
              '{"unit_count":1,"Authorization":"Bearer bearer-secret"}',
            provider_unit_count: '1',
            provider_cost_points: '0.100000000000000001',
            newapi_raw_cost: '0.00001',
            newapi_quota: 5,
            wallet_delta: -5,
            correlation_quality: 'EXACT_TRACE_ID',
            verified_at: 1790827200,
            evidence_json:
              '{"headers":{"cookie":"session-secret"},"result":"https://example.invalid/output?token=url-secret","secret":"evidence-secret"}',
            historical_evidence: {
              run_id: 6,
              model: 'model-a',
              protocol: 'openai_image',
              operation: 'generate',
              mode: 'text',
              fixture_id: 'image-v1',
              endpoint: '/v1/images/generations',
              catalog_hash: 'historical-catalog',
              config_status: 'PASS',
              connectivity_status: 'PASS',
              request_status: 'PASS',
              generation_status: 'PASS',
              parser_status: 'PASS',
              billing_status: 'PASS',
              ledger_status: 'PASS',
              status: 'RUNTIME_VERIFIED',
            },
          },
        ],
      },
    },
    i18n.t
  )
  for (const column of [
    'evidence_scope',
    'config_state',
    'connectivity_state',
    'runtime_state',
    'billing_state',
    'ledger_state',
    'catalog_hash',
    'pricing_snapshot_hash',
    'billing_expr_hash',
    'request_id',
    'task_id',
    'trace_id',
    'terminal_status',
    'normalized_usage',
    'provider_unit_count',
    'provider_cost_points',
    'newapi_quota',
    'newapi_cost',
    'correlation_quality',
    'verified_at',
  ]) {
    expect(csv).toContain(`"${column}"`)
  }
  expect(csv).toContain('"0.100000000000000001"')
  expect(csv).toContain('"READY_FOR_PAID_AUTHORIZATION"')
  expect(csv).toContain('"catalog-hash"')
  expect(csv).toContain('"EXACT_TRACE_ID"')
  const currentRow = csv.split('\r\n').find((row) => row.includes('"current"'))
  const historicalRow = csv
    .split('\r\n')
    .find((row) => row.includes('"historical"'))
  expect(currentRow).toContain('"NOT_TESTED"')
  expect(currentRow).not.toContain('"RUNTIME_VERIFIED"')
  expect(historicalRow).toContain('"RUNTIME_VERIFIED"')
  expect(historicalRow).toContain('"historical-catalog"')
  expect(historicalRow).toContain('"6"')
  expect(csv).not.toMatch(
    /usage-secret|bearer-secret|session-secret|url-secret|evidence-secret/
  )
  const unknownRow = csv.split('\r\n').find((row) => row.includes('"model-b"'))
  expect(unknownRow).toBeDefined()
  expect(unknownRow).not.toContain('"0"')
})
