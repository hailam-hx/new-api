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
import { I18nextProvider } from 'react-i18next'
import { afterEach, expect, test, vi } from 'vitest'

import { handleBatchDisableModels } from '@/features/models/lib/model-actions'
import type { Model } from '@/features/models/types'

import { loadChannelModels } from '../../lib/channel-model-visibility'
import { ChannelTestHideFailed } from '../dialogs/channel-test-hide-failed'

vi.mock('@/features/models/lib/model-actions', () => ({
  handleBatchDisableModels: vi.fn(),
}))
vi.mock('@/features/models/vendor-api', () => ({
  invalidateVendorData: vi.fn(),
}))
vi.mock('../../lib/channel-model-visibility', () => ({
  loadChannelModels: vi.fn(),
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
})
function setup() {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <I18nextProvider i18n={i18n}>
        <ChannelTestHideFailed
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
