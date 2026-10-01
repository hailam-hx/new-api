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
import { ArrowRight, BookOpen, Code2, Settings2 } from 'lucide-react'
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { PublicLayout } from '@/components/layout'
import { LoadingState } from '@/components/loading-state'
import { Alert, AlertDescription } from '@/components/ui/alert'
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
import { usePricingData } from '@/features/pricing/hooks/use-pricing-data'
import { useStatus } from '@/hooks/use-status'
import { getModuleAccessFromStatus } from '@/lib/nav-modules'
import { useAuthStore } from '@/stores/auth-store'

import { ModelCatalog, ModelDocument } from './catalog'
import { CodeExamples } from './code-examples'
import { articles, getArticle } from './content'
import { DocumentationMetadata } from './metadata'
import { DocsNavigation, DocsSearch } from './navigation'
import { ApiReference, ErrorReference } from './reference'

export function Documentation(props: { slug?: string; modelId?: string }) {
  const { t } = useTranslation()
  const status = useStatus()
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
  const canonical =
    origin + (article.slug === 'overview' && !props.modelId ? '/docs' : path)
  const navigationArticles = articles.filter((item) => item.kind !== 'provider')
  const index = navigationArticles.indexOf(article)
  const previous = navigationArticles[index - 1]
  const next = index >= 0 ? navigationArticles[index + 1] : undefined
  const hasExamples = article.kind === 'quickstart' || article.kind === 'sdk'
  const hasCatalog =
    !!props.modelId || ['models', 'pricing', 'matrix'].includes(article.kind)
  useEffect(() => {
    const hash = href.split('#')[1]
    if (hash) {
      document
        .getElementById(decodeURIComponent(hash))
        ?.scrollIntoView({ block: 'start' })
    } else window.scrollTo({ top: 0 })
  }, [href, data.isLoading])
  return (
    <PublicLayout showMainContainer={false}>
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
      <div className='bg-background/95 border-border fixed inset-x-0 top-14 z-30 border-b backdrop-blur'>
        <div className='mx-auto flex max-w-[1500px] items-center justify-between gap-2 px-4 py-2'>
          <div className='lg:hidden'>
            <DocsNavigation slug={article.slug} mobile />
          </div>
          <nav
            aria-label={t('Documentation shortcuts')}
            className='hidden min-w-0 items-center gap-5 text-sm sm:flex'
          >
            <Link to='/docs' className='flex items-center gap-2 font-medium'>
              <BookOpen className='size-4' />
              {t('Docs')}
            </Link>
            <Link to='/docs/$slug' params={{ slug: 'models' }}>
              {t('Models')}
            </Link>
            <Link to='/docs/$slug' params={{ slug: 'pricing' }}>
              {t('Pricing')}
            </Link>
            <Link to='/docs/$slug' params={{ slug: 'api-reference' }}>
              {t('API Reference')}
            </Link>
          </nav>
          <div className='flex items-center gap-2'>
            <DocsSearch
              models={canLoadCatalog && !data.error ? data.models : []}
              catalogRestricted={!canLoadCatalog}
            />
            <Button variant='ghost' size='sm' render={<Link to='/dashboard' />}>
              {t('Console')}
              <ArrowRight className='size-3' />
            </Button>
          </div>
        </div>
      </div>
      <div className='mx-auto grid max-w-[1500px] grid-cols-1 gap-8 px-4 pt-32 lg:grid-cols-[230px_minmax(0,1fr)] lg:px-6 xl:grid-cols-[230px_minmax(0,1fr)_180px]'>
        <aside className='sticky top-32 hidden max-h-[calc(100svh-8rem)] overflow-y-auto overscroll-contain lg:block'>
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
                <div className='mt-10 grid gap-8 border-y py-8 sm:grid-cols-2'>
                  <div>
                    <Code2 className='text-muted-foreground mb-4 size-5' />
                    <h2 className='text-lg font-semibold'>
                      {t('I want to use the API')}
                    </h2>
                    <p className='text-muted-foreground mt-3 text-sm leading-6'>
                      {t(
                        'Create an API key → choose a model → send your first request.'
                      )}
                    </p>
                    <div className='mt-4 flex flex-wrap gap-3'>
                      <Button
                        size='sm'
                        render={
                          <Link
                            to='/docs/$slug'
                            params={{ slug: 'quickstart' }}
                          />
                        }
                      >
                        {t('Quickstart')}
                        <ArrowRight className='size-3' />
                      </Button>
                      <Button
                        size='sm'
                        variant='outline'
                        render={
                          <Link to='/docs/$slug' params={{ slug: 'models' }} />
                        }
                      >
                        {t('Browse Models')}
                      </Button>
                      <Button
                        size='sm'
                        variant='ghost'
                        render={
                          <Link
                            to='/docs/$slug'
                            params={{ slug: 'api-reference' }}
                          />
                        }
                      >
                        {t('API Reference')}
                      </Button>
                    </div>
                  </div>
                  <div>
                    <Settings2 className='text-muted-foreground mb-4 size-5' />
                    <h2 className='text-lg font-semibold'>
                      {t('I manage the platform')}
                    </h2>
                    <p className='text-muted-foreground mt-3 text-sm leading-6'>
                      {t('Configure channels → models → pricing → routing.')}
                    </p>
                    <div className='mt-4 flex flex-wrap gap-3'>
                      <Button
                        size='sm'
                        variant='outline'
                        render={
                          <Link
                            to='/docs/$slug'
                            params={{ slug: 'admin-overview' }}
                          />
                        }
                      >
                        {t('Admin Guide')}
                      </Button>
                      <Button
                        size='sm'
                        variant='ghost'
                        render={
                          <Link
                            to='/docs/$slug'
                            params={{ slug: 'admin-providers' }}
                          />
                        }
                      >
                        {t('Providers')}
                      </Button>
                      <Button
                        size='sm'
                        variant='ghost'
                        render={
                          <Link
                            to='/docs/$slug'
                            params={{ slug: 'admin-pricing' }}
                          />
                        }
                      >
                        {t('Pricing')}
                      </Button>
                      <Button
                        size='sm'
                        variant='ghost'
                        render={
                          <Link
                            to='/docs/$slug'
                            params={{ slug: 'admin-routing' }}
                          />
                        }
                      >
                        {t('Routing')}
                      </Button>
                    </div>
                  </div>
                </div>
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
                    {t(section.body)}
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
            <CodeExamples origin={origin} protocol={article.protocol} />
          )}
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
          <Alert className='bg-muted/30 mt-10'>
            <AlertDescription>
              {t(
                'Documentation articles currently use English as the source language.'
              )}
            </AlertDescription>
          </Alert>
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
        <aside className='sticky top-32 hidden max-h-[calc(100svh-8rem)] overflow-y-auto xl:block'>
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
