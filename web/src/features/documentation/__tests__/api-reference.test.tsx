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
import { beginnerArticles, developerArticles, getArticle } from '../content'
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
    expect(publicApiEndpoints).toHaveLength(106)
    for (const endpoint of publicApiEndpoints) {
      const route = reference.endpoints.find(
        (e) => e.method === endpoint.method && e.path === endpoint.path
      )
      if (endpoint.nativeRoute) {
        expect(endpoint.deploymentDependent).toBe(true)
      } else {
        expect(route).toBeDefined()
        expect(
          route?.middleware.some(
            (item) =>
              item.includes('TokenAuth') ||
              item.includes('PublicAPIKeyReadAuth') ||
              item.includes('TokenOrTaskArtifactAccessAuth')
          ) ||
            ['/mj/image/{id}', '/{mode}/mj/image/{id}'].includes(endpoint.path)
        ).toBe(true)
      }
    }
    expect(
      publicApiEndpoints
        .filter((endpoint) => endpoint.path.startsWith('/api/'))
        .map((endpoint) => endpoint.path)
        .sort()
    ).toEqual([])
  })
  it('documents public catalog and key balance with exact decimal and unlimited semantics', () => {
    const catalog = publicApiEndpoints.find(
      (endpoint) => endpoint.path === '/v1/catalog'
    )
    const balance = publicApiEndpoints.find(
      (endpoint) => endpoint.path === '/v1/key/balance'
    )
    expect(catalog?.response).toMatchObject({
      object: 'catalog',
      schema_version: '1.0',
      currency: 'points',
      models: [
        {
          pricing: {
            variable: false,
            input_per_1m_tokens: '2.5',
            output_per_1m_tokens: '10',
            cached_input_per_1m_tokens: '0.25',
          },
        },
      ],
    })
    expect(balance?.response).toEqual({
      object: 'key.balance',
      unit: 'points',
      total: '100',
      used: '25',
      remaining: '75',
      unlimited: false,
      expires_at: null,
    })
    expect(balance?.responseAlternatives?.[0].response).toEqual({
      object: 'key.balance',
      unit: 'points',
      total: null,
      used: '25',
      remaining: null,
      unlimited: true,
      expires_at: null,
    })
    for (const endpoint of [catalog, balance]) {
      expect(endpoint?.method).toBe('GET')
      expect(endpoint?.fields).toEqual([])
      expect(endpoint?.example).toContain(
        `https://hotx-api.com${endpoint?.path}`
      )
      expect(endpoint?.example).toContain('Authorization: Bearer YOUR_API_KEY')
      expect(endpoint?.example).not.toContain(' -d ')
    }
    expect(
      publicApiEndpoints.some((endpoint) =>
        endpoint.path.startsWith('/v1/usage/requests/')
      )
    ).toBe(false)
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
        ).some((result) =>
          result.href.startsWith(`/docs/api-reference-${endpoint.group}#`)
        )
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
  it('keeps the reference overview short and links to domain pages', () => {
    render(<ApiReference />)
    expect(
      screen.getByText('https://hotx-api.com/v1', { exact: true })
    ).toBeVisible()
    expect(
      screen.getByRole('link', { name: 'Models & Catalog' })
    ).toHaveAttribute('href', '/docs/api-reference-models')
    expect(
      screen.queryByText('Request parameters', { exact: true })
    ).not.toBeInTheDocument()
    expect(document.querySelectorAll('pre').length).toBeLessThan(3)
  })
  it('renders only the chosen domain with parameter and response schemas', () => {
    render(<ApiReference group='images' />)
    expect(
      document.querySelector('#post--v1-images-generations')
    ).toBeInTheDocument()
    expect(
      document.querySelector('#post--v1-chat-completions')
    ).not.toBeInTheDocument()
    expect(screen.getAllByText('Response schema').length).toBeGreaterThan(0)
    expect(screen.queryByText('Management API')).not.toBeInTheDocument()
  })
  it('keeps developer pages outside the global beginner sidebar and uses routable unique slugs', () => {
    expect(developerArticles.length).toBeGreaterThan(10)
    expect(new Set(developerArticles.map((page) => page.slug)).size).toBe(
      developerArticles.length
    )
    expect(
      developerArticles.every((page) => !beginnerArticles.includes(page))
    ).toBe(true)
    expect(
      developerArticles.every((page) => getArticle(page.slug) === page)
    ).toBe(true)
  })
  it('matches native manifest operations and excludes placeholders and console logs', () => {
    const native = publicApiEndpoints.filter((endpoint) => endpoint.nativeRoute)
    expect(native).toHaveLength(29)
    for (const endpoint of native) {
      const manifest = readFileSync(
        resolve(process.cwd(), '..', endpoint.source),
        'utf8'
      )
      const routes = [
        ...manifest.matchAll(
          /method:\s*["'](GET|POST|HEAD)["'][^}]*?path:\s*["']([^"']+)["']/g
        ),
      ]
      expect(
        routes.some(
          (match) =>
            match[1] === endpoint.method &&
            match[2].replaceAll(/:([a-z_]+)/g, '{$1}') === endpoint.path
        ),
        endpoint.path
      ).toBe(true)
    }
    expect(
      publicApiEndpoints.some((endpoint) => endpoint.path === '/api/log/token')
    ).toBe(false)
    for (const endpoint of publicApiEndpoints) {
      const route = reference.endpoints.find(
        (item) => item.method === endpoint.method && item.path === endpoint.path
      )
      expect(route?.handler ?? '').not.toContain('RelayNotImplemented')
      expect(
        new Set(endpoint.responseFields.map((field) => field.name)).size,
        endpoint.path
      ).toBe(endpoint.responseFields.length)
    }
  })
  it('documents protocol-specific schemas and conditional speech without inventing legacy success', () => {
    const speech = publicApiEndpoints.find(
      (endpoint) =>
        endpoint.path === '/v1/audio/speech' && endpoint.method === 'POST'
    )
    expect(
      speech?.fields.find((field) => field.name === 'async')?.conditional
    ).toBe(true)
    expect(speech?.headers).toHaveProperty('Idempotency-Key')
    expect(speech?.responseAlternatives?.[0].response).toHaveProperty('task_id')
    const engine = publicApiEndpoints.find(
      (endpoint) => endpoint.path === '/v1/engines/{model}/embeddings'
    )
    expect(engine?.payload).toHaveProperty('content')
    expect(engine?.payload).not.toHaveProperty('contents')
    expect(engine?.responseFields.map((field) => field.name)).not.toContain(
      'candidates[].content.parts'
    )
    const legacy = publicApiEndpoints.find(
      (endpoint) => endpoint.path === '/v1/edits'
    )
    expect(legacy?.response).toEqual({})
    expect(legacy?.note).toContain('not a usable image-edit API')
    const imageEdit = publicApiEndpoints.find(
      (endpoint) => endpoint.path === '/dflop-image/v1/images/edits'
    )
    expect(imageEdit?.payload).toHaveProperty('image')
    expect(
      imageEdit?.fields.find((field) => field.name === 'image')?.conditional
    ).toBe(true)
    render(<ApiReference group='audio' />)
    expect(
      screen.getByText('Asynchronous voice-tts-pro task response', {
        exact: true,
      })
    ).toBeInTheDocument()
  })
})
