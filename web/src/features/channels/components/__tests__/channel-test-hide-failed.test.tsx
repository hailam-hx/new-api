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
  fireEvent,
  render,
  screen,
  waitFor,
  cleanup,
} from '@testing-library/react'
import { createInstance } from 'i18next'
import { useEffect } from 'react'
import { I18nextProvider } from 'react-i18next'
import { toast } from 'sonner'
import { afterEach, expect, test, vi } from 'vitest'

import {
  handleBatchDisableModels,
  handleBatchEnableModels,
} from '@/features/models/lib/model-actions'
import type { Model } from '@/features/models/types'
import { api } from '@/lib/api'

import {
  loadChannelModels,
  loadChannelModelVisibility,
} from '../../lib/channel-model-visibility'
import type { Channel } from '../../types'
import { ChannelsProvider, useChannels } from '../channels-provider'
import { ChannelTestDialog } from '../dialogs/channel-test-dialog'
import { ChannelTestModelVisibilityAction } from '../dialogs/channel-test-model-visibility-action'
import { TaskConnectivityAction } from '../dialogs/task-connectivity-action'

vi.mock('@/features/models/lib/model-actions', () => ({
  handleBatchDisableModels: vi.fn(),
  handleBatchEnableModels: vi.fn(),
}))
vi.mock('@/features/models/vendor-api', () => ({
  invalidateVendorData: vi.fn(),
}))
vi.mock('../../lib/channel-model-visibility', async (importOriginal) => ({
  ...(await importOriginal<
    typeof import('../../lib/channel-model-visibility')
  >()),
  loadChannelModels: vi.fn(),
  loadChannelModelVisibility: vi.fn().mockResolvedValue({}),
}))
vi.mock('@/lib/handle-server-error', () => ({ handleServerError: vi.fn() }))
vi.mock('@/components/confirm-dialog', () => ({
  ConfirmDialog: ({
    open,
    handleConfirm,
  }: {
    open: boolean
    handleConfirm: () => void
  }) =>
    open ? (
      <button type='button' onClick={handleConfirm}>
        Confirm
      </button>
    ) : null,
}))
const i18n = createInstance()
await i18n.init({ lng: 'en', resources: { en: { translation: {} } } })
afterEach(() => {
  cleanup()
  vi.resetAllMocks()
  vi.restoreAllMocks()
})
function setup(action: 'hide' | 'show' = 'hide') {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <I18nextProvider i18n={i18n}>
        <ChannelTestModelVisibilityAction
          action={action}
          models={['failed', 'synthetic', 'already-hidden']}
          disabled={false}
          onBusyChange={vi.fn()}
        />
      </I18nextProvider>
    </QueryClientProvider>
  )
}
test('requires confirmation and hides only failed concrete models, preserving unrelated rules and successful models', async () => {
  const synthetic = {
    id: 0,
    model_name: 'synthetic',
    name_rule: 0,
    square_state: 'visible',
  } as Model
  vi.mocked(loadChannelModels).mockResolvedValue([
    {
      id: 7,
      model_name: 'failed',
      name_rule: 0,
      square_state: 'visible',
    } as Model,
    synthetic,
    {
      id: 8,
      model_name: 'success',
      name_rule: 0,
      square_state: 'visible',
    } as Model,
    {
      id: 9,
      model_name: 'already-hidden',
      name_rule: 0,
      square_state: 'hidden',
    } as Model,
    {
      id: 10,
      model_name: 'failed',
      name_rule: 1,
      square_state: 'partial',
    } as Model,
  ])
  setup()
  fireEvent.click(
    screen.getByRole('button', { name: 'Hide failed models (3)' })
  )
  expect(handleBatchDisableModels).not.toHaveBeenCalled()
  fireEvent.click(screen.getByText('Confirm'))
  await waitFor(() =>
    expect(handleBatchDisableModels).toHaveBeenCalledWith(
      [7],
      expect.any(QueryClient),
      undefined,
      [synthetic]
    )
  )
})
test('does not mutate visibility if fresh metadata fails to load', async () => {
  vi.mocked(loadChannelModels).mockRejectedValue(new Error('Unavailable'))
  setup()
  fireEvent.click(
    screen.getByRole('button', { name: 'Hide failed models (3)' })
  )
  fireEvent.click(screen.getByText('Confirm'))
  await waitFor(() => expect(loadChannelModels).toHaveBeenCalled())
  expect(handleBatchDisableModels).not.toHaveBeenCalled()
})

