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
import { render } from '@testing-library/react'
import { expect, it } from 'vitest'

import { DocumentationMetadata } from '../metadata'

it('owns one title, description and canonical across page changes and restores site defaults when leaving docs', () => {
  const title = document.createElement('title')
  title.textContent = 'New API'
  const description = document.createElement('meta')
  description.name = 'description'
  description.content = 'Site description'
  document.head.append(title, description)
  const view = render(
    <DocumentationMetadata
      title='Quickstart'
      description='First request'
      canonical='https://gateway.example/docs/quickstart'
    />
  )
  expect(document.head.querySelectorAll('title')).toHaveLength(1)
  expect(
    document.head.querySelectorAll('meta[name="description"]')
  ).toHaveLength(1)
  view.rerender(
    <DocumentationMetadata
      title='Pricing'
      description='Configured prices'
      canonical='https://gateway.example/docs/pricing'
    />
  )
  expect(document.title).toBe('Pricing · New API Documentation')
  expect(document.head.querySelectorAll('link[rel="canonical"]')).toHaveLength(
    1
  )
  expect(document.head.querySelector('link[rel="canonical"]')).toHaveAttribute(
    'href',
    'https://gateway.example/docs/pricing'
  )
  view.unmount()
  expect(document.title).toBe('New API')
  expect(
    document.head.querySelector('meta[name="description"]')
  ).toHaveAttribute('content', 'Site description')
  expect(document.head.querySelector('link[rel="canonical"]')).toBeNull()
  title.remove()
  description.remove()
})
