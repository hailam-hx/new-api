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
import type { ParsedTaskTier } from '@/features/pricing/lib/billing-expr'
import {
  getDynamicPricingSummary,
  getDynamicPriceEntries,
  formatDynamicUnitPrice,
  formatTaskUsageUnitPrice,
  isUnconfiguredTaskUsageModel,
  type DynamicPriceEntry,
  type DynamicPriceOptions,
} from '@/features/pricing/lib/dynamic-price'
import { withPluginPricing } from '@/features/pricing/lib/plugin-pricing'
import { formatPrice, formatRequestPrice } from '@/features/pricing/lib/price'
import type { PricingModel } from '@/features/pricing/types'

export type DocumentationPrice = {
  state: 'priced' | 'missing' | 'details'
  entries: DynamicPriceEntry[]
  variable: boolean
}

/** A display of published selling prices, never an evaluator of billing expressions. */
export function getDocumentationPrice(
  model: PricingModel,
  options: DynamicPriceOptions & {
    selectedGroup?: string
    includeCacheRead?: boolean
  }
): DocumentationPrice {
  const unavailable: DocumentationPrice = {
    state: 'missing',
    entries: [],
    variable: false,
  }
  if (
    !Number.isFinite(options.groupRatioMultiplier ?? 1) ||
    (options.groupRatioMultiplier ?? 1) < 0
  ) {
    return unavailable
  }
  if (
    model.billing_mode === 'tiered_expr' &&
    !model.billing_expr?.trim() &&
    !model.billing_plugin_variants?.length
  ) {
    return unavailable
  }
  if (isUnconfiguredTaskUsageModel(model)) return unavailable
  if (
    model.billing_plugin_variants?.some(
      (variant) =>
        getDocumentationPrice(withPluginPricing(model, variant), options)
          .state !== 'priced'
    )
  ) {
    return { ...unavailable, state: 'details' }
  }
  if (
    model.billing_mode &&
    !['ratio', 'tiered_expr'].includes(model.billing_mode)
  ) {
    return { ...unavailable, state: 'details' }
  }
  const summary = getDynamicPricingSummary(model, {
    ...options,
    showCurrencySymbol: false,
  })
  if (!summary) {
    if (
      model.billing_mode === 'tiered_expr' ||
      model.billing_plugin_variants?.length
    ) {
      return unavailable
    }
    // Legacy image settlement applies request option multipliers absent from the public payload.
    if (model.supported_endpoint_types?.includes('image-generation')) {
      return { ...unavailable, state: 'details' }
    }
    if (model.quota_type === 1) {
      if (
        typeof model.model_price !== 'number' ||
        !Number.isFinite(model.model_price) ||
        model.model_price < 0
      ) {
        return unavailable
      }
      return {
        state: 'priced',
        variable: false,
        entries: [
          {
            key: 'fixed',
            field: 'fixedPrice',
            label: 'Price',
            shortLabel: 'Price',
            labelKind: 'i18n',
            unit: 'request',
            value: model.model_price,
            formatted: formatRequestPrice(
              model,
              false,
              options.priceRate,
              options.usdExchangeRate,
              options.selectedGroup,
              false
            ),
          },
        ],
      }
    }
    if (
      model.quota_type !== 0 ||
      !Number.isFinite(model.model_ratio) ||
      model.model_ratio < 0 ||
      !Number.isFinite(model.model_ratio * model.completion_ratio) ||
      !Number.isFinite(model.completion_ratio) ||
      model.completion_ratio < 0
    ) {
      return unavailable
    }
    const priceTypes: ('input' | 'output' | 'cache')[] = ['input', 'output']
    if (
      options.includeCacheRead &&
      typeof model.cache_ratio === 'number' &&
      Number.isFinite(model.cache_ratio) &&
      model.cache_ratio >= 0 &&
      Number.isFinite(model.model_ratio * model.cache_ratio)
    ) {
      priceTypes.push('cache')
    }
    return {
      state: 'priced',
      variable: false,
      entries: priceTypes.map((type) => ({
        key: type,
        field: {
          input: 'inputPrice',
          output: 'outputPrice',
          cache: 'cacheReadPrice',
        }[type],
        label: {
          input: 'Input',
          output: 'Output',
          cache: 'Cached token price',
        }[type],
        shortLabel: {
          input: 'Input',
          output: 'Output',
          cache: 'Cached token price',
        }[type],
        labelKind: 'i18n',
        unit: 'token',
        value:
          model.model_ratio *
          {
            input: 1,
            output: model.completion_ratio,
            cache: model.cache_ratio ?? 0,
          }[type],
        formatted: formatPrice(
          model,
          type,
          'M',
          false,
          options.priceRate,
          options.usdExchangeRate,
          options.selectedGroup,
          false
        ),
      })),
    }
  }
  if (
    summary.isSpecialExpression ||
    summary.hasRequestRules ||
    summary.hasUnconfiguredProviders
  ) {
    return { ...unavailable, state: 'details' }
  }
  for (const tier of summary.tiers) {
    const fields =
      'unitPrices' in tier
        ? [
            ...Object.values((tier as ParsedTaskTier).unitPrices),
            (tier as ParsedTaskTier).constant,
          ]
        : Object.entries(tier)
            .filter(([key]) => key.endsWith('Price'))
            .map(([, value]) => value)
    if (
      fields.some(
        (value) =>
          typeof value === 'number' && (!Number.isFinite(value) || value < 0)
      )
    ) {
      return unavailable
    }
  }
  const sourceEntries =
    model.billing_plugin_variants?.length || summary.isTimePricing
      ? summary.entries
      : summary.tiers.flatMap((tier) =>
          getDynamicPriceEntries(tier, {
            ...options,
            showCurrencySymbol: false,
            usageSchema: model.billing_usage_schema,
          })
        )
  const ranges = new Map<string, DynamicPriceEntry>()
  for (const entry of sourceEntries) {
    if (!Number.isFinite(entry.value) || entry.value < 0) return unavailable
    const key = `${entry.field}:${entry.unit}`
    const previous = ranges.get(key)
    const min = Math.min(
      previous?.minValue ?? Infinity,
      entry.minValue ?? entry.value
    )
    const max = Math.max(
      previous?.maxValue ?? -Infinity,
      entry.maxValue ?? entry.value
    )
    if (!Number.isFinite(min) || !Number.isFinite(max) || min < 0) {
      return unavailable
    }
    const format =
      entry.unit === 'token' && entry.variable
        ? formatDynamicUnitPrice
        : formatTaskUsageUnitPrice
    ranges.set(key, {
      ...entry,
      key,
      value: min,
      minValue: min,
      maxValue: max,
      formatted: format(min, { ...options, showCurrencySymbol: false }),
    })
  }
  const entries = [...ranges.values()]
  if (
    entries.some(
      (entry) =>
        !Number.isFinite(entry.value * (options.groupRatioMultiplier ?? 1)) ||
        /NaN|Infinity|undefined|∞/.test(entry.formatted)
    )
  ) {
    return unavailable
  }
  if (!entries.length) return { ...unavailable, state: 'details' }
  const variable = entries.some((entry) => entry.minValue !== entry.maxValue)
  // Only expose a separate read price when every published tier has one.
  // Missing tier data must not become a free cached-input price.
  const showCacheRead =
    options.includeCacheRead &&
    summary.tiers.every(
      (tier) =>
        !('unitPrices' in tier) &&
        typeof tier.cacheReadPrice === 'number' &&
        Number.isFinite(tier.cacheReadPrice) &&
        tier.cacheReadPrice >= 0
    )
  // Preserve non-token terms (including a base task fee); cache writes stay in detail.
  const primary = entries.filter(
    (entry) =>
      entry.unit !== 'token' ||
      !entry.variable ||
      ['inputPrice', 'outputPrice'].includes(entry.field) ||
      (showCacheRead && entry.field === 'cacheReadPrice')
  )
  return {
    state: 'priced',
    entries: primary.length ? primary : entries,
    variable,
  }
}
