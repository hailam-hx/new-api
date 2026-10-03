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
import assert from 'node:assert/strict'
import fs from 'node:fs/promises'

const { chromium } = await import(
  process.env.PLAYWRIGHT_MODULE_PATH || 'playwright'
)

const baseUrl = process.env.DOCS_QA_URL || 'http://localhost:5173'

const browser = await chromium.launch({
  headless: true,
  executablePath: process.env.DOCS_CHROME_PATH,
})
const context = await browser.newContext({
  viewport: { width: 1440, height: 1000 },
})
await context.grantPermissions(['clipboard-read', 'clipboard-write'])
await context.addInitScript(() => {
  if (!localStorage.getItem('i18nextLng')) {
    localStorage.setItem('i18nextLng', 'en')
  }
})
let publishedApiAddress = false
let restricted = false,
  pricingRequests = 0
const page = await context.newPage()
const errors = []
page.on('pageerror', (e) => errors.push(e.message))
async function assertAudioTabsFit(label) {
  const fits = await page
    .getByRole('tablist', { name: label, exact: true })
    .evaluate((element) => {
      const bounds = element.getBoundingClientRect()
      return [...element.querySelectorAll('[role="tab"]')].every((tab) => {
        const box = tab.getBoundingClientRect()
        return (
          box.top >= bounds.top + 2 &&
          box.bottom <= bounds.bottom - 2 &&
          box.left >= bounds.left &&
          box.right <= bounds.right
        )
      })
    })
  assert(fits, 'Audio task tabs must stay inside their container with padding')
}

await context.route('**/api/**', async (route) => {
  const path = new URL(route.request().url()).pathname
  let status = 200,
    body = { success: true, data: '' }
  if (path === '/api/status') {
    body = {
      success: true,
      data: {
        system_name: 'New API',
        server_address: 'https://api-fixture.example/proxy/',
        api_info: publishedApiAddress
          ? [
              {
                url: 'https://published-api.example/gateway/v1/',
                route: 'API',
                description: '',
              },
            ]
          : [],
        price: 1,
        usd_exchange_rate: 1,
        HeaderNavModules: {
          pricing: { enabled: true, requireAuth: restricted },
        },
      },
    }
  } else if (path === '/api/setup') {
    body = { success: true, data: { status: true } }
  } else if (path === '/api/user/auth/refresh') {
    status = 401
    body = {
      success: false,
      code: 'AUTH_UNAUTHORIZED',
      message: 'Unauthenticated',
    }
  } else if (path === '/api/pricing') {
    pricingRequests++
    body = {
      success: true,
      data: [
        {
          id: 1,
          model_name: 'fixture/chat-model',
          quota_type: 0,
          model_ratio: 1,
          completion_ratio: 2,
          enable_groups: ['default'],
          supported_endpoint_types: ['openai'],
          vendor_id: 1,
        },
      ],
      vendors: [{ id: 1, name: 'Fixture provider' }],
      group_ratio: { default: 1 },
      usable_group: { default: 'Default' },
      supported_endpoint: {},
      auto_groups: [],
    }
  }
  await route.fulfill({ status, json: body })
})
await page.goto(`${baseUrl}/docs`)
await page.waitForLoadState('networkidle')
await page.evaluate(() => localStorage.setItem('i18nextLng', 'en'))
await page.reload()
await page
  .getByRole('button', { name: 'Change language', exact: true })
  .waitFor()
await page
  .getByRole('heading', { name: 'New API Documentation', exact: true })
  .waitFor()
const overviewMain = page.locator('main')
assert.equal(
  await overviewMain
    .getByRole('navigation', { name: 'Breadcrumb' })
    .textContent()
    .then((text) => text.replaceAll(/\s+/g, ' ').trim()),
  'DocsGetting StartedOverview'
)
assert.deepEqual(
  await overviewMain.locator('section[id] > h2').allTextContents(),
  [
    'Start in 3 steps',
    'You need three things',
    'Popular guides',
    'What is New API?',
  ]
)
assert.deepEqual(
  await page
    .locator('#get-started li a')
    .evaluateAll((links) => links.map((link) => link.getAttribute('href'))),
  ['/docs/api-key', '/docs/models', '/docs/quickstart']
)
assert.deepEqual(
  await page
    .locator('#popular-guides a')
    .evaluateAll((links) => links.map((link) => link.getAttribute('href'))),
  [
    '/docs/text-chat',
    '/docs/image',
    '/docs/video',
    '/docs/audio',
    '/docs/integration-claude-code',
    '/docs/integration-cursor',
  ]
)
assert.equal(
  await overviewMain
    .getByRole('button', { name: 'Copy to clipboard', exact: true })
    .count(),
  1
)
await overviewMain
  .getByRole('button', { name: 'Copy to clipboard', exact: true })
  .click()
