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
import { ChannelRuntimeAction } from '../dialogs/channel-runtime-action'
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
  const content = (nextOpen: boolean, filteredModels?: string[]) => (
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={i18n}>
        <ChannelRuntimeVerification
          channelId={1}
          channelName='DFLOP'
          open={nextOpen}
          models={['image-model']}
          filteredModels={filteredModels}
          results={{}}
        />
      </I18nextProvider>
    </QueryClientProvider>
  )
  const view = render(content(open))
  return {
    ...view,
    setOpen: (nextOpen: boolean) => view.rerender(content(nextOpen)),
    setFilteredModels: (models: string[]) =>
      view.rerender(content(open, models)),
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

test('preparation blocker leaves runtime untested and shows request and usage independently', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        ...evidence,
        items: [
          {
            ...evidence.items[0],
            request_status: 'BLOCKED',
            generation_status: 'NOT_TESTED',
            reason_code: 'OPERATOR_DEPLOY_REQUIRED',
          },
        ],
      },
    },
  })
  setup()
  const row = await screen.findByRole('row', { name: /image-model/ })
  expect(within(row).getByText('Request: BLOCKED')).toBeVisible()
  expect(within(row).getByText('Usage: NOT_TESTED')).toBeVisible()
  expect(within(row).getByText('Runtime: NOT_TESTED')).toBeVisible()
})

test('unrecoverable history does not block a fresh untested canary', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        ...evidence,
        items: [
          {
            ...evidence.items[0],
            request_status: 'PASS',
            generation_status: 'NOT_TESTED',
            billing_status: 'NOT_TESTED',
            status: 'READY_FOR_FRESH_CANARY',
            reason_code: 'LIVE_CANARY_REQUIRED',
            historical_runtime_state: 'HISTORICAL_RUNTIME_UNRECOVERABLE',
            historical_reason_code: 'EXACT_PROVIDER_ID_NOT_CAPTURED',
            historical_run_id: 6,
            current_canary_readiness: 'READY_FOR_FRESH_CANARY',
          },
        ],
      },
    },
  })
  setup()
  const row = await screen.findByRole('row', { name: /image-model/ })
  expect(within(row).getByText('Request: PASS')).toBeVisible()
  expect(within(row).getByText('Runtime: NOT_TESTED')).toBeVisible()
  expect(within(row).getByText('Fresh verification required')).toBeVisible()
  expect(
    within(row).getByText(
      /Historical result unrecoverable: EXACT_PROVIDER_ID_NOT_CAPTURED/
    )
  ).toBeVisible()
})

test('evidence table, visibility counts and export follow the shared model filter', async () => {
  const full = {
    ...evidence.items[0],
    config_status: 'PASS',
    connectivity_status: 'PASS',
    request_status: 'PASS',
    generation_status: 'PASS',
    parser_status: 'PASS',
    billing_status: 'PASS',
    ledger_status: 'PASS',
  } as const
  const get = vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: { ...evidence, items: [full] } },
  })
  const view = setup()
  expect(await screen.findByRole('row', { name: /image-model/ })).toBeVisible()
  expect(
    screen.getByRole('button', { name: 'Show models passing three layers (1)' })
  ).toBeEnabled()
  view.setFilteredModels([])
  expect(
    screen.queryByRole('row', { name: /image-model/ })
  ).not.toBeInTheDocument()
  expect(
    screen.getByRole('button', { name: 'Show models passing three layers (0)' })
  ).toBeDisabled()
  expect(
    screen.getByRole('button', { name: 'Show successful models (0)' })
  ).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Export CSV' })).toBeDisabled()
  view.setFilteredModels(['image-model'])
  expect(screen.getByRole('row', { name: /image-model/ })).toBeVisible()
  expect(screen.getByRole('button', { name: 'Export CSV' })).toBeEnabled()
  expect(get).toHaveBeenCalledTimes(1)
})

test('shows only current models whose every verification target passes all seven layers', async () => {
  const full = {
    ...evidence.items[0],
    model: 'image-model',
    config_status: 'PASS',
    connectivity_status: 'PASS',
    request_status: 'PASS',
    generation_status: 'PASS',
    parser_status: 'PASS',
    billing_status: 'PASS',
    ledger_status: 'PASS',
  } as const
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: { ...evidence, items: [full] } },
  })
  setup()
  expect(
    await screen.findByRole('button', { name: 'Show successful models (1)' })
  ).toBeEnabled()
  expect(
    screen.getByRole('button', { name: 'Runtime verification' })
  ).toBeEnabled()
})

