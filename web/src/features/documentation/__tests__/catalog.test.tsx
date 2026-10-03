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
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it } from 'vitest'

import type { PricingModel } from '@/features/pricing/types'
import { useAuthStore } from '@/stores/auth-store'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import {
  DocumentationModelPrice,
  ModelCatalog,
  ModelDocument,
} from '../catalog'
import {
  filterDocumentationModels,
  getDocumentationModelKinds,
  normalizeDocumentationProvider,
  getDocumentationProvider,
} from '../model-kind'

const model: PricingModel = {
  id: 1,
  model_name: 'sample',
  vendor_name: 'Example',
  quota_type: 0,
  model_ratio: 1,
  completion_ratio: 3,
  enable_groups: ['default', 'premium'],
  group_ratio: { default: 1, premium: 2 },
}
const data = {
  models: [model],
  vendors: [],
  groupRatio: {},
  usableGroup: {},
  endpointMap: {},
  autoGroups: [],
  isLoading: false,
  error: null,
  refetch: async () => ({}),
  priceRate: 1,
  usdExchangeRate: 1,
}
const previousCurrency = useSystemConfigStore.getState().config.currency

afterEach(() => {
  cleanup()
  useAuthStore.getState().auth.reset()
  useSystemConfigStore.getState().setConfig({ currency: previousCurrency })
})

it('keeps unknown models in All without inventing a category from their name', () => {
  const unknown = { ...model, model_name: 'video-image-reasoning' }
  expect(getDocumentationModelKinds(unknown)).toEqual([])
  expect(filterDocumentationModels([unknown], 'IMAGE', 'all')).toEqual([
    unknown,
  ])
  expect(filterDocumentationModels([unknown], '', 'video')).toEqual([])
})

it('filters by supplied modalities, capabilities and registered endpoint metadata', () => {
  const text = {
    ...model,
    model_name: 'chat',
    supported_endpoint_types: ['openai'],
  }
  const image: PricingModel = {
    ...model,
    model_name: 'image',
    output_modalities: ['image'],
  }
  const video = {
    ...model,
    model_name: 'video',
    supported_endpoint_types: ['openai-video'],
  }
  const audio: PricingModel = {
    ...model,
    model_name: 'audio',
    output_modalities: ['audio'],
  }
  expect(
    filterDocumentationModels([text, image, video, audio], '', 'audio')
  ).toEqual([audio])
  expect(getDocumentationModelKinds(image)).toEqual(['image'])
  expect(getDocumentationModelKinds(video)).toEqual(['video'])
  expect(filterDocumentationModels([text, image], ' Example ', 'text')).toEqual(
    [text]
  )
})

it('shows signed-in customer prices with exact units and no ratio controls', () => {
  useSystemConfigStore
    .getState()
    .setConfig({ currency: DEFAULT_CURRENCY_CONFIG })
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'viewer', role: 1, group: 'premium' })
  render(<DocumentationModelPrice model={model} data={data} />)
  expect(screen.getByText('4')).toBeVisible()
  expect(screen.getByText('12')).toBeVisible()
  expect(screen.getByText('USD / 1M tokens')).toBeVisible()
  expect(screen.queryByText('From')).not.toBeInTheDocument()
  expect(screen.queryByText(/ratio|multiplier/i)).not.toBeInTheDocument()
})

it('labels public prices From and preserves zero per-request prices', () => {
  useSystemConfigStore
    .getState()
    .setConfig({ currency: DEFAULT_CURRENCY_CONFIG })
  render(
    <DocumentationModelPrice
      model={{ ...model, quota_type: 1, model_price: 0 }}
      data={data}
    />
  )
  expect(screen.getByText('From')).toBeVisible()
  expect(screen.getByText('0')).toBeVisible()
  expect(screen.getByText('USD / request')).toBeVisible()
})

it('shows final dynamic prices with localized human labels instead of usage names', () => {
  useSystemConfigStore
    .getState()
    .setConfig({ currency: DEFAULT_CURRENCY_CONFIG })
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'viewer', role: 1, group: 'premium' })
  render(
    <DocumentationModelPrice
      model={{
        ...model,
        billing_mode: 'tiered_expr',
        billing_expr: 'tier("video", u("seconds") * 0.000012)',
        billing_usage_schema: {
          seconds: {
            type: 'number',
            unit: 'second',
            description: { en: 'Video generation unit price' },
          },
        },
      }}
      data={data}
    />
  )
  expect(screen.getByText('Video generation unit price')).toBeVisible()
  expect(screen.getByText('$0.000024/s')).toBeVisible()
  expect(screen.queryByText('seconds')).not.toBeInTheDocument()
  expect(screen.queryByText(/tier\(|u\(/)).not.toBeInTheDocument()
})

