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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { StaticDataTable } from '@/components/data-table'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Markdown } from '@/components/ui/markdown'

import { apiReferenceGroups, publicApiEndpoints } from './api-reference-data'
import { DocsCode } from './code-examples'
import { DOC_ERROR_GUIDANCE, DEFAULT_ERROR_GUIDANCE } from './content'
import reference from './generated/reference.json'
import { endpointAnchor } from './lib'

export function ApiReference() {
  const { t } = useTranslation()
  return (
    <div className='mt-8 min-w-0 space-y-9'>
      <section id='authentication' className='scroll-mt-40 space-y-3'>
        <h2 className='text-xl font-semibold'>{t('Authentication')}</h2>
        <p>{t('Base URL')}</p>
        <DocsCode language='bash' code='https://hotx-api.com/v1' />
        <DocsCode language='bash' code='Authorization: Bearer YOUR_API_KEY' />
        <p className='text-muted-foreground text-sm'>
          {t('Every request needs a valid API key.')}
        </p>
        <a href='/docs/api-key' className='text-primary text-sm underline'>
          {t('API Key')} →
        </a>
      </section>
      <section id='operations' className='scroll-mt-40 space-y-3'>
        <h2 className='text-xl font-semibold'>{t('Endpoints')}</h2>
        <StaticDataTable
          data={publicApiEndpoints}
          getRowKey={(endpoint) => endpoint.method + endpoint.path}
          tableClassName='table-fixed w-full [&_td]:whitespace-normal [&_td]:break-words [&_th]:whitespace-normal [&_th]:break-words'
          columns={[
            {
              id: 'method',
              header: t('Method'),
              className: 'w-28',
              cell: (endpoint) => (
                <Badge variant='outline'>{endpoint.method}</Badge>
              ),
            },
            {
              id: 'path',
              header: t('Endpoint'),
              cell: (endpoint) => (
                <a
                  href={`#${endpointAnchor(endpoint.method, endpoint.path)}`}
                  className='font-mono text-xs break-all underline'
                >
                  {endpoint.path}
                </a>
              ),
            },
          ]}
        />
      </section>
      {apiReferenceGroups.map(([group, title]) => (
        <section id={group} key={group} className='scroll-mt-40 space-y-8'>
          <h2 className='text-xl font-semibold'>{t(title)}</h2>
          {publicApiEndpoints
            .filter((endpoint) => endpoint.group === group)
            .map((endpoint) => (
              <section
                id={endpointAnchor(endpoint.method, endpoint.path)}
                key={endpoint.method + endpoint.path}
                className='min-w-0 scroll-mt-40 space-y-4 border-t pt-5'
              >
                <div className='flex min-w-0 items-start gap-2'>
                  <Badge variant='outline'>{endpoint.method}</Badge>
                  <h3 className='min-w-0 flex-1 font-mono text-sm break-all'>
                    {endpoint.path}
                  </h3>
                  <CopyButton value={endpoint.path} size='sm' />
                </div>
                <p>{t(endpoint.description)}</p>
                <p className='text-muted-foreground text-sm'>
                  {t('Authentication')}:{' '}
                  <a className='underline' href='#authentication'>
                    Bearer API Key
                  </a>
                </p>
                {endpoint.note && (
                  <p className='text-muted-foreground text-sm'>
                    {t(endpoint.note)}
                  </p>
                )}
                <h4 className='font-semibold'>{t('Request parameters')}</h4>
                {endpoint.contentType && (
                  <p className='font-mono text-xs break-all'>
                    {t('Request body')}: {endpoint.contentType}
                  </p>
                )}
                {endpoint.fields.length ? (
                  <>
                    <p className='text-muted-foreground text-sm'>
                      {t(
                        'Main parameters only. Optional fields depend on model and channel support.'
                      )}
                    </p>
                    <StaticDataTable
                      data={endpoint.fields}
                      getRowKey={(field) => field.name}
                      tableClassName='w-full table-fixed [&_td]:whitespace-normal [&_td]:break-words [&_th]:whitespace-normal [&_th]:break-words'
                      columns={[
                        {
                          id: 'field',
                          header: t('Field'),
                          cell: (field) => (
                            <code className='text-xs break-all'>
                              {field.name}
                            </code>
                          ),
                        },
                        {
                          id: 'type',
                          header: t('Type'),
                          cell: (field) => (
                            <code className='text-xs break-all'>
                              {field.type}
                            </code>
                          ),
                        },
                        {
                          id: 'required',
                          header: t('Required'),
                          className: 'w-20',
                          cell: (field) => {
                            if (field.conditional) {
                              return t('Model-dependent parameter')
                            }
                            return t(field.required ? 'Yes' : 'No')
                          },
                        },
                        {
                          id: 'description',
                          header: t('Description'),
                          cell: (field) => {
                            if (field.name === 'model') return t('Model ID')
                            if (field.name === 'messages') {
                              return t(
                                'An array of messages with role and content.'
                              )
                            }
                            if (field.name === 'prompt') return t('Prompt')
                            if (field.name === 'input') {
                              return t('Text or structured input.')
                            }
                            if (field.name === 'stream') return t('Streaming')
                            if (
                              [
                                'max_tokens',
                                'max_completion_tokens',
                                'max_output_tokens',
                              ].includes(field.name)
                            ) {
                              return t('Maximum number of output tokens.')
                            }
                            if (field.name.endsWith('_id')) {
                              return t('Task or response identifier')
                            }
                            if (
                              ['file', 'image', 'mask'].includes(field.name)
                            ) {
                              return t('File')
                            }
                            return t('Model-dependent parameter')
                          },
                        },
                      ]}
                    />
                  </>
                ) : (
                  <p className='text-muted-foreground text-sm'>
                    {t('No request body.')}
                  </p>
                )}
                <h4 className='font-semibold'>{t('Example request')}</h4>
                <DocsCode language='bash' code={endpoint.example} />
                <h4 className='font-semibold'>{t('Response')}</h4>
                {typeof endpoint.response === 'string' ? (
                  <p className='text-sm'>
                    {t('Binary response')}: <code>{endpoint.response}</code>
                  </p>
                ) : (
                  <>
                    <p className='text-muted-foreground text-sm'>
                      {t(
                        'Illustrative response shape. Optional fields and usage depend on the model.'
                      )}
                    </p>
                    <DocsCode
                      language='json'
                      code={JSON.stringify(endpoint.response, null, 2)}
                    />
                  </>
                )}
                <p className='text-sm'>
                  {t('Errors')}:{' '}
                  <a
                    className='text-primary underline'
                    href='#reference-errors'
                  >
                    {t('Common errors')}
                  </a>
                </p>
                <details className='text-muted-foreground text-xs'>
                  <summary className='cursor-pointer'>{t('Source')}</summary>
                  <p className='mt-2 break-words'>
                    {endpoint.routeSource} · {endpoint.source}
                  </p>
                  {endpoint.schemaSource && (
                    <p className='mt-1 break-words'>{endpoint.schemaSource}</p>
                  )}
                </details>
              </section>
            ))}
          {['chat', 'images', 'video', 'audio'].includes(group) && (
            <a
              className='text-primary text-sm underline'
              href={`/docs/${({ chat: 'text-chat', images: 'image', video: 'video', audio: 'audio' } as Record<string, string>)[group]}`}
            >
              {t('Read the beginner guide')} →
            </a>
          )}
        </section>
      ))}
      <section id='reference-errors' className='scroll-mt-40 space-y-4'>
        <h2 className='text-xl font-semibold'>{t('Common errors')}</h2>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Inspect HTTP status and error.message. error.type and error.code vary; code may be empty.'
          )}
        </p>
        <StaticDataTable
          tableClassName='w-full table-fixed [&_td]:whitespace-normal [&_td]:break-words [&_th]:whitespace-normal [&_th]:break-words'
          data={[
            { status: 400, label: 'Invalid request' },
            { status: 401, label: 'Invalid, expired or disabled API key' },
            { status: 403, label: 'Access denied' },
            { status: 404, label: 'Task or response not found' },
            { status: 429, label: 'Too many requests' },
            { status: 500, label: 'Server error' },
          ]}
          getRowKey={(row) => String(row.status)}
          columns={[
            {
              id: 'status',
              header: t('HTTP Status'),
              className: 'w-28',
              cell: (row) => row.status,
            },
            {
              id: 'meaning',
              header: t('Description'),
              cell: (row) => t(row.label),
            },
          ]}
        />
        <DocsCode
          language='json'
          code={JSON.stringify(
            {
              error: {
                message: 'ERROR_MESSAGE',
                type: 'new_api_error',
                code: '',
              },
            },
            null,
            2
          )}
        />
      </section>
    </div>
  )
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
                {t(DOC_ERROR_GUIDANCE[error.code] ?? DEFAULT_ERROR_GUIDANCE)}
              </Markdown>
            ),
          },
        ]}
      />
    </section>
  )
}

