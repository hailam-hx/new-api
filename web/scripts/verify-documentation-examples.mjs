import assert from 'node:assert/strict'
import { spawn } from 'node:child_process'
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
import http from 'node:http'

import {
  buildFirstRequest,
  buildQuickstart,
} from '../src/features/documentation/lib.ts'

const calls = []
const server = http.createServer(async (req, res) => {
  let payload = ''
  for await (const part of req) payload += part
  if (req.url === '/v1/models') {
    assert.equal(req.headers.authorization, 'Bearer fixture-only-token')
    res.setHeader('Content-Type', 'application/json')
    res.end(
      JSON.stringify({
        data: [
          {
            id: 'fixture/model-with-quote"',
            supported_endpoint_types: ['openai', 'anthropic', 'gemini'],
          },
        ],
      })
    )
    return
  }
  calls.push({ url: req.url, body: JSON.parse(payload), headers: req.headers })
  res.setHeader('Content-Type', 'application/json')
  res.end('{}')
})
await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve))
const origin = `http://127.0.0.1:${server.address().port}`
try {
  for (const protocol of ['openai', 'anthropic', 'gemini']) {
    for (const stream of [false, true]) {
      const code = buildQuickstart(origin, protocol, stream)
      for (const [lang, text] of Object.entries(code)) {
        if (lang === 'python') {
          const child = spawn(
            'python3',
            ['-c', 'import ast,sys; ast.parse(sys.stdin.read())'],
            { stdio: ['pipe', 'pipe', 'pipe'] }
          )
          child.stdin.end(text)
          assert.equal(await new Promise((r) => child.on('exit', r)), 0)
        } else if (lang === 'typescript') {
          new Bun.Transpiler({ loader: 'ts' }).transformSync(text)
        }
      }
      const child = spawn('/bin/bash', ['-c', code.curl], {
        env: {
          ...process.env,
          PATH:
            (process.env.DOCS_JQ_DIRECTORY
              ? process.env.DOCS_JQ_DIRECTORY + ':'
              : '') + process.env.PATH,
          NEW_API_KEY: 'fixture-only-token',
        },
        stdio: ['ignore', 'pipe', 'pipe'],
      })
      let stderr = ''
      child.stderr.on('data', (d) => (stderr += d))
      assert.equal(await new Promise((r) => child.on('exit', r)), 0, stderr)
      const request = calls.at(-1)
      if (protocol === 'gemini') {
        assert.match(
          request.url,
          /\/v1beta\/models\/fixture%2Fmodel-with-quote%22:/
        )
        assert.equal(request.headers['x-goog-api-key'], 'fixture-only-token')
      } else {
        assert.equal(request.body.model, 'fixture/model-with-quote"')
        assert.equal(!!request.body.stream, stream)
        assert.equal(
          request.url,
          protocol === 'openai' ? '/v1/chat/completions' : '/v1/messages'
        )
      }
    }
  }
  for (const stream of [false, true]) {
    const code = buildFirstRequest(origin, stream)
    for (const shell of ['/bin/bash', '/bin/zsh']) {
      const child = spawn(shell, ['-c', code.curl], {
        env: {
          ...process.env,
          PATH: process.env.DOCS_JQ_DIRECTORY + ':' + process.env.PATH,
          NEW_API_KEY: 'fixture-only-token',
          NEW_API_MODEL: 'fixture/model-with-quote"',
        },
        stdio: ['ignore', 'pipe', 'pipe'],
      })
      let stderr = ''
      child.stderr.on('data', (value) => (stderr += value))
      assert.equal(
        await new Promise((resolve) => child.on('exit', resolve)),
        0,
        stderr
      )
      assert.equal(calls.at(-1).url, '/v1/chat/completions')
      assert.deepEqual(calls.at(-1).body, {
        model: 'fixture/model-with-quote"',
        messages: [{ role: 'user', content: 'Hello' }],
        ...(stream ? { stream: true } : {}),
      })
    }
    const python = spawn(
      'python3',
      ['-c', 'import ast,sys; ast.parse(sys.stdin.read())'],
      { stdio: ['pipe', 'pipe', 'pipe'] }
    )
    python.stdin.end(code.python)
    assert.equal(await new Promise((resolve) => python.on('exit', resolve)), 0)
    new Bun.Transpiler({ loader: 'js' }).transformSync(code.javascript)
  }
} finally {
  server.close()
}
console.log(
  'PASS: four beginner Bash/zsh and six reference cURL executions against local protocol fixture; dynamic model choice, JSON and URL escaping, minimal/stream requests. Six Python examples parse; six TypeScript examples transpile.'
)
