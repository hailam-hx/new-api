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
import type { PricingModel } from '@/features/pricing/types'

export type DocumentationModelKind = 'text' | 'image' | 'video' | 'audio'

const endpointKinds: Record<string, DocumentationModelKind> = {
  openai: 'text',
  'openai-response': 'text',
  'openai-response-compact': 'text',
  'openai-alpha-search': 'text',
  anthropic: 'text',
  gemini: 'text',
  embeddings: 'text',
  'jina-rerank': 'text',
  'image-generation': 'image',
  'openai-video': 'video',
}

/** Supplied metadata only; names never imply a model category. */
export function getDocumentationModelKinds(
  model: PricingModel
): DocumentationModelKind[] {
  const kinds = new Set<DocumentationModelKind>()
  for (const modality of model.output_modalities ?? []) {
    if (modality !== 'file') kinds.add(modality)
  }
  for (const endpoint of model.supported_endpoint_types ?? []) {
    const kind = endpointKinds[endpoint]
    if (kind) kinds.add(kind)
  }
  if (model.input_modalities?.includes('audio')) kinds.add('audio')
  if (model.capabilities?.includes('embeddings')) kinds.add('text')
  return (['text', 'image', 'video', 'audio'] as const).filter((kind) =>
    kinds.has(kind)
  )
}

export function filterDocumentationModels(
  models: PricingModel[],
  query: string,
  kind: DocumentationModelKind | 'all'
): PricingModel[] {
  const search = query.trim().toLowerCase()
  return models.filter((model) => {
    if (kind !== 'all' && !getDocumentationModelKinds(model).includes(kind)) {
      return false
    }
    return (
      !search ||
      [model.model_name, model.vendor_name, model.description]
        .join(' ')
        .toLowerCase()
        .includes(search)
    )
  })
}