assert.equal(
  await page.evaluate(() => navigator.clipboard.readText()),
  'https://api-fixture.example/proxy/v1'
)
for (const width of [1440, 1091, 768, 390]) {
  await page.setViewportSize({ width, height: 1000 })
  assert(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth
    )
  )
  const cards = await page.locator('#get-started li').evaluateAll((items) =>
    items.map((item) => ({
      x: item.getBoundingClientRect().x,
      y: item.getBoundingClientRect().y,
    }))
  )
  if (width === 390) {
    assert(
      cards.every((card) => card.x === cards[0].x) && cards[1].y > cards[0].y
    )
  } else assert(cards.every((card) => card.y === cards[0].y))
}
await page.setViewportSize({ width: 1440, height: 1000 })
await page.screenshot({
  path: '/tmp/new-api-overview-en-desktop.png',
  fullPage: true,
})
await page.getByRole('link', { name: 'Quickstart', exact: true }).last().click()
await page.getByRole('heading', { name: 'Quickstart', exact: true }).waitFor()
const quickstartMain = page.locator('main')
assert.deepEqual(
  await quickstartMain.locator('section[id] h2').allTextContents(),
  [
    '1. Create an API Key',
    '2. Choose a model',
    '3. Send a request',
    '4. View the result',
  ]
)
assert.deepEqual(
  await page
    .getByRole('navigation', { name: 'On this page' })
    .locator('a')
    .evaluateAll((links) => links.map((link) => link.getAttribute('href'))),
  ['#create-key', '#choose-model', '#quick-example', '#response']
)
assert.equal(
  await quickstartMain
    .getByRole('button', { name: 'Create API Key', exact: true })
    .getAttribute('href'),
  '/keys'
)
assert.equal(
  await quickstartMain
    .getByRole('button', { name: 'View Usage & Logs', exact: true })
    .getAttribute('href'),
  '/usage-logs/common'
)
assert(
  !(await quickstartMain.getByText('Advanced options', { exact: true }).count())
)
for (const language of ['cURL', 'Python', 'JavaScript']) {
  await page.getByRole('tab', { name: language, exact: true }).click()
  const panel = page.getByRole('tabpanel', { name: language, exact: true })
  await panel.locator('pre').waitFor()
  assert(!/NEW_API_|jq|os.environ/.test(await panel.textContent()))
  const visible = (await panel.locator('pre').textContent()).trimEnd()
  await panel
    .getByRole('button', { name: 'Copy to clipboard', exact: true })
    .last()
    .click()
  assert.equal(
    await page.evaluate(() => navigator.clipboard.readText()),
    visible
  )
  assert(
    visible.includes('YOUR_API_KEY') &&
      visible.includes('YOUR_MODEL_ID') &&
      visible.includes('https://api-fixture.example/proxy/v1')
  )
}
assert.equal(await page.locator('link[rel=canonical]').count(), 1)
assert.equal(await page.locator('head title').count(), 1)
assert.equal(await page.locator('head meta[name=description]').count(), 1)
for (const width of [1440, 1091, 768, 390]) {
  await page.setViewportSize({ width, height: 1000 })
  assert(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth
    )
  )
}
await page.setViewportSize({ width: 1440, height: 1000 })
publishedApiAddress = true
await page.reload()
await page.getByRole('tab', { name: 'JavaScript', exact: true }).click()
await page
  .getByRole('tabpanel', { name: 'JavaScript', exact: true })
  .getByText('https://published-api.example/gateway/v1', { exact: false })
  .waitFor()
assert(
  !(
    await page
      .getByRole('tabpanel', { name: 'JavaScript', exact: true })
      .textContent()
  ).includes('/v1/v1')
)
await page
  .getByRole('button', { name: 'Search documentation', exact: true })
  .click()
await page
  .getByRole('textbox', { name: 'Search documentation' })
  .fill('/v1/chat/completions')
await page.getByRole('dialog').getByRole('link', { name: /^Chat/ }).click()
await page.getByRole('heading', { name: 'Chat', exact: true }).waitFor()
assert.equal(await page.locator('link[rel=canonical]').count(), 1)
assert.equal(
  await page.locator('link[rel=canonical]').getAttribute('href'),
  `${baseUrl}/docs/text-chat`
)
const chatToc = page.getByRole('navigation', { name: 'On this page' })
assert.deepEqual(
  await chatToc
    .getByRole('link')
    .evaluateAll((links) => links.map((link) => link.getAttribute('href'))),
  [
    '#endpoint',
    '#you-need',
    '#quick-example',
    '#response',
    '#basic-parameters',
    '#advanced-options',
  ]
)
assert.equal(
  await page.locator('#basic-parameters').getByRole('row').count(),
  3
)
assert.equal(
  await page
    .getByRole('button', { name: 'Advanced options', exact: true })
    .getAttribute('aria-expanded'),
  'false'
)
for (const language of ['cURL', 'Python', 'JavaScript']) {
  await page.getByRole('tab', { name: language, exact: true }).click()
  const panel = page.getByRole('tabpanel', { name: language, exact: true })
  const code = await panel.locator('pre').textContent()
  assert(code.includes('YOUR_API_KEY') && code.includes('YOUR_MODEL_ID'))
  assert(!/jq|NEW_API_|os.environ|process.env/.test(code))
  await panel
    .getByRole('button', { name: 'Copy to clipboard', exact: true })
    .last()
    .click()
  assert.equal(
    await page.evaluate(() => navigator.clipboard.readText()),
    code.trimEnd()
  )
}
await page.getByRole('tab', { name: 'cURL', exact: true }).click()
for (const width of [1440, 1091, 768, 390]) {
  await page.setViewportSize({ width, height: 959 })
  assert(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth
    )
  )
  if (width >= 1091) {
    assert(
      await page
        .locator('#quick-example pre')
        .evaluate(
          (element) => element.scrollWidth <= element.parentElement.clientWidth
        )
    )
  }
}
await page.setViewportSize({ width: 1440, height: 1000 })
for (const anchor of [
  'endpoint',
  'you-need',
  'quick-example',
  'response',
  'basic-parameters',
  'advanced-options',
]) {
  await chatToc.locator(`a[href="#${anchor}"]`).click()
  assert.equal(new URL(page.url()).hash, `#${anchor}`)
}
await page
  .getByRole('button', { name: 'Advanced options', exact: true })
  .click()