it('shows an unavailable-price message for expressions that cannot be expanded', () => {
  render(
    <DocumentationModelPrice
      model={{
        ...model,
        billing_mode: 'tiered_expr',
        billing_expr: 'max(u("seconds"), 3) * 0.1',
      }}
      data={data}
    />
  )
  expect(
    screen.getByText(
      'Pricing details are unavailable. Contact support before using this model.'
    )
  ).toBeVisible()
  expect(
    screen.queryByText(/max\(|Special billing expression/)
  ).not.toBeInTheDocument()
})

it('shows four columns and applies keyboard type selection and empty search state', async () => {
  const user = userEvent.setup()
  const root = createRootRoute({
    component: () => (
      <ModelCatalog
        data={{
          ...data,
          models: [
            {
              ...model,
              model_name: 'chat',
              supported_endpoint_types: ['openai'],
            },
            {
              ...model,
              model_name: 'movie',
              supported_endpoint_types: ['openai-video'],
            },
          ],
        }}
      />
    ),
  })
  const router = createRouter({
    routeTree: root,
    history: createMemoryHistory(),
  })
  await router.load()
  render(<RouterProvider router={router} />)
  expect(
    screen.getAllByRole('columnheader').map((head) => head.textContent)
  ).toEqual(['Model ID', 'Provider', 'Type', 'Price'])
  const video = screen.getByRole('button', { name: 'Video' })
  video.focus()
  await user.keyboard('{Enter}')
  expect(video).toHaveAttribute('aria-pressed', 'true')
  expect(screen.queryByRole('link', { name: 'chat' })).not.toBeInTheDocument()
  expect(screen.getByRole('link', { name: 'movie' })).toBeVisible()
  await user.type(
    screen.getByRole('textbox', { name: 'Search models' }),
    'absent'
  )
  expect(screen.getByText('No models found')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Next' })).toBeDisabled()
  await user.click(screen.getByRole('button', { name: 'Reset filters' }))
  expect(screen.getByRole('link', { name: 'chat' })).toBeVisible()
  expect(screen.getByRole('textbox', { name: 'Search models' })).toHaveValue('')
})

it('shows final customer prices for every request tier without internal labels', () => {
  useSystemConfigStore
    .getState()
    .setConfig({ currency: DEFAULT_CURRENCY_CONFIG })
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'viewer', role: 1, group: 'premium' })
  render(
    <DocumentationModelPrice
      detail
      model={{
        ...model,
        billing_mode: 'tiered_expr',
        billing_expr:
          'len < 1000 ? tier("internal-small", p * 2 + c * 8) : tier("internal-large", p * 4 + c * 12)',
      }}
      data={data}
    />
  )
  expect(screen.getByText('Input tokens < 1,000')).toBeVisible()
  expect(screen.getByText('$8/1M token')).toBeVisible()
  expect(screen.getByText('$24/1M token')).toBeVisible()
  expect(
    screen.queryByText(/internal-small|internal-large|tier\(/)
  ).not.toBeInTheDocument()
})

it('does not publish a base price as final when request conditions can change it', () => {
  render(
    <DocumentationModelPrice
      model={{
        ...model,
        billing_mode: 'tiered_expr',
        billing_expr:
          'tier("base", p * 2 + c * 8)|||when(header("x-fast") == "true") * 2',
      }}
      data={data}
    />
  )
  expect(
    screen.getByText(
      'Pricing details are unavailable. Contact support before using this model.'
    )
  ).toBeVisible()
  expect(screen.queryByText(/x-fast|multiplier|when\(/)).not.toBeInTheDocument()
})

it('keeps image and video understanding models out of generation filters', () => {
  const chat: PricingModel = {
    ...model,
    input_modalities: ['text', 'image', 'video'],
    output_modalities: ['text'],
    capabilities: ['vision'],
  }
  expect(getDocumentationModelKinds(chat)).toEqual(['text'])
  const generator: PricingModel = {
    ...model,
    input_modalities: ['text'],
    output_modalities: ['image'],
  }
  expect(getDocumentationModelKinds(generator)).toEqual(['image'])
})

it.each<{
  name: string
  metadata: Partial<PricingModel>
  guide?: string
  path?: string
}>([
  {
    name: 'chat',
    metadata: { supported_endpoint_types: ['openai'] },
    guide: 'Quickstart',
    path: '/docs/quickstart',
  },
  {
    name: 'embeddings only',
    metadata: {
      supported_endpoint_types: ['embeddings'],
      capabilities: ['embeddings'],
      output_modalities: ['text'],
    },
  },
  {
    name: 'image generation',
    metadata: { supported_endpoint_types: ['image-generation'] },
    guide: 'Image',
    path: '/docs/image',
  },
  {
    name: 'video generation',
    metadata: { supported_endpoint_types: ['openai-video'] },
    guide: 'Video',
    path: '/docs/video',
  },
  {
    name: 'speech generation',
    metadata: { output_modalities: ['audio'] },
    guide: 'Audio',
    path: '/docs/audio',
  },
  {
    name: 'transcription',
    metadata: { input_modalities: ['audio'], output_modalities: ['text'] },
    guide: 'Audio',
    path: '/docs/audio',
  },
])(
  'links $name models only to their supported beginner examples',
  async (testCase) => {
    const root = createRootRoute({
      component: () => (
        <ModelDocument
          modelId={model.model_name}
          data={{ ...data, models: [{ ...model, ...testCase.metadata }] }}
        />
      ),
    })
    const router = createRouter({
      routeTree: root,
      history: createMemoryHistory(),
    })
    await router.load()
    render(<RouterProvider router={router} />)
    if (testCase.guide) {
      expect(
        screen.getByRole('button', { name: testCase.guide })
      ).toHaveAttribute('href', testCase.path)
    }
    if (testCase.guide !== 'Quickstart') {
      expect(
        screen.queryByRole('button', { name: 'Quickstart' })
      ).not.toBeInTheDocument()
    }
  }
)

it('keeps embeddings, reranking and gateway-wide protocol claims out of Chat', () => {
  expect(
    getDocumentationModelKinds({
      ...model,
      supported_endpoint_types: ['embeddings', 'openai'],
    })
  ).toEqual(['embeddings'])
  expect(
    getDocumentationModelKinds({
      ...model,
      supported_endpoint_types: ['jina-rerank'],
    })
  ).toEqual(['rerank'])
  expect(
    getDocumentationModelKinds({
      ...model,
      supported_endpoint_types: ['openai', 'anthropic', 'gemini'],
    })
  ).toEqual([])
  expect(
    getDocumentationModelKinds({
      ...model,
      output_modalities: ['image'],
      supported_endpoint_types: ['openai'],
    })
  ).toEqual(['image'])
  expect(
    getDocumentationModelKinds({
      ...model,
      input_modalities: ['audio'],
      output_modalities: ['text'],
    })
  ).toEqual(['audio'])
})

it('normalizes provider metadata and searches aliases without rewriting Model IDs', () => {
  const aliased = { ...model, model_name: 'EXACT-ID', vendor_name: '字节跳动' }
  expect(normalizeDocumentationProvider(aliased.vendor_name)).toBe('ByteDance')
  expect(normalizeDocumentationProvider('阿里巴巴')).toBe('Alibaba / Qwen')
  expect(normalizeDocumentationProvider('智谱')).toBe('Zhipu AI')
  expect(normalizeDocumentationProvider('Moonshot')).toBe('Moonshot AI')
  expect(
    filterDocumentationModels([aliased], 'bytedance', 'all')[0]?.model_name
  ).toBe('EXACT-ID')
})

it('deduplicates published IDs, excludes empty enabled groups and sorts by ID', () => {
  const a = { ...model, model_name: 'a' }
  expect(
    filterDocumentationModels(
      [
        { ...model, model_name: 'z' },
        a,
        a,
        { ...model, model_name: 'disabled', enable_groups: [] },
      ],
      '',
      'all'
    ).map((item) => item.model_name)
  ).toEqual(['a', 'z'])
})

it('paginates 25 results, resets after search and copies the raw ID', async () => {
  const user = userEvent.setup()
  const root = createRootRoute({
    component: () => (
      <ModelCatalog
        data={{
          ...data,
          models: Array.from({ length: 26 }, (_, i) => ({
            ...model,
            model_name: `sample-${String(i).padStart(2, '0')}`,
            vendor_name: 'Moonshot',
          })),
        }}
      />
    ),
  })
  const router = createRouter({
    routeTree: root,
    history: createMemoryHistory(),
  })
  await router.load()
  render(<RouterProvider router={router} />)
  expect(
    screen.queryByRole('link', { name: 'sample-25' })
  ).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Next' }))
  expect(screen.getByRole('link', { name: 'sample-25' })).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Copy model ID' }))
  expect(await navigator.clipboard.readText()).toBe('sample-25')
  await user.type(
    screen.getByRole('textbox', { name: 'Search models' }),
    'sample-00'
  )
  expect(screen.getByRole('link', { name: 'sample-00' })).toBeVisible()
  expect(screen.getByRole('button', { name: 'Previous' })).toBeDisabled()
  expect(screen.getByRole('status')).toHaveTextContent('1 Models')
})

it('shows a pricing-focused table without Provider or Context and uses number before currency', async () => {
  useSystemConfigStore.getState().setConfig({
    currency: {
      ...DEFAULT_CURRENCY_CONFIG,
      quotaDisplayType: 'CUSTOM',
      customCurrencySymbol: '积分',
      customCurrencyExchangeRate: 2,
    },
  })
  const root = createRootRoute({
    component: () => (
      <ModelCatalog
        pricing
        data={{
          ...data,
          models: [
            {
              ...model,
              enable_groups: ['default'],
              billing_mode: 'tiered_expr',
              billing_expr: 'tier("base", p * 1.5 + c * 6)',
            },
          ],
        }}
      />
    ),
  })
  const router = createRouter({
    routeTree: root,
    history: createMemoryHistory(),
  })
  await router.load()
  render(<RouterProvider router={router} />)
  expect(
    screen.getAllByRole('columnheader').map((el) => el.textContent)
  ).toEqual(['Model', 'Type', 'Price'])
  expect(screen.getByText('3 Points / 1M token')).toBeVisible()
  expect(screen.getByText('12 Points / 1M token')).toBeVisible()
  expect(screen.queryByText('From')).not.toBeInTheDocument()
  expect(screen.getByText('Input')).toBeVisible()
  expect(screen.getByText('Output')).toBeVisible()
  expect(
    screen.queryByText(/tier\(|ModelRatio|CompletionRatio|GroupRatio/)
  ).not.toBeInTheDocument()
})

it('classifies gateway Chat and Image from explicit usage while keeping ambiguous tasks unknown', () => {
  const gateway = [
    'openai',
    'openai-response',
    'openai-response-compact',
    'anthropic',
    'gemini',
    'openai-alpha-search',
  ]
  const chat: PricingModel = {
    ...model,
    model_name: 'opaque-chat-id',
    supported_endpoint_types: gateway,
    billing_mode: 'tiered_expr',
    billing_expr: 'tier("base", p * 3 + c * 15 + cr * 0.3)',
  }
  const image: PricingModel = {
    ...chat,
    model_name: 'opaque-media-id',
    billing_expr: 'tier("base", fixed(0.2)) * image_count',
  }
  expect(getDocumentationModelKinds(chat)).toEqual(['text'])
  expect(getDocumentationModelKinds(image)).toEqual(['image'])
  expect(filterDocumentationModels([chat, image], '', 'text')).toEqual([chat])
  expect(filterDocumentationModels([chat, image], '', 'image')).toEqual([image])
  expect(
    getDocumentationModelKinds({ ...chat, output_modalities: ['video'] })
  ).toEqual(['video'])
  expect(
    getDocumentationModelKinds({
      ...chat,
      supported_endpoint_types: ['embeddings'],
    })
  ).toEqual(['embeddings'])
  expect(
    getDocumentationModelKinds({
      ...chat,
      billing_expr: 'tier("base", u("seconds") * 0.5)',
      billing_usage_schema: {
        seconds: {
          type: 'number',
          unit: 'second',
          description: { en: 'Usage unit price' },
        },
      },
    })
  ).toEqual([])
})

it('uses the shared provider fallback for missing metadata and preserves configured ownership', () => {
  const claude = {
    ...model,
    model_name: 'claude-fable-5',
    vendor_name: undefined,
  }
  expect(getDocumentationProvider(claude)).toBe('Anthropic')
  expect(
    getDocumentationProvider({ ...claude, vendor_name: 'Custom Provider' })
  ).toBe('Custom Provider')
  expect(
    getDocumentationProvider({ ...claude, model_name: 'nano-banana' })
  ).toBe('Google')
  expect(
    getDocumentationProvider({ ...claude, model_name: 'tvod-qwen-image' })
  ).toBe('Alibaba / Qwen')
  expect(
    getDocumentationProvider({ ...claude, model_name: 'unknown-model' })
  ).toBe('')
  expect(filterDocumentationModels([claude], 'anthropic', 'all')).toEqual([
    claude,
  ])
})

it('classifies token-billed plugin video from its callable protocol and includes it in the Video filter', () => {
  const video: PricingModel = {
    ...model,
    model_name: 'doubao-seedance-2.0',
    supported_endpoint_types: ['openai', 'anthropic', 'gemini', 'openai-video'],
    billing_mode: 'tiered_expr',
    billing_expr: 'tier("1080p", u("completion_tokens") * 7.65 / 1000000)',
    billing_usage_schema: {
      completion_tokens: {
        type: 'number',
        unit: 'token',
        description: { en: 'Video generation token unit price' },
      },
    },
  }
  expect(getDocumentationModelKinds(video)).toEqual(['video'])
  expect(filterDocumentationModels([video], 'seedan', 'video')).toEqual([video])
  expect(filterDocumentationModels([video], '', 'text')).toEqual([])
})
