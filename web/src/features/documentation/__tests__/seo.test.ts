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
  expect(html).toContain('2. Choose a model')
  expect(html).toContain('3. Send a request')
  expect(html).toContain('4. View the result')
  expect(html).not.toContain('request-examples')
  expect(html).not.toContain('jq -n')
  expect(html).toContain('YOUR_API_KEY')
  expect(html).not.toContain('NEW_API_KEY')
  expect(html).toContain('/v1/chat/completions')
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

it('prerenders the short Chat guide with inline placeholders and ordered section anchors', () => {
  const html = fs.readFileSync(
    path.join(directory, 'docs/text-chat/index.html'),
    'utf8'
  )
  expect(html).toContain('YOUR_API_KEY')
  expect(html).toContain('YOUR_MODEL_ID')
  expect(html).not.toContain('jq -n')
  expect(html).not.toContain('NEW_API_')
  expect(html).toContain(
    '<title data-docs-head>Chat · New API Documentation</title>'
  )
  const positions = [
    'endpoint',
    'you-need',
    'quick-example',
    'response',
    'basic-parameters',
    'advanced-options',
  ].map((id) => html.indexOf(`id="${id}"`))
  expect(
    positions.every(
      (position, index) =>
        position >= 0 && (index === 0 || position > positions[index - 1])
    )
  ).toBe(true)
})

it('prerenders the Image endpoint and SDK examples without task polling or shell wrappers', () => {
  const html = fs.readFileSync(
    path.join(directory, 'docs/image/index.html'),
    'utf8'
  )
  expect(html).toContain('POST /v1/images/generations')
  expect(html).toContain('client.images.generate')
  expect(html).toContain('YOUR_MODEL_ID')
  expect(html).toContain('b64_json')
  expect(html).toContain('504 task_timeout')
  expect(html).not.toContain('jq -n')
  expect(html).not.toContain('NEW_API_')
  expect(html).not.toContain('/v1/tasks/')
  expect(html).toContain('href="/docs/text-chat"')
  expect(html).toContain('href="/docs/video"')
})

it('prerenders separate Video creation, status and download examples with public task status names', () => {
  const html = fs.readFileSync(
    path.join(directory, 'docs/video/index.html'),
    'utf8'
  )
  expect(html).toContain('POST /v1/videos')
  expect(html).toContain('/v1/videos/TASK_ID/content')
  expect(html).toContain('--output video.mp4')
  expect(html).toContain('in_progress')
  expect(html).toContain('task_example')
  expect(html).not.toContain('NEW_API_')
  expect(html).not.toContain('jq ')
  expect(html).not.toContain('range(120)')
  expect(html).toContain('href="/docs/image"')
  expect(html).toContain('href="/docs/audio"')
})

it('prerenders both Audio tasks with distinct anchors and direct examples', () => {
  const html = fs.readFileSync(
    path.join(directory, 'docs/audio/index.html'),
    'utf8'
  )
  expect(html).toContain('/v1/audio/speech')
  expect(html).toContain('/v1/audio/transcriptions')
  expect(html).toContain('YOUR_VOICE_ID')
  expect(html).toContain('YOUR_AUDIO_FILE')
  expect(html).toContain('--output speech.mp3')
  expect(html).toContain('id="transcription-endpoint"')
  expect(html.match(/id="endpoint"/g)).toHaveLength(1)
  expect(html).not.toContain('NEW_API_')
  expect(html).not.toContain('jq ')
  expect(html).toContain('href="/docs/video"')
  expect(html).toContain('href="/docs/integration-claude-code"')
})

it('gives beginners three linked steps followed by connection details and six guides', () => {
  const html = fs.readFileSync(path.join(directory, 'docs/index.html'), 'utf8')
  expect(html).toContain(' / Overview</nav>')
  expect(html).toContain('YOUR_API_KEY')
  expect(html).toContain('YOUR_MODEL_ID')
  expect(html).toContain('https://gateway.example/v1')
  const sections = ['get-started', 'you-need', 'popular-guides', 'step-1']
  const positions = sections.map((id) => html.indexOf(`id="${id}"`))
  expect(positions.every((position) => position >= 0)).toBe(true)
  expect(positions).toEqual([...positions].sort((a, b) => a - b))
  expect(html).toContain('href="/docs/audio"')
  expect(html).not.toContain('OpenAI Base URL')
})

it('publishes a short API Key guide with a Bearer header, authenticated model example and recovery steps', () => {
  const html = fs.readFileSync(
    path.join(directory, 'docs/api-key/index.html'),
    'utf8'
  )
  expect(html).toContain('Authorization: Bearer YOUR_API_KEY')
  expect(html).toContain('https://gateway.example/v1/models')
  expect(html).toContain('Keep your API Key safe')
  expect(html).toContain('Open API Keys')
  expect(html).toContain('Manage API Keys')
  expect(html).toContain('Disable or delete the old API Key')
  expect(html).not.toContain('Copy Connection Info')
  expect(html).not.toContain('Open in Console')
  expect(html).not.toContain('export ')
})

it('publishes New API as the overview heading before JavaScript runs', () => {
  const html = fs.readFileSync(path.join(directory, 'docs/index.html'), 'utf8')
  expect(html).toMatch(/<h1[^>]*>New API Documentation<\/h1>/)
})

it('preserves New API identity in public overview content and page metadata', () => {
  const html = fs.readFileSync(path.join(directory, 'docs/index.html'), 'utf8')
  expect(html).toContain('What is New API?')
  expect(html).toContain('<title data-docs-head>New API Documentation</title>')
  expect(html).toContain('<a href="/">New API</a>')
})
