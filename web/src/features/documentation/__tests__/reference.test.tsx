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
import { render, screen, within } from '@testing-library/react'
import { expect, it } from 'vitest'

import { ApiReference, CommonErrors } from '../reference'

it('shows the deployment base URL and the API key header for customer requests', () => {
  render(<ApiReference origin='https://gateway.example/' />)

  expect(screen.getByText('https://gateway.example/v1')).toBeVisible()
  expect(screen.getByText('Authorization: Bearer YOUR_API_KEY')).toBeVisible()
})

it('groups registered customer endpoints by protocol and media type', () => {
  render(<ApiReference origin='https://gateway.example' />)

  const text = within(screen.getByRole('table', { name: 'Text APIs' }))
  expect(text.getByText('/v1/chat/completions')).toBeVisible()
  expect(text.getByText('/v1/responses')).toBeVisible()
  expect(text.getByText('/v1/responses/compact')).toBeVisible()
  expect(text.getByText('/v1/messages')).toBeVisible()
  expect(text.getByText('/v1beta/models/{model}:generateContent')).toBeVisible()
  expect(
    text.getByText('/v1beta/models/{model}:streamGenerateContent')
  ).toBeVisible()
  expect(
    within(screen.getByRole('table', { name: 'Models' })).getByText(
      '/v1/models'
    )
  ).toBeVisible()
  expect(
    within(screen.getByRole('table', { name: 'Image API' })).getByText(
      '/v1/images/generations'
    )
  ).toBeVisible()
  expect(
    within(screen.getByRole('table', { name: 'Audio API' })).getByText(
      '/v1/audio/speech'
    )
  ).toBeVisible()
  expect(
    within(screen.getByRole('table', { name: 'Video API' })).getByText(
      '/v1/videos/{task_id}/content'
    )
  ).toBeVisible()
})

it('explains model availability before the endpoint tables and qualifies video compatibility', () => {
  render(<ApiReference origin='https://gateway.example' />)

  expect(screen.getByRole('alert')).toHaveTextContent(
    'Available endpoints and options depend on the enabled channel, model and plugin configuration.'
  )
  expect(
    screen.getByText(
      'Video routes require a compatible video provider or enabled plugin. Content download depends on the selected provider and a completed task.'
    )
  ).toBeVisible()
})

it('separates HOTX media task paths from compatible protocol endpoints', () => {
  render(<ApiReference origin='https://gateway.example' />)

  const media = within(
    screen.getByRole('table', { name: 'HOTX media task routes' })
  )
  expect(media.getByText('/dflop-media/videos')).toBeVisible()
  expect(media.getByText('/dflop-media/videos/{task_id}')).toBeVisible()
  expect(media.getByText('/dflop-media/speech')).toBeVisible()
  expect(media.getByText('/dflop-media/speech/{task_id}')).toBeVisible()
  expect(
    screen.getByText(
      'Send these requests to the HOTX origin shown above. These routes use their own JSON request and task response formats.'
    )
  ).toBeVisible()
})

it('shows causes and next steps for common authentication, routing and service errors', () => {
  render(<CommonErrors />)

  const table = within(screen.getByRole('table', { name: 'Common errors' }))
  expect(table.getAllByRole('row')).toHaveLength(7)
  expect(table.getByRole('columnheader', { name: 'Status' })).toBeVisible()
  expect(table.getByRole('columnheader', { name: 'Cause' })).toBeVisible()
  expect(table.getByRole('columnheader', { name: 'Next step' })).toBeVisible()
  expect(table.getByRole('row', { name: /401/ })).toHaveTextContent(
    'Copy an active key from API Keys and check the app settings.'
  )
  expect(table.getByRole('row', { name: /403/ })).toHaveTextContent(
    'Check Wallet, subscription limits and your API key access.'
  )
  expect(table.getByRole('row', { name: /404/ })).toHaveTextContent(
    'Check the Base URL, endpoint and model ID. Avoid adding /v1 twice.'
  )
  expect(table.getByRole('row', { name: /500 \/ 503/ })).toHaveTextContent(
    'Check the response message and Usage Logs; contact support if it continues.'
  )
})

it('advises backoff after rate limits and checks task status before retrying a timeout', () => {
  render(<CommonErrors />)

  const table = within(screen.getByRole('table', { name: 'Common errors' }))
  expect(table.getByRole('row', { name: /429/ })).toHaveTextContent(
    'Honor Retry-After when provided, wait before retrying and reduce simultaneous requests.'
  )
  expect(table.getByRole('row', { name: /Timeout/ })).toHaveTextContent(
    'Check request or task status before retrying. A task may still be running.'
  )
})
