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

import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { buildTaskExample } from '../task-example-code'
import { TaskExamples } from '../task-examples'

const AsyncFunction = Object.getPrototypeOf(async function () {}).constructor

async function runJavaScript(
  kind: 'image' | 'video' | 'speech' | 'transcription',
  replies: { status?: number; json?: unknown; contentType?: string }[]
) {
  const requests: { path: string; options?: RequestInit }[] = []
  const writeFile = vi.fn()
  const fetch = vi.fn(async (url: string, options?: RequestInit) => {
    requests.push({ path: new URL(url).pathname, options })
    const reply = replies.shift()
    if (!reply) throw new Error('Unexpected request')
    return {
      ok: (reply.status ?? 200) < 400,
      status: reply.status ?? 200,
      headers: new Headers({
        'content-type': reply.contentType ?? 'application/json',
      }),
      json: async () => reply.json,
      arrayBuffer: async () => new ArrayBuffer(3),
    }
  })
  const code = buildTaskExample(
    'https://gateway.example/prefix/',
    kind
  ).javascript.replaceAll(/^import .*\n/gm, '')
  await new AsyncFunction(
    'fetch',
    'process',
    'writeFile',
    'readFile',
    'console',
    'setTimeout',
    code
  )(
    fetch,
    {
      env: {
        NEW_API_KEY: 'test-only-key',
        NEW_API_MODEL: 'chosen-model',
        NEW_API_VOICE: 'chosen-voice',
        NEW_API_AUDIO_FILE: '/tmp/test.wav',
      },
    },
    writeFile,
    async () => new Uint8Array([1, 2, 3]),
    { log: vi.fn() },
    (callback: () => void) => callback()
  )
  return { requests, writeFile }
}

describe('copyable media request examples', () => {
  it('uses the registered image endpoint with the user-selected model and a prompt', async () => {
    const result = await runJavaScript('image', [
      { json: { data: [{ b64_json: 'aGVsbG8=' }] } },
    ])
    expect(result.requests[0].path).toBe('/prefix/v1/images/generations')
    expect(result.requests[0].options?.headers).toMatchObject({
      Authorization: 'Bearer test-only-key',
    })
    expect(JSON.parse(String(result.requests[0].options?.body))).toEqual({
      model: 'chosen-model',
      prompt: 'A small red house beside a lake',
    })
  })
  it('uses the public video id to poll until completed and only then downloads content', async () => {
    const result = await runJavaScript('video', [
      { json: { id: 'task_public', object: 'video', status: 'queued' } },
      { json: { id: 'task_public', status: 'in_progress' } },
      { json: { id: 'task_public', status: 'completed' } },
      { contentType: 'video/mp4' },
    ])
    expect(result.requests.map((request) => request.path)).toEqual([
      '/prefix/v1/videos',
      '/prefix/v1/videos/task_public',
      '/prefix/v1/videos/task_public',
      '/prefix/v1/videos/task_public/content',
    ])
    expect(result.writeFile).toHaveBeenCalledWith(
      'video.mp4',
      expect.any(Uint8Array)
    )
  })
  it('stops on failed video tasks without requesting content or creating another task', async () => {
    await expect(
      runJavaScript('video', [
        { json: { id: 'task_public', status: 'queued' } },
        {
          json: {
            id: 'task_public',
            status: 'failed',
            error: { message: 'Rejected by provider' },
          },
        },
      ])
    ).rejects.toThrow('Rejected by provider')
  })
  it('rejects HTTP errors rather than interpreting them as media', async () => {
    await expect(runJavaScript('speech', [{ status: 401 }])).rejects.toThrow(
      'HTTP 401'
    )
  })
  it('does not save asynchronous speech task JSON as an MP3', async () => {
    const result = await runJavaScript('speech', [
      { json: { task_id: 'task_speech', status: 'queued' } },
    ])
    expect(result.writeFile).not.toHaveBeenCalled()
    expect(JSON.parse(String(result.requests[0].options?.body))).toEqual({
      model: 'chosen-model',
      input: 'Hello! Welcome to New API.',
      voice: 'chosen-voice',
      response_format: 'mp3',
    })
  })
  it('sends transcription as multipart without setting a boundary header', async () => {
    const result = await runJavaScript('transcription', [
      { json: { text: 'Hello' } },
    ])
    expect(result.requests[0].path).toBe('/prefix/v1/audio/transcriptions')
    const body = result.requests[0].options?.body as FormData
    expect(body.get('model')).toBe('chosen-model')
    expect(body.get('file')).toBeInstanceOf(File)
    expect(result.requests[0].options?.headers).not.toHaveProperty(
      'Content-Type'
    )
  })
  it.each(['image', 'video', 'speech', 'transcription'] as const)(
    'produces syntactically valid shell and Python for %s including quoted origins',
    (kind) => {
      const example = buildTaskExample(
        "https://gateway.example/it's-here/",
        kind
      )
      expect(spawnSync('bash', ['-n'], { input: example.curl }).status).toBe(0)
      const python = spawnSync(
        'python3',
        ['-c', 'import ast,sys; ast.parse(sys.stdin.read())'],
        { input: example.python }
      )
      expect(python.stderr.toString()).toBe('')
      expect(python.status).toBe(0)
    }
  )
})

describe('media guide controls', () => {
  it('keeps advanced options collapsed until opened by keyboard', async () => {
    const user = userEvent.setup()
    render(<TaskExamples kind='image' origin='https://gateway.example' />)
    const trigger = screen.getByRole('button', { name: 'Advanced options' })
    expect(trigger).toHaveAttribute('aria-expanded', 'false')
    trigger.focus()
    await user.keyboard('{Enter}')
    expect(trigger).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText(/Image requests can include n/)).toBeVisible()
  })
  it('switches from speech to transcription and allows keyboard language selection', async () => {
    const user = userEvent.setup()
    render(<TaskExamples kind='audio' origin='https://gateway.example' />)
    await user.click(screen.getByRole('tab', { name: 'Speech-to-Text' }))
    expect(screen.getByRole('tab', { name: 'Speech-to-Text' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
    const python = screen.getByRole('tab', { name: 'Python' })
    python.focus()
    await user.keyboard('{Enter}')
    expect(python).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByRole('tabpanel', { name: 'Python' })).toHaveTextContent(
      'files={"file": audio}'
    )
  })
})
