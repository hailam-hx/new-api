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
import { useTranslation } from 'react-i18next'

import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import { developerArticles, getArticle } from './content'

export function ReferenceNavigation({ slug }: { slug: string }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const overview = getArticle('api-reference')
  const pages = overview ? [overview, ...developerArticles] : developerArticles
  return (
    <nav
      aria-label={t('Developer reference navigation')}
      className='mt-6 min-w-0 rounded-lg border p-3'
    >
      <div className='sm:hidden'>
        <Select
          value={slug}
          items={pages.map((page) => ({
            value: page.slug,
            label: t(page.title),
          }))}
          onValueChange={(value) => {
            void navigate({
              to: '/docs/$slug',
              params: { slug: String(value) },
            })
          }}
        >
          <SelectTrigger className='w-full' aria-label={t('Reference page')}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              {pages.map((page) => (
                <SelectItem key={page.slug} value={page.slug}>
                  {t(page.title)}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
      </div>
      <ul className='hidden flex-wrap gap-x-4 gap-y-2 text-sm sm:flex'>
        {pages.map((page) => (
          <li key={page.slug}>
            <Link
              to='/docs/$slug'
              params={{ slug: page.slug }}
              aria-current={page.slug === slug ? 'page' : undefined}
              className={
                page.slug === slug
                  ? 'text-primary font-semibold'
                  : 'text-muted-foreground hover:text-foreground'
              }
            >
              {t(page.title)}
            </Link>
          </li>
        ))}
      </ul>
    </nav>
  )
}