test('shows only successful hidden models after confirmation, including exact overrides for inherited hidden rules', async () => {
  const inherited = {
    id: 0,
    model_name: 'synthetic',
    name_rule: 0,
    square_state: 'hidden',
  } as Model
  vi.mocked(loadChannelModels).mockResolvedValue([
    {
      id: 7,
      model_name: 'failed',
      name_rule: 0,
      square_state: 'visible',
    } as Model,
    inherited,
    {
      id: 9,
      model_name: 'already-hidden',
      name_rule: 0,
      square_state: 'hidden',
    } as Model,
    {
      id: 10,
      model_name: 'untested',
      name_rule: 0,
      square_state: 'hidden',
    } as Model,
  ])
  setup('show')
  fireEvent.click(
    screen.getByRole('button', { name: 'Show successful models (3)' })
  )
  expect(handleBatchEnableModels).not.toHaveBeenCalled()
  fireEvent.click(screen.getByText('Confirm'))
  await waitFor(() =>
    expect(handleBatchEnableModels).toHaveBeenCalledWith(
      [9],
      expect.any(QueryClient),
      undefined,
      [inherited]
    )
  )
  expect(handleBatchDisableModels).not.toHaveBeenCalled()
})

const diagnosticChannel = {
  id: 99,
  name: 'Task vendor',
  models: 'partial,untested,failed',
  type: 61,
} as Channel
function DiagnosticDialogFixture() {
  const setCurrentRow = useChannels().setCurrentRow
  useEffect(() => setCurrentRow(diagnosticChannel), [setCurrentRow])
  return <ChannelTestDialog open onOpenChange={() => undefined} />
}

test('mixed task batch keeps partial and untested rows out of hide/delete and never updates channel health', async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  vi.mocked(loadChannelModelVisibility).mockResolvedValue({})
  vi.spyOn(api, 'post').mockImplementation(async (_url, body) => {
    const model = (body as { model: string }).model
    const outcome = model === 'failed' ? 'fail' : model
    return {
      data: {
        success: true,
        data: {
          kind: 'task_plugin',
          mode: 'preflight',
          status:
            outcome === 'fail' ? 'pricing_not_ready' : 'preflight_partial',
          outcome,
          model,
          mapped_model: model,
          plugin: 'media',
          connectivity_tested: false,
          live_generation_tested: false,
          checks: [
            {
              check: 'runtime_evidence',
              status: 'not_tested',
              reason_code: 'LIVE_CANARY_REQUIRED',
              evidence: 'offline metadata',
            },
          ],
        },
      },
    }
  })
  const put = vi
    .spyOn(api, 'put')
    .mockResolvedValue({ data: { success: true } })
  const get = vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: { run: null, items: [] } },
  })
  const info = vi.spyOn(toast, 'info')
  vi.mocked(loadChannelModels).mockResolvedValue([
    { id: 1, model_name: 'partial', name_rule: 0, square_state: 'visible' },
    { id: 2, model_name: 'untested', name_rule: 0, square_state: 'visible' },
    { id: 3, model_name: 'failed', name_rule: 0, square_state: 'visible' },
  ] as Model[])
  render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={i18n}>
        <ChannelsProvider>
          <DiagnosticDialogFixture />
        </ChannelsProvider>
      </I18nextProvider>
    </QueryClientProvider>
  )
  fireEvent.click(
    await screen.findByRole('button', { name: /Test all 3 models/ })
  )
  await screen.findByRole('button', { name: 'Hide failed models (1)' })
  expect(screen.getAllByText('Upstream not tested').length).toBeGreaterThan(0)
  expect(
    screen.getByRole('button', { name: 'Delete failed models (1)' })
  ).toBeInTheDocument()
  fireEvent.click(screen.getAllByRole('button', { name: 'Details' })[0])
  expect(await screen.findByText(/LIVE_CANARY_REQUIRED/)).toBeInTheDocument()
  expect(screen.getByText(/offline metadata/)).toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'Close' }))
  expect(
    get.mock.calls.every(
      ([url]) => url === '/api/channel/99/runtime-verification'
    )
  ).toBe(true)
  expect(put).not.toHaveBeenCalled()
  expect(info).toHaveBeenCalledWith(
    expect.stringContaining('1 partial, 1 untested, 1 failed')
  )
  fireEvent.click(
    screen.getByRole('button', { name: 'Hide failed models (1)' })
  )
  fireEvent.click(screen.getByText('Confirm'))
  await waitFor(() =>
    expect(handleBatchDisableModels).toHaveBeenCalledWith(
      [3],
      expect.any(QueryClient),
      undefined,
      []
    )
  )
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Delete failed models (1)' })
    ).toBeEnabled()
  )
  fireEvent.click(
    screen.getByRole('button', { name: 'Delete failed models (1)' })
  )
  fireEvent.click(screen.getByText('Confirm'))
  await waitFor(() => expect(put).toHaveBeenCalled())
  const payload = put.mock.calls[0][1] as { models: string }
  expect(payload.models.split(',')).toEqual(['partial', 'untested'])
  expect(payload).not.toHaveProperty('status')
})

