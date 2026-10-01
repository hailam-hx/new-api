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
  expect(csv).toContain('"Unknown"')
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
