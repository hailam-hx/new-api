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
import { existsSync, readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { publicApiEndpoints } from '../api-reference-data'
import { beginnerArticles, getArticle } from '../content'
import reference from '../generated/reference.json'
import { searchDocumentation } from '../lib'
import { ApiReference } from '../reference'

describe('customer API reference', () => {
  it('adds one reference item after Audio without endpoint sidebar entries', () => {
    const api = beginnerArticles.filter((page) => page.group === 'API')
    expect(api.map((page) => page.slug)).toEqual([
      'text-chat',
      'image',
      'video',
      'audio',
      'api-reference',
    ])
    expect(getArticle('api-reference')?.kind).toBe('reference')
  })
  it('only documents registered customer operations with token authentication', () => {
    expect(publicApiEndpoints).toHaveLength(12)
    for (const endpoint of publicApiEndpoints) {
      const route = reference.endpoints.find(
        (e) => e.method === endpoint.method && e.path === endpoint.path
      )
      expect(route?.middleware).toContain('middleware.TokenAuth()')
      expect(endpoint.path).toMatch(/^\/v1\//)
    }
    expect(publicApiEndpoints.some((e) => e.path.startsWith('/api/'))).toBe(
      false
    )
  })
  it('finds public endpoints through ordinary docs search but excludes console APIs', () => {
    for (const endpoint of publicApiEndpoints) {
      expect(
        searchDocumentation(
          endpoint.path,
          beginnerArticles,
          reference,
          [],
          (x) => x
        ).some((result) => result.href.startsWith('/docs/api-reference#'))
      ).toBe(true)
    }
    expect(
      searchDocumentation(
        '/api/channel',
        beginnerArticles,
        reference,
        [],
        (x) => x
      )
    ).toEqual([])
  })
  it('keeps documented DTO fields and source paths valid, overriding stale image OpenAPI', () => {
    for (const endpoint of publicApiEndpoints) {
      for (const source of endpoint.source.split('; ')) {
        expect(existsSync(resolve(process.cwd(), '..', source)), source).toBe(
          true
        )
      }
    }
    const image = publicApiEndpoints.find(
      (endpoint) => endpoint.path === '/v1/images/generations'
    )
    expect(image).toBeDefined()
    if (!image) return
    const dto = readFileSync(
      resolve(process.cwd(), '../relaykit/dto/openai_image.go'),
      'utf8'
    )
    for (const field of image.fields) {
      expect(dto).toContain(`json:"${field.name}`)
    }
    expect(image.payload).toHaveProperty('prompt')
    expect(image.fields.some((field) => field.name === 'input')).toBe(false)
    const responses = publicApiEndpoints.find(
      (endpoint) =>
        endpoint.method === 'POST' && endpoint.path === '/v1/responses'
    )
    expect(responses).toBeDefined()
    if (!responses) return
    expect(
      responses.fields.find((field) => field.name === 'input')?.required
    ).toBe(true)
    expect(responses.example.split('\n')[0]).toMatch(/\\$/)
  })
  it('shows auth, request tables, curl and response shapes without management controls', () => {
    render(<ApiReference />)
    expect(
      screen.getByText('https://hotx-api.com/v1', { exact: true })
    ).toBeVisible()
    expect(
      screen.getByText('Authorization: Bearer YOUR_API_KEY', { exact: true })
    ).toBeVisible()
    expect(screen.queryByRole('combobox')).not.toBeInTheDocument()
    expect(screen.getAllByRole('table').length).toBeGreaterThan(10)
    expect(document.querySelector('#post--v1-images-edits')).toHaveTextContent(
      'multipart/form-data'
    )
    expect(document.querySelector('#post--v1-responses')).toHaveTextContent(
      'input'
    )
    expect(
      document.querySelector('#post--v1-audio-transcriptions')
    ).toHaveTextContent('file')
    expect(document.querySelectorAll('pre').length).toBeGreaterThan(12)
    expect(screen.queryByText('Management API')).not.toBeInTheDocument()
  })
})
