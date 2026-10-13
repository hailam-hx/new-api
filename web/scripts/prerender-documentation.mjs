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
import fs from 'node:fs/promises'
import path from 'node:path'

import { createInstance } from 'i18next'
import { marked } from 'marked'

import {
  apiReferenceGroups,
  publicApiEndpoints,
} from '../src/features/documentation/api-reference-data.ts'
import {
  articles,
  beginnerArticles,
  developerArticles,
  groups,
  translateDocSection,
} from '../src/features/documentation/content.ts'
import { buildDeveloperRecipe } from '../src/features/documentation/developer-recipe-code.ts'
import {
  buildAPIKeyExample,
  buildChatExample,
  buildImageExample,
  buildVideoExample,
  buildAudioExample,
  buildQuickstart,
  endpointAnchor,
} from '../src/features/documentation/lib.ts'
import en from '../src/i18n/locales/en.json' with { type: 'json' }

const translations = createInstance()
await translations.init({
  lng: 'en',
  nsSeparator: false,
  resources: { en },
  interpolation: { escapeValue: false },
})

const directory =
  process.argv.find((value) => value.startsWith('--dist='))?.slice(7) || 'dist'
const data = { articles, groups }
const reference = JSON.parse(
  await fs.readFile(
    'src/features/documentation/generated/reference.json',
    'utf8'
  )
)
const template = await fs.readFile(path.join(directory, 'index.html'), 'utf8')
const configured = process.env.DOCS_SITE_URL?.trim()
let origin = ''
if (configured) {
  const url = new URL(configured)
  if (
    !['http:', 'https:'].includes(url.protocol) ||
    url.username ||
    url.password ||
    url.search ||
    url.hash
  ) {
    throw new Error(
      'DOCS_SITE_URL must be a public HTTP(S) deployment URL without credentials, query or fragment.'
    )
  }
  origin = url.href.replace(/\/$/, '')
}
const escape = (value) =>
  String(value)
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;')
const articlePath = (article) =>
  article.slug === 'overview' ? '/docs' : `/docs/${article.slug}`
const navigation = data.groups
  .map(
    (group) =>
      `<section><p class="text-muted-foreground mb-2 text-xs font-semibold uppercase">${escape(translations.t(group))}</p><ul class="mb-6 space-y-2">${beginnerArticles
        .filter(
          (article) => article.group === group && article.kind !== 'provider'
        )
        .map(
          (article) =>
            `<li><a class="text-sm hover:underline" href="${articlePath(article)}">${escape(translations.t(article.navigationTitle ?? article.title))}</a></li>`
        )
        .join('')}</ul></section>`
  )
  .join('')
