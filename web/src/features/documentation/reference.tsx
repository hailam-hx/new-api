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
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { StaticDataTable } from '@/components/data-table'
import { Badge } from '@/components/ui/badge'
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

import { DocsCode } from './code-examples'
import reference from './generated/reference.json'
import { endpointAnchor } from './lib'

export function ApiReference() {
  const { t } = useTranslation()
  const [query, setQuery] = useState('')
  const [scope, setScope] = useState('relay')
  const endpoints = useMemo(
    () =>
      reference.endpoints.filter((endpoint) => {
        const isManagement = endpoint.path.startsWith('/api/')
        if (scope === 'relay' && isManagement) return false
        if (scope === 'management' && !isManagement) return false
        return `${endpoint.method} ${endpoint.path} ${endpoint.summary} ${endpoint.handler}`
          .toLowerCase()
          .includes(query.toLowerCase())
      }),
    [query, scope]
  )
  // An anchor found by global search must reveal its operation even if the default scope hides it.
  const anchor =
    typeof window !== 'undefined' ? window.location.hash.slice(1) : ''
  const target = reference.endpoints.find(
    (endpoint) => endpointAnchor(endpoint.method, endpoint.path) === anchor
  )
  const shown =
    target && !endpoints.includes(target) ? [target, ...endpoints] : endpoints
  return (
    <section className='mt-8 min-w-0' id='operations'>
      <h2 className='mb-4 text-xl font-semibold'>{t('Endpoints')}</h2>
      <div className='mb-6 flex flex-wrap gap-3'>
        <Input
          aria-label={t('Filter endpoints')}
          placeholder={t('Filter endpoints')}
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          className='min-w-40 flex-1'
        />
        <Select
          value={scope}
          onValueChange={(value) => setScope(String(value))}
        >
          <SelectTrigger aria-label={t('API scope')} className='w-44'>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              <SelectItem value='relay'>{t('Relay API')}</SelectItem>
              <SelectItem value='management'>{t('Management API')}</SelectItem>
              <SelectItem value='all'>{t('All')}</SelectItem>
            </SelectGroup>
          </SelectContent>
        </Select>
      </div>
      {shown.length === 0 && <p role='status'>{t('No results found')}</p>}
      <div className='divide-border divide-y'>
        {shown.map((endpoint) => (
          <section
            key={endpoint.method + endpoint.path}
            id={endpointAnchor(endpoint.method, endpoint.path)}
            className='scroll-mt-40 py-5'
          >
            <div className='flex min-w-0 items-start gap-2'>
              <Badge variant='outline'>{endpoint.method}</Badge>
              <h3 className='min-w-0 flex-1 font-mono text-sm break-all'>
                {endpoint.path}
              </h3>
              <CopyButton value={endpoint.path} size='sm' />
            </div>
            <p className='text-muted-foreground mt-2 text-xs'>
              {endpoint.handler}
            </p>
            {endpoint.middleware.length > 0 && (
              <details className='mt-3 text-sm'>
                <summary className='cursor-pointer'>
                  {t('Authentication and middleware')}
                </summary>
                <ul className='mt-2 space-y-1'>
                  {endpoint.middleware.map((value) => (
                    <li key={value} className='font-mono text-xs break-all'>
                      {value}
                    </li>
                  ))}
                </ul>
              </details>
            )}
            {endpoint.schemaSource ? (
              <details className='mt-3 text-sm'>
                <summary className='cursor-pointer'>
                  {t('Parameters, request and response schemas')}
                </summary>
                <p className='text-muted-foreground mt-3 text-xs'>
                  {t(
                    'Checked-in OpenAPI contract; verify provider-specific behavior.'
                  )}
                </p>
                <DocsCode
                  language='json'
                  code={JSON.stringify(
                    {
                      parameters: endpoint.parameters,
                      requestBody: endpoint.requestBody,
                      responses: endpoint.responses,
                    },
                    null,
                    2
                  )}
                />
              </details>
            ) : (
              <p className='text-muted-foreground mt-3 text-xs'>
                {t(
                  'No matching OpenAPI schema is available for this registered operation.'
                )}
              </p>
            )}
            <p className='text-muted-foreground mt-3 text-xs'>
              {t('Source')}: <code>{endpoint.source}</code>
              {endpoint.schemaSource && (
                <>
                  {' '}
                  · <code>{endpoint.schemaSource}</code>
                </>
              )}
            </p>
          </section>
        ))}
      </div>
    </section>
  )
}

const actions: Record<string, string> = {
  invalid_request: 'Correct the request shape and endpoint.',
  model_not_found:
    'Check model access with your API key and confirm channel configuration.',
  model_price_error:
    'Ask an administrator to configure effective model or provider pricing.',
  insufficient_user_quota:
    'Review wallet balance, subscription funding and API key quota.',
  access_denied: 'Check account permissions and API key restrictions.',
}

export function ErrorReference() {
  const { t } = useTranslation()
  const [query, setQuery] = useState('')
  const errors = reference.errors.filter((error) =>
    `${error.name} ${error.code}`
      .toLowerCase()
      .includes(query.toLowerCase().replaceAll('*', ''))
  )
  return (
    <section className='mt-8 min-w-0' id='error-index'>
      <h2 className='mb-4 text-xl font-semibold'>
        {t('Gateway error constants')}
      </h2>
      <Input
        aria-label={t('Filter error codes')}
        placeholder={t('Filter error codes')}
        value={query}
        onChange={(event) => setQuery(event.target.value)}
        className='mb-4'
      />
      <StaticDataTable
        data={errors}
        getRowKey={(error) => error.code}
        emptyContent={t('No results found')}
        columns={[
          {
            id: 'code',
            header: t('Error Code'),
            cell: (error) => (
              <code
                id={error.code.replaceAll(/[^a-z0-9-]/gi, '-')}
                className='scroll-mt-40 text-xs break-all'
              >
                {error.code}
              </code>
            ),
          },
          {
            id: 'status',
            header: t('HTTP Status'),
            cell: (error) =>
              error.status ??
              t(
                error.kind === 'diagnostic'
                  ? 'Diagnostic marker'
                  : 'Depends on call site'
              ),
          },
          {
            id: 'source',
            header: t('Source'),
            cell: (error) => (
              <code className='text-xs break-all'>{error.source}</code>
            ),
          },
          {
            id: 'action',
            header: t('Suggested Action'),
            cell: (error) => (
              <Markdown className='text-sm'>
                {t(
                  actions[error.code] ??
                    'Inspect the response and relevant logs; contact the administrator for configuration errors.'
                )}
              </Markdown>
            ),
          },
        ]}
      />
    </section>
  )
}
