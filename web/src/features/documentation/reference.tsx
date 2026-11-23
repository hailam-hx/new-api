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
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'

type Endpoint = { method: string; path: string; description: string }
type EndpointGroup = { id: string; title: string; endpoints: Endpoint[] }

const endpointGroups: EndpointGroup[] = [
  {
    id: 'models',
    title: 'Models',
    endpoints: [
      {
        method: 'GET',
        path: '/v1/models',
        description: 'List available model IDs.',
      },
      {
        method: 'GET',
        path: '/v1/models/{model}',
        description: 'Read a model entry.',
      },
      {
        method: 'GET',
        path: '/v1beta/models',
        description: 'List models in Gemini format.',
      },
    ],
  },
  {
    id: 'text',
    title: 'Text APIs',
    endpoints: [
      {
        method: 'POST',
        path: '/v1/chat/completions',
        description: 'Create an OpenAI-compatible chat completion.',
      },
      {
        method: 'POST',
        path: '/v1/responses',
        description: 'Create an HTTP Responses request.',
      },
      {
        method: 'POST',
        path: '/v1/responses/compact',
        description: 'Compact a Responses conversation when supported.',
      },
      {
        method: 'POST',
        path: '/v1/messages',
        description: 'Send an Anthropic Messages request.',
      },
      {
        method: 'POST',
        path: '/v1beta/models/{model}:generateContent',
        description: 'Generate content in Gemini format.',
      },
      {
        method: 'POST',
        path: '/v1beta/models/{model}:streamGenerateContent',
        description: 'Stream content in Gemini format.',
      },
    ],
  },
  {
    id: 'images',
    title: 'Image API',
    endpoints: [
      {
        method: 'POST',
        path: '/v1/images/generations',
        description: 'Generate images with a compatible model.',
      },
      {
        method: 'POST',
        path: '/v1/images/edits',
        description: 'Edit images with a compatible model.',
      },
    ],
  },
  {
    id: 'audio',
    title: 'Audio API',
    endpoints: [
      {
        method: 'POST',
        path: '/v1/audio/speech',
        description: 'Convert text to audio with a compatible speech model.',
      },
      {
        method: 'POST',
        path: '/v1/audio/transcriptions',
        description: 'Transcribe an uploaded audio file.',
      },
      {
        method: 'POST',
        path: '/v1/audio/translations',
        description: 'Translate an uploaded audio file.',
      },
    ],
  },
  {
    id: 'video',
    title: 'Video API',
    endpoints: [
      {
        method: 'POST',
        path: '/v1/videos',
        description: 'Create a video task with a compatible model.',
      },
      {
        method: 'GET',
        path: '/v1/videos/{task_id}',
        description: 'Read the video task status.',
      },
      {
        method: 'GET / HEAD',
        path: '/v1/videos/{task_id}/content',
        description: 'Download the completed video when supported.',
      },
    ],
  },
]

const mediaActions = [
  'images',
  'edits',
  'videos',
  'music',
  'speech',
  'voices',
  'avatars',
]

