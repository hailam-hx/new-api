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
import { execFile } from 'node:child_process'
import {
  existsSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from 'node:fs'
import { createServer } from 'node:http'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { promisify } from 'node:util'

import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { CodeExamples } from '../code-examples'
import { articles, getArticle, translateDocSection } from '../content'
import {
  buildAPIKeyExample,
  buildAudioExample,
  buildChatExample,
  buildHotxMediaExample,
  buildImageExample,
  buildVideoExample,
  searchDocumentation,
} from '../lib'

describe('customer documentation examples', () => {
  it('finds a guide by translated paragraph and keeps its section anchor', () => {
    const article = {
      slug: 'quickstart',
      group: 'Getting Started',
      title: 'Quickstart',
      description: 'First request',
      kind: 'article',
      sources: [],
      sections: [
        {
          id: 'choose-model',
          title: 'Choose model',
          body: 'Choose an accessible model',
        },
      ],
    }
    const translate = (key: string) =>
      key === 'Choose an accessible model' ? 'Chọn mô hình được cấp quyền' : key
    expect(searchDocumentation('cấp quyền', [article], translate)).toEqual([
      {
        title: 'Quickstart',
        detail: 'Choose model',
        href: '/docs/quickstart#choose-model',
      },
    ])
    expect(searchDocumentation('   ', [article], translate)).toEqual([])
  })

  it('ranks the translated guide title before an overview that mentions that guide', () => {
    const pages = [
      {
        slug: 'overview',
        group: 'Getting Started',
        title: 'Overview',
        description: 'Read the quickstart',
        kind: 'article',
        sources: [],
        sections: [],
      },
      {
        slug: 'quickstart',
        group: 'Getting Started',
        title: 'Quickstart',
        description: 'Send a request',
        kind: 'article',
        sources: [],
        sections: [],
      },
    ]
    const translate = (key: string) => {
      if (key === 'Quickstart') return 'Bắt đầu nhanh'
      if (key === 'Read the quickstart') return 'Xem hướng dẫn bắt đầu nhanh'
      return key
    }
    expect(
      searchDocumentation('bắt đầu nhanh', pages, translate).map(
        (result) => result.href
      )
    ).toEqual(['/docs/quickstart', '/docs'])
  })

  it('links a transcription search result to the transcription section', () => {
    expect(
      searchDocumentation('a transcription model', articles, (key) => key).find(
        (result) => result.href.startsWith('/docs/audio')
      )?.href
    ).toBe('/docs/audio#transcription-you-need')
  })

  it('resolves customer media routes without exposing administration or fork-only API contracts', () => {
    expect(
      searchDocumentation('/dflop-media/music', articles, (key) => key)[0]?.href
    ).toMatch(/^\/docs\/hotx-media/)
    expect(getArticle('admin-providers')).toBeUndefined()
    expect(searchDocumentation('/v1/catalog', articles, (key) => key)).toEqual(
      []
    )
    expect(getArticle('unknown-guide')).toBeUndefined()
  })

  it('translates section values before interpolating them', () => {
    const section = {
      id: 'model',
      title: 'Model',
      body: 'Use {{name}}',
      values: { name: 'Chat model' },
    }
    const translate = (
      key: string,
      values?: Record<string, string | number>
    ) =>
      key === 'Chat model'
        ? 'Mô hình hội thoại'
        : key.replace('{{name}}', String(values?.name))
    expect(translateDocSection(section, translate)).toBe(
      'Use Mô hình hội thoại'
    )
  })

  it('builds exact API origin paths and preserves apostrophes in request data', () => {
    const code = buildChatExample(
      'https://gateway.example/proxy/',
      "What's next?"
    )
    expect(code.python).toContain('base_url="https://gateway.example/proxy/v1"')
    expect(code.curl).toContain('/proxy/v1/chat/completions')
    expect(code.curl).toContain("What'\\''s next?")
    expect(buildAPIKeyExample('https://gateway.example/proxy/')).toContain(
      '/proxy/v1/models'
    )
    expect(buildImageExample('https://gateway.example').python).toContain(
      'client.images.generate'
    )
    expect(buildImageExample('https://gateway.example').python).toContain(
      'max_retries=0'
    )
  })

  it('separates canonical video creation, status and content requests', () => {
    const code = buildVideoExample('https://gateway.example/')
    expect(code.curl).toContain("https://gateway.example/v1/videos'")
    expect(code.status).toContain('/v1/videos/TASK_ID')
    expect(code.download).toContain('/v1/videos/TASK_ID/content')
    expect(code.download).toContain('--output video.mp4')
  })

  it('uses the deployed native media route without inserting the OpenAI prefix', () => {
    const code = buildHotxMediaExample('https://gateway.example/proxy/')
    expect(code).toContain('https://gateway.example/proxy/dflop-media/images')
    expect(code).toContain(
      'https://gateway.example/proxy/dflop-media/images/TASK_ID'
    )
    expect(code).toContain('YOUR_IMAGE_MODEL_ID')
    expect(code).not.toContain('/v1/dflop-media')
  })

  it('uploads transcription as multipart and saves synchronous speech as binary', () => {
    const transcription = buildAudioExample(
      'https://gateway.example',
      'transcription'
    )
    expect(transcription.curl).toContain('-F "file=@YOUR_AUDIO_FILE"')
    expect(transcription.javascript).toContain('body: form')
    expect(transcription.javascript).not.toContain('"Content-Type"')
    const speech = buildAudioExample('https://gateway.example', 'speech')
    expect(speech.javascript).toContain('response.arrayBuffer()')
    expect(speech.curl).toContain('--output speech.mp3')
    expect(speech.curl).not.toContain('/speech/TASK_ID')
  })

  it.each([
    'video-curl',
    'speech-curl',
    'transcription-curl',
    'speech-javascript',
  ] as const)(
    'rejects a failed HTTP response instead of saving it as media (%s)',
    async (kind) => {
      const server = createServer((_request, response) => {
        response.writeHead(403, { 'Content-Type': 'application/json' })
        response.end('{"error":"Access denied"}')
      })
      await new Promise<void>((resolve) =>
        server.listen(0, '127.0.0.1', resolve)
      )
      const directory = mkdtempSync(path.join(tmpdir(), 'hotx-media-error-'))
      try {
        const address = server.address()
        if (!address || typeof address === 'string') {
          throw new Error('Missing loopback address')
        }
        const origin = `http://127.0.0.1:${address.port}`
        let code = buildAudioExample(origin, 'speech').curl
        let executable = 'sh'
        let args: string[]
        if (kind === 'video-curl') code = buildVideoExample(origin).download
        if (kind === 'transcription-curl') {
          writeFileSync(
            path.join(directory, 'YOUR_AUDIO_FILE'),
            'audio fixture'
          )
          code = buildAudioExample(origin, 'transcription').curl
        }
        if (kind === 'speech-javascript') {
          code = buildAudioExample(origin, 'speech').javascript
          writeFileSync(path.join(directory, 'example.mjs'), code)
          executable = process.execPath
          args = ['example.mjs']
        } else {
          args = ['-c', code]
        }
        await expect(
          promisify(execFile)(executable, args, { cwd: directory })
        ).rejects.toMatchObject({
          code: kind === 'speech-javascript' ? 1 : 22,
        })
        const output = path.join(
          directory,
          kind === 'video-curl' ? 'video.mp4' : 'speech.mp3'
        )
        if (existsSync(output)) expect(readFileSync(output)).toHaveLength(0)
      } finally {
        server.closeAllConnections()
        await new Promise<void>((resolve) => server.close(() => resolve()))
        rmSync(directory, { recursive: true, force: true })
      }
    }
  )
})

describe('customer example controls', () => {
  beforeEach(() => {
    vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  })

  it('localizes connection and prompt labels when Vietnamese is selected', async () => {
    i18next.addResourceBundle(
      'vi',
      'translation',
      {
        'Base URL': 'URL cơ sở',
        'API Key': 'Khóa API',
        'Model ID': 'ID mô hình',
        Prompt: 'Lời nhắc',
      },
      true,
      true
    )
    await i18next.changeLanguage('vi')
    try {
      const router = createRouter({
        routeTree: createRootRoute({
          component: () => (
            <CodeExamples origin='https://gateway.example' image />
          ),
        }),
        history: createMemoryHistory({ initialEntries: ['/docs/image'] }),
      })
      await router.load()
      render(<RouterProvider router={router} />)
      expect(screen.getByText('URL cơ sở', { selector: 'dt' })).toBeVisible()
      expect(screen.getByText('Khóa API', { selector: 'dt' })).toBeVisible()
      expect(screen.getByText('ID mô hình', { selector: 'dt' })).toBeVisible()
      expect(screen.getByText('Lời nhắc', { selector: 'dt' })).toBeVisible()
    } finally {
      await i18next.changeLanguage('en')
    }
  })
  it('switches SDK languages and opens advanced settings from the keyboard', async () => {
    const router = createRouter({
      routeTree: createRootRoute({
        component: () => <CodeExamples origin='https://gateway.example' chat />,
      }),
      history: createMemoryHistory({ initialEntries: ['/docs/text-chat'] }),
    })
    await router.load()
    render(<RouterProvider router={router} />)
    const user = userEvent.setup()
    await user.click(screen.getByRole('tab', { name: 'Python' }))
    expect(screen.getByRole('tabpanel', { name: 'Python' })).toHaveTextContent(
      'client.chat.completions.create'
    )
    const advanced = screen.getByRole('button', { name: 'Advanced options' })
    expect(advanced).toHaveAttribute('aria-expanded', 'false')
    advanced.focus()
    await user.keyboard('{Enter}')
    expect(advanced).toHaveAttribute('aria-expanded', 'true')
  })

  it('switches audio tasks to a transcription request with a local file', async () => {
    const router = createRouter({
      routeTree: createRootRoute({
        component: () => (
          <CodeExamples origin='https://gateway.example' audio />
        ),
      }),
      history: createMemoryHistory({ initialEntries: ['/docs/audio'] }),
    })
    await router.load()
    render(<RouterProvider router={router} />)
    await userEvent
      .setup()
      .click(screen.getByRole('tab', { name: 'Speech → Text' }))
    expect(screen.getByRole('tabpanel', { name: 'cURL' })).toHaveTextContent(
      '/v1/audio/transcriptions'
    )
    expect(screen.getByRole('tabpanel', { name: 'cURL' })).toHaveTextContent(
      'YOUR_AUDIO_FILE'
    )
    expect(
      screen.getByRole('tabpanel', { name: 'cURL' })
    ).not.toHaveTextContent('YOUR_VOICE_ID')
  })

  it('renders task-specific section anchors for a controlled transcription guide', async () => {
    const router = createRouter({
      routeTree: createRootRoute({
        component: () => (
          <CodeExamples
            origin='https://gateway.example'
            audio
            audioTask='transcription'
          />
        ),
      }),
      history: createMemoryHistory({
        initialEntries: ['/docs/audio#transcription-you-need'],
      }),
    })
    await router.load()
    const view = render(<RouterProvider router={router} />)
    const heading = screen.getByRole('heading', { name: 'You need' })
    expect(heading.closest('section')).toHaveAttribute(
      'id',
      'transcription-you-need'
    )
    expect(within(heading).getByRole('link')).toHaveAttribute(
      'href',
      '#transcription-you-need'
    )
    expect(
      view.container.querySelectorAll('#transcription-you-need')
    ).toHaveLength(1)
    expect(view.container.querySelector('#you-need')).toBeNull()
    expect(screen.getByRole('tabpanel', { name: 'cURL' })).toHaveTextContent(
      '/v1/audio/transcriptions'
    )
  })
})
