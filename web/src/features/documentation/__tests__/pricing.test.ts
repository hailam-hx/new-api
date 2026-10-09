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
import { expect, it } from 'vitest'

import type { PricingModel } from '@/features/pricing/types'

import { getDocumentationPrice } from '../pricing-display'

const base: PricingModel = {
  id: 1,
  model_name: 'sample',
  enable_groups: ['default'],
  group_ratio: { default: 1 },
  quota_type: 0,
  model_ratio: 1,
  completion_ratio: 2,
}
const options = { tokenUnit: 'M' as const, groupRatioMultiplier: 1 }
it('does not turn missing, negative or unconfigured prices into free prices', () => {
  expect(
    getDocumentationPrice(
      { ...base, quota_type: 1, model_price: undefined },
      options
    ).state
  ).toBe('missing')
  expect(
    getDocumentationPrice({ ...base, quota_type: 1, model_price: -1 }, options)
      .state
  ).toBe('missing')
  expect(
    getDocumentationPrice(
      { ...base, billing_mode: 'tiered_expr', billing_expr: '' },
      options
    ).state
  ).toBe('missing')
  expect(
    getDocumentationPrice({ ...base, model_ratio: Number.NaN }, options).state
  ).toBe('missing')
})
it('preserves explicitly configured free prices and nonzero micro-prices', () => {
  const free = getDocumentationPrice(
    {
      ...base,
      billing_mode: 'tiered_expr',
      billing_expr: 'tier("free", fixed(0))',
    },
    options
  )
  expect(free.state).toBe('priced')
  expect(free.entries[0]?.value).toBe(0)
  expect(free.variable).toBe(false)
  const micro = getDocumentationPrice(
    {
      ...base,
      billing_mode: 'tiered_expr',
      billing_expr: 'tier("micro", fixed(0.000001))',
    },
    options
  )
  expect(micro.entries[0]?.formatted).not.toBe('0')
})
it('uses actual image and task units and marks only distinct prices variable', () => {
  const image = getDocumentationPrice(
    {
      ...base,
      billing_mode: 'tiered_expr',
      billing_expr: 'tier("image", fixed(0.2)) * image_count',
    },
    options
  )
  expect(image.entries[0]?.unit).toBe('image')
  expect(image.variable).toBe(false)
  const video = getDocumentationPrice(
    {
      ...base,
      billing_mode: 'tiered_expr',
      billing_expr:
        'u("resolution") == "1080p" ? tier("high", u("seconds") * 0.4) : tier("low", u("seconds") * 0.2)',
      billing_usage_schema: {
        seconds: {
          type: 'number',
          unit: 'second',
          description: { en: 'Video generation unit price' },
        },
        resolution: { enum: ['720p', '1080p'] },
      },
    },
    options
  )
  expect(video.entries[0]?.unit).toBe('second')
  expect(video.variable).toBe(true)
  expect(video.entries[0]?.value).toBe(0.2)
  const same = getDocumentationPrice(
    {
      ...base,
      billing_mode: 'tiered_expr',
      billing_expr:
        'len < 1000 ? tier("a",p * 2+c * 6) : tier("b",p * 2+c * 6)',
    },
    options
  )
  expect(same.variable).toBe(false)
})
it('shows Details for unsupported expressions and request-dependent multipliers', () => {
  expect(
    getDocumentationPrice(
      { ...base, billing_mode: 'tiered_expr', billing_expr: 'max(p, 100) * 2' },
      options
    ).state
  ).toBe('details')
  expect(
    getDocumentationPrice(
      {
        ...base,
        billing_mode: 'tiered_expr',
        billing_expr:
          'tier("base",p * 2)|||when(header("x-fast") == "true") * 2',
      },
      options
    ).state
  ).toBe('details')
})