test('explicit connectivity appears only for reviewed capability, preserves catalog evidence and never updates channel health', async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  vi.mocked(loadChannelModelVisibility).mockResolvedValue({})
  const post = vi.spyOn(api, 'post').mockImplementation(async (_url, body) => {
    const request = body as { model: string; mode: string }
    const connectivity = request.mode === 'connectivity'
    return {
      data: {
        success: true,
        data: {
          kind: 'task_plugin',
          mode: request.mode,
          status: connectivity ? 'connectivity_pass' : 'preflight_partial',
          outcome: 'partial',
          model: request.model,
          mapped_model: request.model,
          plugin: 'dflop-media',
          generation: 1,
          diagnostic_duration_ms: 0,
          connectivity_available: request.model === 'partial',
          connectivity_key_indices: [0],
          connectivity_tested: connectivity,
          live_generation_tested: false,
          connectivity_status: connectivity ? 'connectivity_pass' : undefined,
          connectivity_latency_ms: 7,
          model_access: connectivity ? 'confirmed' : undefined,
          credential_identity: connectivity ? 'key_index:0' : undefined,
          checks: [],
        },
      },
    }
  })
  const put = vi
    .spyOn(api, 'put')
    .mockResolvedValue({ data: { success: true } })
  const get = vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: { run: null, items: [] } },
  })
  render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={i18n}>
        <ChannelsProvider>
          <DiagnosticDialogFixture />
        </ChannelsProvider>
      </I18nextProvider>
    </QueryClientProvider>
  )
  expect(
    screen.queryByRole('button', { name: 'Check DFLOP connectivity' })
  ).not.toBeInTheDocument()
  fireEvent.click(
    await screen.findByRole('button', { name: /Test all 3 models/ })
  )
  const button = await screen.findByRole('button', {
    name: 'Check DFLOP connectivity',
  })
  expect(
    screen.getAllByRole('button', { name: 'Check DFLOP connectivity' })
  ).toHaveLength(1)
  expect(
    post.mock.calls.every(
      (call) => (call[1] as { mode: string }).mode === 'preflight'
    )
  ).toBe(true)
  fireEvent.click(button)
  await screen.findByText('DFLOP connectivity verified')
  expect(
    screen.getByText('Model visible to the selected API key')
  ).toBeInTheDocument()
  expect(screen.getByText('Content generation not tested')).toBeInTheDocument()
  expect(post).toHaveBeenLastCalledWith(
    '/api/channel/test/99/task',
    expect.objectContaining({ mode: 'connectivity', model: 'partial' }),
    expect.anything()
  )
  expect(
    get.mock.calls.every(
      ([url]) => url === '/api/channel/99/runtime-verification'
    )
  ).toBe(true)
  expect(put).not.toHaveBeenCalled()
  expect(
    screen.queryByRole('button', { name: /Hide failed models/ })
  ).not.toBeInTheDocument()
})

