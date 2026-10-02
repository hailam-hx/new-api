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
import type { ParsedTier } from '@/features/pricing/lib/billing-expr'
import { getDynamicPricingTiers } from '@/features/pricing/lib/dynamic-price'
import type { PricingModel } from '@/features/pricing/types'
import { resolveModelProvider } from '@/lib/model-provider'

export type DocumentationModelKind =
  | 'text'
  | 'image'
  | 'video'
  | 'audio'
  | 'embeddings'
  | 'rerank'
  | 'moderation'
export type DocumentationModelFilter =
  | 'all'
  | 'text'
  | 'image'
  | 'video'
  | 'audio'

const providerAliases: Record<string, string> = {
  gemini: 'Google',
  qwen: 'Alibaba / Qwen',
  doubao: 'ByteDance',
  zhipu: 'Zhipu AI',
  kling: 'Kuaishou',
  字节跳动: 'ByteDance',
  阿里巴巴: 'Alibaba / Qwen',
  智谱: 'Zhipu AI',
  moonshot: 'Moonshot AI',
  腾讯: 'Tencent',
  百度: 'Baidu',
  讯飞: 'iFlytek',
  快手: 'Kuaishou',
  零一万物: '01.AI',
}

/** Normalize supplied vendor names only; never derive ownership from Model IDs. */
export function normalizeDocumentationProvider(name?: string): string {
  const value = name?.trim() || ''
  return providerAliases[value.toLowerCase()] ?? value
}

/** Configured ownership wins; reuse the shared model-family display fallback. */
export function getDocumentationProvider(model: PricingModel): string {
  return normalizeDocumentationProvider(
    model.vendor_name?.trim() || resolveModelProvider(model.model_name)?.name
  )
}

/** Model modalities outrank protocol compatibility, which can be gateway-wide. */
export function getDocumentationModelKinds(
  model: PricingModel
): DocumentationModelKind[] {
  const endpoints = model.supported_endpoint_types ?? []
  const kinds = new Set<DocumentationModelKind>()
  if (
    model.capabilities?.includes('embeddings') ||
    endpoints.includes('embeddings')
  ) {
    kinds.add('embeddings')
  }
  if (endpoints.includes('jina-rerank')) kinds.add('rerank')
  if (endpoints.includes('moderation')) kinds.add('moderation')
  if (kinds.size) return [...kinds]
  for (const modality of model.output_modalities ?? []) {
    if (modality !== 'file') kinds.add(modality)
  }
  if (model.input_modalities?.includes('audio')) {
    kinds.delete('text')
    kinds.add('audio')
  }
  if (!kinds.size) {
    if (endpoints.includes('image-generation')) kinds.add('image')
    if (endpoints.includes('openai-video')) kinds.add('video')
    if (
      endpoints.some((endpoint) =>
        ['audio-speech', 'audio-transcription', 'audio-translation'].includes(
          endpoint
        )
      )
    ) {
      kinds.add('audio')
    }
    // Gateway protocol lists are broad. Parsed output/image usage provides a
    // narrower fallback without guessing a model family from its name.
    if (!kinds.size) {
      const tiers = getDynamicPricingTiers(model).filter(
        (tier): tier is ParsedTier => !('unitPrices' in tier)
      )
      if (
        tiers.some(
          (tier) => tier.imageCount || tier.imageOutputPrice !== undefined
        )
      ) {
        kinds.add('image')
      }
      if (
        tiers.some(
          (tier) =>
            tier.audioInputPrice !== undefined ||
            tier.audioOutputPrice !== undefined
        )
      ) {
        kinds.add('audio')
      }
      if (
        !kinds.size &&
        tiers.some((tier) => tier.outputPrice !== undefined) &&
        endpoints.some((endpoint) =>
          [
            'openai',
            'openai-response',
            'openai-response-compact',
            'anthropic',
            'gemini',
          ].includes(endpoint)
        )
      ) {
        kinds.add('text')
      }
    }
    // Protocol compatibility alone remains insufficient for broad gateway lists.
    if (
      !kinds.size &&
      endpoints.length &&
      !(
        endpoints.includes('openai') &&
        endpoints.includes('anthropic') &&
        endpoints.includes('gemini')
      ) &&
      endpoints.every((endpoint) =>
        [
          'openai',
          'openai-response',
          'openai-response-compact',
          'anthropic',
          'gemini',
        ].includes(endpoint)
      )
    ) {
      kinds.add('text')
    }
  }
  return (
    [
      'text',
      'image',
      'video',
      'audio',
      'embeddings',
      'rerank',
      'moderation',
    ] as const
  ).filter((kind) => kinds.has(kind))
}

export function filterDocumentationModels(
  models: PricingModel[],
  query: string,
  kind: DocumentationModelFilter
): PricingModel[] {
  const search = query.trim().toLowerCase()
  const seen = new Set<string>()
  return models
    .filter((model) => {
      if (
        !model.model_name.trim() ||
        !model.enable_groups?.length ||
        seen.has(model.model_name)
      ) {
        return false
      }
      seen.add(model.model_name)
      if (kind !== 'all' && !getDocumentationModelKinds(model).includes(kind)) {
        return false
      }
      return (
        !search ||
        [
          model.model_name,
          model.vendor_name,
          getDocumentationProvider(model),
          model.description,
        ]
          .join(' ')
          .toLowerCase()
          .includes(search)
      )
    })
    .sort((a, b) => a.model_name.localeCompare(b.model_name, 'en'))
}
