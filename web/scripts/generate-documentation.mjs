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
import fs from 'node:fs/promises'
import path from 'node:path'

const root = path.resolve('..')
const normalize = (value) =>
  value.replaceAll(/:[^/]+|\*[^/]+|\{[^}]+\}/g, '{}').replace(/\/$/, '')
const specs = await Promise.all(
  ['relay', 'api'].map(async (name) => ({
    name,
    spec: JSON.parse(
      await fs.readFile(path.join(root, 'docs/openapi', `${name}.json`), 'utf8')
    ),
  }))
)
const operations = new Map()
for (const { name, spec } of specs) {
  for (const [url, item] of Object.entries(spec.paths)) {
    for (const [method, operation] of Object.entries(item)) {
      if (
        !['get', 'post', 'put', 'patch', 'delete', 'head', 'options'].includes(
          method
        )
      ) {
        continue
      }
      operations.set(`${method.toUpperCase()} ${normalize(url)}`, {
        name,
        spec,
        item,
        operation,
      })
    }
  }
}

// Expand only schema/parameter structures. Examples are intentionally omitted:
// old OpenAPI fixtures contain stale model names and are not execution evidence.
function resolve(value, spec, seen = new Set()) {
  if (!value || typeof value !== 'object') return value
  if (value.$ref) {
    if (seen.has(value.$ref)) return { $ref: value.$ref }
    const target = value.$ref
      .slice(2)
      .split('/')
      .reduce(
        (node, key) => node?.[key.replaceAll('~1', '/').replaceAll('~0', '~')],
        spec
      )
    if (!target) return { $ref: value.$ref }
    return resolve(target, spec, new Set([...seen, value.$ref]))
  }
  if (Array.isArray(value)) {
    return value.map((item) => resolve(item, spec, seen))
  }
  return Object.fromEntries(
    Object.entries(value)
      .filter(
        ([key]) =>
          !key.startsWith('x-') &&
          !['example', 'examples', 'default'].includes(key)
      )
      .map(([key, item]) => [key, resolve(item, spec, seen)])
  )
}
const raw = execFileSync('go', ['run', 'scripts/docs-routes/main.go'], {
  encoding: 'utf8',
})
const endpoints = []
for (const line of raw.trim().split('\n')) {
  const [method, ginPath, middleware, handler, location] = line.split('\t')
  const url = ginPath
    .replaceAll(/:([^/]+)/g, '{$1}')
    .replaceAll(/\*([^/]+)/g, '{$1}')
  const found = operations.get(`${method} ${normalize(url)}`)
  const endpoint = {
    method,
    path: url,
    middleware: middleware.split(' | ').filter(Boolean),
    source: location.replace('../', ''),
    handler:
      handler.match(/controller\.\w+/g)?.join(', ') || handler.slice(0, 180),
    summary: found?.operation.summary || '',
    schemaSource: found ? `docs/openapi/${found.name}.json` : '',
    parameters: found
      ? resolve(
          [
            ...(found.item.parameters || []),
            ...(found.operation.parameters || []),
          ],
          found.spec
        )
      : [],
    requestBody: found
      ? resolve(found.operation.requestBody || null, found.spec)
      : null,
    responses: found
      ? resolve(found.operation.responses || {}, found.spec)
      : {},
  }
  if (!endpoints.some((item) => item.method === method && item.path === url)) {
    endpoints.push(endpoint)
  }
}
endpoints.sort(
  (a, b) => a.path.localeCompare(b.path) || a.method.localeCompare(b.method)
)
const errorSource = await fs.readFile(
  path.join(root, 'relaykit/types/error.go'),
  'utf8'
)
const errors = [
  ...errorSource.matchAll(/(ErrorCode\w+)\s+ErrorCode\s*=\s*"([^"]+)"/g),
].map((match) => ({
  name: match[1],
  code: match[2],
  source: 'relaykit/types/error.go',
  kind: 'relay',
  status: null,
}))
// Authentication codes expose an explicit HTTP mapping. Billing markers are
// diagnostic strings, distinct from the relay response's error.code.
const authSource = await fs.readFile(
  path.join(root, 'service/auth_session.go'),
  'utf8'
)
const statuses = {
  Conflict: 409,
  TooManyRequests: 429,
  Unauthorized: 401,
  InternalServerError: 500,
}
for (const match of authSource.matchAll(
  /return http\.Status(\w+), "(AUTH_[A-Z_]+)"/g
)) {
  errors.push({
    name: match[2],
    code: match[2],
    source: 'service/auth_session.go',
    kind: 'authentication',
    status: statuses[match[1]] ?? null,
  })
}
for (const source of [
  'model/billing_reservation_log.go',
  'service/billing.go',
  'service/billing_reservation.go',
  'service/task_billing.go',
  'service/text_quota.go',
]) {
  const text = await fs.readFile(path.join(root, source), 'utf8')
  for (const match of text.matchAll(/errors\.New\("(BILLING_[A-Z_]+)"\)/g)) {
    if (!errors.some((item) => item.code === match[1])) {
      errors.push({
        name: match[1],
        code: match[1],
        source,
        kind: 'diagnostic',
        status: null,
      })
    }
  }
}
const excludedOpenApi = [...operations.keys()]
  .filter(
    (key) =>
      !endpoints.some(
        (item) => key === `${item.method} ${normalize(item.path)}`
      )
  )
  .sort()
// Extract shipped manifest identity without executing plugin source or exporting
// configured credentials, model seed lists, usage facts or pricing expressions.
const taskPlugins = []
for (const directory of (
  await fs.readdir(path.join(root, 'plugins/tasks'))
).sort()) {
  const source = `plugins/tasks/${directory}/plugin.js`
  const text = await fs.readFile(path.join(root, source), 'utf8')
  const manifest = text.slice(text.indexOf('export const meta'))
  const key = manifest.match(/\bkey:\s*["']([^"']+)["']/)?.[1]
  const name = manifest.match(/\bname:\s*["']([^"']+)["']/)?.[1]
  const version = manifest.match(/\bversion:\s*["']([^"']+)["']/)?.[1]
  if (!key || !name || !version) {
    throw new Error(`Cannot extract manifest identity: ${source}`)
  }
  taskPlugins.push({ key, name, version, source })
}
const result = `${JSON.stringify({ endpoints, errors, excludedOpenApi, taskPlugins }, null, 2)}\n`
const destination = 'src/features/documentation/generated/reference.json'
await fs.mkdir(path.dirname(destination), { recursive: true })
if (process.argv.includes('--check')) {
  if ((await fs.readFile(destination, 'utf8')) !== result) {
    throw new Error(
      'Documentation reference is stale. Run bun run docs:generate.'
    )
  }
} else {
  await fs.writeFile(destination, result)
}
console.log(
  `Documentation: ${endpoints.length} registered operations, ${errors.length} error constants; ${excludedOpenApi.length} stale OpenAPI operations excluded.`
)
