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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { ErrorState } from '@/components/error-state'
import { PublicLayout } from '@/components/layout'
import { LoadingState } from '@/components/loading-state'
import { Alert, AlertTitle, AlertDescription } from '@/components/ui/alert'
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
import { CodeExamples, DocsCode } from './code-examples'
import {
  articles,
  beginnerArticles,
  getArticle,
  translateDocSection,
} from './content'
import { buildAPIKeyExample } from './lib'
import { DocumentationMetadata } from './metadata'
import { DocsNavigation, DocsSearch } from './navigation'
import { PricingGuide, PricingUsageLink } from './pricing-guide'
import { ApiReference, ErrorReference, CommonErrors } from './reference'

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
  const data = usePricingData(canLoadCatalog, { inlineErrors: true })
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
  const [audioTask, setAudioTask] = useState<'speech' | 'transcription'>(
    'speech'
  )
  const hasExamples = ['sdk'].includes(article.kind)
  const hasCatalog =
    !!props.modelId || ['models', 'pricing', 'matrix'].includes(article.kind)
  let catalogAnchor = '#live-catalog'
  let catalogLabel = t('Live catalog')
  if (article.kind === 'pricing') {
    catalogAnchor = '#price-table'
    catalogLabel = t('Price list')
  }
  if (props.modelId) {
    catalogAnchor = '#model-pricing'
    catalogLabel = t('Pricing')
  }
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
      <div
        className={`mx-auto grid max-w-[1500px] grid-cols-1 gap-8 px-4 pt-24 lg:grid-cols-[230px_minmax(0,1fr)] lg:px-6 ${article.kind === 'models' ? '' : 'xl:grid-cols-[230px_minmax(0,1fr)_180px]'}`}
      >
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
                  {props.modelId
                    ? t('Model Detail')
                    : t(article.kind === 'home' ? 'Overview' : article.title)}
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
              <p
                className={`text-muted-foreground max-w-2xl text-base leading-7 ${['chat', 'image', 'video', 'audio'].includes(article.kind) ? 'mt-2' : 'mt-4'}`}
              >
                {t(article.description)}
              </p>
              {article.slug === 'api-key' && (
                <p className='mt-3 text-sm leading-7'>
                  {t('An API Key authenticates requests sent to HOTX API.')}
                </p>
              )}
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
                  <section id='get-started' className='scroll-mt-40'>
                    <h2 className='mb-4 text-xl font-semibold'>
                      {t('Start in 3 steps')}
                    </h2>
                    <ol className='grid gap-4 sm:grid-cols-3'>
                      {['api-key', 'models', 'quickstart'].map((slug, step) => (
                        <li key={slug}>
                          <Link
                            className='hover:bg-muted/50 focus-visible:ring-ring block h-full rounded-xl border p-5 focus-visible:ring-2'
                            to='/docs/$slug'
                            params={{ slug }}
                          >
                            <span className='text-primary text-sm font-semibold'>
                              {step + 1}
                            </span>
                            <span className='mt-3 block font-medium'>
                              {t(
                                [
                                  'Create an API key',
                                  'Choose a model',
                                  'Send your first request',
                                ][step]
                              )}
                            </span>
                          </Link>
                        </li>
                      ))}
                    </ol>
                  </section>
                  <section id='you-need' className='scroll-mt-40'>
                    <h2 className='mb-4 text-xl font-semibold'>
                      {t('You need three things')}
                    </h2>
                    <dl className='divide-y rounded-xl border px-4'>
                      <div className='grid gap-2 py-4 sm:grid-cols-[100px_minmax(0,1fr)]'>
                        <dt className='text-sm font-medium'>{t('Base URL')}</dt>
                        <dd className='flex min-w-0 items-center justify-between gap-3'>
                          <code className='text-sm break-all'>
                            {apiOrigin}/v1
                          </code>
                          <CopyButton value={`${apiOrigin}/v1`} />
                        </dd>
                      </div>
                      {['api-key', 'models'].map((slug) => (
                        <div
                          key={slug}
                          className='grid gap-2 py-4 sm:grid-cols-[100px_minmax(0,1fr)]'
                        >
                          <dt className='text-sm font-medium'>
                            {t(slug === 'api-key' ? 'API Key' : 'Model ID')}
                          </dt>
                          <dd className='min-w-0'>
                            <div className='flex flex-wrap items-center justify-between gap-3'>
                              <code className='text-sm break-all'>
                                {slug === 'api-key'
                                  ? 'YOUR_API_KEY'
                                  : 'YOUR_MODEL_ID'}
                              </code>
                              <Button
                                variant='outline'
                                size='sm'
                                render={
                                  <Link to='/docs/$slug' params={{ slug }} />
                                }
                              >
                                {t(
                                  slug === 'api-key'
                                    ? 'Create an API key'
                                    : 'View models'
                                )}
                              </Button>
                            </div>
                            <p className='text-muted-foreground mt-2 text-sm'>
                              {t(
                                slug === 'api-key'
                                  ? 'API Key authenticates your requests.'
                                  : 'Model ID is the exact name of the model you want to use.'
                              )}
                            </p>
                          </dd>
                        </div>
                      ))}
                    </dl>
                    <p className='text-muted-foreground mt-3 text-sm'>
                      {t(
                        'Replace API Key and Model ID in the Quickstart example.'
                      )}
                    </p>
                  </section>
                  <section id='popular-guides' className='scroll-mt-40'>
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
                            'audio',
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
                            {t(page.navigationTitle ?? page.title)}
                          </Button>
                        ))}
                    </div>
                  </section>
                  <section id='step-1' className='scroll-mt-40'>
                    <h2 className='mb-4 text-xl font-semibold'>
                      {t('What is New API?')}
                    </h2>
                    <Markdown className='text-sm leading-7'>
                      {translateDocSection(article.sections[3], t)}
                    </Markdown>
                  </section>
                </section>
              )}
              {['quickstart', 'integration'].includes(article.kind) && (
                <section className='mt-8 rounded-xl border p-4'>
                  <div className='flex items-center justify-between gap-3'>
                    <div className='min-w-0'>
                      <p className='text-muted-foreground mb-2 text-xs'>
                        {article.kind === 'quickstart'
                          ? 'Base URL'
                          : t(
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
              {![
                'home',
                'quickstart',
                'chat',
                'image',
                'video',
                'audio',
              ].includes(article.kind) &&
                article.sections
                  .filter(() => article.kind !== 'pricing')
                  .map((section) => (
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
                      {article.slug === 'api-key' &&
                        section.id === 'step-2' && (
                          <div className='mt-4 min-w-0 space-y-4'>
                            <DocsCode
                              language='bash'
                              code='Authorization: Bearer YOUR_API_KEY'
                            />
                            <DocsCode
                              language='bash'
                              code={buildAPIKeyExample(apiOrigin)}
                            />
                            <Alert className='px-4 py-3'>
                              <AlertTitle>
                                {t('Keep your API Key safe')}
                              </AlertTitle>
                              <AlertDescription>
                                <Markdown>
                                  {t(
                                    '- Do not share your API Key.\n- Never put an API Key in public frontend code or a public repository.\n- In real applications, store your API Key in an environment variable or secret manager.'
                                  )}
                                </Markdown>
                              </AlertDescription>
                            </Alert>
                          </div>
                        )}
                      {article.slug === 'api-key' &&
                        ['step-1', 'step-3'].includes(section.id) && (
                          <Button
                            className='mt-4'
                            variant='outline'
                            size='sm'
                            render={<Link to='/keys' />}
                          >
                            {t(
                              section.id === 'step-1'
                                ? 'Open API Keys'
                                : 'Manage API Keys'
                            )}
                            <ArrowRight className='size-4' />
                          </Button>
                        )}
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
          {['quickstart', 'chat', 'image', 'video', 'audio'].includes(
            article.kind
          ) && (
            <div className='mt-8'>
              <CodeExamples
                origin={apiOrigin}
                beginner={article.kind === 'quickstart'}
                chat={article.kind === 'chat'}
                image={article.kind === 'image'}
                video={article.kind === 'video'}
                audio={article.kind === 'audio'}
                audioTask={audioTask}
                onAudioTaskChange={setAudioTask}
              />
            </div>
          )}
          {hasExamples && (
            <CodeExamples
              origin={apiOrigin}
              protocol={article.protocol}
              beginner={['quickstart', 'chat'].includes(article.kind)}
            />
          )}
          {article.kind === 'common-errors' && <CommonErrors />}
          {article.kind === 'pricing' && (
            <PricingGuide
              models={canLoadCatalog && !data.error ? data.models : []}
            />
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
              {canLoadCatalog && data.isLoading && (
                <LoadingState
                  message={
                    article.kind === 'pricing'
                      ? t('Loading prices...')
                      : undefined
                  }
                />
              )}
              {canLoadCatalog && data.error && (
                <ErrorState
                  description={t(
                    article.kind === 'pricing'
                      ? 'Unable to load prices.'
                      : 'Unable to load the live catalog. Try again.'
                  )}
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
                    pricing={article.kind === 'pricing'}
                  />
                ))}
            </>
          )}
          {article.kind === 'pricing' && <PricingUsageLink />}
          {article.kind === 'reference' && <ApiReference />}
          {article.kind === 'errors' && <ErrorReference />}
          {article.kind !== 'models' && !beginnerArticles.includes(article) && (
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
                {t(previous.slug === 'overview' ? 'Overview' : previous.title)}
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
        {article.kind !== 'models' && (
          <aside className='sticky top-24 hidden max-h-[calc(100svh-6rem)] self-start overflow-y-auto xl:block'>
            <nav aria-label={t('On this page')}>
              <p className='mb-4 text-xs font-semibold'>{t('On this page')}</p>
              <ul className='text-muted-foreground space-y-3 text-xs'>
                {!props.modelId &&
                  article.sections
                    .filter(
                      (section) =>
                        (article.kind !== 'video' ||
                          section.id !== 'response') &&
                        (!section.task || section.task === audioTask)
                    )
                    .map((section) => (
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
                    <a href={catalogAnchor}>{catalogLabel}</a>
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
        )}
      </div>
    </PublicLayout>
  )
}