test('multi-key connectivity requires explicit enabled key selection before any probe', async () => {
  const client = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  })
  const complete = vi.fn()
  const post = vi.spyOn(api, 'post').mockResolvedValue({
    data: {
      success: true,
      data: {
        mode: 'connectivity',
        outcome: 'partial',
        connectivity_status: 'connectivity_pass',
      },
    },
  })
  render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={i18n}>
        <TaskConnectivityAction
          channelId={99}
          model='video'
          multiKey
          disabled={false}
          onComplete={complete}
          diagnostic={{
            kind: 'task_plugin',
            mode: 'preflight',
            outcome: 'partial',
            status: 'preflight_partial',
            generation: 1,
            model: 'video',
            mapped_model: 'video',
            diagnostic_duration_ms: 0,
            connectivity_tested: false,
            live_generation_tested: false,
            connectivity_available: true,
            connectivity_key_indices: [1, 3],
            checks: [],
          }}
        />
      </I18nextProvider>
    </QueryClientProvider>
  )
  const button = screen.getByRole('button', {
    name: 'Check DFLOP connectivity',
  })
  expect(button).toBeDisabled()
  expect(post).not.toHaveBeenCalled()
  fireEvent.click(
    screen.getByRole('button', { name: 'Select an enabled API key' })
  )
  fireEvent.click(
    await screen.findByRole('option', { name: 'API key index 3' })
  )
  expect(button).toBeEnabled()
  fireEvent.click(button)
  await waitFor(() => expect(complete).toHaveBeenCalled())
  expect(post).toHaveBeenCalledWith(
    '/api/channel/test/99/task',
    expect.objectContaining({
      mode: 'connectivity',
      model: 'video',
      key_index: 3,
    }),
    expect.anything()
  )
})

test('ambiguous runtime result explains missing evidence instead of displaying an internal status', async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  vi.mocked(loadChannelModelVisibility).mockResolvedValue({})
  vi.spyOn(api, 'post').mockResolvedValue({
    data: {
      success: true,
      data: {
        kind: 'ordinary',
        mode: 'preflight',
        status: 'preflight_partial',
        outcome: 'partial',
        checks: [],
      },
    },
  })
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: false,
      message: 'network outcome unknown',
      diagnostic: {
        kind: 'ordinary',
        mode: 'runtime',
        status: 'runtime_failure_unclassified',
        outcome: 'partial',
        model: 'partial',
        mapped_model: 'partial',
        runtime_attempted: true,
        connectivity_tested: false,
        live_generation_tested: false,
        checks: [
          {
            check: 'runtime_evidence',
            status: 'not_tested',
            reason_code: 'INSUFFICIENT_EVIDENCE',
            message: 'network outcome unknown',
          },
        ],
      },
    },
  })
  render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={i18n}>
        <ChannelsProvider>
          <DiagnosticDialogFixture />
        </ChannelsProvider>
      </I18nextProvider>
    </QueryClientProvider>
  )
  fireEvent.click(
    await screen.findByRole('button', { name: /Test all 3 models/ })
  )
  expect(
    (
      await screen.findAllByText(
        'Upstream result is inconclusive; exact request evidence is required'
      )
    ).length
  ).toBe(3)
  expect(
    screen.queryByText('runtime_failure_unclassified')
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: /Hide failed models/ })
  ).not.toBeInTheDocument()
})
