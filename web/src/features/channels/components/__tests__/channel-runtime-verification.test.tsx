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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import type { ChannelVerificationData } from '../../types'
import { ChannelRuntimeVerification } from '../dialogs/channel-runtime-verification'

const i18n = createInstance()
await i18n.init({ lng: 'en', resources: { en: { translation: {} } } })
afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})
const evidence: ChannelVerificationData = {
  run: {
    id: 7,
    channel_id: 1,
    catalog_hash: 'catalog-sha',
    started_at: 1790827200,
    status: 'PARTIAL',
    source: 'DFLOP',
  },
  items: [
    {
      id: 11,
      model: 'image-model',
      protocol: 'openai_image',
      operation: 'generate',
      mode: 'text',
      fixture_id: 'fixture-image',
      endpoint: '/v1/images/generations',
      config_status: 'PASS',
      connectivity_status: 'NOT_TESTED',
      request_status: 'NOT_TESTED',
      generation_status: 'NOT_TESTED',
      parser_status: 'NOT_TESTED',
      billing_status: 'BLOCKED',
      ledger_status: 'NOT_TESTED',
      status: 'PARTIAL',
      reason_code: 'CONNECTIVITY_NOT_TESTED',
      billing_source: 'frozen-catalog',
      normalized_usage_json: '{"image_count":1}',
      pricing_snapshot_hash: 'pricing-sha',
      billing_expr_hash: 'expr-sha',
      evidence_json: '{"api_key":"secret-value"}',
      correlation_quality: 'NONE',
    },
  ],
}
function setup(open = true) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const content = (nextOpen: boolean) => (
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={i18n}>
        <ChannelRuntimeVerification
          channelId={1}
          channelName='DFLOP'
          open={nextOpen}
          models={['image-model']}
          results={{}}
        />
      </I18nextProvider>
    </QueryClientProvider>
  )
  const view = render(content(open))
  return {
    ...view,
    setOpen: (nextOpen: boolean) => view.rerender(content(nextOpen)),
  }
}

test('opening and reopening hydrates persisted evidence and replaces outdated layer states', async () => {
  const get = vi
    .spyOn(api, 'get')
    .mockResolvedValue({ data: { success: true, data: evidence } })
  const view = setup(false)
  expect(get).not.toHaveBeenCalled()
  view.setOpen(true)
  const row = await screen.findByRole('row', { name: /image-model/ })
  expect(within(row).getByText('CONNECTIVITY_NOT_TESTED')).toBeVisible()
  expect(within(row).getByText('Config: PASS')).toBeVisible()
  expect(within(row).getByText('Connectivity: NOT_TESTED')).toBeVisible()
  expect(within(row).getByText('Runtime: NOT_TESTED')).toBeVisible()
  expect(within(row).getByText('Billing: BLOCKED')).toBeVisible()
  expect(within(row).getByText('Ledger: NOT_TESTED')).toBeVisible()
  view.setOpen(false)
  get.mockResolvedValue({
    data: {
      success: true,
      data: {
        ...evidence,
        items: [
          {
            ...evidence.items[0],
            connectivity_status: 'PASS',
            reason_code: 'READY_FOR_PAID_AUTHORIZATION',
          },
        ],
      },
    },
  })
  view.setOpen(true)
  await screen.findByText('READY_FOR_PAID_AUTHORIZATION')
  expect(screen.queryByText('CONNECTIVITY_NOT_TESTED')).not.toBeInTheDocument()
})

test('view evidence reloads the exact persisted run and displays hashes and sanitized usage', async () => {
  const get = vi
    .spyOn(api, 'get')
    .mockResolvedValue({ data: { success: true, data: evidence } })
  setup()
  fireEvent.click(
    await screen.findByRole('button', {
      name: 'View evidence for image-model',
    })
  )
  const dialog = await screen.findByRole('dialog', {
    name: 'Verification evidence: image-model',
  })
  await waitFor(() =>
    expect(get).toHaveBeenCalledWith(
      '/api/channel/1/runtime-verification/7',
      expect.any(Object)
    )
  )
  expect(within(dialog).getByText(/pricing-sha/)).toBeVisible()
  expect(within(dialog).getByText(/image_count/)).toBeVisible()
  expect(within(dialog).queryByText(/secret-value/)).not.toBeInTheDocument()
})

test('rerun sends only the zero-cost prepare request and renders the saved result', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: evidence },
  })
  const post = vi
    .spyOn(api, 'post')
    .mockResolvedValue({ data: { success: true, data: evidence } })
  setup()
  fireEvent.click(
    await screen.findByRole('button', {
      name: 'Re-run verification for image-model',
    })
  )
  await waitFor(() => expect(post).toHaveBeenCalledOnce())
  expect(post).toHaveBeenCalledWith(
    '/api/channel/1/runtime-verification',
    { model: 'image-model' },
    expect.any(Object)
  )
  expect(
    screen.getByText(
      'Re-run checks configuration and free connectivity only. Paid generation requires an approved authorization manifest.'
    )
  ).toBeVisible()
})

test('failed hydration shows retry instead of cached verification and empty runs remain explicitly untested', async () => {
  const get = vi.spyOn(api, 'get').mockRejectedValue(new Error('unavailable'))
  setup()
  expect(
    await screen.findByText('Failed to load verification evidence')
  ).toBeVisible()
  get.mockResolvedValue({
    data: { success: true, data: { run: null, items: [] } },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
  expect(
    await screen.findByText('No persisted verification evidence')
  ).toBeVisible()
  expect(
    screen.queryByRole('button', { name: 'View evidence for image-model' })
  ).not.toBeInTheDocument()
})

test('previous paid evidence stays historical and opens its original run without promoting current layers', async () => {
  const historical = {
    ...evidence.items[0],
    id: 9,
    run_id: 6,
    catalog_hash: 'previous-catalog-sha',
    request_status: 'PASS' as const,
    generation_status: 'PASS' as const,
    parser_status: 'PASS' as const,
    billing_status: 'PASS' as const,
    ledger_status: 'PASS' as const,
    status: 'RUNTIME_VERIFIED',
    verified_at: 1790827200,
  }
  const get = vi.spyOn(api, 'get').mockImplementation(async (url) => ({
    data: {
      success: true,
      data: url.endsWith('/6')
        ? { run: { ...evidence.run, id: 6 }, items: [historical] }
        : {
            ...evidence,
            items: [{ ...evidence.items[0], historical_evidence: historical }],
          },
    },
  }))
  setup()
  const row = await screen.findByRole('row', { name: /image-model/ })
  expect(within(row).getByText('Runtime: NOT_TESTED')).toBeVisible()
  expect(within(row).getByText('Billing: BLOCKED')).toBeVisible()
  expect(
    within(row).getByText('Historical verification: RUNTIME_VERIFIED')
  ).toBeVisible()
  fireEvent.click(
    within(row).getByRole('button', {
      name: 'View previous evidence for image-model',
    })
  )
  const dialog = await screen.findByRole('dialog')
  await waitFor(() =>
    expect(get).toHaveBeenCalledWith(
      '/api/channel/1/runtime-verification/6',
      expect.any(Object)
    )
  )
  expect(within(dialog).getByText(/previous-catalog-sha/)).toBeVisible()
})
