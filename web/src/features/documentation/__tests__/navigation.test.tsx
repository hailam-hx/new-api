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
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'

import { DocsNavigation, DocsSearch } from '../navigation'

async function renderNavigation(component: React.ReactNode) {
  const root = createRootRoute({ component: () => component })
  const page = createRoute({ getParentRoute: () => root, path: '/docs/$slug' })
  const router = createRouter({
    routeTree: root.addChildren([page]),
    history: createMemoryHistory({ initialEntries: ['/docs/quickstart'] }),
  })
  await router.load()
  render(<RouterProvider router={router} />)
  return router
}

describe('documentation navigation', () => {
  it('marks the current page in the sidebar for assistive technology', async () => {
    await renderNavigation(<DocsNavigation slug='quickstart' />)
    expect(screen.getByRole('link', { name: 'Quickstart' })).toHaveAttribute(
      'aria-current',
      'page'
    )
  })
  it('opens mobile navigation by keyboard and closes after a page selection', async () => {
    const user = userEvent.setup()
    await renderNavigation(<DocsNavigation slug='quickstart' mobile />)
    const trigger = screen.getByRole('button', { name: 'Documentation menu' })
    trigger.focus()
    await user.keyboard('{Enter}')
    expect(screen.getByRole('dialog')).toBeVisible()
    await user.click(screen.getByRole('link', { name: 'FAQ' }))
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
  })
  it('finds the short Text / Chat guide without exposing endpoint inventories', async () => {
    const user = userEvent.setup()
    const router = await renderNavigation(<DocsSearch models={[]} />)
    await user.click(
      screen.getByRole('button', { name: 'Search documentation' })
    )
    await user.type(
      screen.getByRole('textbox', { name: 'Search documentation' }),
      '/v1/chat/completions'
    )
    expect(
      screen.queryByRole('link', { name: /POST \/v1\/chat\/completions/ })
    ).not.toBeInTheDocument()
    await user.click(screen.getByRole('link', { name: /^Text \/ Chat/ }))
    await waitFor(() =>
      expect(router.state.location.pathname).toBe('/docs/text-chat')
    )
  })
  it('shows a visible empty result for a query with no matching source', async () => {
    const user = userEvent.setup()
    await renderNavigation(<DocsSearch models={[]} />)
    await user.click(
      screen.getByRole('button', { name: 'Search documentation' })
    )
    await user.type(
      screen.getByRole('textbox', { name: 'Search documentation' }),
      'nonexistent-endpoint-zzzz'
    )
    expect(screen.getByText('No results found')).toBeVisible()
  })
})

it('keeps the primary sidebar to eighteen user guides and hides administration even on a legacy provider URL', async () => {
  await renderNavigation(<DocsNavigation slug='provider-14' />)
  expect(screen.getAllByRole('link')).toHaveLength(18)
  expect(screen.getByRole('link', { name: 'API Key' })).toBeVisible()
  expect(
    screen.queryByRole('link', { name: 'Providers / Channels' })
  ).not.toBeInTheDocument()
  expect(screen.queryByText('Administration')).not.toBeInTheDocument()
})
