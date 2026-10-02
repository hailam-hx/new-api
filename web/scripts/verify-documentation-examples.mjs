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
import { spawn } from 'node:child_process'
import fs from 'node:fs/promises'
import http from 'node:http'
import os from 'node:os'
import path from 'node:path'

import {
  buildAPIKeyExample,
  buildChatExample,
  buildImageExample,
  buildVideoExample,
  buildAudioExample,
  buildFirstRequest,
  buildQuickstart,
} from '../src/features/documentation/lib.ts'
import { buildTaskExample } from '../src/features/documentation/task-example-code.ts'

const calls = []
let imageSdkTimeout = false
const server = http.createServer(async (req, res) => {
  let payload = ''
  for await (const part of req) payload += part
  if (req.url === '/v1/models') {
    assert(
      ['Bearer fixture-only-token', 'Bearer YOUR_API_KEY'].includes(
        req.headers.authorization
      )
    )
    calls.push({
      url: req.url,
      method: req.method,
      headers: req.headers,
      body: payload,
    })
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
  if (req.url.startsWith('/v1/videos')) {
    calls.push({
      url: req.url,
      body: payload ? JSON.parse(payload) : undefined,
      headers: req.headers,
    })
    if (req.url.endsWith('/content')) {
      res.end('fixture-video')
      return
    }
    res.setHeader('Content-Type', 'application/json')
    res.end(
      JSON.stringify({
        id: 'task_fixture',
        status: req.method === 'POST' ? 'queued' : 'completed',
      })
    )
    return
  }
  if (req.url.startsWith('/v1/audio/')) {
    assert.equal(req.headers.authorization, 'Bearer YOUR_API_KEY')
    if (req.url === '/v1/audio/speech') {
      calls.push({
        url: req.url,
        body: JSON.parse(payload),
        headers: req.headers,
      })
      res.setHeader('Content-Type', 'audio/mpeg')
      res.end('fixture-audio-bytes')
    } else {
      assert(
        req.headers['content-type'].startsWith('multipart/form-data; boundary=')
      )
      assert(
        payload.includes('name="model"') && payload.includes('YOUR_MODEL_ID')
      )
      assert(
        payload.includes('name="file"; filename=') &&
          payload.includes('fixture-upload-audio')
      )
      assert(!payload.includes('name="voice"'))
      calls.push({ url: req.url, body: payload, headers: req.headers })
      res.setHeader('Content-Type', 'application/json')
      res.end(JSON.stringify({ text: 'fixture transcription' }))
    }
    return
  }
  calls.push({ url: req.url, body: JSON.parse(payload), headers: req.headers })
  res.setHeader('Content-Type', 'application/json')
  if (req.url === '/v1/images/generations') {
    res.statusCode = imageSdkTimeout ? 504 : 200
    res.end(
      JSON.stringify(
        imageSdkTimeout
          ? {
              error: {
                message: 'task_timeout',
                type: 'task_timeout',
                code: 'task_timeout',
              },
            }
          : {
              created: 1720000000,
              data: [{ url: 'https://example.com/generated-image.png' }],
            }
      )
    )
  } else if (req.url === '/v1/chat/completions') {
    res.end(
      JSON.stringify({
        choices: [
          { message: { role: 'assistant', content: 'fixture chat answer' } },
        ],
      })
    )
  } else {
    res.end('{}')
  }
})
await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve))
const origin = `http://127.0.0.1:${server.address().port}`
try {
  for (const shell of ['/bin/bash', '/bin/zsh']) {
    const child = spawn(shell, ['-c', buildAPIKeyExample(origin)], {
      stdio: ['ignore', 'pipe', 'pipe'],
    })
    let stdout = '',
      stderr = ''
    child.stdout.on('data', (value) => {
      stdout += value
    })
    child.stderr.on('data', (value) => {
      stderr += value
    })
    assert.equal(
      await new Promise((resolve) => child.on('exit', resolve)),
      0,
      stderr
    )
    assert.equal(JSON.parse(stdout).data[0].id, 'fixture/model-with-quote"')
    assert.equal(calls.at(-1).method, 'GET')
    assert.equal(calls.at(-1).url, '/v1/models')
    assert.equal(calls.at(-1).headers.authorization, 'Bearer YOUR_API_KEY')
    assert.equal(calls.at(-1).body, '')
  }
  console.log(
    'PASS: API Key minimal GET /v1/models example runs in Bash/zsh with a Bearer header and no request body.'
  )
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
          PATH: `${process.env.DOCS_JQ_DIRECTORY}:${process.env.PATH}`,
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
  const inlineChat = buildChatExample(origin, "Hello! Let's try chat.")
  for (const shell of ['/bin/bash', '/bin/zsh']) {
    const child = spawn(shell, ['-c', inlineChat.curl], {
      stdio: ['ignore', 'pipe', 'pipe'],
    })
    let stderr = ''
    child.stderr.on('data', (value) => (stderr += value))
    assert.equal(
      await new Promise((resolve) => child.on('exit', resolve)),
      0,
      stderr
    )
    assert.equal(calls.at(-1).headers.authorization, 'Bearer YOUR_API_KEY')
    assert.deepEqual(calls.at(-1).body, {
      model: 'YOUR_MODEL_ID',
      messages: [{ role: 'user', content: "Hello! Let's try chat." }],
    })
  }
  const inlinePython = spawn(
    'python3',
    ['-c', 'import ast,sys; ast.parse(sys.stdin.read())'],
    { stdio: ['pipe', 'pipe', 'pipe'] }
  )
  inlinePython.stdin.end(inlineChat.python)
  assert.equal(
    await new Promise((resolve) => inlinePython.on('exit', resolve)),
    0
  )
  new Bun.Transpiler({ loader: 'js' }).transformSync(inlineChat.javascript)
  if (
    process.env.DOCS_OPENAI_JS_MODULE &&
    process.env.DOCS_OPENAI_PYTHON_PATH
  ) {
    for (const language of ['python', 'javascript']) {
      const source =
        language === 'python'
          ? inlineChat.python
          : inlineChat.javascript.replace(
              '"openai"',
              JSON.stringify(process.env.DOCS_OPENAI_JS_MODULE)
            )
      const child = spawn(
        language === 'python'
          ? process.env.DOCS_PYTHON_EXECUTABLE || 'python3'
          : 'node',
        language === 'python'
          ? ['-c', source]
          : ['--input-type=module', '-e', source],
        {
          env: {
            ...process.env,
            PYTHONPATH: process.env.DOCS_OPENAI_PYTHON_PATH,
          },
          stdio: ['ignore', 'pipe', 'pipe'],
        }
      )
      let stdout = '',
        stderr = ''
      child.stdout.on('data', (value) => {
        stdout += value
      })
      child.stderr.on('data', (value) => {
        stderr += value
      })
      assert.equal(
        await new Promise((resolve) => child.on('exit', resolve)),
        0,
        stderr
      )
      assert.equal(stdout.trim(), 'fixture chat answer')
      assert.equal(calls.at(-1).url, '/v1/chat/completions')
      assert.equal(calls.at(-1).headers.authorization, 'Bearer YOUR_API_KEY')
      assert.deepEqual(calls.at(-1).body, {
        model: 'YOUR_MODEL_ID',
        messages: [{ role: 'user', content: "Hello! Let's try chat." }],
      })
    }
    console.log(
      'PASS: Quickstart Python and JavaScript SDKs send the minimal authenticated chat request and print the answer.'
    )
  }
  const inlineImage = buildImageExample(origin, "A house beside John's lake")
  for (const shell of ['/bin/bash', '/bin/zsh']) {
    const child = spawn(shell, ['-c', inlineImage.curl], {
      stdio: ['ignore', 'pipe', 'pipe'],
    })
    assert.equal(await new Promise((resolve) => child.on('exit', resolve)), 0)
    assert.equal(calls.at(-1).url, '/v1/images/generations')
    assert.equal(calls.at(-1).headers.authorization, 'Bearer YOUR_API_KEY')
    assert.deepEqual(calls.at(-1).body, {
      model: 'YOUR_MODEL_ID',
      prompt: "A house beside John's lake",
    })
  }
  const imagePython = spawn(
    'python3',
    ['-c', 'import ast,sys; ast.parse(sys.stdin.read())'],
    { stdio: ['pipe', 'pipe', 'pipe'] }
  )
  imagePython.stdin.end(inlineImage.python)
  assert.equal(
    await new Promise((resolve) => imagePython.on('exit', resolve)),
    0
  )
  new Bun.Transpiler({ loader: 'js' }).transformSync(inlineImage.javascript)
  // Optional real SDK execution against the same local fixture; no upstream calls.
  if (
    process.env.DOCS_OPENAI_JS_MODULE &&
    process.env.DOCS_OPENAI_PYTHON_PATH
  ) {
    for (const timeout of [false, true]) {
      imageSdkTimeout = timeout
      for (const language of ['python', 'javascript']) {
        const before = calls.length
        const source =
          language === 'python'
            ? inlineImage.python
            : inlineImage.javascript.replace(
                '"openai"',
                JSON.stringify(process.env.DOCS_OPENAI_JS_MODULE)
              )
        const child = spawn(
          language === 'python'
            ? process.env.DOCS_PYTHON_EXECUTABLE || 'python3'
            : 'node',
          language === 'python'
            ? ['-c', source]
            : ['--input-type=module', '-e', source],
          {
            env: {
              ...process.env,
              PYTHONPATH: process.env.DOCS_OPENAI_PYTHON_PATH,
            },
            stdio: ['ignore', 'pipe', 'pipe'],
          }
        )
        let stdout = '',
          stderr = ''
        child.stdout.on('data', (value) => {
          stdout += value
        })
        child.stderr.on('data', (value) => {
          stderr += value
        })
        const status = await new Promise((resolve) => child.on('exit', resolve))
        assert.equal(status, timeout ? 1 : 0, stderr)
        assert.equal(
          calls.length - before,
          1,
          'Image SDK must not retry a timed-out generation'
        )
        assert.equal(calls.at(-1).url, '/v1/images/generations')
        assert.equal(calls.at(-1).headers.authorization, 'Bearer YOUR_API_KEY')
        assert.deepEqual(calls.at(-1).body, {
          model: 'YOUR_MODEL_ID',
          prompt: "A house beside John's lake",
        })
        assert(
          (timeout ? stderr : stdout).includes(
            timeout ? 'task_timeout' : 'generated-image.png'
          )
        )
      }
    }
    imageSdkTimeout = false
    console.log(
      'PASS: real Python and JavaScript Image SDK requests and response parsing; a 504 causes exactly one request per SDK.'
    )
  }
  const audioDirectory = await fs.mkdtemp(path.join(os.tmpdir(), 'docs-audio-'))
  try {
    await fs.writeFile(
      path.join(audioDirectory, 'YOUR_AUDIO_FILE'),
      'fixture-upload-audio'
    )
    for (const task of ['speech', 'transcription']) {
      const code = buildAudioExample(origin, task, "Hello! Let's try audio.")
      for (const language of ['bash', 'zsh', 'python', 'javascript']) {
        let executable = `/bin/${language}`,
          args = ['-c', code.curl]
        if (language === 'python') {
          executable = process.env.DOCS_PYTHON_EXECUTABLE || 'python3'
          args = ['-c', code.python]
        }
        if (language === 'javascript') {
          executable = 'node'
          args = ['--input-type=module', '-e', code.javascript]
        }
        const child = spawn(executable, args, {
          cwd: audioDirectory,
          env: process.env,
          stdio: ['ignore', 'pipe', 'pipe'],
        })
        let stdout = '',
          stderr = ''
        child.stdout.on('data', (value) => {
          stdout += value
        })
        child.stderr.on('data', (value) => {
          stderr += value
        })
        assert.equal(
          await new Promise((resolve) => child.on('exit', resolve)),
          0,
          stderr
        )
        assert.equal(
          calls.at(-1).url,
          task === 'speech' ? '/v1/audio/speech' : '/v1/audio/transcriptions'
        )
        if (task === 'speech') {
          assert.deepEqual(calls.at(-1).body, {
            model: 'YOUR_MODEL_ID',
            input: "Hello! Let's try audio.",
            voice: 'YOUR_VOICE_ID',
            response_format: 'mp3',
          })
          assert.equal(
            await fs.readFile(path.join(audioDirectory, 'speech.mp3'), 'utf8'),
            'fixture-audio-bytes'
          )
        } else {
          assert(stdout.includes('fixture transcription'))
        }
      }
    }
    console.log(
      'PASS: Audio JSON/voice/MP3 and multipart/model/file/default JSON; Bash/zsh, real requests and Node fetch; saved audio bytes and recognized text.'
    )
  } finally {
    await fs.rm(audioDirectory, { recursive: true, force: true })
  }
  const beginnerVideo = buildVideoExample(origin, "A boat on John's lake")
  const videoDirectory = await fs.mkdtemp(
    path.join(os.tmpdir(), 'docs-beginner-video-')
  )
  try {
    for (const shell of ['/bin/bash', '/bin/zsh']) {
      for (const operation of ['curl', 'status', 'download']) {
        const child = spawn(shell, ['-c', beginnerVideo[operation]], {
          cwd: videoDirectory,
          stdio: ['ignore', 'pipe', 'pipe'],
        })
        let stdout = '',
          stderr = ''
        child.stdout.on('data', (value) => {
          stdout += value
        })
        child.stderr.on('data', (value) => {
          stderr += value
        })
        assert.equal(
          await new Promise((resolve) => child.on('exit', resolve)),
          0,
          stderr
        )
        const request = calls.at(-1)
        assert.equal(request.headers.authorization, 'Bearer YOUR_API_KEY')
        if (operation === 'curl') {
          assert.equal(request.url, '/v1/videos')
          assert.deepEqual(request.body, {
            model: 'YOUR_MODEL_ID',
            prompt: "A boat on John's lake",
          })
          assert.equal(JSON.parse(stdout).id, 'task_fixture')
          assert.equal(JSON.parse(stdout).status, 'queued')
        } else {
          assert.equal(
            request.url,
            operation === 'status'
              ? '/v1/videos/TASK_ID'
              : '/v1/videos/TASK_ID/content'
          )
          if (operation === 'status') {
            assert.equal(JSON.parse(stdout).status, 'completed')
          } else {
            assert.equal(
              await fs.readFile(path.join(videoDirectory, 'video.mp4'), 'utf8'),
              'fixture-video'
            )
          }
        }
      }
    }
    for (const language of ['python', 'javascript']) {
      const child = spawn(
        language === 'python'
          ? process.env.DOCS_PYTHON_EXECUTABLE || 'python3'
          : 'node',
        language === 'python'
          ? ['-c', beginnerVideo.python]
          : ['--input-type=module', '-e', beginnerVideo.javascript],
        { stdio: ['ignore', 'pipe', 'pipe'] }
      )
      let stdout = '',
        stderr = ''
      child.stdout.on('data', (value) => {
        stdout += value
      })
      child.stderr.on('data', (value) => {
        stderr += value
      })
      assert.equal(
        await new Promise((resolve) => child.on('exit', resolve)),
        0,
        stderr
      )
      assert(stdout.includes('task_fixture'))
      assert.deepEqual(calls.at(-1).body, {
        model: 'YOUR_MODEL_ID',
        prompt: "A boat on John's lake",
      })
    }
    console.log(
      'PASS: beginner Video create/status/download cURL in Bash/zsh, real Python requests and Node fetch; payload, task ID, statuses and downloaded bytes.'
    )
  } finally {
    await fs.rm(videoDirectory, { recursive: true, force: true })
  }
  const mediaDirectory = await fs.mkdtemp(path.join(os.tmpdir(), 'docs-video-'))
  try {
    for (const shell of ['/bin/bash', '/bin/zsh']) {
      const child = spawn(
        shell,
        [
          '-c',
          `${buildTaskExample(origin, 'video').curl}\nprintf 'EXAMPLE_RETURNED'`,
        ],
        {
          cwd: mediaDirectory,
          env: {
            ...process.env,
            PATH: `${process.env.DOCS_JQ_DIRECTORY}:${process.env.PATH}`,
            NEW_API_KEY: 'fixture-only-token',
            NEW_API_MODEL: 'fixture/model-with-quote"',
          },
          stdio: ['ignore', 'pipe', 'pipe'],
        }
      )
      let stderr = '',
        stdout = ''
      child.stderr.on('data', (value) => (stderr += value))
      child.stdout.on('data', (value) => (stdout += value))
      assert.equal(
        await new Promise((resolve) => child.on('exit', resolve)),
        0,
        stderr
      )
      assert(stdout.includes('EXAMPLE_RETURNED'))
      assert.equal(
        await fs.readFile(path.join(mediaDirectory, 'video.mp4'), 'utf8'),
        'fixture-video'
      )
      assert.deepEqual(
        calls.slice(-3).map((request) => request.url),
        [
          '/v1/videos',
          '/v1/videos/task_fixture',
          '/v1/videos/task_fixture/content',
        ]
      )
    }
  } finally {
    await fs.rm(mediaDirectory, { recursive: true, force: true })
  }
} finally {
  server.close()
}
console.log(
  'PASS: two inline Image and two inline Chat, four beginner chat, two video Bash/zsh and six reference cURL executions against local protocol fixture; dynamic model choice, JSON and URL escaping, minimal/stream requests. Six Python examples parse; six TypeScript examples transpile.'
)
