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

import { CopyButton } from '@/components/copy-button'

import { getArticle } from './content'

export function DocumentationOverview(props: { origin: string }) {
  const { t } = useTranslation()
  const steps = [
    { slug: 'api-key', title: '1. Create an API Key' },
    { slug: 'models', title: '2. Choose a model' },
    { slug: 'quickstart', title: '3. Send a request' },
  ]
  return (
    <div className='mt-9 space-y-10'>
      <section id='get-started' className='scroll-mt-40'>
        <h2 className='mb-4 text-xl font-semibold'>{t('Start in 3 steps')}</h2>
        <ol
          aria-label={t('Start in 3 steps')}
          className='grid gap-3 sm:grid-cols-3'
        >
          {steps.map((step) => (
            <li key={step.slug}>
              <Link
                to='/docs/$slug'
                params={{ slug: step.slug }}
                className='hover:bg-muted block h-full rounded-xl border p-4 text-sm font-medium'
              >
                {t(step.title)}
              </Link>
            </li>
          ))}
        </ol>
      </section>
      <section id='you-need' className='scroll-mt-40'>
        <h2 className='mb-4 text-xl font-semibold'>
          {t('You need three things')}
        </h2>
        <dl className='grid grid-cols-[auto_minmax(0,1fr)] items-center gap-x-4 gap-y-4 rounded-xl border p-5 text-sm'>
          <dt className='text-muted-foreground'>{t('Base URL')}</dt>
          <dd className='flex min-w-0 items-center gap-2'>
            <code className='break-all'>{props.origin}/v1</code>
            <CopyButton value={`${props.origin}/v1`} size='sm' />
          </dd>
          <dt className='text-muted-foreground'>{t('API Key')}</dt>
          <dd>
            <Link
              to='/keys'
              className='text-primary underline underline-offset-4'
            >
              {t('Create API Key')}
            </Link>
          </dd>
          <dt className='text-muted-foreground'>{t('Model ID')}</dt>
          <dd>
            <Link
              to='/pricing'
              className='text-primary underline underline-offset-4'
            >
              {t('Model Square')}
            </Link>
          </dd>
        </dl>
      </section>
      <section id='popular-guides' className='scroll-mt-40'>
        <h2 className='mb-4 text-xl font-semibold'>{t('Popular guides')}</h2>
        <div className='grid gap-3 sm:grid-cols-2'>
          {['quickstart', 'text-chat', 'image', 'hotx-media'].map((slug) => {
            const guide = getArticle(slug)
            if (!guide) return null
            return (
              <Link
                key={slug}
                to='/docs/$slug'
                params={{ slug }}
                className='hover:bg-muted flex flex-col gap-2 rounded-xl border p-5 transition-colors'
              >
                <span className='font-medium'>{t(guide.title)}</span>
                <span className='text-muted-foreground text-sm leading-6'>
                  {t(guide.description)}
                </span>
              </Link>
            )
          })}
        </div>
      </section>
    </div>
  )
}
