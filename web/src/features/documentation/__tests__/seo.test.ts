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
import { execFileSync } from 'node:child_process'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'

import { afterAll, beforeAll, expect, it } from 'vitest'

import { articles } from '../content'

const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'new-api-docs-seo-'))
beforeAll(() => {
  fs.writeFileSync(
    path.join(directory, 'index.html'),
    '<html><head><title>New API</title><meta name="description" content="Site"></head><body><div id="root"></div></body></html>'
  )
  execFileSync(
    'bun',
    ['scripts/prerender-documentation.mjs', `--dist=${directory}`],
    { env: { ...process.env, DOCS_SITE_URL: 'https://gateway.example' } }
  )
})
afterAll(() => fs.rmSync(directory, { recursive: true, force: true }))
it('publishes semantic article text, SDK examples and one set of page metadata before JavaScript runs', () => {
  const html = fs.readFileSync(
    path.join(directory, 'docs/quickstart/index.html'),
    'utf8'
  )
  expect(html.match(/<title\b/g)).toHaveLength(1)
  expect(html.match(/rel="canonical"/g)).toHaveLength(1)
  expect(html).toContain('href="https://gateway.example/docs/quickstart"')
  expect(html).toContain('data-docs-head')
  expect(html).toContain('data-docs-prerender')
  expect(html).toContain('<h1')
  expect(html).toContain('Choose a chat model')
  expect(html).toContain('NEW_API_KEY')
  expect(html).toContain('/v1/models')
  expect(html).not.toContain('sk-')
})
it('indexes every canonical article once and publishes provider navigation with no duplicate overview route', () => {
  const sitemap = fs.readFileSync(
    path.join(directory, 'docs-sitemap.xml'),
    'utf8'
  )
  expect(sitemap.match(/<loc>/g)).toHaveLength(articles.length)
  expect(sitemap).not.toContain('/docs/overview')
  expect(fs.existsSync(path.join(directory, 'docs/overview/index.html'))).toBe(
    false
  )
  expect(fs.readFileSync(path.join(directory, 'robots.txt'), 'utf8')).toContain(
    'Sitemap: https://gateway.example/docs-sitemap.xml'
  )
  expect(
    fs.readFileSync(
      path.join(directory, 'docs/admin-providers/index.html'),
      'utf8'
    )
  ).toContain('/docs/provider-14')
})