const urls = []
for (const article of data.articles) {
  const url = articlePath(article)
  urls.push(origin + url)
  const canonical = origin + url
  const isDeveloper =
    article.slug === 'api-reference' || developerArticles.includes(article)
  const navigationArticles = isDeveloper
    ? [
        beginnerArticles.find((page) => page.slug === 'api-reference'),
        ...developerArticles,
      ].filter(Boolean)
    : beginnerArticles
  const index = navigationArticles.indexOf(article)
  const previous = navigationArticles[index - 1]
  const next = index >= 0 ? navigationArticles[index + 1] : undefined
  let extra = ''
  if (article.kind === 'reference') {
    extra = `
      <section id="authentication">
        <h2>${escape(translations.t('Base URL and Authentication'))}</h2>
        <code>https://hotx-api.com/v1</code>
        <pre>Authorization: Bearer YOUR_API_KEY</pre>
        <p>${escape(translations.t('Protocol paths can also use /v1beta, /mj or documented compatibility URLs. Use the exact path on each reference page.'))}</p>
        <a href="/docs/developer-authentication">${escape(translations.t('Authentication'))}</a>
      </section>
      <section id="operations">
        <h2>${escape(translations.t('API Groups'))}</h2>
        <p>${escape(translations.t('Routes describe integration surfaces, not a guarantee that every model supports every operation. Check access and capability with your API key.'))}</p>
        <ul>${apiReferenceGroups.map(([group, title]) => `<li><a href="/docs/api-reference-${group}">${escape(translations.t(title))}</a></li>`).join('')}</ul>
      </section>
      <section id="reference-policy">
        <h2>${escape(translations.t('Errors and Limits'))}</h2>
        <a href="/docs/developer-errors-retries">${escape(translations.t('Errors and Retries'))}</a>
        <a href="/docs/developer-rate-limits">${escape(translations.t('Rate Limits'))}</a>
        <a href="/docs/developer-compatibility-versioning">${escape(translations.t('Compatibility and Versioning'))}</a>
        <a href="/docs/developer-billing-usage">${escape(translations.t('Billing and Usage'))}</a>
      </section>`
  }
  if (article.kind === 'reference-domain') {
    const group = article.slug.replace('api-reference-', '')
    const endpoints = publicApiEndpoints.filter(
      (endpoint) => endpoint.group === group
    )
    extra = `
      <section id="operations">
        <h2>${escape(translations.t('Endpoints'))}</h2>
        <p>${escape(translations.t('Availability depends on enabled channels, plugins and API key access. Use GET /v1/models to confirm accessible models; a registered route alone is not enough.'))}</p>
        <ul>${endpoints.map((endpoint) => `<li><a href="#${endpointAnchor(endpoint.method, endpoint.path)}">${escape(endpoint.method)} ${escape(endpoint.path)}</a></li>`).join('')}</ul>
      </section>
      <section id="${escape(group)}">
        <h2>${escape(translations.t('Endpoint details'))}</h2>
        ${endpoints
          .map((endpoint, index) => {
            const isPublicImage = endpoint.path.endsWith('/mj/image/{id}')
            const isArtifactContent =
              endpoint.path.includes('/artifacts/') &&
              endpoint.path.endsWith('/content')
            const usesAccessNote = isPublicImage || isArtifactContent
            const authentication = usesAccessNote
              ? translations.t(endpoint.note ?? '')
              : `Bearer ${translations.t('API Key')}`
            const requestRows = endpoint.fields
              .map((field) => {
                let required = field.required ? 'Yes' : 'No'
                if (field.conditional) required = 'Conditional'
                return `<tr><td><code>${escape(field.name)}</code></td><td><code>${escape(field.type)}</code></td><td>${escape(translations.t(required))}</td></tr>`
              })
              .join('')
            const requestSchema = endpoint.fields.length
              ? `<p>${escape(translations.t('Main parameters only. Optional fields depend on model and channel support.'))}</p><div class="overflow-x-auto"><table><thead><tr><th>${escape(translations.t('Field'))}</th><th>${escape(translations.t('Type'))}</th><th>${escape(translations.t('Required'))}</th></tr></thead><tbody>${requestRows}</tbody></table></div>`
              : `<p>${escape(translations.t(endpoint.contentType ? 'See implementation notes for model-specific request fields.' : 'No request body.'))}</p>`
            const responseRows = endpoint.responseFields
              .map(
                (field) =>
                  `<tr><td><code>${escape(field.name)}</code></td><td><code>${escape(field.type)}</code></td></tr>`
              )
              .join('')
            const responseSchema = endpoint.responseFields.length
              ? `<div class="overflow-x-auto"><table><thead><tr><th>${escape(translations.t('Field'))}</th><th>${escape(translations.t('Type'))}</th></tr></thead><tbody>${responseRows}</tbody></table></div>`
              : ''
            const responseExample =
              typeof endpoint.response === 'string'
                ? `<p>${escape(translations.t('Binary response'))}: <code>${escape(endpoint.response)}</code></p>`
                : `<p>${escape(translations.t('Illustrative response shape. Optional fields and usage depend on the model.'))}</p><pre class="overflow-x-auto"><code>${escape(JSON.stringify(endpoint.response, null, 2))}</code></pre>`
            return `
            <details${index === 0 ? ' open' : ''}>
              <summary class="break-all">${escape(endpoint.method)} ${escape(endpoint.path)}</summary>
              <section id="${endpointAnchor(endpoint.method, endpoint.path)}">
                <p>${escape(translations.t(endpoint.description))}</p>
                <p>${escape(translations.t('Authentication'))}: ${escape(authentication)}</p>
                ${endpoint.deploymentDependent ? `<p>${escape(translations.t('Deployment-dependent: this route requires the installed and enabled plugin. Its availability and accepted fields depend on that plugin.'))}</p>` : ''}
                ${endpoint.schemaNote ? `<p>${escape(translations.t(endpoint.schemaNote))}</p>` : ''}
                ${endpoint.headers ? `<h3>${escape(translations.t('Headers'))}</h3><pre class="overflow-x-auto"><code>${escape(JSON.stringify(endpoint.headers, null, 2))}</code></pre>` : ''}
                ${endpoint.query ? `<h3>${escape(translations.t('Query'))}</h3><pre class="overflow-x-auto"><code>${escape(JSON.stringify(endpoint.query, null, 2))}</code></pre>` : ''}
                ${endpoint.note && !usesAccessNote ? `<p>${escape(translations.t(endpoint.note))}</p>` : ''}
                <h3>${escape(translations.t('Request parameters'))}</h3>
                ${endpoint.contentType ? `<p>${escape(translations.t('Request body'))}: <code>${escape(endpoint.contentType)}</code></p>` : ''}
                ${requestSchema}
                <h3>${escape(translations.t('Example request'))}</h3>
                <pre class="overflow-x-auto"><code>${escape(endpoint.example)}</code></pre>
                <h3>${escape(translations.t('Response schema'))}</h3>
                <p>${escape(translations.t('Core response fields are shown below. Provider-specific fields may differ; these examples are not live responses.'))}</p>
                ${responseSchema}
                ${responseExample}
                ${(endpoint.responseAlternatives ?? [])
                  .map(
                    (alternative) => `
                  <h4>${escape(translations.t(alternative.description))}</h4>
                  <div class="overflow-x-auto"><table><thead><tr><th>${escape(translations.t('Field'))}</th><th>${escape(translations.t('Type'))}</th></tr></thead><tbody>${alternative.responseFields.map((field) => `<tr><td><code>${escape(field.name)}</code></td><td><code>${escape(field.type)}</code></td></tr>`).join('')}</tbody></table></div>
                  <pre class="overflow-x-auto"><code>${escape(JSON.stringify(alternative.response, null, 2))}</code></pre>`
                  )
                  .join('')}
                <a href="/docs/developer-errors-retries">${escape(translations.t('Errors and Retries'))}</a>
              </section>
            </details>`
          })
          .join('')}
      </section>`
  }
  if (isDeveloper) {
    extra = `<nav aria-label="${escape(translations.t('Developer reference navigation'))}"><ul class="flex flex-wrap gap-4">${navigationArticles.map((page) => `<li><a href="${articlePath(page)}">${escape(translations.t(page.title))}</a></li>`).join('')}</ul></nav>${extra}`
  }
  if (article.kind === 'errors') {
    extra = `<section id="error-index"><h2>${escape(translations.t('Gateway error constants'))}</h2><table><thead><tr><th>${escape(translations.t('Error Code'))}</th><th>${escape(translations.t('HTTP Status'))}</th><th>${escape(translations.t('Source'))}</th></tr></thead><tbody>${reference.errors.map((error) => `<tr id="${error.code.replaceAll(/[^a-z0-9-]/gi, '-')}"><td>${escape(error.code)}</td><td>${escape(error.status ?? translations.t(error.kind === 'diagnostic' ? 'Diagnostic marker' : 'Depends on call site'))}</td><td>${escape(error.source)}</td></tr>`).join('')}</tbody></table></section>`
  }
  if (article.slug === 'admin-providers') {
    extra += `<section id="provider-directory"><h2>${escape(translations.t('Provider documentation'))}</h2><ul>${data.articles
      .filter((item) => item.kind === 'provider')
      .map(
        (provider) =>
          `<li><a href="${articlePath(provider)}">${escape(provider.title)}</a></li>`
      )
      .join('')}</ul></section>`
  }
  if (article.kind === 'sdk') {
    if (origin) {
      extra += `<section id="request-examples"><h2>${escape(translations.t('Request examples'))}</h2>${[
        false,
        true,
      ]
        .map(
          (stream) =>
            `<section><h3>${escape(translations.t(stream ? 'Streaming' : 'Minimal request'))}</h3>${Object.entries(
              buildQuickstart(origin, article.protocol ?? 'openai', stream)
            )
              .map(
                ([language, code]) =>
                  `<details><summary>${escape(language)}</summary><pre class="overflow-x-auto"><code>${escape(code)}</code></pre></details>`
              )
              .join('')}</section>`
        )
        .join('')}</section>`
    } else {
      extra += `<p>${escape(translations.t('Open this page to copy your Base URL.'))}</p>`
    }
  }
  const articleSections = (
    ['reference', 'reference-domain'].includes(article.kind)
      ? []
      : article.sections
  )
    .map((section) => {
      const paragraph = marked.parse(
        translateDocSection(section, translations.t.bind(translations))
      )
      if (article.kind === 'home') {
        let body = paragraph
        if (section.id === 'get-started') {
          body = `<ol class="grid gap-4 sm:grid-cols-3">${['api-key', 'models', 'quickstart'].map((slug, index) => `<li><a class="block rounded-xl border p-5" href="/docs/${slug}">${index + 1}. ${escape(translations.t(['Create an API key', 'Choose a model', 'Send your first request'][index]))}</a></li>`).join('')}</ol>`
        }
        if (section.id === 'you-need') {
          body = `<dl><dt>${escape(translations.t('Base URL'))}</dt><dd><code>${escape(origin ? `${origin}/v1` : translations.t('Open this page to copy your Base URL.'))}</code></dd><dt>${escape(translations.t('API Key'))}</dt><dd><code>YOUR_API_KEY</code><p>${escape(translations.t('API Key authenticates your requests.'))}</p><a href="/docs/api-key">${escape(translations.t('Create an API key'))}</a></dd><dt>${escape(translations.t('Model ID'))}</dt><dd><code>YOUR_MODEL_ID</code><p>${escape(translations.t('Model ID is the exact name of the model you want to use.'))}</p><a href="/docs/models">${escape(translations.t('View models'))}</a></dd></dl><p>${escape(translations.t('Replace API Key and Model ID in the Quickstart example.'))}</p>`
        }
        if (section.id === 'popular-guides') {
          body = `<div class="flex flex-wrap gap-3">${beginnerArticles
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
            .map(
              (page) =>
                `<a href="/docs/${page.slug}">${escape(translations.t(page.navigationTitle ?? page.title))}</a>`
            )
            .join('')}</div>`
        }
        return `<section id="${section.id}" class="mt-8 scroll-mt-40"><h2 class="mb-4 text-xl font-semibold">${escape(translations.t(section.title))}</h2>${body}</section>`
      }
      if (article.slug === 'developer-recipes') {
        const example = buildDeveloperRecipe(section.id, 'YOUR_API_BASE_URL')
        const blocks = [example.curl, example.code].filter(Boolean)
        const body =
          paragraph +
          blocks
            .map(
              (code) =>
                `<pre class="overflow-x-auto"><code>${escape(code)}</code></pre>`
            )
            .join('')
        return `<section id="${section.id}" class="mt-8 scroll-mt-40"><h2 class="mb-4 text-xl font-semibold"><a href="#${section.id}">${escape(translations.t(section.title))}</a></h2>${body}</section>`
      }
      if (article.slug === 'api-key') {
        let body = paragraph
        if (section.id === 'step-1') {
          body += `<a href="/keys">${escape(translations.t('Open API Keys'))}</a>`
        }
        if (section.id === 'step-2') {
          body += `<pre class="overflow-x-auto"><code>Authorization: Bearer YOUR_API_KEY</code></pre>${origin ? `<pre class="overflow-x-auto"><code>${escape(buildAPIKeyExample(origin))}</code></pre>` : ''}<aside><h3>${escape(translations.t('Keep your API Key safe'))}</h3>${marked.parse(translations.t('- Do not share your API Key.\n- Never put an API Key in public frontend code or a public repository.\n- In real applications, store your API Key in an environment variable or secret manager.'))}</aside>`
        }
        if (section.id === 'step-3') {
          body += `<a href="/keys">${escape(translations.t('Manage API Keys'))}</a>`
        }
        return `<section id="${section.id}" class="mt-8 scroll-mt-40"><h2>${escape(translations.t(section.title))}</h2>${body}</section>`
      }
      if (
        !['quickstart', 'chat', 'image', 'video', 'audio'].includes(
          article.kind
        )
      ) {
        return `<section id="${section.id}" class="mt-8 scroll-mt-40"><h2 class="mb-4 text-xl font-semibold"><a href="#${section.id}">${escape(translations.t(section.title))}</a></h2>${paragraph}</section>`
      }
      let body = paragraph
      const sectionId =
        section.task === 'transcription'
          ? `transcription-${section.id}`
          : section.id
      if (section.id === 'endpoint') {
        const endpoint = {
          chat: 'chat/completions',
          image: 'images/generations',
          video: 'videos',
          audio:
            section.task === 'speech' ? 'audio/speech' : 'audio/transcriptions',
        }[article.kind]
        const label =
          article.kind === 'audio'
            ? `<h2>${escape(translations.t(section.task === 'speech' ? 'Text → Speech' : 'Speech → Text'))}</h2>`
            : ''
        body = `${label}<pre><code>POST /v1/${endpoint}</code></pre>${paragraph}`
      }
      if (section.id === 'you-need' && origin) {
        body = `<dl><dt>${escape(translations.t('Base URL'))}</dt><dd>${escape(origin)}/v1</dd><dt>${escape(translations.t('API Key'))}</dt><dd>YOUR_API_KEY</dd><dt>${escape(translations.t('Model ID'))}</dt><dd>YOUR_MODEL_ID · <a href="/docs/models">${escape(translations.t('View Models'))}</a></dd>${article.kind !== 'chat' ? `<dt>${escape(translations.t('Prompt'))}</dt><dd>${escape(translations.t(article.kind === 'video' ? 'Describe the video you want to create' : 'Describe the image you want to create'))}</dd>` : ''}</dl>`
      }
      if (section.id === 'you-need' && article.kind === 'audio' && origin) {
        const fields =
          section.task === 'speech'
            ? `<dt>${escape(translations.t('Voice'))}</dt><dd>YOUR_VOICE_ID</dd><dt>${escape(translations.t('Text'))}</dt><dd>${escape(translations.t('Text you want to read aloud'))}</dd>`
            : `<dt>${escape(translations.t('Audio file'))}</dt><dd>YOUR_AUDIO_FILE</dd>`
        body = `<dl><dt>${escape(translations.t('Base URL'))}</dt><dd>${escape(origin)}/v1</dd><dt>${escape(translations.t('API Key'))}</dt><dd>YOUR_API_KEY</dd><dt>${escape(translations.t('Model ID'))}</dt><dd>YOUR_MODEL_ID</dd>${fields}</dl>`
      }
      if (['quick-example', 'create-video'].includes(section.id) && origin) {
        let examples = buildChatExample(origin, translations.t('Hello!'))
        if (article.kind === 'image') {
          examples = buildImageExample(
            origin,
            translations.t('A small red house beside a lake')
          )
        }
        if (article.kind === 'video') {
          examples = buildVideoExample(
            origin,
            translations.t('A paper boat drifting on a calm lake')
          )
        }
        if (article.kind === 'audio') {
          examples = buildAudioExample(
            origin,
            section.task,
            translations.t('Hello! Welcome to New API.'),
            translations.t('Saved speech.mp3')
          )
        }
        body += Object.entries(examples)
          .filter(([language]) =>
            ['curl', 'python', 'javascript'].includes(language)
          )
          .map(
            ([language, code]) =>
              `<details><summary>${escape(language)}</summary><pre class="overflow-x-auto"><code>${escape(code)}</code></pre></details>`
          )
          .join('')
      }
      if (section.id === 'result' && section.task === 'transcription') {
        body = `<pre><code>${escape(JSON.stringify({ text: translations.t('Hello, this is the transcribed text.') }, null, 2))}</code></pre>${paragraph}`
      }
      if (section.id === 'response') {
        let response = {
          choices: [
            {
              message: {
                role: 'assistant',
                content: 'Hello! How can I help you?',
              },
            },
          ],
        }
        if (article.kind === 'image') {
          response = {
            created: 1720000000,
            data: [{ url: 'https://example.com/generated-image.png' }],
          }
        }
        if (article.kind === 'video') {
          response = { id: 'task_example', status: 'queued' }
        }
        body = `<pre><code>${escape(JSON.stringify(response, null, 2))}</code></pre>${paragraph}`
      }
      if (
        article.kind === 'video' &&
        origin &&
        ['check-status', 'download-video'].includes(section.id)
      ) {
        const examples = buildVideoExample(origin)
        const code =
          section.id === 'check-status' ? examples.status : examples.download
        const response =
          section.id === 'check-status'
            ? `<pre><code>${escape(JSON.stringify({ id: 'task_example', status: 'completed', progress: 100 }, null, 2))}</code></pre>`
            : ''
        body = `<pre><code>${escape(code)}</code></pre>${response}${paragraph}`
      }
      if (article.kind === 'quickstart') {
        if (section.id === 'create-key') {
          body += `<a href="/keys">${escape(translations.t('Create API Key'))}</a>`
        }
        if (section.id === 'choose-model') {
          body += `<a href="/docs/models">${escape(translations.t('View models'))}</a>`
        }
        if (section.id === 'quick-example') {
          body += `<a href="/docs/text-chat">${escape(translations.t('More options in Chat'))}</a>`
        }
        if (section.id === 'response') {
          body += `<p>${escape(translations.t('View request history, models and costs in Usage & Logs.'))}</p><a href="/usage-logs/common">${escape(translations.t('View Usage & Logs'))}</a>`
        }
      }
      if (section.id === 'advanced-options') {
        return `<section id="${sectionId}" class="mt-8 scroll-mt-24"><details><summary>${escape(translations.t(section.title))}</summary>${body}</details></section>`
      }
      return `<section id="${sectionId}" class="mt-8 scroll-mt-24"><h2 class="mb-3 text-xl font-semibold"><a href="#${sectionId}">${escape(translations.t(section.title))}</a></h2>${body}</section>`
    })
    .join('')
  if (article.kind === 'image') {
    extra += marked.parse(
      translations.t(
        'Image generation can take longer than chat. If the request times out, check [Usage & Logs](/usage-logs/common) and [Task Logs](/usage-logs/task) before trying again. Some image tasks can continue after a timeout. A `504 task_timeout` can mean the result is still being processed.'
      )
    )
  }
  if (article.kind === 'video') {
    extra += marked.parse(
      translations.t(
        'Video generation can take a few minutes depending on the model. Keep your Task ID to check later. Before resubmitting a slow or failed request, check its status, [Usage & Logs](/usage-logs/common) and [Task Logs](/usage-logs/task).'
      )
    )
  }
  if (article.kind === 'audio') {
    extra += `<p>${escape(translations.t('Not every audio model supports both tasks. Check the model type before using it.'))}</p><p>${escape(translations.t('These examples use synchronous speech models with MP3 output. Some models return Task IDs and require different request fields. Follow the model instructions for task-based speech.'))}</p>`
  }
  if (
    ['quickstart', 'chat', 'image', 'video', 'audio'].includes(article.kind)
  ) {
    extra += `<aside><h2>${escape(translations.t('Security note'))}</h2><p>${escape(translations.t('Do not share your API Key. In real applications, store it in an environment variable or secret manager instead of source code.'))}</p><a href="/docs/api-key">${escape(translations.t('API Key'))}</a></aside>`
  }
  if (article.kind === 'quickstart') {
    extra += `<p>${escape(translations.t('Never put an API Key in public frontend code.'))}</p>`
  }
  const quickstartBase =
    article.kind === 'quickstart'
      ? `<div><p>${escape(translations.t('Base URL'))}</p><code>${escape(origin ? `${origin}/v1` : translations.t('Open this page to copy your Base URL.'))}</code></div>`
      : ''
  const content = `<a href="#docs-content" class="sr-only focus:not-sr-only">${escape(translations.t('Skip to content'))}</a><header class="border-b px-6 py-4"><a href="/">New API</a> · <a href="/docs">${escape(translations.t('Docs'))}</a> · <a href="/docs/models">${escape(translations.t('Models'))}</a> · <a href="/docs/pricing">${escape(translations.t('Pricing'))}</a> · <a href="/dashboard">${escape(translations.t('Console'))}</a></header><div class="mx-auto grid max-w-[1500px] gap-8 p-6 lg:grid-cols-[230px_minmax(0,1fr)_180px]"><aside class="hidden lg:block"><nav aria-label="${escape(translations.t('Documentation navigation'))}">${navigation}</nav></aside><main id="docs-content" class="min-w-0"><nav aria-label="${escape(translations.t('Breadcrumb'))}"><a href="/docs">${escape(translations.t('Docs'))}</a> / ${escape(translations.t(article.group))} / ${escape(translations.t(article.kind === 'home' ? 'Overview' : article.title))}</nav><article class="prose max-w-none"><h1 class="mt-6 text-4xl font-semibold">${escape(translations.t(article.title))}</h1><p class="text-muted-foreground my-4">${escape(translations.t(article.description))}</p>${article.kind === 'home' ? `<div class="flex flex-wrap gap-3"><a href="/docs/quickstart">${escape(translations.t('Quickstart'))}</a><a href="/docs/models">${escape(translations.t('View models'))}</a><a href="/docs/pricing">${escape(translations.t('View pricing'))}</a></div>` : ''}${article.slug === 'api-key' ? `<p>${escape(translations.t('An API Key authenticates requests sent to HOTX API.'))}</p>` : ''}${quickstartBase}${articleSections}${extra}${article.console ? `<p class="mt-6"><a href="${escape(article.console)}">${escape(translations.t('Open in Console'))}</a></p>` : ''}</article><p class="mt-8 text-sm text-muted-foreground">${escape(translations.t('Model search follows catalog access settings.'))}</p><nav aria-label="${escape(translations.t('Previous and next page'))}" class="mt-8 flex justify-between border-t pt-6">${previous ? `<a href="${articlePath(previous)}">${escape(translations.t('Previous'))}: ${escape(translations.t(previous.slug === 'overview' ? 'Overview' : previous.title))}</a>` : '<span></span>'}${next ? `<a href="${articlePath(next)}">${escape(translations.t('Next'))}: ${escape(translations.t(next.title))}</a>` : ''}</nav></main><aside class="hidden lg:block"><nav aria-label="${escape(translations.t('On this page'))}">${article.sections
    .filter(
      (section) =>
        (article.kind !== 'video' || section.id !== 'response') &&
        (!section.task || section.task === 'speech')
    )
    .map(
      (section) =>
        `<p class="mb-3 text-xs"><a href="#${section.id}">${escape(translations.t(section.title))}</a></p>`
    )
    .join('')}</nav></aside></div>`
  const documentationTitle = translations.t('New API Documentation')
  const pageTitle = translations.t(article.title)
  const metadataTitle =
    pageTitle === documentationTitle
      ? documentationTitle
      : `${pageTitle} · ${documentationTitle}`
  const metadata = `<title>${escape(metadataTitle)}</title><meta name="description" content="${escape(translations.t(article.description))}"><link rel="canonical" href="${escape(canonical)}"><meta property="og:title" content="${escape(metadataTitle)}"><meta property="og:description" content="${escape(translations.t(article.description))}"><meta property="og:type" content="article"><meta property="og:url" content="${escape(canonical)}">`
  const ownedMetadata = metadata.replaceAll(
    /<(title|meta|link)\b/g,
    '<$1 data-docs-head'
  )
  let html = template
    .replaceAll(/<title>[\s\S]*?<\/title>/gi, '')
    .replaceAll(
      /<meta\b[^>]*(?:name=["'](?:description|title)["']|property=["']og:[^"']+["'])[^>]*>/gi,
      ''
    )
    .replaceAll(/<link\b[^>]*rel=["']canonical["'][^>]*>/gi, '')
  html = html
    .replace('</head>', `${ownedMetadata}</head>`)
    .replace(
      /<div\b(?=[^>]*\bid=["']root["'])[^>]*>\s*<\/div>/,
      `<div id="root" translate="no" class="notranslate" data-docs-prerender>${content}</div>`
    )
  html = html.replace(
    /<html([^>]*)>/,
    (_, attrs) =>
      `<html${attrs.replace(/\s+lang=["'][^"']*["']/, '')} lang="en">`
  )
  const target = path.join(directory, url, 'index.html')
  await fs.mkdir(path.dirname(target), { recursive: true })
  await fs.writeFile(target, html)
}
// Sitemap needs absolute URLs. Do not publish a fake deployment hostname.
if (origin) {
  await fs.writeFile(
    path.join(directory, 'docs-sitemap.xml'),
    `<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">${urls.map((url) => `<url><loc>${escape(url)}</loc></url>`).join('')}</urlset>\n`
  )
  const robotsPath = path.join(directory, 'robots.txt')
  const robots = await fs
    .readFile(robotsPath, 'utf8')
    .catch(() => 'User-agent: *\nAllow: /docs/\n')
  if (!robots.includes(`${origin}/docs-sitemap.xml`)) {
    await fs.writeFile(
      robotsPath,
      `${robots.trim()}\nSitemap: ${origin}/docs-sitemap.xml\n`
    )
  }
} else {
  console.log(
    'DOCS_SITE_URL is unset: relative canonical URLs generated; absolute sitemap requires a deployment origin.'
  )
}
console.log(
  `Prerendered ${data.articles.length} crawlable documentation pages from the article registry.`
)
