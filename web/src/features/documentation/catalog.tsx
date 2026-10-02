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
import { ModelPriceCell } from '@/features/pricing/components/model-price-cell'
import { useBillingTime } from '@/features/pricing/hooks/use-billing-time'
import type { ParsedTaskTier } from '@/features/pricing/lib/billing-expr'
import { formatBillingCondition } from '@/features/pricing/lib/billing-expression/condition-display'
import {
  getDynamicDisplayGroupRatio,
  getDynamicPriceEntries,
  getDynamicPriceUnitLabelKey,
  getDynamicPricingSummary,
  getDynamicPricingTiers,
  hasDynamicRequestRules,
  isDynamicPricingModel,
} from '@/features/pricing/lib/dynamic-price'
import { withPluginPricing } from '@/features/pricing/lib/plugin-pricing'
import {
  taskPriceLabel,
  taskTierConditions,
  taskUsageUnitLabel,
} from '@/features/pricing/lib/task-price-display'
import type { PricingModel } from '@/features/pricing/types'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import {
  filterDocumentationModels,
  getDocumentationModelKinds,
  type DocumentationModelKind,
} from './model-kind'

type CatalogData = {
  models: PricingModel[]
  priceRate: number
  usdExchangeRate: number
}

export function ModelCatalog(props: { data: CatalogData; matrix?: boolean }) {
  const { t, i18n } = useTranslation()
  const [query, setQuery] = useState('')
  const [kind, setKind] = useState<DocumentationModelKind | 'all'>('all')
  const [page, setPage] = useState(0)
  const models = useMemo(
    () => filterDocumentationModels(props.data.models, query, kind),
    [props.data.models, query, kind]
  )
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const unknown = t('Not provided')
  const lastPage = Math.max(0, Math.ceil(models.length / 40) - 1)
  const currentPage = Math.min(page, lastPage)
  const labels = {
    all: t('All'),
    text: t('Text'),
    image: t('Image'),
    video: t('Video'),
    audio: t('Audio'),
  }

  return (
    <section className='mt-8 min-w-0 space-y-4' id='live-catalog'>
      <h2 className='text-xl font-semibold'>{t('Models')}</h2>
      <Input
        aria-label={t('Search models')}
        placeholder={t('Search models')}
        value={query}
        onChange={(event) => {
          setQuery(event.target.value)
          setPage(0)
        }}
      />
      <div
        role='group'
        aria-label={t('Model type')}
        className='flex flex-wrap gap-2'
      >
        {(['all', 'text', 'image', 'video', 'audio'] as const).map((value) => (
          <Button
            key={value}
            variant={kind === value ? 'secondary' : 'outline'}
            size='sm'
            aria-pressed={kind === value}
            onClick={() => {
              setKind(value)
              setPage(0)
            }}
          >
            {labels[value]}
          </Button>
        ))}
      </div>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Copy a model ID to use it in your app. Check that your API key can access it.'
        )}
      </p>
      <StaticDataTable
        data={models.slice(currentPage * 40, (currentPage + 1) * 40)}
        getRowKey={(model) => model.model_name}
        emptyContent={t('No models found')}
        tableClassName='min-w-[640px]'
        columns={[
          {
            id: 'model',
            header: t('Model'),
            cell: (model) => (
              <div className='flex items-start gap-1'>
                <Link
                  to='/docs/models/$modelId'
                  params={{ modelId: model.model_name }}
                  className='max-w-64 font-mono text-xs break-all underline underline-offset-4'
                >
                  {model.model_name}
                </Link>
                <CopyButton
                  value={model.model_name}
                  size='sm'
                  aria-label={t('Copy model ID')}
                />
              </div>
            ),
          },
          {
            id: 'provider',
            header: t('Provider'),
            cell: (model) => model.vendor_name || unknown,
          },
          {
            id: 'type',
            header: t('Type'),
            cell: (model) =>
              getDocumentationModelKinds(model)
                .map((value) => labels[value])
                .join(', ') || unknown,
          },
          {
            id: 'context',
            header: t('Context'),
            cell: (model) =>
              model.context_length &&
              Number.isFinite(model.context_length) &&
              model.context_length > 0
                ? formatNumber(model.context_length, locale)
                : unknown,
          },
          {
            id: 'price',
            header: t('Price'),
            cell: (model) => (
              <DocumentationModelPrice model={model} data={props.data} />
            ),
          },
        ]}
      />
      <p className='text-muted-foreground text-xs'>
        {t(
          'From shows the lowest available price. Your API key and request options can affect the cost. Check Usage Logs for the amount charged.'
        )}
      </p>
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
          'Choose a model from the catalog, then copy its model ID.'
        )}
        action={
          <Button
            render={<Link to='/docs/$slug' params={{ slug: 'models' }} />}
          >
            {t('Models')}
          </Button>
        }
      />
    )
  }
  const labels = {
    text: t('Text'),
    image: t('Image'),
    video: t('Video'),
    audio: t('Audio'),
  }
  const types = getDocumentationModelKinds(model)
  const capabilityLabels = {
    function_calling: t('Tool Calling'),
    streaming: t('Streaming'),
    vision: t('Image input'),
    json_mode: t('JSON mode'),
    structured_output: t('Structured Output'),
    reasoning: t('Reasoning'),
    tools: t('Tools'),
    system_prompt: t('System prompt'),
    web_search: t('Web search'),
    code_interpreter: t('Code interpreter'),
    caching: t('Caching'),
    embeddings: t('Embeddings'),
  }
  const unknown = t('Not provided')
  const fields = [
    { label: t('Model ID'), value: model.model_name },
    { label: t('Provider'), value: model.vendor_name || unknown },
    {
      label: t('Used for'),
      value: types.map((kind) => labels[kind]).join(', ') || unknown,
    },
    {
      label: t('Context'),
      value:
        model.context_length &&
        Number.isFinite(model.context_length) &&
        model.context_length > 0
          ? formatNumber(model.context_length, locale)
          : unknown,
    },
    {
      label: t('Capabilities'),
      value:
        model.capabilities
          ?.map((capability) => capabilityLabels[capability] ?? unknown)
          .join(', ') || unknown,
    },
  ]
  return (
    <div className='min-w-0 space-y-8'>
      <div className='flex items-start gap-2'>
        <h1 className='min-w-0 font-mono text-2xl font-semibold break-all'>
          {model.model_name}
        </h1>
        <CopyButton value={model.model_name} aria-label={t('Copy model ID')} />
      </div>
      <dl className='divide-border divide-y'>
        {fields.map((field) => (
          <div
            key={field.label}
            className='grid grid-cols-[minmax(0,1fr)_minmax(0,2fr)] gap-4 py-3 text-sm'
          >
            <dt className='text-muted-foreground'>{field.label}</dt>
            <dd className='break-words'>{field.value}</dd>
          </div>
        ))}
      </dl>
      <section id='model-pricing' className='scroll-mt-40 space-y-4'>
        <h2 className='text-xl font-semibold'>{t('Pricing')}</h2>
        <DocumentationModelPrice model={model} data={props.data} detail />
        <p className='text-muted-foreground text-sm'>
          {t(
            'From shows the lowest available price. Your API key and request options can affect the cost. Check Usage Logs for the amount charged.'
          )}
        </p>
      </section>
      <section id='model-examples' className='scroll-mt-40 space-y-3'>
        <h2 className='text-xl font-semibold'>{t('Example')}</h2>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Copy the model ID above, then follow the request example for its supported API.'
          )}
        </p>
        {model.supported_endpoint_types?.includes('openai') && (
          <Button
            variant='outline'
            render={<Link to='/docs/$slug' params={{ slug: 'quickstart' }} />}
          >
            {t('Quickstart')}
          </Button>
        )}
        {model.supported_endpoint_types?.includes('image-generation') && (
          <Button
            variant='outline'
            render={<Link to='/docs/$slug' params={{ slug: 'image' }} />}
          >
            {t('Image')}
          </Button>
        )}
        {model.supported_endpoint_types?.includes('openai-video') && (
          <Button
            variant='outline'
            render={<Link to='/docs/$slug' params={{ slug: 'video' }} />}
          >
            {t('Video')}
          </Button>
        )}
        {types.includes('audio') && (
          <Button
            variant='outline'
            render={<Link to='/docs/$slug' params={{ slug: 'audio' }} />}
          >
            {t('Audio')}
          </Button>
        )}
      </section>
    </div>
  )
}

