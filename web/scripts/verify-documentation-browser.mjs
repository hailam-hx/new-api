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
await context.addInitScript(() => localStorage.setItem('i18nextLng', 'en'))
let restricted = false,
  pricingRequests = 0
const page = await context.newPage()
const errors = []
page.on('pageerror', (e) => errors.push(e.message))
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
await page.getByRole('link', { name: 'Quickstart', exact: true }).last().click()
await page.getByRole('heading', { name: 'Quickstart', exact: true }).waitFor()
assert.equal(
  await page.getByText('Open in Console', { exact: true }).getAttribute('href'),
  '/keys'
)
await page.getByRole('tab', { name: 'Python', exact: true }).click()
await page.getByText('Advanced options', { exact: true }).click()
await page.getByRole('tab', { name: 'Streaming', exact: true }).click()
await page
  .getByRole('tabpanel')
  .getByText('stream=True', { exact: false })
  .waitFor()
assert.equal(await page.locator('link[rel=canonical]').count(), 1)
assert.equal(await page.locator('head title').count(), 1)
assert.equal(await page.locator('head meta[name=description]').count(), 1)
await page
  .getByRole('tabpanel')
  .getByRole('button', { name: 'Copy to clipboard', exact: true })
  .click()
const copiedExample = await page.evaluate(() => navigator.clipboard.readText())
assert(
  copiedExample.includes('https://api-fixture.example/proxy/v1') &&
    copiedExample.includes('NEW_API_MODEL') &&
    copiedExample.includes('stream=True')
)
assert.equal(
  await page.locator('link[rel=canonical]').last().getAttribute('href'),
  `${baseUrl}/docs/quickstart`
)
await page
  .getByRole('button', { name: 'Search documentation', exact: true })
  .click()
await page
  .getByRole('textbox', { name: 'Search documentation' })
  .fill('/v1/chat/completions')
await page
  .getByRole('dialog')
  .getByRole('link', { name: /^Text \/ Chat/ })
  .click()
await page.getByRole('heading', { name: 'Text / Chat', exact: true }).waitFor()
assert.equal(await page.locator('link[rel=canonical]').count(), 1)
assert.equal(
  await page.locator('link[rel=canonical]').getAttribute('href'),
  `${baseUrl}/docs/text-chat`
)
await page.goto(`${baseUrl}/docs/models`)
await page
  .getByRole('link', { name: 'fixture/chat-model', exact: true })
  .waitFor()
await page
  .getByRole('link', { name: 'fixture/chat-model', exact: true })
  .click()
await page
  .getByRole('heading', { name: 'fixture/chat-model', exact: true })
  .waitFor()
assert((await page.getByText('Not provided', { exact: true }).count()) >= 2)
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
    .getByText(currentLocale[overview.sections[0].body], { exact: true })
    .waitFor()
  assert.equal(
    await page.title(),
    `${currentLocale[overview.title]} · ${currentLocale['New API Documentation']}`
  )
}
console.log(
  'PASS: desktop navigation, SDK tabs/streaming, endpoint anchors, live catalog/model detail, mobile drawer/overflow, dark mode, catalog access gate, Vietnamese header layout and article prose/title switching in all seven languages; no page errors.'
)
await browser.close()
