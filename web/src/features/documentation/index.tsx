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
import { Link, useRouterState } from '@tanstack/react-router'
import { ArrowRight } from 'lucide-react'
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { ErrorState } from '@/components/error-state'
import { PublicLayout } from '@/components/layout'
import { LoadingState } from '@/components/loading-state'
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb'
import { Button } from '@/components/ui/button'
import { Markdown } from '@/components/ui/markdown'
import { useApiInfo } from '@/features/dashboard/hooks/use-status-data'
import { usePricingData } from '@/features/pricing/hooks/use-pricing-data'
import { useStatus } from '@/hooks/use-status'
import { getModuleAccessFromStatus } from '@/lib/nav-modules'
import { useAuthStore } from '@/stores/auth-store'

import { ModelCatalog, ModelDocument } from './catalog'
import { CodeExamples } from './code-examples'
import {
  articles,
  beginnerArticles,
  getArticle,
  translateDocSection,
} from './content'
import { DocumentationMetadata } from './metadata'
import { DocsNavigation, DocsSearch } from './navigation'
import { ApiReference, ErrorReference, CommonErrors } from './reference'
import { TaskExamples } from './task-examples'

export function Documentation(props: { slug?: string; modelId?: string }) {
  const { t } = useTranslation()
  const status = useStatus()
  const { items: apiAddresses } = useApiInfo()
  const user = useAuthStore((state) => state.auth.user)
  const access = getModuleAccessFromStatus(
    status.status as Record<string, unknown> | null,
    'pricing'
  )
  const canLoadCatalog =
    !!status.status && access.enabled && (!access.requireAuth || !!user)
  const data = usePricingData(canLoadCatalog)
  const article = getArticle(props.slug ?? 'overview') ?? articles[0]
  const title = props.modelId ?? t(article.title)
  const href = useRouterState({ select: (state) => state.location.href })
  const path = props.modelId
    ? `/docs/models/${encodeURIComponent(props.modelId)}`
    : `/docs/${article.slug}`
  const origin = typeof window !== 'undefined' ? window.location.origin : ''
  const apiOrigin = (
    apiAddresses[0]?.url?.trim() ||
    (typeof status.status?.server_address === 'string'
      ? status.status.server_address.trim()
      : '') ||
    origin
  )
    .replace(/\/$/, '')
    .replace(/\/v1$/, '')
  const canonical =
    origin + (article.slug === 'overview' && !props.modelId ? '/docs' : path)
  const navigationArticles = beginnerArticles
  const index = navigationArticles.indexOf(article)
  const previous = navigationArticles[index - 1]
  const next = index >= 0 ? navigationArticles[index + 1] : undefined
  const hasExamples = ['quickstart', 'chat', 'sdk'].includes(article.kind)
  const hasCatalog =
    !!props.modelId || ['models', 'pricing', 'matrix'].includes(article.kind)
  useEffect(() => {
    const hash = href.split('#')[1]
    if (hash) {
      document
        .querySelector(`#${CSS.escape(decodeURIComponent(hash))}`)
        ?.scrollIntoView({ block: 'start' })
    } else window.scrollTo({ top: 0 })
  }, [href, data.isLoading])
  return (
    <PublicLayout
      showMainContainer={false}
      navContent={
        <nav
          aria-label={t('Documentation shortcuts')}
          className='flex items-center gap-5 text-sm'
        >
          <Link to='/docs'>{t('Docs')}</Link>
          <Link to='/docs/$slug' params={{ slug: 'models' }}>
            {t('Models')}
          </Link>
          <Link to='/docs/$slug' params={{ slug: 'pricing' }}>
            {t('Pricing')}
          </Link>
        </nav>
      }
      headerProps={{
        variant: 'solid',
        showAuthButtons: false,
        showNotifications: false,
        showNavigation: false,
        leftContent: (
          <div className='lg:hidden'>
            <DocsNavigation slug={article.slug} mobile />
          </div>
        ),
        rightContent: (
          <div className='flex items-center gap-2'>
            <DocsSearch
              models={canLoadCatalog && !data.error ? data.models : []}
              catalogRestricted={!canLoadCatalog}
            />
            <Button
              variant='ghost'
              size='sm'
              aria-label={t('Console')}
              render={<Link to='/dashboard' />}
            >
              <span className='hidden sm:inline'>{t('Console')}</span>
              <ArrowRight className='size-4' />
            </Button>
          </div>
        ),
      }}
    >
      <DocumentationMetadata
        title={title}
        description={t(article.description)}
        canonical={canonical}
      />
      <a
        href='#docs-content'
        className='bg-background fixed top-2 left-2 z-100 -translate-y-20 rounded-md border px-4 py-2 focus:translate-y-0'
      >
        {t('Skip to content')}
      </a>
      <div className='mx-auto grid max-w-[1500px] grid-cols-1 gap-8 px-4 pt-24 lg:grid-cols-[230px_minmax(0,1fr)] lg:px-6 xl:grid-cols-[230px_minmax(0,1fr)_180px]'>
        <aside className='sticky top-24 hidden max-h-[calc(100svh-6rem)] self-start overflow-y-auto overscroll-contain lg:block'>
          <DocsNavigation slug={article.slug} />
        </aside>
        <main id='docs-content' className='min-w-0 pb-16' tabIndex={-1}>
          <Breadcrumb className='mb-7' aria-label={t('Breadcrumb')}>
            <BreadcrumbList>
              <BreadcrumbItem>
                <BreadcrumbLink render={<Link to='/docs' />}>
                  {t('Docs')}
                </BreadcrumbLink>
              </BreadcrumbItem>
              <BreadcrumbSeparator />
              <BreadcrumbItem>{t(article.group)}</BreadcrumbItem>
              <BreadcrumbSeparator />
              <BreadcrumbItem>
                <BreadcrumbPage>
                  {props.modelId ? t('Model Detail') : title}
                </BreadcrumbPage>
              </BreadcrumbItem>
            </BreadcrumbList>
          </Breadcrumb>
          {!props.modelId && (
            <>
              <p className='text-muted-foreground mb-3 text-xs font-medium tracking-wider uppercase'>
                {t(article.group)}
              </p>
              <h1 className='text-3xl leading-tight font-semibold tracking-tight sm:text-4xl'>
                {title}
              </h1>
              <p className='text-muted-foreground mt-4 max-w-2xl text-base leading-7'>
                {t(article.description)}
              </p>
              {article.kind === 'home' && (
                <section className='mt-8 space-y-8'>
                  <div className='flex flex-wrap gap-3'>
                    <Button
                      render={
                        <Link
                          to='/docs/$slug'
                          params={{ slug: 'quickstart' }}
                        />
                      }
                    >
                      {t('Quickstart')}
                      <ArrowRight className='size-4' />
                    </Button>
                    <Button
                      variant='outline'
                      render={
                        <Link to='/docs/$slug' params={{ slug: 'models' }} />
                      }
                    >
                      {t('View models')}
                    </Button>
                    <Button
                      variant='ghost'
                      render={
                        <Link to='/docs/$slug' params={{ slug: 'pricing' }} />
                      }
                    >
                      {t('View pricing')}
                    </Button>
                  </div>
                  <ol className='grid gap-4 sm:grid-cols-3'>
                    {['api-key', 'models', 'quickstart'].map((slug, step) => (
                      <li key={slug} className='rounded-xl border p-5'>
                        <span className='text-primary text-sm font-semibold'>
                          {step + 1}
                        </span>
                        <Link
                          className='mt-3 block font-medium'
                          to='/docs/$slug'
                          params={{ slug }}
                        >
                          {t(
                            [
                              'Create an API key',
                              'Choose a model',
                              'Send a request',
                            ][step]
                          )}
                        </Link>
                      </li>
                    ))}
                  </ol>
                  <div>
                    <h2 className='mb-3 text-lg font-semibold'>
                      {t('Popular guides')}
                    </h2>
                    <div className='flex flex-wrap gap-3'>
                      {beginnerArticles
                        .filter((page) =>
                          [
                            'text-chat',
                            'image',
                            'video',
                            'integration-claude-code',
                            'integration-cursor',
                          ].includes(page.slug)
                        )
                        .map((page) => (
                          <Button
                            key={page.slug}
                            variant='outline'
                            size='sm'
                            render={
                              <Link
                                to='/docs/$slug'
                                params={{ slug: page.slug }}
                              />
                            }
                          >
                            {t(page.title)}
                          </Button>
                        ))}
                    </div>
                  </div>
                </section>
              )}
              {['home', 'quickstart', 'integration'].includes(article.kind) && (
                <section className='mt-8 rounded-xl border p-4'>
                  <div className='flex items-center justify-between gap-3'>
                    <div className='min-w-0'>
                      <p className='text-muted-foreground mb-2 text-xs'>
                        {t(
                          article.protocol === 'anthropic'
                            ? 'Gateway URL'
                            : 'OpenAI Base URL'
                        )}
                      </p>
                      <code className='text-sm break-all'>
                        {apiOrigin}
                        {article.protocol === 'anthropic' ? '' : '/v1'}
                      </code>
                    </div>
                    <CopyButton
                      value={
                        apiOrigin +
                        (article.protocol === 'anthropic' ? '' : '/v1')
                      }
                    />
                  </div>
                </section>
              )}
              {article.sections.map((section) => (
                <section
                  key={section.id}
                  id={section.id}
                  className='mt-9 scroll-mt-40'
                >
                  <h2 className='group mb-4 text-xl font-semibold tracking-tight'>
                    <a href={`#${section.id}`} className='hover:underline'>
                      {t(section.title)}
                      <span
                        className='text-muted-foreground ml-2 opacity-0 group-hover:opacity-100'
                        aria-hidden='true'
                      >
                        #
                      </span>
                    </a>
                  </h2>
                  <Markdown className='text-sm leading-7 [&_table]:block [&_table]:overflow-x-auto'>
                    {translateDocSection(section, t)}
                  </Markdown>
                </section>
              ))}
              {article.slug === 'admin-providers' && (
                <section id='provider-directory' className='mt-8'>
                  <h2 className='mb-4 text-xl font-semibold'>
                    {t('Provider documentation')}
                  </h2>
                  <ul className='grid gap-x-6 gap-y-3 sm:grid-cols-2'>
                    {articles
                      .filter((item) => item.kind === 'provider')
                      .map((provider) => (
                        <li key={provider.slug}>
                          <Link
                            to='/docs/$slug'
                            params={{ slug: provider.slug }}
                            className='text-primary text-sm underline underline-offset-4'
                          >
                            {t(provider.title)}
                          </Link>
                        </li>
                      ))}
                  </ul>
                </section>
              )}
              {article.console && (
                <Button
                  className='mt-6'
                  variant='outline'
                  render={<Link to={article.console} />}
                >
                  {t('Open in Console')}
                  <ArrowRight className='size-4' />
                </Button>
              )}
            </>
          )}
          {hasExamples && (
            <CodeExamples
              origin={apiOrigin}
              protocol={article.protocol}
              beginner={['quickstart', 'chat'].includes(article.kind)}
            />
          )}
          {['image', 'video', 'audio'].includes(article.kind) && (
            <TaskExamples
              kind={article.kind as 'image' | 'video' | 'audio'}
              origin={apiOrigin}
            />
          )}
          {article.kind === 'common-errors' && <CommonErrors />}
          {hasCatalog && (
            <>
              {status.loading && <LoadingState />}
              {!status.loading && !canLoadCatalog && (
                <ErrorState
                  title={t('Catalog access restricted')}
                  description={t(
                    'Catalog visibility follows the platform pricing settings. Sign in if required.'
                  )}
                  action={
                    <Button
                      render={
                        <Link to='/sign-in' search={{ redirect: path }} />
                      }
                    >
                      {t('Sign in')}
                    </Button>
                  }
                />
              )}
              {canLoadCatalog && data.isLoading && <LoadingState />}
              {canLoadCatalog && data.error && (
                <ErrorState
                  description={t('Unable to load the live catalog. Try again.')}
                  onRetry={() => void data.refetch()}
                />
              )}
              {canLoadCatalog &&
                !data.isLoading &&
                !data.error &&
                (props.modelId ? (
                  <ModelDocument modelId={props.modelId} data={data} />
                ) : (
                  <ModelCatalog
                    key={article.slug}
                    data={data}
                    matrix={article.kind === 'matrix'}
                  />
                ))}
            </>
          )}
          {article.kind === 'reference' && <ApiReference />}
          {article.kind === 'errors' && <ErrorReference />}
          {!beginnerArticles.includes(article) && (
            <details className='text-muted-foreground mt-6 text-xs'>
              <summary className='cursor-pointer'>
                {t('Implementation sources')}
              </summary>
              <ul className='mt-2 space-y-1'>
                {article.sources.map((source) => (
                  <li key={source}>
                    <code>{source}</code>
                  </li>
                ))}
              </ul>
            </details>
          )}
          <nav
            className='mt-10 grid grid-cols-2 gap-4 border-t pt-6'
            aria-label={t('Previous and next page')}
          >
            {previous ? (
              <Link
                to={previous.slug === 'overview' ? '/docs' : '/docs/$slug'}
                params={{ slug: previous.slug }}
                className='hover:text-primary text-sm'
              >
                <span className='text-muted-foreground block text-xs'>
                  {t('Previous')}
                </span>
                {t(previous.title)}
              </Link>
            ) : (
              <span />
            )}
            {next && (
              <Link
                to='/docs/$slug'
                params={{ slug: next.slug }}
                className='hover:text-primary text-right text-sm'
              >
                <span className='text-muted-foreground block text-xs'>
                  {t('Next')}
                </span>
                {t(next.title)}
              </Link>
            )}
          </nav>
        </main>
        <aside className='sticky top-24 hidden max-h-[calc(100svh-6rem)] self-start overflow-y-auto xl:block'>
          <nav aria-label={t('On this page')}>
            <p className='mb-4 text-xs font-semibold'>{t('On this page')}</p>
            <ul className='text-muted-foreground space-y-3 text-xs'>
              {!props.modelId &&
                article.sections.map((section) => (
                  <li key={section.id}>
                    <a
                      href={`#${section.id}`}
                      className='hover:text-foreground'
                    >
                      {t(section.title)}
                    </a>
                  </li>
                ))}
              {hasExamples && (
                <li>
                  <a href='#request-examples'>{t('Request examples')}</a>
                </li>
              )}
              {hasCatalog && (
                <li>
                  <a href={props.modelId ? '#model-pricing' : '#live-catalog'}>
                    {t(props.modelId ? 'Pricing' : 'Live catalog')}
                  </a>
                </li>
              )}
              {article.kind === 'reference' && (
                <li>
                  <a href='#operations'>{t('Endpoints')}</a>
                </li>
              )}
              {article.kind === 'errors' && (
                <li>
                  <a href='#error-index'>{t('Gateway error constants')}</a>
                </li>
              )}
            </ul>
          </nav>
        </aside>
      </div>
    </PublicLayout>
  )
}