/** Composes published pricing APIs without exposing billing source or multipliers. */
export function DocumentationModelPrice(props: {
  model: PricingModel
  data: CatalogData
  detail?: boolean
}) {
  const { t, i18n } = useTranslation()
  const userGroup = useAuthStore((state) => state.auth.user?.group)
  useSystemConfigStore((state) => state.config.currency)
  const selectedGroup =
    userGroup && props.model.enable_groups.includes(userGroup)
      ? userGroup
      : undefined
  const billingTime = useBillingTime(props.model.billing_expr)
  const options = {
    tokenUnit: 'M' as const,
    priceRate: props.data.priceRate,
    usdExchangeRate: props.data.usdExchangeRate,
    groupRatioMultiplier: getDynamicDisplayGroupRatio(
      props.model,
      selectedGroup
    ),
    now: billingTime === undefined ? undefined : new Date(billingTime),
  }
  // Subscribe above because the shared price formatter reads currency from its store.
  const summary = getDynamicPricingSummary(props.model, options)
  if (props.model.billing_plugin_variants?.length && props.detail) {
    return (
      <div className='space-y-4'>
        {props.model.billing_plugin_variants.map((variant) => (
          <section key={variant.plugin_key} className='space-y-2'>
            <h3 className='text-sm font-medium'>{variant.plugin_name}</h3>
            <DocumentationModelPrice
              model={withPluginPricing(props.model, variant)}
              data={props.data}
              detail
            />
          </section>
        ))}
      </div>
    )
  }
  if (!isDynamicPricingModel(props.model)) {
    return (
      <div className='space-y-1'>
        {!selectedGroup && (
          <span className='text-muted-foreground text-xs'>{t('From')}</span>
        )}
        <ModelPriceCell
          model={props.model}
          options={{
            priceRate: props.data.priceRate,
            usdExchangeRate: props.data.usdExchangeRate,
            selectedGroup,
          }}
          showExpression={false}
        />
      </div>
    )
  }
  if (
    !summary ||
    summary.isSpecialExpression ||
    hasDynamicRequestRules(props.model) ||
    !summary.entries.length
  ) {
    return (
      <p className='text-muted-foreground text-xs'>
        {t(
          'Pricing details are unavailable. Contact support before using this model.'
        )}
      </p>
    )
  }
  const entries = props.detail ? summary.entries : summary.primaryEntries
  const prices = entries.length ? entries : summary.entries
  return (
    <div className='space-y-2'>
      {!selectedGroup && (
        <span className='text-muted-foreground text-xs'>{t('From')}</span>
      )}
      <dl className='space-y-1'>
        {prices.map((entry) => {
          const unitKey = getDynamicPriceUnitLabelKey(entry)
          const unit = taskUsageUnitLabel(
            entry,
            i18n.language,
            unitKey ? t(unitKey) : t('1M token')
          )
          const label =
            entry.labelKind === 'schema'
              ? taskPriceLabel(entry.description, t('Price'), i18n.language)
              : t(entry.label)
          return (
            <div key={entry.key} className='flex flex-wrap gap-x-2 text-xs'>
              <dt className='text-muted-foreground'>{label}</dt>
              <dd className='font-mono tabular-nums'>
                {entry.formattedRange ?? entry.formatted}/{unit}
              </dd>
            </div>
          )
        })}
      </dl>
      {summary.tierCount > 1 && !props.detail && (
        <span className='text-muted-foreground text-xs'>
          {t('Price varies by request options. Open the model for details.')}
        </span>
      )}
      {props.detail &&
        !props.model.billing_plugin_variants?.length &&
        getDynamicPricingTiers(props.model).length > 1 && (
          <div className='space-y-3'>
            {getDynamicPricingTiers(props.model).map((tier) => {
              let condition = t('Other cases')
              if ('unitPrices' in tier) {
                condition =
                  taskTierConditions(
                    tier as ParsedTaskTier,
                    props.model.billing_usage_schema,
                    i18n.language,
                    t
                  ) || condition
              } else if (tier.conditionText) {
                condition =
                  formatBillingCondition(
                    tier.conditionText,
                    t,
                    toIntlLocale(i18n.resolvedLanguage || i18n.language)
                  ) || t('Request-specific price')
              } else if (tier.conditions.length) {
                condition = tier.conditions
                  .map((item) => {
                    const label =
                      item.var === 'c' ? t('Output tokens') : t('Input tokens')
                    return `${label} ${item.op} ${formatNumber(item.value, toIntlLocale(i18n.resolvedLanguage || i18n.language))}`
                  })
                  .join(' · ')
              }
              // Unknown conditions must never leak expression source into beginner docs.
              if (
                /\b(?:u|hour|weekday|month|day|header|body)\s*\(/.test(
                  condition
                )
              ) {
                condition = t('Request-specific price')
              }
              return (
                <section
                  key={`${tier.label}:${tier.conditionText ?? JSON.stringify(tier.conditions)}`}
                  className='rounded-lg border p-3'
                >
                  <h3 className='mb-2 text-xs font-medium'>{condition}</h3>
                  <dl className='space-y-1'>
                    {getDynamicPriceEntries(tier, {
                      ...options,
                      usageSchema: props.model.billing_usage_schema,
                    }).map((entry) => {
                      const unitKey = getDynamicPriceUnitLabelKey(entry)
                      const unit = taskUsageUnitLabel(
                        entry,
                        i18n.language,
                        unitKey ? t(unitKey) : t('1M token')
                      )
                      const label =
                        entry.labelKind === 'schema'
                          ? taskPriceLabel(
                              entry.description,
                              t('Price'),
                              i18n.language
                            )
                          : t(entry.label)
                      return (
                        <div
                          key={entry.key}
                          className='flex flex-wrap gap-x-2 text-xs'
                        >
                          <dt className='text-muted-foreground'>{label}</dt>
                          <dd className='font-mono tabular-nums'>
                            {entry.formatted}/{unit}
                          </dd>
                        </div>
                      )
                    })}
                  </dl>
                </section>
              )
            })}
          </div>
        )}
    </div>
  )
}
