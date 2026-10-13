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
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { describe, expect, it } from 'vitest'

import { publicApiEndpoints } from '../api-reference-data'

const root = resolve(process.cwd(), '..')
const specPath = resolve(root, 'docs/openapi/public.json')
const normalize = (value: string) => value.replaceAll(/\{[^}]+\}/g, '{}')
const verbs = new Set([
  'get',
  'post',
  'put',
  'patch',
  'delete',
  'head',
  'options',
])

describe('published public OpenAPI contract', () => {
  it('covers advertised callable contracts without publishing limitation-only or internal routes', () => {
    const spec = JSON.parse(readFileSync(specPath, 'utf8'))
    const actual = Object.entries(spec.paths).flatMap(([path, item]) =>
      Object.keys(item as object)
        .filter((method) => verbs.has(method))
        .map((method) => `${method.toUpperCase()} ${normalize(path)}`)
    )
    const expected = publicApiEndpoints
      .filter((entry) => entry.path !== '/v1/edits')
      .map((entry) => `${entry.method} ${normalize(entry.path)}`)
    expect(actual.sort()).toEqual([...new Set(expected)].sort())
    expect(
      actual.some(
        (key) =>
          key.includes(' /api/') ||
          key.includes('/files') ||
          key.includes('/fine-tunes') ||
          key.includes('/usage/requests/')
      )
    ).toBe(false)
    const ids = Object.values(spec.paths).flatMap((item) =>
      Object.entries(item as object)
        .filter(([method]) => verbs.has(method))
        .map(([, operation]) => operation.operationId)
    )
    expect(ids.every((id) => typeof id === 'string' && id.length > 0)).toBe(
      true
    )
    expect(new Set(ids).size).toBe(ids.length)
  })
  it('retains the exact public DTO property and omission contracts', () => {
    const spec = JSON.parse(readFileSync(specPath, 'utf8'))
    for (const [file, names] of Object.entries({
      'developer_catalog.go': [
        'DeveloperCatalog',
        'DeveloperCatalogModel',
        'DeveloperCatalogEndpoint',
        'DeveloperCatalogPricing',
        'DeveloperCatalogPriceComponent',
        'DeveloperCatalogBilling',
      ],
      'developer_balance.go': ['DeveloperKeyBalance'],
      'developer_usage.go': ['DeveloperKeyUsage', 'DeveloperUsageComponent'],
      'developer_tasks.go': [
        'DeveloperTask',
        'DeveloperTaskError',
        'DeveloperTaskList',
      ],
    })) {
      const source = readFileSync(resolve(root, 'dto', file), 'utf8')
      for (const name of names) {
        const body = source.match(
          new RegExp(`type ${name} struct \\{([\\s\\S]*?)\\n\\}`)
        )?.[1]
        expect(body, name).toBeTruthy()
        if (!body) throw new Error(`Missing DTO: ${name}`)
        const fields = [...body.matchAll(/json:"([^",]+)(,omitempty)?"/g)]
        const schema = spec.components.schemas[name]
        expect(Object.keys(schema.properties).sort(), name).toEqual(
          fields.map((field) => field[1]).sort()
        )
        expect((schema.required ?? []).sort(), name).toEqual(
          fields
            .filter((field) => !field[2])
            .map((field) => field[1])
            .sort()
        )
      }
    }
    const balance = spec.components.schemas.DeveloperKeyBalance.properties
    expect(balance.total).toMatchObject({ type: 'string', nullable: true })
    expect(balance.remaining).toMatchObject({ type: 'string', nullable: true })
    expect(balance.expires_at).toMatchObject({
      type: 'integer',
      nullable: true,
    })
    expect(
      spec.components.schemas.DeveloperKeyUsage.properties.recorded_net.type
    ).toBe('string')
    expect(
      spec.components.schemas.DeveloperTask.properties.status.enum
    ).toEqual(['queued', 'in_progress', 'completed', 'failed', 'unknown'])
  })
  it('uses bearer security and actual binary/streaming transport types', () => {
    const spec = JSON.parse(readFileSync(specPath, 'utf8'))
    expect(spec.components.securitySchemes.BearerAuth).toMatchObject({
      type: 'http',
      scheme: 'bearer',
    })
    for (const path of [
      '/v1/catalog',
      '/v1/key/balance',
      '/v1/key/usage',
      '/v1/tasks',
      '/v1/tasks/{key}',
    ]) {
      expect(spec.paths[path].get.security).toContainEqual({ BearerAuth: [] })
    }
    expect(
      spec.paths['/v1/videos/{task_id}/content'].get.responses['200'].content[
        'video/mp4'
      ].schema
    ).toEqual({ type: 'string', format: 'binary' })
    expect(
      spec.paths['/v1/chat/completions'].post.responses['200'].content
    ).toHaveProperty('text/event-stream')
    expect(spec.paths['/v1/responses'].get.responses).toHaveProperty('101')
    expect(spec.paths['/v1/responses'].get.responses).not.toHaveProperty('200')
  })
})