await page
  .getByText('Adjusts randomness if the model supports it', { exact: true })
  .waitFor()
assert.equal(
  await page.locator('#advanced-options').getByRole('row').count(),
  4
)
const chatPagination = page.getByRole('navigation', {
  name: 'Previous and next page',
})
assert.deepEqual(
  await chatPagination
    .getByRole('link')
    .evaluateAll((links) => links.map((link) => link.getAttribute('href'))),
  ['/docs/pricing', '/docs/image']
)
await page
  .getByRole('button', { name: 'Advanced options', exact: true })
  .click()
await page.evaluate(() => window.scrollTo(0, 0))
await page.screenshot({ path: '/tmp/new-api-chat-desktop.png' })
await page.setViewportSize({ width: 390, height: 844 })
await page.screenshot({ path: '/tmp/new-api-chat-mobile.png' })
await page.setViewportSize({ width: 1440, height: 1000 })
await page.goto(`${baseUrl}/docs/models`)
await page
  .getByRole('link', { name: 'fixture/chat-model', exact: true })
  .waitFor()
assert.deepEqual(
  await page
    .locator('#docs-content')
    .getByRole('columnheader')
    .allTextContents(),
  ['Model ID', 'Provider', 'Type', 'Price']
)
const modelsApi = page.locator('#list-models-api')
await modelsApi
  .getByRole('heading', {
    name: 'Get the model list with the API',
    exact: true,
  })
  .waitFor()
assert(
  await modelsApi.evaluate(
    (section) =>
      !!(
        document
          .querySelector('#live-catalog')
          .compareDocumentPosition(section) & Node.DOCUMENT_POSITION_FOLLOWING
      )
  )
)
assert((await modelsApi.innerText()).includes('GET /v1/models'))
assert.deepEqual(
  JSON.parse(await modelsApi.locator('pre').nth(1).innerText()),
  {
    object: 'list',
    data: [{ id: 'MODEL_ID', object: 'model' }],
  }
)
await modelsApi
  .getByRole('button', { name: 'Copy to clipboard', exact: true })
  .nth(1)
  .click()
assert.equal(
  await page.evaluate(() => navigator.clipboard.readText()),
  'curl https://hotx-api.com/v1/models \\' +
    '\n  -H "Authorization: Bearer YOUR_API_KEY"'
)
await page.setViewportSize({ width: 390, height: 844 })
assert(
  await page
    .locator('#docs-content tbody tr')
    .first()
    .evaluate(
      (row) =>
        row.lastElementChild.getBoundingClientRect().bottom <=
        row.getBoundingClientRect().bottom + 1
    ),
  'mobile model cells must remain inside their row'
)
assert(
  await page.evaluate(
    () => document.documentElement.scrollWidth <= window.innerWidth
  ),
  'model catalog must not overflow the mobile viewport'
)
await page.setViewportSize({ width: 1440, height: 1000 })
await page
  .getByRole('link', { name: 'fixture/chat-model', exact: true })
  .click()
await page
  .getByRole('heading', { name: 'fixture/chat-model', exact: true })
  .waitFor()
assert((await page.getByText('Not provided', { exact: true }).count()) >= 1)
assert(!(await page.getByText('Context', { exact: true }).count()))
await page.goto(`${baseUrl}/docs/pricing`)
assert.equal(await page.locator('#list-models-api').count(), 0)
await page
  .getByRole('heading', { name: 'How to read prices', exact: true })
  .waitFor()
await page
  .getByRole('link', { name: 'fixture/chat-model', exact: true })
  .waitFor()
assert.deepEqual(
  await page
    .locator('#docs-content')
    .getByRole('columnheader')
    .allTextContents(),
  ['Model', 'Type', 'Price']
)
assert(
  await page
    .locator('#price-table')
    .getByText('2 USD / 1M token', { exact: true })
    .isVisible()
)
assert(
  await page
    .locator('#price-table')
    .getByText('4 USD / 1M token', { exact: true })
    .isVisible()
)
assert.equal(
  await page.locator('#price-table').getByText('From', { exact: true }).count(),
  0
)
assert.deepEqual(
  await page
    .getByRole('navigation', { name: 'On this page' })
    .getByRole('link')
    .allTextContents(),
  ['How to read prices', 'Price list']
)
for (const [slug, title, endpoint] of [
  ['image', 'Image', 'client.images.generate'],
  ['video', 'Video', '/v1/videos'],
  ['audio', 'Audio', '/v1/audio/speech'],
]) {
  await page.goto(`${baseUrl}/docs/${slug}`)
  await page.getByRole('heading', { name: title, exact: true }).waitFor()
  await page.getByRole('tab', { name: 'JavaScript', exact: true }).click()
  await page
    .getByRole('tabpanel', { name: 'JavaScript' })
    .getByText(endpoint, { exact: false })
    .waitFor()
  assert.equal(
    await page
      .getByRole('button', { name: 'Advanced options' })
      .getAttribute('aria-expanded'),
    'false'
  )
}
await page.goto(`${baseUrl}/docs/image`)
await page.getByRole('heading', { name: 'Image', exact: true }).waitFor()
assert.equal(
  await page.locator('#basic-parameters').getByRole('row').count(),
  3
)
assert.equal(
  await page
    .getByRole('navigation', { name: 'On this page' })
    .getByRole('link')
    .count(),
  6
)
for (const anchor of [
  'endpoint',
  'you-need',
  'quick-example',
  'response',
  'basic-parameters',
  'advanced-options',
]) {
  await page
    .getByRole('navigation', { name: 'On this page' })
    .locator(`a[href="#${anchor}"]`)
    .click()
  assert.equal(new URL(page.url()).hash, `#${anchor}`)
}

