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
import { spawnSync } from 'node:child_process'
import fs from 'node:fs/promises'
import os from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

// --install downloads pinned tools only into a temporary directory. Alternatively,
// --tool-root DIR reuses DIR/node_modules and DIR/venv. Neither changes the repo.
const root = fileURLToPath(new URL('../../', import.meta.url))
const args = process.argv.slice(2)
const toolIndex = args.indexOf('--tool-root')
assert(
  args.includes('--install') || toolIndex >= 0,
  'Use --install or --tool-root DIR'
)
const workspace = await fs.mkdtemp(path.join(os.tmpdir(), 'hotx-public-sdk-'))
const tools = toolIndex >= 0 ? path.resolve(args[toolIndex + 1]) : workspace
const python = path.join(tools, 'venv', 'bin', 'python')
const bins = path.join(tools, 'node_modules', '.bin')
const env = {
  ...process.env,
  PATH: `${path.join(tools, 'venv', 'bin')}${path.delimiter}${process.env.PATH}`,
}
const specPath = path.join(root, 'docs/openapi/public.json')
const run = (command, commandArgs, cwd = workspace) => {
  console.log(`$ ${command} ${commandArgs.join(' ')}`)
  const result = spawnSync(command, commandArgs, {
    cwd,
    env,
    encoding: 'utf8',
    maxBuffer: 8 * 1024 * 1024,
  })
  if (result.error) throw result.error
  const output = result.stdout + result.stderr
  if (result.status !== 0) {
    throw new Error(`${output}\nCommand failed (${result.status}): ${command}`)
  }
  return output
}