it.each(['second', 'count', 'token', 'credit', 'character'] as const)(
  'keeps the declared %s task unit and configured price',
  (unit) => {
    const view = getDocumentationPrice(
      {
        ...base,
        billing_mode: 'tiered_expr',
        billing_expr:
          unit === 'token'
            ? 'tier("base", u("quantity") * 0.5 / 1000000)'
            : 'tier("base", u("quantity") * 0.5)',
        billing_usage_schema: {
          quantity: {
            type: 'number',
            unit,
            description: { en: 'Usage unit price' },
          },
        },
      },
      options
    )
    expect(view.state).toBe('priced')
    expect(view.entries[0]?.unit).toBe(unit)
    expect(view.entries[0]?.value).toBe(0.5)
  }
)
it('uses configured viewer pricing in legacy mode and rejects overflow', () => {
  const view = getDocumentationPrice(
    { ...base, enable_groups: ['premium'], group_ratio: { premium: 2 } },
    { ...options, selectedGroup: 'premium', groupRatioMultiplier: 2 }
  )
  expect(view.entries[0]?.formatted).toBe('4')
  expect(
    getDocumentationPrice(
      { ...base, model_ratio: 1e300, completion_ratio: 1e300 },
      options
    ).state
  ).toBe('missing')
  expect(
    getDocumentationPrice(
      {
        ...base,
        billing_mode: 'tiered_expr',
        billing_expr: 'tier("invalid", p * -1 + c * 2)',
      },
      options
    ).state
  ).toBe('details')
})

it('does not present a legacy image base price as a final per-request price', () => {
  expect(
    getDocumentationPrice(
      {
        ...base,
        quota_type: 1,
        model_price: 0.2,
        supported_endpoint_types: ['image-generation'],
      },
      options
    ).state
  ).toBe('details')
})

it.each([0.1, 0])(
  'includes explicit legacy cache read ratio %s using the shared viewer price',
  (cache_ratio) => {
    const view = getDocumentationPrice(
      {
        ...base,
        cache_ratio,
        create_cache_ratio: 1.25,
        enable_groups: ['premium'],
        group_ratio: { premium: 2 },
      },
      {
        ...options,
        selectedGroup: 'premium',
        groupRatioMultiplier: 2,
        includeCacheRead: true,
      }
    )
    expect(view.entries.map((e) => e.field)).toEqual([
      'inputPrice',
      'outputPrice',
      'cacheReadPrice',
    ])
    expect(view.entries[2].formatted).toBe(cache_ratio === 0 ? '0' : '0.4')
  }
)
it.each([undefined, null, -1, Number.NaN, Infinity])(
  'omits missing or invalid cache ratio %s without changing input/output',
  (cache_ratio) => {
    const view = getDocumentationPrice(
      { ...base, cache_ratio },
      { ...options, includeCacheRead: true }
    )
    expect(view.entries.map((e) => e.field)).toEqual([
      'inputPrice',
      'outputPrice',
    ])
  }
)
it('includes dynamic cache read pricing but excludes cache creation and incomplete cache tiers', () => {
  const model = {
    ...base,
    billing_mode: 'tiered_expr',
    billing_expr: 'tier("base",p*3+c*15+cr*0+cc*3.75+cc1h*6)',
  }
  const price = getDocumentationPrice(model, {
    ...options,
    includeCacheRead: true,
  })
  expect(price.entries.map((e) => e.field)).toEqual([
    'inputPrice',
    'outputPrice',
    'cacheReadPrice',
  ])
  expect(price.entries[2].formatted).toBe('0')
  expect(
    getDocumentationPrice(model, options).entries.map((e) => e.field)
  ).toEqual(['inputPrice', 'outputPrice'])
  const partial = {
    ...model,
    billing_expr: 'len < 1000 ? tier("a",p*3+c*15+cr*0.3) : tier("b",p*6+c*30)',
  }
  expect(
    getDocumentationPrice(partial, {
      ...options,
      includeCacheRead: true,
    }).entries.some((e) => e.field === 'cacheReadPrice')
  ).toBe(false)
})
