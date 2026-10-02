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
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import type { PricingModel } from '@/features/pricing/types'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { getDocumentationModelKinds } from './model-kind'
import { getDocumentationPrice } from './pricing-display'

export function PricingGuide({ models }: { models: PricingModel[] }) {
  const { t } = useTranslation()
  const currency = useSystemConfigStore((state) => state.config.currency)
  const units = new Map<string, Set<string>>()
  for (const model of models) {
    const price = getDocumentationPrice(model, { tokenUnit: 'M' })
    for (const kind of getDocumentationModelKinds(model)) {
      const current = units.get(kind) ?? new Set<string>()
      for (const entry of price.entries) current.add(entry.unit)
      units.set(kind, current)
    }
  }
  const tokenBased = models.some((model) =>
    getDocumentationPrice(model, { tokenUnit: 'M' }).entries.some(
      (entry) => entry.unit === 'token'
    )
  )
  const variation = models.some(
    (model) =>
      new Set(
        model.enable_groups
          .map((group) => model.group_ratio?.[group])
          .filter((value) => typeof value === 'number')
      ).size > 1
  )
  const labels = {
    text: t('Chat'),
    image: t('Image'),
    video: t('Video'),
    audio: t('Audio'),
  }
  const unitLabels: Record<string, string> = {
    token: t('1M token'),
    image: t('image'),
    second: t('second'),
    request: t('request'),
    count: t('unit'),
    credit: t('credit'),
    character: t('character'),
  }
  return (
    <section id='read-prices' className='mt-8 scroll-mt-32 space-y-4'>
      <h2 className='text-xl font-semibold'>{t('How to read prices')}</h2>
      <p className='text-muted-foreground text-sm'>
        {t('Prices use the billing unit shown for each model.')}
      </p>
      <dl className='grid grid-cols-[auto_minmax(0,1fr)] gap-x-6 gap-y-2 text-sm'>
        {(['text', 'image', 'video', 'audio'] as const).map((kind) => {
          const modelUnits = units.get(kind)
          let caption = t('See the unit beside each price')
          if (modelUnits?.size) {
            caption = [...modelUnits]
              .map((unit) => unitLabels[unit] ?? t('Model-specific unit'))
              .join(' / ')
          } else if (kind === 'text' && tokenBased) caption = t('1M token')
          return (
            <div key={kind} className='contents'>
              <dt>{labels[kind]}</dt>
              <dd className='text-muted-foreground'>{caption}</dd>
            </div>
          )
        })}
      </dl>
      <Alert>
        <AlertDescription>
          {t('These are the usage prices HOTX API applies to users.')}
          {variation && (
            <p>
              {t(
                'Your final price may differ if your account has discounts or a special pricing policy.'
              )}
            </p>
          )}
        </AlertDescription>
      </Alert>
      {currency.quotaDisplayType === 'CUSTOM' &&
        currency.customCurrencySymbol === '积分' && (
          <p className='text-muted-foreground text-sm'>
            {t('Points are the payment unit used in your HOTX API account.')}{' '}
            <Button
              variant='link'
              className='h-auto p-0'
              render={
                <Link to='/docs/$slug' params={{ slug: 'console-wallet' }} />
              }
            >
              {t('Balance & Top-up')} →
            </Button>
          </p>
        )}
    </section>
  )
}

export function PricingUsageLink() {
  const { t } = useTranslation()
  return (
    <section id='actual-cost' className='mt-6 space-y-2'>
      <p className='text-muted-foreground text-sm'>
        {t('The prices shown are current HOTX API prices and may be updated.')}
      </p>
      <Alert>
        <AlertDescription>
          {t('See the actual cost of each request in Usage Logs.')}{' '}
          <Button
            variant='link'
            className='h-auto p-0'
            render={
              <Link to='/usage-logs/$section' params={{ section: 'common' }} />
            }
          >
            {t('View Usage & Logs')} →
          </Button>
        </AlertDescription>
      </Alert>
    </section>
  )
}
