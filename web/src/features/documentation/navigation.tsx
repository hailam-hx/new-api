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
import { Link, useNavigate } from '@tanstack/react-router'
import { Menu, Search } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import type { PricingModel } from '@/features/pricing/types'
import { cn } from '@/lib/utils'

import { beginnerArticles, groups } from './content'
import reference from './generated/reference.json'
import { searchDocumentation } from './lib'

export function DocsNavigation(props: { slug: string; mobile?: boolean }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const activeSlug = props.slug
  const navigation = (
    <nav aria-label={t('Documentation navigation')} className='space-y-6 pb-10'>
      {groups.map((group) => (
        <div key={group}>
          <p className='text-muted-foreground mb-2 px-3 text-xs font-semibold tracking-wider uppercase'>
            {t(group)}
          </p>
          <ul className='space-y-0.5'>
            {beginnerArticles
              .filter(
                (article) =>
                  article.group === group && article.kind !== 'provider'
              )
              .map((article) => (
                <li key={article.slug}>
                  <Link
                    to='/docs/$slug'
                    params={{ slug: article.slug }}
                    onClick={() => setOpen(false)}
                    aria-current={
                      activeSlug === article.slug ? 'page' : undefined
                    }
                    className={cn(
                      'block rounded-md px-3 py-1.5 text-sm transition-colors',
                      activeSlug === article.slug
                        ? 'bg-accent text-accent-foreground font-medium'
                        : 'text-muted-foreground hover:bg-muted hover:text-foreground'
                    )}
                  >
                    {t(
                      article.slug === 'overview'
                        ? 'Overview'
                        : (article.navigationTitle ?? article.title)
                    )}
                  </Link>
                </li>
              ))}
          </ul>
        </div>
      ))}
    </nav>
  )
  if (!props.mobile) return navigation
  return (
    <>
      <Button
        variant='ghost'
        size='sm'
        aria-label={t('Documentation menu')}
        aria-expanded={open}
        onClick={() => setOpen(true)}
      >
        <Menu className='size-4' />
        <span className='hidden sm:inline'>{t('Menu')}</span>
      </Button>
      <Dialog
        open={open}
        onOpenChange={setOpen}
        title={t('Documentation menu')}
        contentClassName='max-h-[85svh] sm:max-w-sm'
        bodyClassName='pt-3'
      >
        {navigation}
      </Dialog>
    </>
  )
}

export function DocsSearch(props: {
  models: PricingModel[]
  catalogRestricted?: boolean
}) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const results = useMemo(
    () =>
      searchDocumentation(query, beginnerArticles, reference, props.models, t),
    [query, props.models, t]
  )
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent): void => {
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'k') {
        event.preventDefault()
        setOpen((value) => !value)
      }
    }
    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [])
  return (
    <>
      <Button
        variant='outline'
        size='sm'
        className='text-muted-foreground gap-2'
        aria-label={t('Search documentation')}
        onClick={() => setOpen(true)}
      >
        <Search className='size-4' />
        <span className='hidden sm:inline'>{t('Search documentation')}</span>
        <kbd className='hidden text-xs lg:inline'>⌘/Ctrl K</kbd>
      </Button>
      <Dialog
        open={open}
        onOpenChange={setOpen}
        title={t('Search documentation')}
        description={t('Search guides and models.')}
        contentClassName='sm:max-w-2xl'
        contentHeight='min(60svh, 30rem)'
      >
        <Input
          aria-label={t('Search documentation')}
          placeholder={t('Search documentation')}
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === 'Enter' && results[0]) {
              event.preventDefault()
              setOpen(false)
              void navigate({ href: results[0].href })
            }
          }}
          autoFocus
        />
        {props.catalogRestricted && (
          <p className='text-muted-foreground mt-3 text-xs'>
            {t('Model search follows catalog access settings.')}
          </p>
        )}
        <ul className='mt-4 space-y-1' aria-label={t('Search results')}>
          {results.map((result) => (
            <li key={result.href + result.title}>
              <Link
                to={result.href}
                onClick={() => setOpen(false)}
                className='hover:bg-accent focus-visible:bg-accent block rounded-md px-3 py-2'
              >
                <span className='block text-sm font-medium break-all'>
                  {result.title}
                </span>
                <span className='text-muted-foreground text-xs'>
                  {result.detail}
                </span>
              </Link>
            </li>
          ))}
        </ul>
        {query.trim() && results.length === 0 && (
          <p
            role='status'
            className='text-muted-foreground py-6 text-center text-sm'
          >
            {t('No results found')}
          </p>
        )}
      </Dialog>
    </>
  )
}