try {
  if (args.includes('--install') && toolIndex < 0) {
    console.log(
      run('bun', [
        'add',
        '--exact',
        'openapi-typescript@7.13.0',
        'openapi-fetch@0.17.0',
        'typescript@5.9.3',
        '@types/node@26.6.5',
      ])
    )
    run('python3', ['-m', 'venv', path.join(tools, 'venv')])
    console.log(
      run(python, [
        '-m',
        'pip',
        'install',
        'openapi-python-client==0.29.1',
        'mypy==2.4.0',
        'openapi-spec-validator==0.9.0',
      ])
    )
  }
  if (tools !== workspace) {
    await fs.symlink(
      path.join(tools, 'node_modules'),
      path.join(workspace, 'node_modules')
    )
  }
  await fs.writeFile(
    path.join(workspace, 'package.json'),
    '{"type":"module"}\n'
  )
  const spec = JSON.parse(await fs.readFile(specPath, 'utf8'))
  const operations = Object.values(spec.paths).flatMap((item) =>
    Object.values(item).filter((value) => value.operationId)
  )
  assert.equal(
    new Set(operations.map((operation) => operation.operationId)).size,
    operations.length,
    'operationIds must be unique'
  )
  console.log(
    run(python, [
      '-c',
      'import json,sys; from openapi_spec_validator import validate; validate(json.load(open(sys.argv[1]))); print("OpenAPI validation PASS")',
      specPath,
    ])
  )
  console.log(
    run(path.join(bins, 'openapi-typescript'), [
      specPath,
      '-o',
      path.join(workspace, 'schema.d.ts'),
    ])
  )

  const taskPath = Object.keys(spec.paths).find((entry) =>
    /^\/v1\/tasks\/\{[^}]+\}$/.test(entry)
  )
  assert(taskPath, 'Task read operation required')
  const taskParameter = taskPath.match(/\{([^}]+)\}/)[1]
  const task = {
    object: 'task',
    id: 'fixture/task',
    type: 'video',
    model: 'fixture-model',
    status: 'queued',
    progress: '0%',
    created_at: 1,
  }
  const fixtures = {
    '/v1/models': { success: true, object: 'list', data: [] },
    '/v1/catalog': {
      object: 'catalog',
      schema_version: '1.0',
      currency: 'points',
      models: [],
      future_field: 'ignored',
    },
    '/v1/key/balance': {
      object: 'key.balance',
      unit: 'points',
      total: null,
      used: '0.125',
      remaining: null,
      unlimited: true,
      expires_at: null,
    },
    '/v1/key/usage': {
      object: 'key.usage',
      unit: 'points',
      recorded_consumption: '0',
      recorded_refunds: '0.125',
      recorded_net: '-0.125',
      component_status: 'unknown',
      start_time: 1,
      end_time: 2,
      billing_events: 0,
      error_events: 0,
      components: [],
    },
    '/v1/tasks': { object: 'list', data: [task], has_more: false },
    '/v1/tasks/fixture%2Ftask': task,
  }
  await fs.writeFile(
    path.join(workspace, 'fixtures.json'),
    JSON.stringify(fixtures)
  )
  const tsSample = `import assert from 'node:assert/strict';
import createClient from 'openapi-fetch';
import type { paths, components } from './schema.js';
const fixtures: Record<string, unknown> = ${JSON.stringify(fixtures)};
const calls: string[] = [];
const client = createClient<paths>({ baseUrl: 'https://fixture.invalid', headers: { Authorization: 'Bearer fixture-only-token' }, fetch: async request => {
  assert.equal(request.headers.get('Authorization'), 'Bearer fixture-only-token');
  assert.equal(request.method, 'GET');
  const url = new URL(request.url); calls.push(url.pathname);
  if(url.pathname === '/v1/videos/fixture-video/content') return new Response(new Uint8Array([0, 1, 255]), { headers: { 'Content-Type': 'video/mp4' } });
  assert(Object.hasOwn(fixtures, url.pathname));
  if(url.pathname === '/v1/key/usage') assert.equal(url.searchParams.get('start'), '1');
  return new Response(JSON.stringify(fixtures[url.pathname]), { headers: { 'Content-Type': 'application/json' } });
} });
const balanceFixture: components['schemas']['DeveloperKeyBalance'] = ${JSON.stringify(fixtures['/v1/key/balance'])};
assert.equal(balanceFixture.remaining, null);
await client.GET('/v1/models');
const catalog = await client.GET('/v1/catalog'); assert.equal(catalog.data?.schema_version, '1.0');
const balance = await client.GET('/v1/key/balance'); assert.equal(balance.data?.remaining, null); assert.equal(balance.data?.used, '0.125');
const usage = await client.GET('/v1/key/usage', { params: { query: { start: 1, end: 2 } } }); assert.equal(usage.data?.recorded_net, '-0.125');
const tasks = await client.GET('/v1/tasks'); assert.equal(tasks.data?.has_more, false);
const task = await client.GET('${taskPath}', { params: { path: { ${taskParameter}: 'fixture/task' } } }); assert.equal(task.data?.id, 'fixture/task');
const video = await client.GET('/v1/videos/{task_id}/content', { params: { path: { task_id: 'fixture-video' } }, parseAs: 'arrayBuffer' });
assert(video.data instanceof ArrayBuffer); assert.deepEqual([...new Uint8Array(video.data)], [0, 1, 255]);
assert.equal(calls.length, 7);
if (false) {
  // @ts-expect-error The generated path parameter must be supplied.
  await client.GET('${taskPath}');
}
console.log('TypeScript strict typecheck + 7 mock transport calls (6 JSON + binary video) PASS');
`
  await fs.writeFile(path.join(workspace, 'smoke.ts'), tsSample)
  console.log(
    run(path.join(bins, 'tsc'), [
      '--strict',
      '--noUncheckedIndexedAccess',
      '--module',
      'NodeNext',
      '--moduleResolution',
      'NodeNext',
      '--target',
      'ES2022',
      '--outDir',
      'dist',
      'smoke.ts',
    ])
  )
  console.log(run('node', ['dist/smoke.js']))

  // The generator supports bytes only under application/octet-stream. Its
  // documented override maps declared binary media to bytes without rewriting
  // the public contract or its actual HTTP Content-Type.
  const contentTypeOverrides = {}
  for (const operation of operations) {
    for (const response of Object.values(operation.responses)) {
      for (const [mediaType, content] of Object.entries(
        response.content ?? {}
      )) {
        if (content.schema?.format === 'binary') {
          contentTypeOverrides[mediaType] = 'application/octet-stream'
        }
      }
    }
  }
  await fs.writeFile(
    path.join(workspace, 'python-config.json'),
    JSON.stringify({
      package_name_override: 'hotx_public_client',
      content_type_overrides: contentTypeOverrides,
    })
  )
  const generator = path.join(tools, 'venv', 'bin', 'openapi-python-client')
  const generatorOutput = run(generator, [
    'generate',
    '--path',
    specPath,
    '--meta',
    'none',
    '--config',
    'python-config.json',
    '--output-path',
    'hotx_public_client',
  ])
  console.log(generatorOutput)
  const modules = (
    await fs.readdir(path.join(workspace, 'hotx_public_client/api'), {
      recursive: true,
    })
  ).filter((entry) => entry.endsWith('.py') && !entry.endsWith('__init__.py'))
  console.log(
    `Python generated endpoint modules: ${modules.length}/${operations.length}`
  )
  const imports = [
    'list_models',
    'get_developer_catalog',
    'get_developer_key_balance',
    'get_developer_key_usage',
    'get_developer_tasks',
    'get_developer_task',
    'get_video_content',
  ]
    .map((name) => {
      const module = modules.find(
        (entry) => path.basename(entry, '.py') === name
      )
      assert(module, `Python endpoint not generated: ${name}`)
      return `from hotx_public_client.api.${path.dirname(module).replaceAll(path.sep, '.')} import ${name}`
    })
    .join('\n')
  const pySample = `import json
import httpx
from hotx_public_client import AuthenticatedClient
from hotx_public_client.models.developer_key_balance import DeveloperKeyBalance
from hotx_public_client.models.developer_key_usage import DeveloperKeyUsage
from hotx_public_client.models.developer_catalog import DeveloperCatalog
from hotx_public_client.models.developer_task import DeveloperTask
from hotx_public_client.models.developer_task_list import DeveloperTaskList
from hotx_public_client.types import File
${imports}
fixtures = json.load(open('fixtures.json'))
calls: list[str] = []
def transport(request: httpx.Request) -> httpx.Response:
    assert request.headers['Authorization'] == 'Bearer fixture-only-token'
    assert request.method == 'GET'
    uri = request.url.raw_path.decode().split('?')[0]
    if uri == '/v1/videos/fixture-video/content':
        calls.append(uri)
        return httpx.Response(200, content=bytes([0, 1, 255]), headers={'Content-Type': 'video/mp4'})
    assert uri in fixtures, uri
    if uri == '/v1/key/usage': assert request.url.params['start'] == '1'
    calls.append(uri)
    return httpx.Response(200, json=fixtures[uri])
client = AuthenticatedClient(base_url='https://fixture.invalid', token='fixture-only-token')
client.set_httpx_client(httpx.Client(base_url=client._base_url, headers={'Authorization': 'Bearer fixture-only-token'}, transport=httpx.MockTransport(transport)))
list_models.sync(client=client)
catalog = get_developer_catalog.sync(client=client)
assert isinstance(catalog, DeveloperCatalog)
assert catalog.schema_version.value == '1.0'
assert catalog.additional_properties['future_field'] == 'ignored'
balance = get_developer_key_balance.sync(client=client)
assert isinstance(balance, DeveloperKeyBalance)
assert balance.remaining is None and balance.total is None and balance.expires_at is None
assert balance.used == '0.125'
usage = get_developer_key_usage.sync(client=client, start=1, end=2)
assert isinstance(usage, DeveloperKeyUsage) and usage.recorded_net == '-0.125'
tasks = get_developer_tasks.sync(client=client)
assert isinstance(tasks, DeveloperTaskList) and tasks.has_more is False
task = get_developer_task.sync(${taskParameter}='fixture/task', client=client)
assert isinstance(task, DeveloperTask) and task.id == 'fixture/task'
video = get_video_content.sync(task_id='fixture-video', client=client)
assert isinstance(video, File) and video.payload.read() == bytes([0, 1, 255])
assert len(calls) == 7
print('Python compile + typed sample + 7 mock transport calls (6 JSON + binary video) PASS')
`
  await fs.writeFile(path.join(workspace, 'smoke.py'), pySample)
  console.log(
    run(python, ['-m', 'compileall', '-q', 'hotx_public_client', 'smoke.py'])
  )
  console.log(
    run(python, ['-m', 'mypy', '--follow-imports=silent', 'smoke.py'])
  )
  console.log(run(python, ['smoke.py']))
  if (
    /WARNING|Warning\(s\)/.test(generatorOutput) ||
    modules.length !== operations.length
  ) {
    throw new Error(
      'Python generator emitted warnings or omitted endpoint modules; full SDK readiness BLOCKED (see output).'
    )
  }
  console.log(
    `SDK generation smoke PASS: TypeScript and Python generated ${operations.length} operations; Go NOT_TESTED. Network/upstream requests: 0.`
  )
} catch (error) {
  if (error.stdout) console.error(error.stdout.toString())
  if (error.stderr) console.error(error.stderr.toString())
  console.error(error.message)
  process.exitCode = 1
} finally {
  await fs.rm(workspace, { recursive: true, force: true })
}
