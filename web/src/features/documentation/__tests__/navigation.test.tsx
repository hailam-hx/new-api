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
import i18next from 'i18next'
import { afterEach, describe, expect, it, vi as vitest } from 'vitest'

import vi from '@/i18n/locales/vi.json'

import { DocsNavigation, DocsSearch } from '../navigation'

async function renderNavigation(component: React.ReactNode) {
  vitest.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
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

afterEach(async () => {
  vitest.restoreAllMocks()
  await i18next.changeLanguage('en')
})

describe('customer documentation navigation', () => {
  it('identifies the selected guide and includes the HotX media guide', async () => {
    await renderNavigation(<DocsNavigation slug='quickstart' />)
    expect(screen.getByRole('link', { name: 'Quickstart' })).toHaveAttribute(
      'aria-current',
      'page'
    )
    expect(
      screen.getByRole('link', { name: 'HOTX media tasks' })
    ).toHaveAttribute('href', '/docs/hotx-media')
    expect(screen.queryByText('Administration')).not.toBeInTheDocument()
  })

  it('opens the mobile menu by keyboard and closes after choosing a guide', async () => {
    const user = userEvent.setup()
    const router = await renderNavigation(
      <DocsNavigation slug='quickstart' mobile />
    )
    screen.getByRole('button', { name: 'Documentation menu' }).focus()
    await user.keyboard('{Enter}')
    expect(screen.getByRole('dialog')).toBeVisible()
    await user.click(screen.getByRole('link', { name: 'FAQ' }))
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
    expect(router.state.location.pathname).toBe('/docs/faq')
  })

  it('searches translated guide text and navigates using the keyboard', async () => {
    i18next.addResourceBundle('vi', 'translation', vi.translation, true, true)
    await i18next.changeLanguage('vi')
    const user = userEvent.setup()
    const router = await renderNavigation(<DocsSearch />)
    await user.click(
      screen.getByRole('button', { name: i18next.t('Search documentation') })
    )
    await user.type(screen.getByRole('textbox'), 'bắt đầu nhanh')
    expect(screen.getByRole('link', { name: /Bắt đầu nhanh/i })).toBeVisible()
    await user.keyboard('{Enter}')
    await waitFor(() =>
      expect(router.state.location.pathname).toBe('/docs/quickstart')
    )
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('shows an explicit empty result for a query without a matching guide', async () => {
    const user = userEvent.setup()
    await renderNavigation(<DocsSearch />)
    await user.click(
      screen.getByRole('button', { name: 'Search documentation' })
    )
    await user.type(screen.getByRole('textbox'), 'missing-guide-zzzz')
    expect(screen.getByRole('status')).toHaveTextContent('No results found')
  })
})
