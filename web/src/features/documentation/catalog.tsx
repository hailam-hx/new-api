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
import { Link } from '@tanstack/react-router'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { StaticDataTable } from '@/components/data-table'
import { EmptyState } from '@/components/empty-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Markdown } from '@/components/ui/markdown'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { DynamicPricingBreakdown } from '@/features/pricing/components/dynamic-pricing-breakdown'
import { ModelPriceCell } from '@/features/pricing/components/model-price-cell'
import type { usePricingData } from '@/features/pricing/hooks/use-pricing-data'
import type { PricingModel } from '@/features/pricing/types'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

import { filterCatalog } from './lib'

type CatalogData = ReturnType<typeof usePricingData>

export function ModelCatalog(props: { data: CatalogData; matrix?: boolean }) {
  const { t, i18n } = useTranslation()
  const [query, setQuery] = useState('')
  const [capability, setCapability] = useState('')
  const [vendor, setVendor] = useState('')
  const [group, setGroup] = useState('')
  const [page, setPage] = useState(0)
  const models = filterCatalog(
    props.data.models,
    query,
    capability,
    vendor
  ).filter(
    (model) =>
      !group ||
      model.enable_groups.includes(group) ||
      model.enable_groups.includes('all')
  )
  const vendors = useMemo(
    () =>
      [
        ...new Set(
          props.data.models
            .map((model) => model.vendor_name)
            .filter((name): name is string => !!name)
        ),
      ].sort(),
    [props.data.models]
  )
  const capabilities = useMemo(
    () =>
      [
        ...new Set(
          props.data.models.flatMap((model) => [
            ...(model.capabilities ?? []),
            ...(model.input_modalities ?? []),
            ...(model.output_modalities ?? []),
            ...(model.supported_endpoint_types ?? []),
          ])
        ),
      ].sort(),
    [props.data.models]
  )
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const unknown = t('Not provided')
  const lastPage = Math.max(0, Math.ceil(models.length / 40) - 1)
  const currentPage = Math.min(page, lastPage)
  return (
    <section className='mt-8 min-w-0 space-y-4' id='live-catalog'>
      <h2 className='text-xl font-semibold'>{t('Live catalog')}</h2>
      <div className='flex flex-wrap gap-2'>
        <Input
          className='min-w-40 flex-1'
          aria-label={t('Search models')}
          placeholder={t('Search models')}
          value={query}
          onChange={(event) => {
            setQuery(event.target.value)
            setPage(0)
          }}
        />
        <Select
          value={vendor || 'all'}
          onValueChange={(value) => {
            setVendor(value === 'all' ? '' : String(value))
            setPage(0)
          }}
        >
          <SelectTrigger className='w-40' aria-label={t('Provider')}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              <SelectItem value='all'>{t('All providers')}</SelectItem>
              {vendors.map((name) => (
                <SelectItem key={name} value={name}>
                  {name}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
        <Select
          value={capability || 'all'}
          onValueChange={(value) => {
            setCapability(value === 'all' ? '' : String(value))
            setPage(0)
          }}
        >
          <SelectTrigger className='w-44' aria-label={t('Capabilities')}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              <SelectItem value='all'>{t('All capabilities')}</SelectItem>
              {capabilities.map((name) => (
                <SelectItem key={name} value={name}>
                  {t(name)}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
        <Select
          value={group || 'base'}
          onValueChange={(value) =>
            setGroup(value === 'base' ? '' : String(value))
          }
        >
          <SelectTrigger className='w-40' aria-label={t('Group')}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              <SelectItem value='base'>{t('Lowest group price')}</SelectItem>
              {Object.keys(props.data.usableGroup).map((name) => (
                <SelectItem key={name} value={name}>
                  {name}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
      </div>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Only supplied metadata can be filtered. Listed models still require API key access.'
        )}
      </p>
      <StaticDataTable
        data={models.slice(currentPage * 40, (currentPage + 1) * 40)}
        getRowKey={(model) => model.model_name}
        emptyContent={t('No models found')}
        tableClassName='min-w-[900px]'
        columns={[
          {
            id: 'model',
            header: t('Model'),
            cell: (model) => (
              <div className='flex items-start gap-1'>
                <Link
                  to='/docs/models/$modelId'
                  params={{ modelId: model.model_name }}
                  className='font-mono text-xs underline underline-offset-4'
                >
                  {model.model_name}
                </Link>
                <CopyButton value={model.model_name} size='sm' />
              </div>
            ),
          },
          {
            id: 'provider',
            header: t('Provider'),
            cell: (model) => model.vendor_name ?? unknown,
          },
          {
            id: 'category',
            header: t('Category'),
            cell: () => unknown,
          },
          {
            id: 'apis',
            header: t('Supported APIs'),
            cell: (model) => (
              <span className='text-xs whitespace-normal'>
                {model.supported_endpoint_types?.join(', ') || unknown}
              </span>
            ),
          },
          {
            id: 'input',
            header: t('Input modalities'),
            cell: (model) => model.input_modalities?.join(', ') || unknown,
          },
          {
            id: 'output',
            header: t('Output modalities'),
            cell: (model) => model.output_modalities?.join(', ') || unknown,
          },
          {
            id: 'context',
            header: t('Context window'),
            cell: (model) =>
              model.context_length === undefined
                ? unknown
                : formatNumber(model.context_length, locale),
          },
          {
            id: 'capabilities',
            header: t('Capabilities'),
            cell: (model) => (
              <span className='text-xs whitespace-normal'>
                {model.capabilities?.join(', ') || unknown}
              </span>
            ),
          },
          {
            id: 'pricing',
            header: t('Pricing'),
            cell: (model) => (
              <ModelPriceCell
                model={model}
                options={{
                  priceRate: props.data.priceRate,
                  usdExchangeRate: props.data.usdExchangeRate,
                  selectedGroup: group,
                }}
              />
            ),
          },
          {
            id: 'status',
            header: t('Status'),
            cell: () => t('Listed in catalog'),
          },
        ]}
      />
      <div className='flex items-center justify-between gap-2'>
        <p className='text-muted-foreground text-xs'>
          {formatNumber(models.length, locale)} {t('Models')}
        </p>
        <div className='flex gap-2'>
          <Button
            variant='outline'
            size='sm'
            disabled={currentPage === 0}
            onClick={() => setPage(currentPage - 1)}
          >
            {t('Previous')}
          </Button>
          <Button
            variant='outline'
            size='sm'
            disabled={currentPage === lastPage}
            onClick={() => setPage(currentPage + 1)}
          >
            {t('Next')}
          </Button>
        </div>
      </div>
    </section>
  )
}

export function ModelDocument(props: { modelId: string; data: CatalogData }) {
  const { t, i18n } = useTranslation()
  const model = props.data.models.find(
    (item) => item.model_name === props.modelId
  )
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  if (!model) {
    return (
      <EmptyState
        title={t('Model not found')}
        description={t(
          'This model is not present in your visible catalog. Check access settings or choose another model.'
        )}
        action={
          <Button
            render={<Link to='/docs/$slug' params={{ slug: 'models' }} />}
          >
            {t('Model Catalog')}
          </Button>
        }
      />
    )
  }
  const unknown = t('Not provided')
  const related = props.data.models
    .filter(
      (item) =>
        item.model_name !== model.model_name &&
        model.vendor_id !== undefined &&
        item.vendor_id === model.vendor_id
    )
    .slice(0, 6)
  const fields: [string, string][] = [
    ['Provider', model.vendor_name ?? unknown],
    ['Model ID', model.model_name],
    ['Supported APIs', model.supported_endpoint_types?.join(', ') || unknown],
    ['Capabilities', model.capabilities?.join(', ') || unknown],
    [
      'Context window',
      model.context_length === undefined
        ? unknown
        : formatNumber(model.context_length, locale),
    ],
    ['Input modalities', model.input_modalities?.join(', ') || unknown],
    ['Output modalities', model.output_modalities?.join(', ') || unknown],
    ['Status', t('Listed in catalog')],
  ]
  return (
    <div className='min-w-0 space-y-8'>
      <div className='flex items-center gap-2'>
        <h1 className='min-w-0 font-mono text-2xl font-semibold break-all'>
          {model.model_name}
        </h1>
        <CopyButton value={model.model_name} />
      </div>
      {model.description && <Markdown>{model.description}</Markdown>}
      <dl className='divide-border divide-y'>
        {fields.map(([label, value]) => (
          <div
            key={label}
            className='grid grid-cols-[minmax(0,1fr)_minmax(0,2fr)] gap-4 py-3 text-sm'
          >
            <dt className='text-muted-foreground'>{t(label)}</dt>
            <dd className='break-all'>{value}</dd>
          </div>
        ))}
      </dl>
      <section id='model-pricing' className='scroll-mt-40'>
        <h2 className='mb-4 text-xl font-semibold'>{t('Pricing')}</h2>
        <ModelPriceCell
          model={model}
          options={{
            priceRate: props.data.priceRate,
            usdExchangeRate: props.data.usdExchangeRate,
          }}
        />
        <ModelPricingBreakdown model={model} />
        <Button
          variant='link'
          render={
            <Link
              to='/pricing/$modelId'
              params={{ modelId: model.model_name }}
            />
          }
        >
          {t('View pricing')}
        </Button>
      </section>
      <section id='model-examples' className='scroll-mt-40 space-y-3'>
        <h2 className='text-xl font-semibold'>{t('Request examples')}</h2>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Copy this model ID into NEW_API_MODEL, then use the SDK guide for a supported API.'
          )}
        </p>
        <div className='flex flex-wrap gap-2'>
          {(model.supported_endpoint_types ?? []).map((endpoint) => (
            <Link
              key={endpoint}
              to='/docs/$slug'
              params={{ slug: sdkForEndpoint(endpoint) }}
              className='text-primary text-sm underline underline-offset-4'
            >
              {endpoint}
            </Link>
          ))}
        </div>
      </section>
      <section id='model-limitations' className='scroll-mt-40 space-y-3'>
        <h2 className='text-xl font-semibold'>{t('Limitations')}</h2>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Catalog presence does not guarantee availability for your key. Missing metadata is unknown. Response shapes depend on the selected API and provider.'
          )}
        </p>
        <Link
          to='/docs/$slug'
          params={{ slug: 'api-reference' }}
          className='text-primary text-sm underline'
        >
          {t('Response schemas')}
        </Link>
      </section>
      {related.length > 0 && (
        <section id='related-models' className='scroll-mt-40'>
          <h2 className='mb-4 text-xl font-semibold'>{t('Related models')}</h2>
          <ul className='space-y-2'>
            {related.map((item) => (
              <li key={item.model_name}>
                <Link
                  to='/docs/models/$modelId'
                  params={{ modelId: item.model_name }}
                  className='text-primary font-mono text-sm underline'
                >
                  {item.model_name}
                </Link>
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  )
}

function sdkForEndpoint(endpoint: string): string {
  if (endpoint === 'anthropic') return 'sdk-anthropic'
  if (endpoint === 'gemini') return 'sdk-gemini'
  if (endpoint === 'openai') return 'sdk-openai'
  return 'api-reference'
}

function ModelPricingBreakdown(props: { model: PricingModel }) {
  const variants = props.model.billing_plugin_variants ?? []
  if (variants.length) {
    return (
      <div className='mt-4 space-y-4'>
        {variants.map((variant) => (
          <section key={variant.plugin_key}>
            <h3 className='mb-2 text-sm font-medium'>{variant.plugin_name}</h3>
            <DynamicPricingBreakdown
              billingExpr={variant.billing_expr}
              usageSchema={variant.billing_usage_schema}
            />
          </section>
        ))}
      </div>
    )
  }
  if (!props.model.billing_expr) return null
  return (
    <div className='mt-4'>
      <DynamicPricingBreakdown
        billingExpr={props.model.billing_expr}
        usageSchema={props.model.billing_usage_schema}
      />
    </div>
  )
}