assert.deepEqual(
  await page
    .getByRole('navigation', { name: 'Previous and next page' })
    .getByRole('link')
    .evaluateAll((links) => links.map((link) => link.getAttribute('href'))),
  ['/docs/text-chat', '/docs/video']
)
for (const language of ['cURL', 'Python', 'JavaScript']) {
  await page.getByRole('tab', { name: language, exact: true }).click()
  const panel = page.getByRole('tabpanel', { name: language })
  const code = await panel.locator('pre').textContent()
  assert(code.includes('YOUR_API_KEY') && code.includes('YOUR_MODEL_ID'))
  assert(
    !code.includes('NEW_API_') &&
      !code.includes('jq ') &&
      !code.includes('response_format')
  )
  await panel
    .getByRole('button', { name: 'Copy to clipboard', exact: true })
    .last()
    .click()
  assert.equal(
    await page.evaluate(() => navigator.clipboard.readText()),
    code.trimEnd()
  )
}
await page
  .getByRole('button', { name: 'Advanced options', exact: true })
  .click()
await page.locator('#advanced-options').getByRole('row').first().waitFor()
assert.equal(
  await page.locator('#advanced-options').getByRole('row').count(),
  5
)
await page
  .getByRole('button', { name: 'Advanced options', exact: true })
  .click()
for (const width of [1440, 1091, 768, 390]) {
  await page.setViewportSize({ width, height: 959 })
  assert(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth
    )
  )
  if (width >= 1091) {
    assert(
      await page
        .locator('#quick-example pre')
        .evaluate(
          (element) => element.scrollWidth <= element.parentElement.clientWidth
        )
    )
  }
}
await page.setViewportSize({ width: 1440, height: 1000 })
await page.goto(`${baseUrl}/docs/video`)
await page.getByRole('heading', { name: 'Video', exact: true }).waitFor()
const videoToc = page.getByRole('navigation', { name: 'On this page' })
assert.equal(await videoToc.getByRole('link').count(), 8)
for (const id of [
  'endpoint',
  'you-need',
  'how-it-works',
  'create-video',
  'check-status',
  'download-video',
  'basic-parameters',
  'advanced-options',
]) {
  await videoToc.locator(`a[href="#${id}"]`).click()
  assert.equal(new URL(page.url()).hash, `#${id}`)
}
assert.equal(
  await page.locator('#basic-parameters').getByRole('row').count(),
  3
)
assert.equal(await page.locator('#check-status').getByRole('row').count(), 6)
assert.deepEqual(
  await page
    .getByRole('navigation', { name: 'Previous and next page' })
    .getByRole('link')
    .evaluateAll((links) => links.map((link) => link.getAttribute('href'))),
  ['/docs/image', '/docs/audio']
)
for (const language of ['cURL', 'Python', 'JavaScript']) {
  await page.getByRole('tab', { name: language, exact: true }).click()
  const panel = page.getByRole('tabpanel', { name: language, exact: true })
  const code = await panel.locator('pre').textContent()
  assert(
    code.includes('/v1/videos') &&
      code.includes('YOUR_MODEL_ID') &&
      code.includes('YOUR_API_KEY')
  )
  assert(
    !/jq|NEW_API_|sleep|setTimeout|writeFile|TASK_ID|encodeURIComponent|for \(/.test(
      code
    )
  )
  await panel
    .getByRole('button', { name: 'Copy to clipboard', exact: true })
    .last()
    .click()
  assert.equal(
    await page.evaluate(() => navigator.clipboard.readText()),
    code.trimEnd()
  )
}
assert(
  (await page.locator('#check-status pre').first().textContent()).includes(
    '/v1/videos/TASK_ID'
  )
)
assert(
  (await page.locator('#download-video pre').textContent()).includes(
    '--output video.mp4'
  )
)
await page
  .getByRole('button', { name: 'Advanced options', exact: true })
  .click()
await page.locator('#advanced-options').getByRole('row').first().waitFor()
assert.equal(
  await page.locator('#advanced-options').getByRole('row').count(),
  4
)
await page
  .getByRole('button', { name: 'Advanced options', exact: true })
  .click()
for (const width of [1440, 1091, 768, 390]) {
  await page.setViewportSize({ width, height: 959 })
  assert(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth
    )
  )
  const columns = await page
    .locator('#how-it-works ol')
    .evaluate(
      (element) =>
        getComputedStyle(element).gridTemplateColumns.split(' ').length
    )
  assert.equal(columns, width >= 768 ? 3 : 1)
}
await page.setViewportSize({ width: 1440, height: 1000 })
await page.goto(`${baseUrl}/docs/audio`)
await page.getByRole('heading', { name: 'Audio', exact: true }).waitFor()
assert.equal(
  await page
    .locator('aside')
    .first()
    .locator('a[aria-current="page"]')
    .getAttribute('href'),
  '/docs/audio'
)
assert.deepEqual(
  await page
    .getByRole('navigation', { name: 'Previous and next page' })
    .getByRole('link')
    .evaluateAll((links) => links.map((link) => link.getAttribute('href'))),
  ['/docs/video', '/docs/integration-claude-code']
)
for (const [task, label, endpoint, basicRows] of [
  ['speech', 'Text → Speech', '/v1/audio/speech', 4],
  ['transcription', 'Speech → Text', '/v1/audio/transcriptions', 3],
]) {
  await page.getByRole('tab', { name: label, exact: true }).click()
  await assertAudioTabsFit('Audio task')
  const taskPanel = page.getByRole('tabpanel', { name: label, exact: true })
  await taskPanel
    .locator('#endpoint')
    .getByText(`POST ${endpoint}`, { exact: true })
    .waitFor()
  const toc = page.getByRole('navigation', { name: 'On this page' })
  assert.equal(await toc.getByRole('link').count(), 6)
  for (const id of [
    'endpoint',
    'you-need',
    'quick-example',
    'result',
    'basic-parameters',
    'advanced-options',
  ]) {
    await toc.locator(`a[href="#${id}"]`).click()
    assert.equal(new URL(page.url()).hash, `#${id}`)
  }
  assert.equal(
    await taskPanel.locator('#basic-parameters').getByRole('row').count(),
    basicRows
  )
  assert.equal(
    await taskPanel
      .getByRole('button', { name: 'Advanced options', exact: true })
      .getAttribute('aria-expanded'),
    'false'
  )
  for (const language of ['cURL', 'Python', 'JavaScript']) {
    await taskPanel.getByRole('tab', { name: language, exact: true }).click()
    const panel = taskPanel.getByRole('tabpanel', {
      name: language,
      exact: true,
    })
    const code = await panel.locator('pre').textContent()
    assert(
      code.includes(endpoint) &&
        code.includes('YOUR_API_KEY') &&
        code.includes('YOUR_MODEL_ID')
    )
    assert(
      !/jq|NEW_API_|process.env|os.environ|async function request|set -euo/.test(
        code
      )
    )
    assert(
      code.includes(task === 'speech' ? 'YOUR_VOICE_ID' : 'YOUR_AUDIO_FILE')
    )
    await panel
      .getByRole('button', { name: 'Copy to clipboard', exact: true })
      .last()
      .click()
    assert.equal(
      await page.evaluate(() => navigator.clipboard.readText()),
      code.trimEnd()
    )
  }
  await taskPanel
    .getByRole('button', { name: 'Advanced options', exact: true })
    .click()
  await taskPanel
    .locator('#advanced-options')
    .getByRole('row')
    .first()
    .waitFor()
  assert.equal(
    await taskPanel.locator('#advanced-options').getByRole('row').count(),
    3
  )
  await taskPanel
    .getByRole('button', { name: 'Advanced options', exact: true })
    .click()
  for (const width of [1440, 1091, 768, 390]) {
    await page.setViewportSize({ width, height: 959 })
    assert(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth
      )
    )
  }
  await page.setViewportSize({ width: 1440, height: 1000 })
}
await page.goto(`${baseUrl}/docs`)
await page
  .getByRole('heading', { name: 'New API Documentation', exact: true })
  .waitFor()