export function CommonErrors() {
  const { t } = useTranslation()
  const rows = [
    {
      status: '401',
      cause: t('Invalid, expired or disabled API key'),
      fix: t('Copy an active key from API Keys and check the app settings.'),
    },
    {
      status: '403',
      cause: t('Insufficient balance, key quota or permission'),
      fix: t('Check Wallet, subscription limits and your API key access.'),
    },
    {
      status: '404',
      cause: t('The requested URL does not exist'),
      fix: t('Check the Base URL and endpoint. Avoid adding /v1 twice.'),
    },
    {
      status: '429',
      cause: t('Too many requests'),
      fix: t('Wait before retrying and reduce simultaneous requests.'),
    },
    {
      status: '500 / 503',
      cause: t('A processing error or an unavailable service'),
      fix: t(
        'Check the response message and Usage Logs; contact support if it continues.'
      ),
    },
    {
      status: t('Timeout'),
      cause: t('The request did not finish within the allowed time'),
      fix: t(
        'Check request or task status before retrying. A task may still be running.'
      ),
    },
  ]
  return (
    <section id='common-errors' className='mt-8 scroll-mt-32'>
      <StaticDataTable
        data={rows}
        getRowKey={(row) => row.status}
        columns={[
          { id: 'status', header: t('HTTP Status'), cell: (row) => row.status },
          { id: 'cause', header: t('Cause'), cell: (row) => row.cause },
          { id: 'fix', header: t('Try this'), cell: (row) => row.fix },
        ]}
      />
    </section>
  )
}
