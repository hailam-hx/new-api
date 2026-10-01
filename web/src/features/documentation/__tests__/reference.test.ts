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
import { describe, expect, it } from 'vitest'

import type { PricingModel } from '@/features/pricing/types'

import { articles, groups } from '../content'
import reference from '../generated/reference.json'
import { searchDocumentation, filterCatalog, buildQuickstart } from '../lib'

const model = {
  model_name: "vendor/model'with-quote",
  quota_type: 0,
  model_ratio: 1,
  completion_ratio: 1,
  id: 1,
  enable_groups: ['default'],
  supported_endpoint_types: ['openai'],
} satisfies PricingModel

describe('documentation source contracts', () => {
  it('publishes registered protocol routes but excludes unimplemented files and stale vendor routes', () => {
    const paths = reference.endpoints.map((e) => e.path)
    expect(paths).toContain('/v1/chat/completions')
    expect(paths).toContain('/v1/tasks/{key}')
    expect(paths).toContain('/v1/responses/{response_id}')
    expect(paths).not.toContain('/v1/files')
    expect(paths).not.toContain('/kling/v1/videos/text2video')
  })
  it('preserves administrator and token authentication boundaries in generated reference', () => {
    expect(
      reference.endpoints.find(
        (e) => e.path === '/api/channel/' && e.method === 'POST'
      )?.middleware
    ).toContain('middleware.AdminAuth()')
    expect(
      reference.endpoints.find((e) => e.path === '/v1/chat/completions')
        ?.middleware
    ).toContain('middleware.TokenAuth()')
  })
  it('keeps article slugs unique and navigation in the nine actual groups', () => {
    expect(new Set(articles.map((a) => a.slug)).size).toBe(articles.length)
    expect(groups).toHaveLength(9)
    expect(articles.every((a) => groups.includes(a.group))).toBe(true)
    expect(articles.every((a) => a.sources.length > 0)).toBe(true)
  })
  it('finds an endpoint, error constant, model ID and vendor in search', () => {
    for (const query of [
      '/v1/chat/completions',
      'model_price_error',
      'BILLING_*',
      'OpenAI',
      model.model_name,
      'DFLOP',
    ]) {
      expect(
        searchDocumentation(
          query,
          articles,
          reference,
          [{ ...model, vendor_name: 'DFLOP' }],
          (x) => x
        ).length
      ).toBeGreaterThan(0)
    }
  })
  it('does not infer capabilities from a model name when metadata is missing', () => {
    expect(filterCatalog([model], '', 'reasoning', '')).toEqual([])
    expect(
      filterCatalog(
        [{ ...model, capabilities: ['reasoning'] }],
        '',
        'reasoning',
        ''
      )
    ).toHaveLength(1)
  })
  it('selects the chat model from the credential-scoped model list rather than a fixed demo name', () => {
    const examples = buildQuickstart('https://gateway.example', 'openai', false)
    expect(examples.curl).toContain('/v1/models')
    expect(examples.python).toContain('client.models.list()')
    expect(examples.python).toContain('supported_endpoint_types')
    expect(examples.typescript).toContain('client.models.list()')
    expect(examples.curl).not.toContain('sk-')
  })
})

it('keeps host protocol authentication and rate limits from the registered handlers', () => {
  expect(
    reference.endpoints.find(
      (e) => e.method === 'POST' && e.path === '/v1/responses'
    )?.middleware
  ).toContain('middleware.ModelRequestRateLimit()')
  expect(
    reference.errors.find((e) => e.code === 'AUTH_TOKEN_EXPIRED')?.status
  ).toBe(401)
  expect(
    reference.errors.find((e) => e.code === 'BILLING_JOURNAL_STORE_UNSUPPORTED')
      ?.kind
  ).toBe('diagnostic')
})

it('finds shipped DFLOP task plugins without asserting deployment availability or relying on model catalog access', () => {
  const results = searchDocumentation(
    'DFLOP',
    articles,
    reference,
    [],
    (x) => x
  )
  expect(
    results.some((result) =>
      result.href.startsWith('/docs/provider-plugin-dflop')
    )
  ).toBe(true)
  expect(
    reference.taskPlugins.every((plugin) =>
      plugin.source.startsWith('plugins/tasks/')
    )
  ).toBe(true)
})