await page.screenshot({
  path: `${process.env.DOCS_QA_DIRECTORY || '/tmp'}/new-api-docs-desktop.png`,
  fullPage: false,
})
await page.setViewportSize({ width: 390, height: 844 })
await page.goto(`${baseUrl}/docs/quickstart`)
await page.getByRole('heading', { name: 'Quickstart', exact: true }).waitFor()
await page.getByRole('button', { name: 'Documentation menu' }).click()
await page
  .getByRole('dialog')
  .getByRole('link', { name: 'FAQ', exact: true })
  .click()
await page.getByRole('heading', { name: 'FAQ', exact: true }).waitFor()
await page.getByRole('dialog').waitFor({ state: 'hidden' })
await page.goto(`${baseUrl}/docs/quickstart`)
await page.getByRole('heading', { name: 'Quickstart', exact: true }).waitFor()
assert(
  await page.evaluate(
    () => document.documentElement.scrollWidth <= window.innerWidth
  )
)
await page.evaluate(() => localStorage.setItem('newapi:theme:v1:mode', 'dark'))
await page.reload()
await page.getByRole('heading', { name: 'Quickstart', exact: true }).waitFor()
assert(await page.locator('html').evaluate((e) => e.classList.contains('dark')))
await page.screenshot({
  path: `${process.env.DOCS_QA_DIRECTORY || '/tmp'}/new-api-docs-mobile-dark.png`,
  fullPage: false,
})
restricted = true
await context.clearCookies()
await page.evaluate(() => {
  localStorage.removeItem('status')
  localStorage.removeItem('system-config-storage')
})
pricingRequests = 0
await page.goto(`${baseUrl}/docs/models`)
await page.getByText('Catalog access restricted', { exact: true }).waitFor()
assert.equal(pricingRequests, 0)
assert.deepEqual(errors, [])
await page.setViewportSize({ width: 1440, height: 1000 })
await page.goto(`${baseUrl}/docs`)
await page.getByRole('button', { name: 'Change language', exact: true }).click()
await page.getByRole('menuitem', { name: 'Tiếng Việt', exact: true }).click()
await page.getByRole('button', { name: 'Tìm tài liệu', exact: true }).waitFor()
// Vietnamese navigation must remain readable at the reported viewport while scrolling.
await page.setViewportSize({ width: 1091, height: 959 })
await page.getByRole('button', { name: 'Tìm tài liệu', exact: true }).waitFor()
await page.evaluate(() => window.scrollTo(0, 220))
const header = page.locator('header').first()
assert(
  !['transparent', 'rgba(0, 0, 0, 0)'].includes(
    await header.evaluate(
      (element) => getComputedStyle(element).backgroundColor
    )
  )
)
assert(
  await header
    .getByRole('link')
    .evaluateAll((links) =>
      links
        .filter((link) => link.getBoundingClientRect().width > 0)
        .every((link) => link.scrollWidth <= link.clientWidth)
    )
)
const pricingShortcut = await header
  .getByRole('navigation', { name: 'Liên kết nhanh tài liệu', exact: true })
  .getByRole('link', { name: 'Giá cả', exact: true })
  .boundingBox()
