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
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'

import { CodeExamples } from '../code-examples'

async function renderApiGuide(
  origin: string,
  kind: 'chat' | 'image' | 'video' | 'audio' | 'quickstart' = 'chat'
) {
  const router = createRouter({
    routeTree: createRootRoute({
      component: () => (
        <CodeExamples
          origin={origin}
          beginner={kind === 'quickstart'}
          chat={kind === 'chat'}
          image={kind === 'image'}
          video={kind === 'video'}
          audio={kind === 'audio'}
        />
      ),
    }),
    history: createMemoryHistory({ initialEntries: ['/docs/text-chat'] }),
  })
  await router.load()
  render(<RouterProvider router={router} />)
}

describe('beginner chat guide', () => {
  it('links the Chat API Key placeholder to key management', async () => {
    await renderApiGuide('https://gateway.example')
    expect(screen.getByRole('link', { name: 'Get API Key' })).toHaveAttribute(
      'href',
      '/keys'
    )
  })
  it('lets beginners copy a multiline request without environment setup', async () => {
    await renderApiGuide('https://gateway.example/proxy')
    expect(screen.getByRole('heading', { name: 'Quick example' })).toBeVisible()
    const panel = screen.getByRole('tabpanel', { name: 'cURL' })
    expect(panel).toHaveTextContent('Bearer YOUR_API_KEY')
    expect(panel).toHaveTextContent('YOUR_MODEL_ID')
    expect(panel).toHaveTextContent(
      'https://gateway.example/proxy/v1/chat/completions'
    )
    expect(panel).not.toHaveTextContent(/jq|NEW_API_|export/)
    expect(screen.getByRole('heading', { name: 'Response' })).toBeVisible()
    expect(
      screen.getByRole('heading', { name: 'Basic parameters' })
    ).toBeVisible()
    expect(
      screen.getByRole('button', { name: 'Advanced options' })
    ).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByText(/Adjusts randomness/)).not.toBeInTheDocument()
  })
  it('switches SDK instructions and exposes supported options only after opening Advanced', async () => {
    const user = userEvent.setup()
    await renderApiGuide('https://gateway.example')
    await user.click(screen.getByRole('tab', { name: 'Python' }))
    expect(screen.getByText('pip install openai')).toBeVisible()
    expect(
      screen.getByRole('tabpanel', { name: 'Python' })
    ).not.toHaveTextContent(/os.environ|import os/)
    await user.click(screen.getByRole('tab', { name: 'JavaScript' }))
    expect(screen.getByText('npm install openai')).toBeVisible()
    expect(
      screen.getByRole('tabpanel', { name: 'JavaScript' })
    ).not.toHaveTextContent('process.env')
    expect(
      screen.getByText(/This JavaScript example runs in Node.js/)
    ).toBeVisible()
    const advanced = screen.getByRole('button', { name: 'Advanced options' })
    advanced.focus()
    await user.keyboard('{Enter}')
    expect(advanced).toHaveAttribute('aria-expanded', 'true')
    expect(
      screen.getByText('Adjusts randomness if the model supports it')
    ).toBeVisible()
  })
})

describe('beginner image guide', () => {
  it('shows a minimal prompt request, image response guidance and a timeout note without polling setup', async () => {
    await renderApiGuide('https://gateway.example/proxy', 'image')
    const panel = screen.getByRole('tabpanel', { name: 'cURL' })
    expect(panel).toHaveTextContent('/v1/images/generations')
    expect(panel).toHaveTextContent('Bearer YOUR_API_KEY')
    expect(panel).toHaveTextContent('YOUR_MODEL_ID')
    expect(panel).toHaveTextContent('A small red house beside a lake')
    expect(panel).not.toHaveTextContent(
      /jq|NEW_API_|set -euo|result=|response_format|quality/
    )
    expect(
      screen.getByRole('button', { name: 'Advanced options' })
    ).toHaveAttribute('aria-expanded', 'false')
    expect(
      screen.getByText(/Open the URL to view or save the image/)
    ).toBeVisible()
    expect(
      screen.getByText(/Decode Base64 to save it as an image file/)
    ).toBeVisible()
    expect(
      screen.getByText(/Some image tasks can continue after a timeout/)
    ).toBeVisible()
  })
  it('uses images.generate in both SDK tabs and keeps model-dependent options collapsed', async () => {
    const user = userEvent.setup()
    await renderApiGuide('https://gateway.example', 'image')
    for (const language of ['Python', 'JavaScript']) {
      await user.click(screen.getByRole('tab', { name: language }))
      const panel = screen.getByRole('tabpanel', { name: language })
      expect(panel).toHaveTextContent('client.images.generate')
      expect(panel).not.toHaveTextContent(
        /process.env|os.environ|function request/
      )
    }
    await user.click(screen.getByRole('button', { name: 'Advanced options' }))
    expect(
      screen.getByText('Image dimensions, if the model supports it')
    ).toBeVisible()
    expect(
      screen.getByText('URL or Base64, if the model supports it')
    ).toBeVisible()
  })
})

