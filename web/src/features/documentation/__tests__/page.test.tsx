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
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { ThemeProvider } from '@/context/theme-provider'
import { STATUS_QUERY_KEY } from '@/lib/status-query'

import { Documentation } from '../index'

afterEach(() => vi.restoreAllMocks())

async function renderPage(slug: string, hash = '') {
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
  const queryClient = new QueryClient({
    defaultOptions: { queries: { staleTime: Infinity, retry: false } },
  })
  queryClient.setQueryData(STATUS_QUERY_KEY, {
    server_address: 'https://gateway.example/proxy/v1/',
    HeaderNavModules: { pricing: { enabled: false } },
  })
  queryClient.setQueryData(['notice'], { success: true, data: '' })
  const root = createRootRoute({
    component: () => <Documentation slug={slug} />,
  })
  const page = createRoute({ getParentRoute: () => root, path: '/docs/$slug' })
  const router = createRouter({
    routeTree: root.addChildren([page]),
    history: createMemoryHistory({ initialEntries: [`/docs/${slug}${hash}`] }),
  })
  await router.load()
  const view = render(
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <RouterProvider router={router} />
      </ThemeProvider>
    </QueryClientProvider>
  )
  return { ...view, queryClient, router }
}

it('renders a public guide and working SDK tabs without pricing access', async () => {
  const user = userEvent.setup()
  const view = await renderPage('quickstart')
  const main = screen.getByRole('main')
  expect(
    within(main).getByRole('heading', { level: 1, name: 'Quickstart' })
  ).toBeVisible()
  await user.click(within(main).getByRole('tab', { name: 'Python' }))
  expect(within(main).getByRole('tabpanel')).toHaveTextContent(
    'https://gateway.example/proxy/v1'
  )
  expect(within(main).getByRole('tabpanel')).not.toHaveTextContent('/v1/v1')
  expect(
    view.queryClient.getQueryCache().find({ queryKey: ['pricing'] })
  ).toBeUndefined()
  expect(
    within(main).getByRole('link', { name: 'New API · QuantumNous' })
  ).toHaveAttribute('href', 'https://github.com/QuantumNous/new-api')
  view.unmount()
  view.queryClient.clear()
})

it('keeps model discovery on the existing pricing screen', async () => {
  const view = await renderPage('models')
  expect(
    within(screen.getByRole('main')).getByRole('link', { name: 'Model Square' })
  ).toHaveAttribute('href', '/pricing')
  view.unmount()
  view.queryClient.clear()
})

it('shows the unversioned Anthropic base URL required by Claude Code', async () => {
  const view = await renderPage('integration-claude-code')
  const panel = within(screen.getByRole('main')).getByRole('region', {
    name: 'Base URL',
  })
  expect(panel).toHaveTextContent('https://gateway.example/proxy')
  expect(panel).not.toHaveTextContent('/v1')
  view.unmount()
  view.queryClient.clear()
})

it('gives the overview actionable steps and a copyable configured base URL', async () => {
  const view = await renderPage('overview')
  const main = within(screen.getByRole('main'))
  expect(main.getByRole('list', { name: 'Start in 3 steps' })).toBeVisible()
  expect(
    main.getByText('https://gateway.example/proxy/v1', { selector: 'code' })
  ).toBeVisible()
  view.unmount()
  view.queryClient.clear()
})

it('opens the transcription tab from a deep link and keeps tab changes shareable', async () => {
  const user = userEvent.setup()
  const view = await renderPage('audio', '#transcription-endpoint')
  expect(screen.getByRole('tab', { name: 'Speech → Text' })).toHaveAttribute(
    'aria-selected',
    'true'
  )
  const contents = screen.getByRole('navigation', { name: 'On this page' })
  expect(within(contents).getAllByRole('link')[0]).toHaveAttribute(
    'href',
    '#transcription-endpoint'
  )
  await user.click(screen.getByRole('tab', { name: 'Text → Speech' }))
  await screen.findByRole('tab', { name: 'Text → Speech', selected: true })
  expect(view.router.state.location.hash).toBe('speech-endpoint')
  view.unmount()
  view.queryClient.clear()
})