export function ApiReference(props: { origin: string }) {
  const { t } = useTranslation()
  const origin = props.origin.replace(/\/+$/, '')

  return (
    <div className='min-w-0 space-y-8'>
      <section className='space-y-3' aria-labelledby='reference-connection'>
        <h2 id='reference-connection' className='text-xl font-semibold'>
          {t('Base URL and authentication')}
        </h2>
        <dl className='space-y-3 text-sm'>
          <div>
            <dt className='text-muted-foreground'>{t('API origin')}</dt>
            <dd>
              <code className='break-all'>{origin}</code>
            </dd>
          </div>
          <div>
            <dt className='text-muted-foreground'>
              {t('OpenAI-compatible base URL')}
            </dt>
            <dd>
              <code className='break-all'>{origin}/v1</code>
            </dd>
          </div>
        </dl>
        <p className='text-sm'>
          {t('Use your HOTX API key for customer requests.')}
        </p>
        <code className='block overflow-x-auto rounded-lg border p-3 text-xs'>
          Authorization: Bearer YOUR_API_KEY
        </code>
      </section>
      <Alert>
        <AlertTitle>{t('Model compatibility')}</AlertTitle>
        <AlertDescription>
          {t(
            'Available endpoints and options depend on the enabled channel, model and plugin configuration.'
          )}
        </AlertDescription>
      </Alert>
      {endpointGroups.map((group) => (
        <section
          key={group.id}
          id={`reference-${group.id}`}
          className='scroll-mt-24 space-y-3'
        >
          <h2 className='text-xl font-semibold'>{t(group.title)}</h2>
          {group.id === 'video' && (
            <p className='text-muted-foreground text-sm leading-6'>
              {t(
                'Video routes require a compatible video provider or enabled plugin. Content download depends on the selected provider and a completed task.'
              )}
            </p>
          )}
          <StaticDataTable
            data={group.endpoints}
            getRowKey={(endpoint) => endpoint.path}
            tableProps={{ 'aria-label': t(group.title) }}
            columns={[
              {
                id: 'method',
                header: t('Method'),
                cell: (endpoint) => <code>{endpoint.method}</code>,
              },
              {
                id: 'path',
                header: t('Endpoint'),
                cell: (endpoint) => (
                  <code className='break-all whitespace-normal'>
                    {endpoint.path}
                  </code>
                ),
              },
              {
                id: 'description',
                header: t('Description'),
                cellClassName: 'min-w-48 whitespace-normal',
                cell: (endpoint) => t(endpoint.description),
              },
            ]}
          />
        </section>
      ))}
      <section id='reference-hotx-media' className='scroll-mt-24 space-y-3'>
        <h2 className='text-xl font-semibold'>{t('HOTX media task routes')}</h2>
        <p className='text-muted-foreground text-sm leading-6'>
          {t(
            'Send these requests to the HOTX origin shown above. These routes use their own JSON request and task response formats.'
          )}
        </p>
        <p className='text-sm leading-6'>
          {t(
            'Use these exact public paths when the HOTX media adapter is enabled. Save the returned task ID and use the matching query route to check progress.'
          )}
        </p>
        <StaticDataTable
          data={mediaActions}
          getRowKey={(action) => action}
          tableProps={{ 'aria-label': t('HOTX media task routes') }}
          columns={[
            {
              id: 'submit',
              header: t('Submit (POST)'),
              cell: (action) => <code>/dflop-media/{action}</code>,
            },
            {
              id: 'query',
              header: t('Check task (GET)'),
              cell: (action) => (
                <code>
                  /dflop-media/{action}/{'{task_id}'}
                </code>
              ),
            },
          ]}
        />
      </section>
    </div>
  )
}

export function CommonErrors() {
  const { t } = useTranslation()
  const rows = [
    {
      status: '401',
      cause: t('Invalid, expired or disabled API key'),
      next: t('Copy an active key from API Keys and check the app settings.'),
    },
    {
      status: '403',
      cause: t('Insufficient balance, key quota or permission'),
      next: t('Check Wallet, subscription limits and your API key access.'),
    },
    {
      status: '404',
      cause: t('The requested endpoint or model was not found'),
      next: t(
        'Check the Base URL, endpoint and model ID. Avoid adding /v1 twice.'
      ),
    },
    {
      status: '429',
      cause: t('Too many requests'),
      next: t(
        'Honor Retry-After when provided, wait before retrying and reduce simultaneous requests.'
      ),
    },
    {
      status: '500 / 503',
      cause: t('A processing error or an unavailable service'),
      next: t(
        'Check the response message and Usage Logs; contact support if it continues.'
      ),
    },
    {
      status: t('Timeout'),
      cause: t('The request did not finish within the allowed time'),
      next: t(
        'Check request or task status before retrying. A task may still be running.'
      ),
    },
  ]

  return (
    <section id='common-errors' className='mt-8 scroll-mt-24 space-y-3'>
      <h2 className='text-xl font-semibold'>{t('Common errors')}</h2>
      <StaticDataTable
        data={rows}
        getRowKey={(row) => row.status}
        tableProps={{ 'aria-label': t('Common errors') }}
        columns={[
          { id: 'status', header: t('Status'), cell: (row) => row.status },
          {
            id: 'cause',
            header: t('Cause'),
            cellClassName: 'min-w-44 whitespace-normal',
            cell: (row) => row.cause,
          },
          {
            id: 'next',
            header: t('Next step'),
            cellClassName: 'min-w-48 whitespace-normal',
            cell: (row) => row.next,
          },
        ]}
      />
    </section>
  )
}