describe('beginner video guide', () => {
  it('separates creating a task from checking and downloading it', async () => {
    await renderApiGuide('https://gateway.example', 'video')
    const panel = screen.getByRole('tabpanel', { name: 'cURL' })
    expect(panel).toHaveTextContent('/v1/videos')
    expect(panel).toHaveTextContent('YOUR_MODEL_ID')
    expect(panel).not.toHaveTextContent(/jq|NEW_API_|sleep|--output|TASK_ID/)
    expect(
      screen.getByRole('heading', { name: 'How video works' })
    ).toBeVisible()
    expect(
      screen.getByRole('heading', { name: 'Check video status' })
    ).toBeVisible()
    expect(
      screen.getByRole('heading', { name: 'Download video' })
    ).toBeVisible()
    expect(screen.getByText('in_progress', { exact: true })).toBeVisible()
    expect(screen.getByText('unknown', { exact: true })).toBeVisible()
    expect(
      screen.getByRole('button', { name: 'Advanced options' })
    ).toHaveAttribute('aria-expanded', 'false')
  })
  it('keeps Python and Node creation examples free of polling and filesystem code', async () => {
    const user = userEvent.setup()
    await renderApiGuide('https://gateway.example', 'video')
    await user.click(screen.getByRole('tab', { name: 'Python' }))
    expect(screen.getByText('pip install requests')).toBeVisible()
    expect(screen.getByRole('tabpanel', { name: 'Python' })).toHaveTextContent(
      'requests.post'
    )
    expect(
      screen.getByRole('tabpanel', { name: 'Python' })
    ).not.toHaveTextContent(/os.environ|time.sleep|range\(120\)|open\(/)
    await user.click(screen.getByRole('tab', { name: 'JavaScript' }))
    expect(
      screen.getByRole('tabpanel', { name: 'JavaScript' })
    ).toHaveTextContent('await fetch')
    expect(
      screen.getByRole('tabpanel', { name: 'JavaScript' })
    ).not.toHaveTextContent(
      /writeFile|setTimeout|encodeURIComponent|process.env|for \(/
    )
  })
})

describe('beginner audio guide', () => {
  it('starts with synchronous speech and switches to an isolated transcription guide', async () => {
    const user = userEvent.setup()
    await renderApiGuide('https://gateway.example', 'audio')
    const speech = screen.getByRole('tabpanel', { name: 'Text → Speech' })
    expect(speech).toHaveTextContent('/v1/audio/speech')
    expect(speech).toHaveTextContent('YOUR_VOICE_ID')
    expect(speech).toHaveTextContent('--output speech.mp3')
    expect(speech).not.toHaveTextContent(/jq|NEW_API_|audio\/transcriptions/)
    await user.click(screen.getByRole('tab', { name: 'Speech → Text' }))
    const transcription = screen.getByRole('tabpanel', {
      name: 'Speech → Text',
    })
    expect(transcription).toHaveTextContent('/v1/audio/transcriptions')
    expect(transcription).toHaveTextContent('YOUR_AUDIO_FILE')
    expect(transcription).not.toHaveTextContent(/YOUR_VOICE_ID|audio\/speech/)
    expect(
      screen.getByRole('button', { name: 'Advanced options' })
    ).toHaveAttribute('aria-expanded', 'false')
  })
  it('shows short requests and Node file examples without environment variables or HTTP wrappers', async () => {
    const user = userEvent.setup()
    await renderApiGuide('https://gateway.example', 'audio')
    await user.click(screen.getByRole('tab', { name: 'Python' }))
    expect(screen.getByText('pip install requests')).toBeVisible()
    expect(screen.getByRole('tabpanel', { name: 'Python' })).toHaveTextContent(
      'response.content'
    )
    await user.click(screen.getByRole('tab', { name: 'Speech → Text' }))
    await user.click(screen.getByRole('tab', { name: 'JavaScript' }))
    const panel = screen.getByRole('tabpanel', { name: 'JavaScript' })
    expect(panel).toHaveTextContent('new FormData')
    expect(panel).toHaveTextContent('result.text')
    expect(panel).not.toHaveTextContent(
      /process.env|async function request|NEW_API_/
    )
  })
})

it('puts runnable SDK examples inside the third Quickstart step without advanced setup', async () => {
  await renderApiGuide('https://gateway.example/proxy', 'quickstart')
  expect(
    screen.getByRole('heading', { name: '3. Send a request' })
  ).toBeVisible()
  expect(screen.getByRole('tabpanel', { name: 'cURL' })).toHaveTextContent(
    'Bearer YOUR_API_KEY'
  )
  expect(screen.queryByText('Advanced options')).not.toBeInTheDocument()
  expect(screen.queryByText('Request examples')).not.toBeInTheDocument()
  expect(
    screen.getByRole('button', { name: 'Create API Key' })
  ).toHaveAttribute('href', '/keys')
  expect(
    screen.getByRole('button', { name: 'View Usage & Logs' })
  ).toHaveAttribute('href', '/usage-logs/common')
  await userEvent.setup().click(screen.getByRole('tab', { name: 'Python' }))
  expect(screen.getByRole('tabpanel', { name: 'Python' })).toHaveTextContent(
    'pip install openai'
  )
  expect(
    screen.getByRole('tabpanel', { name: 'Python' })
  ).not.toHaveTextContent('os.environ')
})