test('keeps show PASS action disabled for mixed modes or historical-only success', async () => {
  const full = {
    ...evidence.items[0],
    config_status: 'PASS',
    connectivity_status: 'PASS',
    request_status: 'PASS',
    generation_status: 'PASS',
    parser_status: 'PASS',
    billing_status: 'PASS',
    ledger_status: 'PASS',
  } as const
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        ...evidence,
        items: [
          full,
          {
            ...full,
            mode: 'image',
            ledger_status: 'NOT_TESTED',
            historical_evidence: full,
          },
        ],
      },
    },
  })
  setup()
  expect(
    await screen.findByRole('button', { name: 'Show successful models (0)' })
  ).toBeDisabled()
})

test.each(['PASS', 'BLOCKED', 'NOT_TESTED'] as const)(
  'three-layer visibility requires current request PASS (%s)',
  async (requestStatus) => {
    const partial = {
      ...evidence.items[0],
      config_status: 'PASS',
      connectivity_status: 'PASS',
      request_status: 'PASS',
    } as const
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: {
          ...evidence,
          items: [
            partial,
            {
              ...partial,
              id: 12,
              mode: 'image',
              request_status: requestStatus,
              historical_evidence: partial,
            },
          ],
        },
      },
    })
    setup()
    const count = requestStatus === 'PASS' ? 1 : 0
    const button = await screen.findByRole('button', {
      name: `Show models passing three layers (${count})`,
    })
    expect(
      screen.getByRole('button', { name: 'Show successful models (0)' })
    ).toBeDisabled()
    if (requestStatus === 'PASS') {
      expect(button).toBeEnabled()
      fireEvent.click(button)
      const dialog = screen.getByRole('alertdialog')
      expect(
        within(dialog).getByText(
          /Showing models does not change verification states/
        )
      ).toBeVisible()
      expect(within(dialog).getByText('image-model')).toBeVisible()
    } else {
      expect(button).toBeDisabled()
    }
  }
)

test('paid runtime execution requires a separate budget confirmation and is never automatically retried', async () => {
  const plan = {
    plan_version: 'canary-plan-v6',
    run_id: 7,
    source_channel_id: 1,
    catalog_hash: 'catalog-sha',
    known_maximum_provider_points: '0.25',
    planned_posts: 1,
    proposed_authorization: { funding_user_id: 9, targets: [] },
    targets: [
      {
        model: 'image-model',
        fixture: { id: 'image', protocol: 'openai_image', mode: 'text' },
        maximum_provider_points: '0.25',
      },
    ],
  }
  const post = vi
    .spyOn(api, 'post')
    .mockResolvedValueOnce({ data: { success: true, data: plan } })
    .mockRejectedValueOnce(new Error('ambiguous timeout'))
  render(
    <QueryClientProvider client={new QueryClient()}>
      <I18nextProvider i18n={i18n}>
        <ChannelRuntimeAction
          channelId={1}
          models={['image-model']}
          disabled={false}
          onComplete={vi.fn()}
          onBusyChange={vi.fn()}
        />
      </I18nextProvider>
    </QueryClientProvider>
  )
  fireEvent.click(screen.getByRole('button', { name: 'Runtime verification' }))
  fireEvent.change(
    screen.getByRole('textbox', { name: 'Verification plan JSON' }),
    { target: { value: JSON.stringify(plan) } }
  )
  expect(
    screen.getByRole('button', { name: 'Review paid execution' })
  ).toBeDisabled()
  const manifest = {
    approved: true,
    signature: 'operator-signed',
    expires_at: Math.floor(Date.now() / 1000) + 300,
    max_requests: 1,
    max_total_provider_points: '0.25',
    targets: [
      {
        model: 'image-model',
        protocol: 'openai_image',
        mode: 'text',
        fixture_id: 'image',
        maximum_provider_points: '0.25',
      },
    ],
  }
  fireEvent.change(
    screen.getByRole('textbox', { name: 'Signed authorization JSON' }),
    { target: { value: JSON.stringify(manifest) } }
  )
  fireEvent.click(screen.getByRole('button', { name: 'Review paid execution' }))
  expect(post).not.toHaveBeenCalled()
  expect(screen.getByRole('alertdialog')).toHaveTextContent('0.25')
  // Only the paid boundary is mocked. Real dialog confirmation and hash
  // computation are used; a transport ambiguity must disable another submit.
  post.mockReset().mockRejectedValue(new Error('ambiguous timeout'))
  fireEvent.click(screen.getByRole('button', { name: 'Run paid verification' }))
  await waitFor(() => expect(post).toHaveBeenCalledTimes(1))
  expect(post).toHaveBeenCalledWith(
    '/api/channel/1/runtime-verification/execute',
    expect.objectContaining({
      plan,
      manifest_json: JSON.stringify(manifest),
      confirmed_manifest_hash: expect.stringMatching(/^[a-f0-9]{64}$/),
    }),
    expect.objectContaining({ skipAuthRefresh: true })
  )
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Review paid execution' })
    ).toBeDisabled()
  )
})