const searchShortcut = await header
  .getByRole('button', { name: 'Tìm tài liệu', exact: true })
  .boundingBox()
assert(pricingShortcut && searchShortcut)
assert(
  searchShortcut.x - (pricingShortcut.x + pricingShortcut.width) >= 16,
  'Documentation shortcuts must have breathing room before search'
)
await page.screenshot({ path: '/tmp/new-api-docs-header-vi.png' })
await page.setViewportSize({ width: 1440, height: 1000 })
const documentation = JSON.parse(
  await fs.readFile('src/features/documentation/beginner-data.json', 'utf8')
)
const overview = documentation.articles.find(
  (article) => article.slug === 'overview'
)
let currentLocale = JSON.parse(
  await fs.readFile('src/i18n/locales/vi.json', 'utf8')
).translation
for (const [locale, label] of [
  ['en', 'English'],
  ['zh', '简体中文'],
  ['zh-TW', '繁體中文'],
  ['fr', 'Français'],
  ['ja', '日本語'],
  ['ru', 'Русский'],
  ['vi', 'Tiếng Việt'],
]) {
  await page
    .getByRole('button', {
      name: currentLocale['Change language'],
      exact: true,
    })
    .click()
  await page.getByRole('menuitem', { name: label, exact: true }).click()
  currentLocale = JSON.parse(
    await fs.readFile(`src/i18n/locales/${locale}.json`, 'utf8')
  ).translation
  await page
    .getByText(
      currentLocale[
        overview.sections.find((section) => section.id === 'step-1').body
      ],
      { exact: true }
    )
    .waitFor()
  assert.equal(
    await page
      .locator('#popular-guides')
      .getByRole('button', { name: currentLocale['Image API'], exact: true })
      .getAttribute('href'),
    '/docs/image'
  )
  assert.equal(
    await page
      .locator('#popular-guides')
      .getByRole('button', { name: currentLocale['Audio API'], exact: true })
      .getAttribute('href'),
    '/docs/audio'
  )
  await page
    .getByRole('heading', {
      name: currentLocale['You need three things'],
      exact: true,
    })
    .waitFor()
  await page
    .getByText(currentLocale['API Key authenticates your requests.'], {
      exact: true,
    })
    .waitFor()
  await page
    .getByRole('heading', {
      name: currentLocale['New API Documentation'],
      exact: true,
    })
    .waitFor()
  assert.equal(await page.title(), currentLocale['New API Documentation'])
  if (locale === 'vi') {
    for (const mode of ['light', 'dark']) {
      await page.evaluate(
        (mode) => localStorage.setItem('newapi:theme:v1:mode', mode),
        mode
      )
      await page.reload()
      await page
        .getByRole('heading', {
          name: currentLocale['You need three things'],
          exact: true,
        })
        .waitFor()
      await page.waitForFunction(
        (mode) =>
          document.documentElement.classList.contains('dark') ===
          (mode === 'dark'),
        mode
      )
      for (const width of [1440, 390]) {
        await page.setViewportSize({ width, height: 1000 })
        assert(
          await page.evaluate(
            () => document.documentElement.scrollWidth <= window.innerWidth
          )
        )
        await page.screenshot({
          path: `/tmp/new-api-overview-vi-${width}-${mode}.png`,
          fullPage: true,
        })
      }
    }
    await page.setViewportSize({ width: 1440, height: 1000 })
  }
  await page
    .locator('aside')
    .first()
    .getByRole('link', { name: currentLocale['Chat API'], exact: true })
    .click()
  await page
    .getByRole('heading', { name: currentLocale['Chat API'], exact: true })
    .waitFor()
  assert(
    (await page.locator('#quick-example pre').textContent()).includes(
      currentLocale['Hello!']
    )
  )
  await page
    .getByRole('button', {
      name: currentLocale['Advanced options'],
      exact: true,
    })
    .click()
  assert.equal(
    await page.locator('#advanced-options').getByRole('row').count(),
    4
  )
  await page
    .locator('aside')
    .first()
    .getByRole('link', { name: currentLocale['Image API'], exact: true })
    .click()
  await page
    .getByRole('heading', { name: currentLocale['Image API'], exact: true })
    .waitFor()
  assert(
    (await page.locator('#quick-example pre').textContent()).includes(
      currentLocale['A small red house beside a lake']
    )
  )
  assert(
    await page
      .getByRole('link', { name: currentLocale['Get API Key'], exact: true })
      .count()
  )
  assert.equal(
    await page
      .getByRole('button', {
        name: currentLocale['Advanced options'],
        exact: true,
      })
      .getAttribute('aria-expanded'),
    'false'
  )
  await page
    .getByRole('button', {
      name: currentLocale['Advanced options'],
      exact: true,
    })
    .click()
  await page.locator('#advanced-options').getByRole('row').first().waitFor()
  assert.equal(
    await page.locator('#advanced-options').getByRole('row').count(),
    5
  )
  await page
    .getByRole('button', {
      name: currentLocale['Advanced options'],
      exact: true,
    })
    .click()
  if (locale === 'vi') {
    await page.evaluate(() => window.scrollTo(0, 0))
    await page.screenshot({ path: '/tmp/new-api-image-vi-desktop.png' })
    await page.setViewportSize({ width: 390, height: 844 })
    await page.screenshot({ path: '/tmp/new-api-image-vi-mobile.png' })
    assert(
      await page
        .locator('html')
        .evaluate((element) => element.classList.contains('dark'))
    )
    await page.setViewportSize({ width: 1440, height: 1000 })
  }
  await page
    .locator('aside')
    .first()
    .getByRole('link', { name: currentLocale['Video'], exact: true })
    .click()
  await page
    .getByRole('heading', { name: currentLocale['Video'], exact: true })
    .waitFor()
  assert(
    (await page.locator('#create-video pre').textContent()).includes(
      currentLocale['A paper boat drifting on a calm lake']
    )
  )
  assert.equal(
    await page
      .getByRole('button', {
        name: currentLocale['Advanced options'],
        exact: true,
      })
      .getAttribute('aria-expanded'),
    'false'
  )
  assert.equal(await page.locator('#check-status').getByRole('row').count(), 6)
  if (locale === 'vi') {
    await page.evaluate(() => window.scrollTo(0, 0))
    await page.screenshot({ path: '/tmp/new-api-video-vi-desktop-dark.png' })
    await page.setViewportSize({ width: 390, height: 844 })
    await page.screenshot({ path: '/tmp/new-api-video-vi-mobile-dark.png' })
    await page.evaluate(() =>
      localStorage.setItem('newapi:theme:v1:mode', 'light')
    )
    await page.reload()
    await page
      .getByRole('heading', { name: currentLocale['Video'], exact: true })
      .waitFor()
    assert(
      !(await page
        .locator('html')
        .evaluate((element) => element.classList.contains('dark')))
    )
    await page.screenshot({ path: '/tmp/new-api-video-vi-mobile-light.png' })
    await page.setViewportSize({ width: 1440, height: 1000 })
    await page.screenshot({ path: '/tmp/new-api-video-vi-desktop-light.png' })
    await page.evaluate(() =>
      localStorage.setItem('newapi:theme:v1:mode', 'dark')
    )
    await page.reload()
    await page
      .getByRole('heading', { name: currentLocale['Video'], exact: true })
      .waitFor()
  }
  await page
    .locator('aside')
    .first()
    .getByRole('link', { name: currentLocale['Audio API'], exact: true })
    .click()
  await page
    .getByRole('heading', { name: currentLocale['Audio API'], exact: true })
    .waitFor()
  for (const [task, taskLabel] of [
    ['speech', 'Text → Speech'],
    ['transcription', 'Speech → Text'],
  ]) {
    await page
      .getByRole('tab', { name: currentLocale[taskLabel], exact: true })
      .click()
    await assertAudioTabsFit(currentLocale['Audio task'])
    const taskPanel = page.getByRole('tabpanel', {
      name: currentLocale[taskLabel],
      exact: true,
    })
    await taskPanel.getByRole('tab', { name: 'cURL', exact: true }).click()
    if (task === 'speech') {
      assert(
        (await taskPanel.locator('#quick-example pre').textContent()).includes(
          currentLocale['Hello! Welcome to New API.']
        )
      )
    } else {
      assert(
        (await taskPanel.locator('#result pre').textContent()).includes(
          currentLocale['Hello, this is the transcribed text.']
        )
      )
    }
    assert.equal(
      await page
        .getByRole('navigation', { name: currentLocale['On this page'] })
        .getByRole('link')
        .count(),
      6
    )
    if (locale === 'vi') {
      await page.evaluate(() => window.scrollTo(0, 0))
      await page.mouse.move(1400, 900)
      await page.screenshot({
        path: `/tmp/new-api-audio-${task}-vi-desktop-dark.png`,
      })
      await page.setViewportSize({ width: 390, height: 844 })
      assert(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth
        )
      )
      await assertAudioTabsFit(currentLocale['Audio task'])
      await page.screenshot({
        path: `/tmp/new-api-audio-${task}-vi-mobile-dark.png`,
      })
      await page.setViewportSize({ width: 1440, height: 1000 })
    }
  }
  await page
    .locator('aside')
    .first()
    .getByRole('link', { name: currentLocale['Quickstart'], exact: true })
    .click()
  await page
    .getByRole('heading', {
      name: currentLocale['3. Send a request'],
      exact: true,
    })
    .waitFor()
  await page
    .getByText(currentLocale['Send your first AI request in a few minutes.'], {
      exact: true,
    })
    .waitFor()
  assert.equal(
    await page
      .getByRole('navigation', { name: currentLocale['On this page'] })
      .getByRole('link')
      .count(),
    4
  )
  for (const language of ['cURL', 'Python', 'JavaScript']) {
    await page.getByRole('tab', { name: language, exact: true }).click()
    const code = await page
      .getByRole('tabpanel', { name: language, exact: true })
      .locator('pre')
      .textContent()
    assert(
      code.includes(currentLocale['Hello!']) &&
        code.includes('YOUR_API_KEY') &&
        code.includes('YOUR_MODEL_ID')
    )
    assert(!/NEW_API_|jq|os.environ/.test(code))
  }
  if (locale === 'vi') {
    for (const mode of ['light', 'dark']) {
      await page.evaluate(
        (mode) => localStorage.setItem('newapi:theme:v1:mode', mode),
        mode
      )
      await page.reload()
      await page
        .getByRole('heading', {
          name: currentLocale['3. Send a request'],
          exact: true,
        })
        .waitFor()
      await page.waitForFunction(
        (mode) =>
          document.documentElement.classList.contains('dark') ===
          (mode === 'dark'),
        mode
      )
      for (const width of [1440, 390]) {
        await page.setViewportSize({ width, height: 1000 })
        assert(
          await page.evaluate(
            () => document.documentElement.scrollWidth <= innerWidth
          )
        )
        await page.screenshot({
          path: `/tmp/new-api-quickstart-vi-${width}-${mode}.png`,
          fullPage: true,
        })
      }
    }
    await page.setViewportSize({ width: 1440, height: 1000 })
  }
  await page
    .locator('aside')
    .first()
    .getByRole('link', { name: currentLocale['API Key'], exact: true })
    .click()
  await page
    .getByRole('heading', {
      name: currentLocale['Use your API Key'],
      exact: true,
    })
    .waitFor()
  await page
    .getByText(
      currentLocale['An API Key authenticates requests sent to HOTX API.'],
      { exact: true }
    )
    .waitFor()
  assert.equal(
    await page
      .getByRole('navigation', { name: currentLocale['On this page'] })
      .getByRole('link')
      .count(),
    3
  )
  assert.equal(
    await page
      .locator('main')
      .getByRole('button', {
        name: currentLocale['Open API Keys'],
        exact: true,
      })
      .getAttribute('href'),
    '/keys'
  )
  assert.equal(
    await page
      .locator('main')
      .getByRole('button', {
        name: currentLocale['Manage API Keys'],
        exact: true,
      })
      .getAttribute('href'),
    '/keys'
  )
  for (const index of [0, 1]) {
    const source = (
      await page.locator('#step-2 pre').nth(index).textContent()
    ).trimEnd()
    await page
      .locator('#step-2')
      .getByRole('button', {
        name: currentLocale['Copy to clipboard'],
        exact: true,
      })
      .nth(index)
      .click()
    assert.equal(
      await page.evaluate(() => navigator.clipboard.readText()),
      source
    )
    assert(source.includes('Authorization: Bearer YOUR_API_KEY'))
    if (index === 1) {
      assert(source.includes('https://published-api.example/gateway/v1/models'))
    }
  }
  if (locale === 'vi') {
    for (const mode of ['light', 'dark']) {
      await page.evaluate(
        (mode) => localStorage.setItem('newapi:theme:v1:mode', mode),
        mode
      )
      await page.reload()
      await page
        .getByRole('heading', {
          name: currentLocale['Use your API Key'],
          exact: true,
        })
        .waitFor()
      await page.waitForFunction(
        (mode) =>
          document.documentElement.classList.contains('dark') ===
          (mode === 'dark'),
        mode
      )
      for (const width of [1440, 1091, 768, 390]) {
        await page.setViewportSize({ width, height: 1000 })
        assert(
          await page.evaluate(
            () => document.documentElement.scrollWidth <= innerWidth
          )
        )
        if ([1440, 390].includes(width)) {
          await page.screenshot({
            path: `/tmp/new-api-key-vi-${width}-${mode}.png`,
            fullPage: true,
          })
        }
      }
    }
    await page.setViewportSize({ width: 1440, height: 1000 })
  }
  await page
    .locator('aside')
    .first()
    .getByRole('link', { name: currentLocale['Overview'], exact: true })
    .click()
}
await page
  .locator('aside')
  .first()
  .getByRole('link', { name: currentLocale['Chat API'], exact: true })
  .click()
await page
  .getByRole('heading', { name: currentLocale['Chat API'], exact: true })
  .waitFor()
await page.screenshot({ path: '/tmp/new-api-chat-vi-desktop.png' })
await page.setViewportSize({ width: 390, height: 844 })
await page.screenshot({ path: '/tmp/new-api-chat-vi-mobile.png' })
console.log(
  'PASS: API Key three sections, contextual CTAs, header/cURL copy, seven languages, responsive light/dark; Quickstart four steps, SDK copy/tabs, four anchors, direct CTAs, seven locales and responsive light/dark; Overview three linked cards, breadcrumb, single Base URL copy, ordered sections, six guides, seven locales, four viewport widths and Vietnamese light/dark; Audio isolated task tabs, JSON/multipart code/copy, dynamic six-entry TOC, seven locales and four viewport widths; Video create/copy, eight anchors, status/download, responsive flow, seven locales and light/dark; Image minimal examples/copy, six anchors, conditional options, Chat-to-Image defaults in seven languages, four viewport widths; desktop navigation, SDK tabs/streaming, endpoint anchors, live catalog/model detail, mobile drawer/overflow, dark mode, catalog access gate, Vietnamese header layout and article prose/title switching in all seven languages; no page errors.'
)
await browser.close()
