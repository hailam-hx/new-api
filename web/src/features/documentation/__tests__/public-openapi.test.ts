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
import { spawnSync } from 'node:child_process'
import {
  existsSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from 'node:fs'
import { tmpdir } from 'node:os'
import { resolve } from 'node:path'

import { describe, expect, it } from 'vitest'

import { publicApiEndpoints } from '../api-reference-data'
import { buildDeveloperRecipe } from '../developer-recipe-code'

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

it.each([
  'removed enum value',
  'changed authentication',
  'changed auth scheme',
  'narrowed numeric input',
])('hard-fails %s against the original contract', async (mutation) => {
  const { contractSnapshot, compareContracts } =
    await import('../../../../scripts/public-openapi-contract.mjs')
  const original = JSON.parse(readFileSync(specPath, 'utf8'))
  const changed = structuredClone(original)
  if (mutation === 'removed enum value') {
    changed.components.schemas.DeveloperTask.properties.status.enum.pop()
  }
  if (mutation === 'changed authentication') {
    changed.paths['/v1/catalog'].get.security = [{ ImageAccess: [] }]
  }
  if (mutation === 'changed auth scheme') {
    changed.components.securitySchemes.BearerAuth.scheme = 'basic'
  }
  if (mutation === 'narrowed numeric input') {
    const parameter = changed.paths['/v1/tasks'].get.parameters.find(
      (entry: { name: string }) => entry.name === 'limit'
    )
    parameter.schema.maximum = 1
  }
  expect(
    compareContracts(
      contractSnapshot(original),
      contractSnapshot(changed)
    ).some((change) => change.kind === 'BREAKING')
  ).toBe(true)
})

it('reads the committed public spec independently of the editable baseline snapshot', async () => {
  const { readCommittedContract, contractSnapshot, compareContracts } =
    await import('../../../../scripts/public-openapi-contract.mjs')
  const committed = readCommittedContract(root)
  expect(committed.commit).toMatch(/^[a-f0-9]{40}$/)
  const changed = JSON.parse(readFileSync(specPath, 'utf8'))
  delete changed.paths['/v1/catalog']
  expect(
    compareContracts(committed.snapshot, contractSnapshot(changed))
  ).toContainEqual({
    kind: 'BREAKING',
    at: 'GET /v1/catalog',
    reason: 'Operation removed',
  })
})

it('detects path-level required parameters and accepts new response headers', async () => {
  const { contractSnapshot, compareContracts } =
    await import('../../../../scripts/public-openapi-contract.mjs')
  const original = JSON.parse(readFileSync(specPath, 'utf8'))
  const changed = structuredClone(original)
  changed.paths['/v1/catalog'].parameters = [
    { name: 'region', in: 'query', required: true, schema: { type: 'string' } },
  ]
  expect(
    compareContracts(contractSnapshot(original), contractSnapshot(changed))
  ).toContainEqual({
    kind: 'BREAKING',
    at: 'GET /v1/catalog',
    reason: 'Parameter added: query/region',
  })
  delete changed.paths['/v1/catalog'].parameters
  changed.paths['/v1/catalog'].get.responses['200'].headers = {
    'X-Trace': { schema: { type: 'string' } },
  }
  expect(
    compareContracts(
      contractSnapshot(original),
      contractSnapshot(changed)
    ).every((entry) => entry.kind === 'ADDITIVE')
  ).toBe(true)
})

it('compares effective operation parameter overrides rather than shadowed path defaults', async () => {
  const { contractSnapshot, compareContracts } =
    await import('../../../../scripts/public-openapi-contract.mjs')
  const original = JSON.parse(readFileSync(specPath, 'utf8'))
  const changed = structuredClone(original)
  changed.paths['/v1/tasks'].parameters = [
    { name: 'limit', in: 'query', schema: { type: 'integer', maximum: 1 } },
  ]
  expect(
    compareContracts(contractSnapshot(original), contractSnapshot(changed))
  ).toEqual([])
  changed.paths['/v1/tasks'].get.parameters = changed.paths[
    '/v1/tasks'
  ].get.parameters.filter(
    (parameter: { name: string }) => parameter.name !== 'limit'
  )
  expect(
    compareContracts(
      contractSnapshot(original),
      contractSnapshot(changed)
    ).some((entry) => entry.kind === 'BREAKING')
  ).toBe(true)
})

type RecipeFixture = { path: string; body: unknown }

function executeDeveloperRecipe(
  section: string,
  fixtures: RecipeFixture[],
  overrides: Record<string, string> = {}
) {
  const workspace = mkdtempSync(resolve(tmpdir(), 'hotx-recipe-fixture-'))
  const recipe = buildDeveloperRecipe(section, 'https://fixture.invalid/')
  const env = {
    ...process.env,
    PATH: `${workspace}:${process.env.PATH}`,
    NEW_API_KEY: 'fixture-only-token',
    NEW_API_MODEL: 'quoted "model" $value; `literal`',
    START_UNIX_SECONDS: '1',
    END_UNIX_SECONDS: '2',
    ESTIMATE_UNIT: 'points',
    ESTIMATED_COST: '0.125',
    ...overrides,
  }
  try {
    writeFileSync(resolve(workspace, 'fixtures.json'), JSON.stringify(fixtures))
    writeFileSync(resolve(workspace, 'calls.json'), '[]')
    writeFileSync(resolve(workspace, 'jq'), '#!/bin/sh\nexit 99\n', {
      mode: 0o755,
    })
    // Only transport is replaced. The displayed Bash/Python and its JSON parsing
    // execute verbatim. Every unexpected call fails without touching the network.
    const transport = String.raw`import json,sys,io
from pathlib import Path
from urllib.parse import urlparse
fixtures = json.loads(Path('fixtures.json').read_text())
def fixture_request(url, args, payload):
    calls = json.loads(Path('calls.json').read_text())
    assert urlparse(url).netloc == 'fixture.invalid', url
    fixture = fixtures[len(calls)]
    assert urlparse(url).path == fixture['path'], url
    calls.append({'url':url,'args':args,'payload':payload})
    Path('calls.json').write_text(json.dumps(calls))
    return fixture['body']
`
    if (recipe.language === 'python') {
      const wrapper =
        transport +
        String.raw`
import urllib.request
def urlopen(request):
    assert request.headers['Authorization'] == 'Bearer fixture-only-token'
    payload = json.loads(request.data) if request.data else None
    body = fixture_request(request.full_url, [], payload)
    return io.BytesIO(json.dumps(body).encode())
urllib.request.urlopen = urlopen
exec(Path('recipe.py').read_text(), {'__name__':'__main__'})
`
      writeFileSync(resolve(workspace, 'recipe.py'), recipe.code)
      writeFileSync(resolve(workspace, 'run.py'), wrapper)
    } else {
      const mockCurl = [
        '#!/usr/bin/env python3\n',
        transport,
        String.raw`
args = sys.argv[1:]
assert 'Authorization: Bearer fixture-only-token' in args, args
url = next(value for value in args if value.startswith('https://'))
payload = json.load(sys.stdin) if '--data-binary' in args else None
body = fixture_request(url, args, payload)
if '--output' in args:
    Path(args[args.index('--output') + 1]).write_bytes(bytes(body))
else:
    print(json.dumps(body) if not isinstance(body, str) else body)
`,
      ].join('')
      writeFileSync(resolve(workspace, 'curl'), mockCurl, { mode: 0o755 })
      writeFileSync(resolve(workspace, 'recipe.sh'), recipe.code)
    }
    const command = recipe.language === 'python' ? 'python3' : 'bash'
    const args = recipe.language === 'python' ? ['run.py'] : ['recipe.sh']
    const result = spawnSync(command, args, {
      cwd: workspace,
      env,
      encoding: 'utf8',
      timeout: 10000,
    })
    if (result.error) throw result.error
    const calls = JSON.parse(
      readFileSync(resolve(workspace, 'calls.json'), 'utf8')
    )
    let savedTask: string | undefined
    let video: number[] | undefined
    if (section === 'video' && calls.length) {
      if (existsSync(resolve(workspace, 'video-task-id.txt'))) {
        savedTask = readFileSync(
          resolve(workspace, 'video-task-id.txt'),
          'utf8'
        )
      }
      if (existsSync(resolve(workspace, 'video.mp4'))) {
        video = [...readFileSync(resolve(workspace, 'video.mp4'))]
      }
    }
    return { ...result, calls, savedTask, video }
  } finally {
    rmSync(workspace, { recursive: true, force: true })
  }
}

const quotedRecipeModel = 'quoted "model" $value; `literal`'

describe('displayed developer recipes with standard-library JSON fixtures', () => {
  it('discovers a compatible quoted model and submits its JSON without requiring jq', () => {
    const result = executeDeveloperRecipe('discover', [
      {
        path: '/v1/catalog',
        body: {
          models: [
            { id: 'not-chat', endpoints: [] },
            {
              id: quotedRecipeModel,
              endpoints: [{ method: 'POST', path: '/v1/chat/completions' }],
            },
          ],
        },
      },
      { path: '/v1/chat/completions', body: { id: 'fixture-chat' } },
    ])
    expect(result.status, result.stderr).toBe(0)
    expect(result.calls).toHaveLength(2)
    expect(result.calls[1].payload).toEqual({
      model: quotedRecipeModel,
      messages: [{ role: 'user', content: 'Hello' }],
    })
  })
  it('checks exact decimal allowance before submitting the balance recipe once', () => {
    const result = executeDeveloperRecipe('balance', [
      {
        path: '/v1/key/balance',
        body: { unit: 'points', remaining: '0.125', unlimited: false },
      },
      { path: '/v1/chat/completions', body: { id: 'fixture-chat' } },
    ])
    expect(result.status, result.stderr).toBe(0)
    expect(result.calls).toHaveLength(2)
    expect(result.calls[1].payload.model).toBe(quotedRecipeModel)
  })
  it('preserves the usage recipe time range as URL-encoded curl arguments', () => {
    const result = executeDeveloperRecipe('usage', [
      { path: '/v1/key/usage', body: { recorded_net: '-0.125' } },
    ])
    expect(result.status, result.stderr).toBe(0)
    expect(result.calls).toHaveLength(1)
    expect(result.calls[0].args).toEqual(
      expect.arrayContaining(['--get', '--data-urlencode', 'start=1', 'end=2'])
    )
  })
  it('submits video once, saves its ID with a newline, then fetches binary content after completion', () => {
    const result = executeDeveloperRecipe('video', [
      { path: '/v1/videos', body: { id: 'fixture-video' } },
      { path: '/v1/tasks/fixture-video', body: { status: 'completed' } },
      { path: '/v1/videos/fixture-video/content', body: [0, 1, 255] },
    ])
    expect(result.status, result.stderr).toBe(0)
    expect(result.calls).toHaveLength(3)
    expect(result.calls[0].payload).toEqual({
      model: quotedRecipeModel,
      prompt: 'A calm ocean',
    })
    expect(result.savedTask).toBe('fixture-video\n')
    expect(result.video).toEqual([0, 1, 255])
  })
  it('passes opaque task cursors as one URL-encoded argument and stops at the terminal page', () => {
    const cursor = 'opaque+/=? "cursor" $value'
    const result = executeDeveloperRecipe('tasks', [
      {
        path: '/v1/tasks',
        body: { data: [{ id: 'one' }], has_more: true, next_cursor: cursor },
      },
      { path: '/v1/tasks', body: { data: [], has_more: false } },
    ])
    expect(result.status, result.stderr).toBe(0)
    expect(result.calls).toHaveLength(2)
    expect(result.calls[1].args).toContain(`cursor=${cursor}`)
  })
  it.each(['future_status', 'failed'])(
    'retains the saved task and avoids resubmission/content retrieval for %s',
    (status) => {
      const result = executeDeveloperRecipe('video', [
        { path: '/v1/videos', body: { id: 'fixture-video' } },
        { path: '/v1/tasks/fixture-video', body: { status } },
      ])
      expect(result.status).toBe(1)
      expect(result.calls).toHaveLength(2)
      expect(result.savedTask).toBe('fixture-video\n')
      expect(result.video).toBeUndefined()
    }
  )
  it('rejects malformed catalog JSON before submitting paid work', () => {
    const result = executeDeveloperRecipe('discover', [
      { path: '/v1/catalog', body: '{malformed' },
    ])
    expect(result.status).not.toBe(0)
    expect(result.calls).toHaveLength(1)
  })
  it('rejects insufficient key allowance before submitting paid work', () => {
    const result = executeDeveloperRecipe('balance', [
      {
        path: '/v1/key/balance',
        body: { unit: 'points', remaining: '0.124', unlimited: false },
      },
    ])
    expect(result.status).not.toBe(0)
    expect(result.stderr).toContain('Insufficient key allowance')
    expect(result.calls).toHaveLength(1)
  })
})

it('documents idempotency only for task-backed mutations and retains ordinary relay and read contracts', () => {
  const spec = JSON.parse(readFileSync(specPath, 'utf8'))
  for (const path of [
    '/v1/videos',
    '/v1/tasks/{key}',
    '/v1/responses',
    '/v1/images/edits',
    '/doubao/api/v3/contents/generations/tasks',
  ]) {
    const operation = spec.paths[path].post
    expect(operation.parameters).toContainEqual(
      expect.objectContaining({ name: 'Idempotency-Key', in: 'header' })
    )
    expect(operation.responses).toHaveProperty('409')
    expect(operation.responses).toHaveProperty('503')
  }
  for (const [path, method] of [
    ['/v1/chat/completions', 'post'],
    ['/v1/responses', 'get'],
    ['/v1/tasks', 'get'],
    ['/suno/fetch', 'post'],
    ['/mj/submit/imagine', 'post'],
  ]) {
    expect(
      (spec.paths[path][method].parameters ?? []).some(
        (parameter: { name: string }) => parameter.name === 'Idempotency-Key'
      )
    ).toBe(false)
  }
})
