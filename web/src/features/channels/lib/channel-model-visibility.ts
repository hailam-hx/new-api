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
import { getModels } from '@/features/models/api'
import type { Model, ModelSquareState } from '@/features/models/types'
import { requireServerSuccess } from '@/lib/server-error-message'

export type ChannelModelVisibilityFilter = 'all' | 'visible' | 'hidden'
export type ChannelModelVisibility = Record<string, ModelSquareState>

/** Use resolved backend states so inherited display rules have the same meaning here. */
export async function loadChannelModels(): Promise<Model[]> {
  const items: Model[] = []
  let loaded = 0
  for (let page = 1; ; page++) {
    const response = requireServerSuccess(
      await getModels({
        include_channel_models: true,
        p: page,
        page_size: 100,
      })
    )
    const data = response.data
    if (!data || (data.items.length === 0 && loaded < data.total)) {
      throw new Error('Failed to load models')
    }
    items.push(...data.items)
    loaded += data.items.length
    if (loaded >= data.total) return items
  }
}

export async function loadChannelModelVisibility(): Promise<ChannelModelVisibility> {
  const states: ChannelModelVisibility = {}
  for (const model of await loadChannelModels()) {
    if (model.name_rule === 0 && model.square_state) {
      states[model.model_name] = model.square_state
    }
  }
  return states
}

export function filterChannelModels(
  models: string[],
  search: string,
  visibility: ChannelModelVisibilityFilter,
  states: ChannelModelVisibility | undefined
): string[] {
  const keyword = search.toLowerCase()
  return models.filter(
    (model) =>
      model.toLowerCase().includes(keyword) &&
      (visibility === 'all' || states?.[model] === visibility)
  )
}
