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
import {
  CHANNEL_TYPE_OPTIONS,
  CHANNEL_PROVIDER_PRESENTATION,
  TYPE_TO_KEY_PROMPT,
  CHANNEL_TYPE_WARNINGS,
} from '@/features/channels/constants'

import beginnerData from './beginner-data.json'
import data from './content-data.json'
import reference from './generated/reference.json'

export type DocSection = {
  id: string
  title: string
  body: string
  values?: Record<string, string | number>
}
export type DocArticle = {
  slug: string
  group: string
  title: string
  description: string
  sources: string[]
  sections: DocSection[]
  kind: string
  console?: string
  protocol?: string
}

export const groups: string[] = beginnerData.groups
export const beginnerArticles: DocArticle[] = beginnerData.articles
// Derive administrator provider pages from the same registry as the channel picker.
// These describe configuration, not configured credentials or model availability.
const providers: DocArticle[] = CHANNEL_TYPE_OPTIONS.map((provider) => ({
  slug: `provider-${provider.value}`,
  group: 'Administration',
  title: provider.label,
  description:
    CHANNEL_PROVIDER_PRESENTATION[provider.value]?.descriptionKey ??
    'Configure a channel using the current provider settings.',
  kind: 'provider',
  console: '/channels',
  sources: [
    'web/src/features/channels/constants.ts',
    'web/src/features/channels/components/drawers/channel-mutate-drawer.tsx',
    'constant/channel.go',
  ],
  sections: [
    {
      id: 'provider-type',
      title: 'Provider Type',
      body: 'Channel type {{type}}: {{provider}}.\n\n{{detail}}',
      values: {
        type: provider.value,
        provider: provider.label,
        detail: CHANNEL_PROVIDER_PRESENTATION[provider.value]?.detailKey ?? '',
      },
    },
    {
      id: 'provider-credential',
      title: 'Credential',
      body:
        TYPE_TO_KEY_PROMPT[provider.value] ?? 'Enter API key for this channel',
    },
    {
      id: 'provider-setup',
      title: 'Channel Configuration',
      body: 'In Channels, add a channel and select this provider type. Use the provider-specific fields shown by the channel form. Set Base URL only when required by your deployment; no configured upstream address or credential is exposed in these docs. Fetch or enter models, configure model mapping and groups, then run Channel Test. Configure effective pricing before enabling traffic.',
    },
    {
      id: 'provider-limits',
      title: 'Known Limitations',
      body:
        CHANNEL_TYPE_WARNINGS[provider.value] ??
        'Supported APIs, model listing, balance queries and test behavior depend on the selected provider implementation and configured models. Task plugins additionally require an installed, enabled binding. See the provider, model, pricing and routing guides before enabling traffic.',
    },
  ],
}))
const taskPluginProviders: DocArticle[] = reference.taskPlugins.map(
  (plugin) => ({
    slug: `provider-plugin-${plugin.key}`,
    group: 'Administration',
    title: plugin.name,
    description:
      'Shipped task plugin configuration; installation and enabled bindings determine runtime availability.',
    kind: 'provider',
    console: '/task-plugins',
    sources: [
      plugin.source,
      'docs/plugin-api/v1.md',
      'pkg/jsplugin/routing.go',
    ],
    sections: [
      {
        id: 'provider-type',
        title: 'Provider Type',
        body: '{{name}} · task plugin {{key}} · source version {{version}}. This is shipped source metadata, not a list of enabled plugins on your deployment.',
        values: { name: plugin.name, key: plugin.key, version: plugin.version },
      },
      {
        id: 'provider-setup',
        title: 'Channel Configuration',
        body: 'Open Task Plugins to install or inspect the plugin, then bind it to a compatible channel. Configure the channel credential and Base URL in the administrator console. Use the installed manifest for supported channel types, upstream modes and routes; the runtime validates bindings and capabilities. Verify models and effective pricing, then test before enabling traffic.',
      },
      {
        id: 'provider-limits',
        title: 'Known Limitations',
        body: 'Native plugin routes are registered only for installed and enabled plugins. The model catalog and pricing page describe effective configuration; a model named in shipped plugin source is not proof that your API key can call it. Do not use upstream credentials as relay API keys. Read the Async Tasks and Billing guides for polling, usage and settlement behavior.',
      },
    ],
  })
)
export const articles: DocArticle[] = [
  ...beginnerArticles,
  ...data.articles.filter(
    (article) => !beginnerArticles.some((page) => page.slug === article.slug)
  ),
  ...providers,
  ...taskPluginProviders,
]
export const getArticle = (slug: string): DocArticle | undefined =>
  articles.find((article) => article.slug === slug)

export function translateDocSection(
  section: DocSection,
  translate: (key: string, values?: Record<string, string | number>) => string
): string {
  const values = Object.fromEntries(
    Object.entries(section.values ?? {}).map(([key, value]) => [
      key,
      typeof value === 'string' ? translate(value) : value,
    ])
  )
  return translate(section.body, values)
}

export const DOC_ERROR_GUIDANCE: Record<string, string> = {
  invalid_request: 'Correct the request shape and endpoint.',
  model_not_found:
    'Check model access with your API key and confirm channel configuration.',
  model_price_error:
    'Ask an administrator to configure effective model or provider pricing.',
  insufficient_user_quota:
    'Review wallet balance, subscription funding and API key quota.',
  access_denied: 'Check account permissions and API key restrictions.',
}

export const DEFAULT_ERROR_GUIDANCE =
  'Inspect the response and relevant logs; contact the administrator for configuration errors.'
