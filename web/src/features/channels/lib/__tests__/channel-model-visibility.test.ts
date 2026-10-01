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
import { afterEach, expect, test, vi } from 'vitest'

import { getModels } from '@/features/models/api'
import type { Model } from '@/features/models/types'

import {
  filterChannelModels,
  loadChannelModelVisibility,
} from '../channel-model-visibility'

vi.mock('@/features/models/api', () => ({ getModels: vi.fn() }))
afterEach(() => vi.resetAllMocks())

const row = (
  model_name: string,
  square_state: Model['square_state'],
  name_rule = 0
) => ({ model_name, square_state, name_rule }) as Model

test('loads every page and uses backend visibility for concrete models including inherited rules', async () => {
  vi.mocked(getModels)
    .mockResolvedValueOnce({
      success: true,
      data: {
        items: [row('prefix-', 'partial', 1), row('shown', 'visible')],
        total: 4,
        page: 1,
        page_size: 2,
      },
    })
    .mockResolvedValueOnce({
      success: true,
      data: {
        items: [row('prefix-hidden', 'hidden'), row('offline', 'unavailable')],
        total: 4,
        page: 2,
        page_size: 2,
      },
    })
  const states = await loadChannelModelVisibility()
  expect(getModels).toHaveBeenNthCalledWith(2, {
    include_channel_models: true,
    p: 2,
    page_size: 100,
  })
  expect(states).toEqual({
    shown: 'visible',
    'prefix-hidden': 'hidden',
    offline: 'unavailable',
  })
})

test('combines search and visibility without misclassifying unavailable or unknown models', () => {
  const models = ['shown', 'prefix-hidden', 'offline', 'unknown']
  const states = {
    shown: 'visible',
    'prefix-hidden': 'hidden',
    offline: 'unavailable',
  } as const
  expect(filterChannelModels(models, '', 'all', undefined)).toEqual(models)
  expect(filterChannelModels(models, '', 'visible', states)).toEqual(['shown'])
  expect(filterChannelModels(models, 'PREFIX', 'hidden', states)).toEqual([
    'prefix-hidden',
  ])
  expect(filterChannelModels(models, 'shown', 'hidden', states)).toEqual([])
  expect(filterChannelModels(models, '', 'hidden', undefined)).toEqual([])
})

test('rejects failed or incomplete pages instead of exposing partial visibility', async () => {
  vi.mocked(getModels).mockResolvedValueOnce({
    success: false,
    message: 'Denied',
  })
  await expect(loadChannelModelVisibility()).rejects.toThrow('Denied')
  vi.mocked(getModels).mockResolvedValueOnce({
    success: true,
    data: { items: [], total: 10, page: 1, page_size: 100 },
  })
  await expect(loadChannelModelVisibility()).rejects.toThrow()
})
