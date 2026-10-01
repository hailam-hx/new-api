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

import { marked } from 'marked'

import { articles, groups } from '../src/features/documentation/content.ts'
import { buildQuickstart } from '../src/features/documentation/lib.ts'

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
      `<section><p class="text-muted-foreground mb-2 text-xs font-semibold uppercase">${escape(group)}</p><ul class="mb-6 space-y-2">${data.articles
        .filter(
          (article) => article.group === group && article.kind !== 'provider'
        )
        .map(
          (article) =>
            `<li><a class="text-sm hover:underline" href="${articlePath(article)}">${escape(article.title)}</a></li>`
        )
        .join('')}</ul></section>`
  )
  .join('')
const urls = []
for (const article of data.articles) {
  const url = articlePath(article)
  urls.push(origin + url)
  const canonical = origin + url
  const navigationArticles = data.articles.filter(
    (item) => item.kind !== 'provider'
  )
  const index = navigationArticles.indexOf(article)
  const previous = navigationArticles[index - 1]
  const next = index >= 0 ? navigationArticles[index + 1] : undefined
  let extra = ''
  if (article.kind === 'reference') {
    extra = `<section id="operations"><h2>Endpoints</h2>${reference.endpoints.map((endpoint) => `<section id="${`${endpoint.method}-${endpoint.path}`.toLowerCase().replaceAll(/[^a-z0-9-]/g, '-')}"><h3>${escape(endpoint.method)} ${escape(endpoint.path)}</h3><p>${escape(endpoint.handler)}</p><details><summary>Authentication and middleware</summary><pre>${escape(endpoint.middleware.join('\n'))}</pre></details>${endpoint.schemaSource ? `<details><summary>Parameters, request and response schemas</summary><pre>${escape(JSON.stringify({ parameters: endpoint.parameters, requestBody: endpoint.requestBody, responses: endpoint.responses }, null, 2))}</pre></details>` : '<p>No matching OpenAPI schema is available for this registered operation.</p>'}<p>Source: ${escape(endpoint.source)}</p></section>`).join('')}</section>`
  }
  if (article.kind === 'errors') {
    extra = `<section id="error-index"><h2>Gateway error constants</h2><table><thead><tr><th>Error Code</th><th>HTTP Status</th><th>Source</th></tr></thead><tbody>${reference.errors.map((error) => `<tr id="${error.code.replaceAll(/[^a-z0-9-]/gi, '-')}"><td>${escape(error.code)}</td><td>${escape(error.status ?? (error.kind === 'diagnostic' ? 'Diagnostic marker' : 'Depends on call site'))}</td><td>${escape(error.source)}</td></tr>`).join('')}</tbody></table></section>`
  }
  if (article.slug === 'admin-providers') {
    extra += `<section id="provider-directory"><h2>Provider documentation</h2><ul>${data.articles
      .filter((item) => item.kind === 'provider')
      .map(
        (provider) =>
          `<li><a href="${articlePath(provider)}">${escape(provider.title)}</a></li>`
      )
      .join('')}</ul></section>`
  }
  if (['quickstart', 'sdk'].includes(article.kind)) {
    if (origin) {
      extra += `<section id="request-examples"><h2>Request examples</h2>${[
        false,
        true,
      ]
        .map(
          (stream) =>
            `<section><h3>${stream ? 'Streaming' : 'Minimal request'}</h3>${Object.entries(
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
      extra +=
        '<p>Open this page in your browser for SDK examples using your current deployment URL.</p>'
    }
  }
  const content = `<a href="#docs-content" class="sr-only focus:not-sr-only">Skip to content</a><header class="border-b px-6 py-4"><a href="/">New API</a> · <a href="/docs">Docs</a> · <a href="/docs/models">Models</a> · <a href="/docs/pricing">Pricing</a> · <a href="/docs/api-reference">API Reference</a> · <a href="/dashboard">Console</a></header><div class="mx-auto grid max-w-[1500px] gap-8 p-6 lg:grid-cols-[230px_minmax(0,1fr)_180px]"><aside class="hidden lg:block"><nav aria-label="Documentation navigation">${navigation}</nav></aside><main id="docs-content" class="min-w-0"><nav aria-label="Breadcrumb"><a href="/docs">Docs</a> / ${escape(article.group)} / ${escape(article.title)}</nav><article class="prose max-w-none"><h1 class="mt-6 text-4xl font-semibold">${escape(article.title)}</h1><p class="text-muted-foreground my-4">${escape(article.description)}</p>${article.kind === 'home' ? '<section class="grid gap-6 border-y py-6 sm:grid-cols-2"><div><h2>I want to use the API</h2><p>Create an API key → choose a model → send your first request.</p><a href="/docs/quickstart">Quickstart</a> · <a href="/docs/models">Browse Models</a></div><div><h2>I manage the platform</h2><p>Configure channels → models → pricing → routing.</p><a href="/docs/admin-overview">Admin Guide</a> · <a href="/docs/admin-providers">Providers</a></div></section>' : ''}${article.sections.map((section) => `<section id="${section.id}" class="mt-8 scroll-mt-40"><h2 class="mb-4 text-xl font-semibold"><a href="#${section.id}">${escape(section.title)}</a></h2>${marked.parse(section.body)}</section>`).join('')}${extra}${article.console ? `<p class="mt-6"><a href="${escape(article.console)}">Open in Console</a></p>` : ''}</article><p class="mt-8 text-sm text-muted-foreground">Documentation articles currently use English as the source language. Live catalogs follow platform access settings.</p><nav aria-label="Previous and next page" class="mt-8 flex justify-between border-t pt-6">${previous ? `<a href="${articlePath(previous)}">Previous: ${escape(previous.title)}</a>` : '<span></span>'}${next ? `<a href="${articlePath(next)}">Next: ${escape(next.title)}</a>` : ''}</nav></main><aside class="hidden lg:block"><nav aria-label="On this page">${article.sections.map((section) => `<p class="mb-3 text-xs"><a href="#${section.id}">${escape(section.title)}</a></p>`).join('')}</nav></aside></div>`
  const metadata = `<title>${escape(article.title)} · New API Documentation</title><meta name="description" content="${escape(article.description)}"><link rel="canonical" href="${escape(canonical)}"><meta property="og:title" content="${escape(article.title)} · New API Documentation"><meta property="og:description" content="${escape(article.description)}"><meta property="og:type" content="article"><meta property="og:url" content="${escape(canonical)}">`
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