describe('public contract validation and compatibility gate', () => {
  it('detects removed operations, required parameters, response field removals and type changes', async () => {
    const { contractSnapshot, compareContracts } =
      await import('../../../../scripts/public-openapi-contract.mjs')
    const original = JSON.parse(readFileSync(specPath, 'utf8'))
    const changed = structuredClone(original)
    delete changed.paths['/v1/catalog']
    changed.paths['/v1/tasks'].get.parameters.push({
      name: 'required_filter',
      in: 'query',
      required: true,
      schema: { type: 'string' },
    })
    delete changed.components.schemas.DeveloperKeyBalance.properties.remaining
    changed.components.schemas.DeveloperKeyUsage.properties.recorded_net.type =
      'number'
    const diff = compareContracts(
      contractSnapshot(original),
      contractSnapshot(changed)
    )
    for (const fragment of [
      'GET /v1/catalog',
      'GET /v1/tasks',
      '.remaining',
      '.recorded_net',
    ]) {
      expect(
        diff.some(
          (entry: { kind: string; at: string }) =>
            entry.kind === 'BREAKING' && entry.at.includes(fragment)
        )
      ).toBe(true)
    }
  })
  it('allows additive optional response fields and flags enum expansion for compatibility review', async () => {
    const { contractSnapshot, compareContracts } =
      await import('../../../../scripts/public-openapi-contract.mjs')
    const original = JSON.parse(readFileSync(specPath, 'utf8'))
    const changed = structuredClone(original)
    changed.components.schemas.DeveloperTask.properties.extra = {
      type: 'string',
    }
    let diff = compareContracts(
      contractSnapshot(original),
      contractSnapshot(changed)
    )
    expect(
      diff.every((entry: { kind: string }) => entry.kind === 'ADDITIVE')
    ).toBe(true)
    changed.components.schemas.DeveloperTask.properties.status.enum.push(
      'new_status'
    )
    diff = compareContracts(
      contractSnapshot(original),
      contractSnapshot(changed)
    )
    expect(
      diff.some(
        (entry: { kind: string }) => entry.kind === 'POTENTIALLY_BREAKING'
      )
    ).toBe(true)
  })
  it('rejects duplicate operation IDs, unresolved refs and mismatched response examples', async () => {
    const { validatePublicContract } =
      await import('../../../../scripts/public-openapi-contract.mjs')
    const reference = JSON.parse(
      readFileSync(
        resolve(
          root,
          'web/src/features/documentation/generated/reference.json'
        ),
        'utf8'
      )
    )
    const original = JSON.parse(readFileSync(specPath, 'utf8'))
    const duplicate = structuredClone(original)
    duplicate.paths['/v1/key/usage'].get.operationId =
      duplicate.paths['/v1/catalog'].get.operationId
    expect(() =>
      validatePublicContract(duplicate, publicApiEndpoints, reference, root)
    ).toThrow(/duplicate operationId/)
    const unresolved = structuredClone(original)
    unresolved.paths['/v1/catalog'].get.responses['200'].content[
      'application/json'
    ].schema.$ref = '#/components/schemas/Absent'
    expect(() =>
      validatePublicContract(unresolved, publicApiEndpoints, reference, root)
    ).toThrow(/Unresolved reference/)
    const invalid = structuredClone(original)
    invalid.paths['/v1/key/usage'].get.responses['200'].content[
      'application/json'
    ].example.recorded_net = 25
    expect(() =>
      validatePublicContract(invalid, publicApiEndpoints, reference, root)
    ).toThrow(/Example type/)
  })
})

it('models null chat content, DFLOP replay headers and speech SSE without claiming global idempotency', () => {
  const spec = JSON.parse(readFileSync(specPath, 'utf8'))
  expect(
    spec.components.schemas.CreateChatCompletionResponse.properties.choices
      .items.properties.message.properties.content.nullable
  ).toBe(true)
  for (const path of [
    '/dflop-image/v1/images/generations',
    '/dflop/v1/videos/generations',
    '/dflop/v1/audio/voices',
  ]) {
    expect(spec.paths[path].post.parameters).toContainEqual(
      expect.objectContaining({
        name: 'Idempotency-Key',
        in: 'header',
        required: true,
      })
    )
  }
  const speech = spec.paths['/v1/audio/speech'].post
  expect(speech.responses['200'].content).toHaveProperty('text/event-stream')
  const request =
    spec.components.schemas[
      speech.requestBody.content['application/json'].schema.$ref
        .split('/')
        .at(-1)
    ]
  expect(request.properties.stream_format.type).toBe('string')
})

it('detects security changes and deleted request parameters', async () => {
  const { contractSnapshot, compareContracts } =
    await import('../../../../scripts/public-openapi-contract.mjs')
  const original = JSON.parse(readFileSync(specPath, 'utf8'))
  const changed = structuredClone(original)
  changed.paths['/v1/catalog'].get.security = [{ ImageAccess: [] }]
  changed.paths['/v1/tasks'].get.parameters = []
  const changes = compareContracts(
    contractSnapshot(original),
    contractSnapshot(changed)
  )
  expect(
    changes.some(
      (change) =>
        change.at === 'GET /v1/catalog' && change.reason.includes('Security')
    )
  ).toBe(true)
  expect(
    changes.some(
      (change) =>
        change.at === 'GET /v1/tasks' &&
        change.reason.includes('Parameter removed')
    )
  ).toBe(true)
})

it('rejects duplicate JSON keys and invalid named response examples', async () => {
  const { parsePublicSpec, validatePublicContract } =
    await import('../../../../scripts/public-openapi-contract.mjs')
  expect(() =>
    parsePublicSpec('{"paths":{"/v1/catalog":{},"/v1/catalog":{}}}')
  ).toThrow(/Duplicate/)
  const spec = JSON.parse(readFileSync(specPath, 'utf8'))
  spec.paths['/v1/chat/completions'].post.responses['200'].content[
    'application/json'
  ].examples.tool_call.value.choices[0].message.content = 12
  const reference = JSON.parse(
    readFileSync(
      resolve(root, 'web/src/features/documentation/generated/reference.json'),
      'utf8'
    )
  )
  expect(() =>
    validatePublicContract(spec, publicApiEndpoints, reference, root)
  ).toThrow(/Example type/)
})
