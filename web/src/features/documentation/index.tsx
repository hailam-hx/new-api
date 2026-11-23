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
import { Link, useNavigate, useRouterState } from '@tanstack/react-router'
import { ArrowLeft, ArrowRight, BookOpen } from 'lucide-react'
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { PublicLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { Markdown } from '@/components/ui/markdown'
import { useApiInfo } from '@/features/dashboard/hooks/use-status-data'
import { useStatus } from '@/hooks/use-status'
import { useSystemConfig } from '@/hooks/use-system-config'

import { CodeExamples, DocsCode } from './code-examples'
import { articles, getArticle, translateDocSection } from './content'
import { buildAPIKeyExample, buildHotxMediaExample } from './lib'
import { DocumentationMetadata } from './metadata'
import { DocsNavigation, DocsSearch } from './navigation'
import { resolveDocsOrigin } from './origin'
import { DocumentationOverview } from './overview'
import { ApiReference, CommonErrors } from './reference'

const exampleKinds = new Set(['quickstart', 'chat', 'image', 'video', 'audio'])

export function Documentation(props: { slug?: string }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { status } = useStatus()
  const { systemName } = useSystemConfig()
  const { items: apiAddresses } = useApiInfo()
  const href = useRouterState({ select: (state) => state.location.href })
  const audioTask = href.split('#')[1]?.startsWith('transcription-')
    ? 'transcription'
    : 'speech'
  const article = getArticle(props.slug ?? 'overview') ?? articles[0]
  const pageOrigin = typeof window === 'undefined' ? '' : window.location.origin
  const apiOrigin = resolveDocsOrigin([
    ...apiAddresses.map((item) => item.url ?? ''),
    typeof status?.server_address === 'string' ? status.server_address : '',
    pageOrigin,
  ])
  const index = articles.indexOf(article)
  const previous = articles[index - 1]
  const next = articles[index + 1]
  const isExample = exampleKinds.has(article.kind)
  const integrationBase =
    article.protocol === 'anthropic' ? apiOrigin : `${apiOrigin}/v1`
  const path = article.slug === 'overview' ? '/docs' : `/docs/${article.slug}`

  useEffect(() => {
    const hash = href.split('#')[1]
    if (!hash) {
      window.scrollTo({ top: 0 })
      return
    }
    try {
      document
        .getElementById(decodeURIComponent(hash))
        ?.scrollIntoView({ block: 'start' })
    } catch {
      // An invalid URL fragment must not prevent reading the guide.
    }
  }, [href])

  return (
    <PublicLayout showMainContainer={false}>
      <DocumentationMetadata
        title={t(article.title)}
        description={t(article.description)}
        canonical={pageOrigin + path}
      />
      <a
        href='#docs-content'
        className='bg-background fixed top-2 left-2 z-100 -translate-y-20 rounded-md border px-4 py-2 focus:translate-y-0'
      >
        {t('Skip to content')}
      </a>
      <div className='mx-auto max-w-[1500px] px-4 pt-24 lg:px-6'>
        <div className='bg-background/95 sticky top-16 z-30 mb-8 flex min-w-0 items-center justify-between gap-3 border-b py-3 backdrop-blur'>
          <div className='flex min-w-0 items-center gap-2'>
            <div className='lg:hidden'>
              <DocsNavigation slug={article.slug} mobile />
            </div>
            <Link
              to='/docs'
              className='flex min-w-0 items-center gap-2 text-sm font-semibold'
            >
              <BookOpen className='size-4 shrink-0' aria-hidden='true' />
              <span className='truncate'>
                {systemName} · {t('Docs')}
              </span>
            </Link>
          </div>
          <div className='flex shrink-0 items-center gap-2'>
            <DocsSearch />
            <Button
              variant='ghost'
              size='sm'
              aria-label={t('Console')}
              render={<Link to='/dashboard' />}
            >
              <span className='hidden sm:inline'>{t('Console')}</span>
              <ArrowRight className='size-4' aria-hidden='true' />
            </Button>
          </div>
        </div>
        <div className='grid grid-cols-1 gap-8 lg:grid-cols-[220px_minmax(0,1fr)] xl:grid-cols-[220px_minmax(0,1fr)_180px]'>
          <aside className='sticky top-36 hidden max-h-[calc(100svh-10rem)] self-start overflow-y-auto overscroll-contain lg:block'>
            <DocsNavigation slug={article.slug} />
          </aside>
          <main id='docs-content' className='min-w-0 pb-16' tabIndex={-1}>
            <nav
              aria-label={t('Breadcrumb')}
              className='text-muted-foreground mb-6 flex flex-wrap items-center gap-2 text-sm'
            >
              <Link to='/docs' className='hover:text-foreground'>
                {t('Docs')}
              </Link>
              <span aria-hidden='true'>/</span>
              <span>{t(article.group)}</span>
            </nav>
            <h1 className='text-3xl leading-tight font-semibold tracking-tight sm:text-4xl'>
              {t(article.title)}
            </h1>
            <p className='text-muted-foreground mt-4 max-w-2xl text-base leading-7'>
              {t(article.description)}
            </p>

            {article.kind === 'home' && (
              <DocumentationOverview origin={apiOrigin} />
            )}

            {article.kind === 'integration' && (
              <section
                className='mt-8 rounded-xl border p-4'
                aria-label={t('Base URL')}
              >
                <p className='mb-2 text-sm font-medium'>{t('Base URL')}</p>
                <div className='flex min-w-0 items-center gap-2'>
                  <code className='min-w-0 flex-1 overflow-x-auto text-sm'>
                    {integrationBase}
                  </code>
                  <CopyButton value={integrationBase} />
                </div>
              </section>
            )}

            {isExample ? (
              <CodeExamples
                key={article.slug}
                origin={apiOrigin}
                beginner={article.kind === 'quickstart'}
                chat={article.kind === 'chat'}
                image={article.kind === 'image'}
                video={article.kind === 'video'}
                audio={article.kind === 'audio'}
                audioTask={audioTask}
                onAudioTaskChange={(task) => {
                  void navigate({
                    to: '/docs/$slug',
                    params: { slug: 'audio' },
                    hash: `${task}-endpoint`,
                    replace: true,
                  })
                }}
              />
            ) : (
              <div className='mt-8 space-y-8'>
                {article.sections
                  .filter(
                    (section) =>
                      article.kind !== 'home' || section.id === 'step-1'
                  )
                  .map((section) => (
                    <section
                      key={section.id}
                      id={section.id}
                      className='scroll-mt-40'
                    >
                      <h2 className='mb-3 text-xl font-semibold'>
                        {t(section.title)}
                      </h2>
                      <Markdown>{translateDocSection(section, t)}</Markdown>
                    </section>
                  ))}
              </div>
            )}

            {article.slug === 'api-key' && (
              <div className='mt-6'>
                <DocsCode
                  code={buildAPIKeyExample(apiOrigin)}
                  language='bash'
                />
              </div>
            )}
            {article.slug === 'hotx-media' && (
              <div className='mt-6'>
                <DocsCode
                  code={buildHotxMediaExample(apiOrigin)}
                  language='bash'
                />
              </div>
            )}
            {article.slug === 'api-reference' && (
              <ApiReference origin={apiOrigin} />
            )}
            {article.slug === 'error-codes' && <CommonErrors />}
            {['models', 'pricing'].includes(article.slug) && (
              <Button className='mt-6' render={<Link to='/pricing' />}>
                {t('View Models')}
                <ArrowRight className='size-4' aria-hidden='true' />
              </Button>
            )}
            {article.console && (
              <Button
                className='mt-6'
                variant='outline'
                render={<Link to={article.console} />}
              >
                {t('Open Console')}
                <ArrowRight className='size-4' aria-hidden='true' />
              </Button>
            )}

            <nav
              aria-label={t('Documentation pages')}
              className='mt-12 grid grid-cols-2 gap-3 border-t pt-6'
            >
              <div>
                {previous && (
                  <Link
                    to='/docs/$slug'
                    params={{ slug: previous.slug }}
                    className='hover:bg-muted flex h-full gap-2 rounded-lg border p-4'
                  >
                    <ArrowLeft
                      className='mt-1 size-4 shrink-0'
                      aria-hidden='true'
                    />
                    <span>
                      <span className='text-muted-foreground mb-1 block text-xs'>
                        {t('Previous')}
                      </span>
                      <span className='text-sm font-medium'>
                        {t(previous.title)}
                      </span>
                    </span>
                  </Link>
                )}
              </div>
              <div>
                {next && (
                  <Link
                    to='/docs/$slug'
                    params={{ slug: next.slug }}
                    className='hover:bg-muted flex h-full justify-end gap-2 rounded-lg border p-4 text-right'
                  >
                    <span>
                      <span className='text-muted-foreground mb-1 block text-xs'>
                        {t('Next')}
                      </span>
                      <span className='text-sm font-medium'>
                        {t(next.title)}
                      </span>
                    </span>
                    <ArrowRight
                      className='mt-1 size-4 shrink-0'
                      aria-hidden='true'
                    />
                  </Link>
                )}
              </div>
            </nav>
            <footer className='text-muted-foreground mt-10 border-t pt-5 text-xs leading-6'>
              <p>
                {t('Frontend design and development by New API contributors.')}
              </p>
              <a
                href='https://github.com/QuantumNous/new-api'
                target='_blank'
                rel='noopener noreferrer'
                className='underline underline-offset-4'
              >
                New API · QuantumNous
              </a>
            </footer>
          </main>
          <aside className='sticky top-36 hidden max-h-[calc(100svh-10rem)] self-start overflow-y-auto xl:block'>
            <p className='mb-3 text-sm font-medium'>{t('On this page')}</p>
            <nav aria-label={t('On this page')} className='space-y-3'>
              {article.sections
                .filter(
                  (section) => !section.task || section.task === audioTask
                )
                .map((section) => (
                  <a
                    key={section.id}
                    href={`#${section.task ? `${section.task}-` : ''}${section.id}`}
                    className='text-muted-foreground hover:text-foreground block text-xs leading-5'
                  >
                    {t(section.title)}
                  </a>
                ))}
            </nav>
          </aside>
        </div>
      </div>
    </PublicLayout>
  )
}