test('saved execution evidence refreshes the seven layers and enables showing the verified model', async () => {
  const passed = {
    ...evidence,
    run: { ...evidence.run, id: 8 },
    items: [
      {
        ...evidence.items[0],
        run_id: 8,
        status: 'RUNTIME_VERIFIED',
        config_status: 'PASS',
        connectivity_status: 'PASS',
        request_status: 'PASS',
        generation_status: 'PASS',
        parser_status: 'PASS',
        billing_status: 'PASS',
        ledger_status: 'PASS',
      },
    ],
  }
  const get = vi
    .spyOn(api, 'get')
    .mockResolvedValueOnce({ data: { success: true, data: evidence } })
    .mockResolvedValue({ data: { success: true, data: passed } })
  vi.spyOn(api, 'post').mockResolvedValue({
    data: { success: true, data: passed },
  })
  setup()
  await screen.findByRole('button', { name: 'Show successful models (0)' })
  fireEvent.click(screen.getByRole('button', { name: 'Runtime verification' }))
  fireEvent.change(
    screen.getByRole('textbox', { name: 'Verification plan JSON' }),
    {
      target: {
        value: JSON.stringify({
          plan_version: 'canary-plan-v6',
          targets: [
            {
              model: 'image-model',
              fixture: { id: 'image', protocol: 'openai_image', mode: 'text' },
              maximum_provider_points: '0.25',
            },
          ],
          known_maximum_provider_points: '0.25',
          planned_posts: 1,
        }),
      },
    }
  )
  fireEvent.change(
    screen.getByRole('textbox', { name: 'Signed authorization JSON' }),
    {
      target: {
        value: JSON.stringify({
          approved: true,
          signature: 'signed',
          expires_at: Math.floor(Date.now() / 1000) + 300,
          max_requests: 1,
          max_total_provider_points: '0.25',
          targets: [
            {
              model: 'image-model',
              fixture_id: 'image',
              protocol: 'openai_image',
              mode: 'text',
              maximum_provider_points: '0.25',
            },
          ],
        }),
      },
    }
  )
  fireEvent.click(screen.getByRole('button', { name: 'Review paid execution' }))
  fireEvent.click(screen.getByRole('button', { name: 'Run paid verification' }))
  await waitFor(() =>
    expect(get).toHaveBeenCalledWith(
      '/api/channel/1/runtime-verification/8',
      expect.anything()
    )
  )
  // Close the runtime form to inspect the durable evidence table.
  await waitFor(() =>
    expect(screen.getAllByRole('button', { name: 'Close' })[0]).toBeEnabled()
  )
  fireEvent.click(screen.getAllByRole('button', { name: 'Close' })[0])
  const row = await screen.findByRole('row', { name: /image-model/ })
  for (const label of [
    'Config: PASS',
    'Connectivity: PASS',
    'Request: PASS',
    'Runtime: PASS',
    'Billing: PASS',
    'Ledger: PASS',
  ]) {
    expect(within(row).getByText(label)).toBeVisible()
  }
  expect(
    await screen.findByRole('button', { name: 'Show successful models (1)' })
  ).toBeEnabled()
})
