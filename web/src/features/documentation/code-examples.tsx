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
import { Link } from '@tanstack/react-router'
import { ChevronDown } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { ThemedToken } from 'shiki'

import { CodeBlockFrame } from '@/components/ai-elements/code-block'
import { CopyButton } from '@/components/copy-button'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleTrigger,
  CollapsibleContent,
} from '@/components/ui/collapsible'
import { Markdown } from '@/components/ui/markdown'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useTheme } from '@/context/theme-provider'

import { getArticle, translateDocSection } from './content'
import {
  buildQuickstart,
  buildChatExample,
  buildImageExample,
  buildVideoExample,
  buildAudioExample,
} from './lib'

export function DocsCode(props: {
  code: string
  language: 'bash' | 'python' | 'typescript' | 'javascript' | 'json'
}) {
  const { resolvedTheme } = useTheme()
  const [tokens, setTokens] = useState<
    { offset: number; tokens: ThemedToken[] }[] | null
  >(null)
  useEffect(() => {
    let active = true
    void import('shiki/bundle/web')
      .then(async ({ codeToTokens }) => {
        const result = await codeToTokens(props.code, {
          lang: props.language,
          theme: resolvedTheme === 'dark' ? 'github-dark' : 'github-light',
        })
        if (active) {
          let offset = 0
          setTokens(
            result.tokens.map((line) => {
              const rendered = { offset, tokens: line }
              offset +=
                line.reduce(
                  (length, token) => length + token.content.length,
                  0
                ) + 1
              return rendered
            })
          )
        }
      })
      .catch(() => {
        if (active) setTokens(null)
      })
    return () => {
      active = false
    }
  }, [props.code, props.language, resolvedTheme])
  return (
    <CodeBlockFrame
      showToolbar
      title={props.language}
      endActions={<CopyButton value={props.code} size='sm' />}
      bodyClassName='overflow-x-auto p-4'
    >
      <pre className='min-w-max font-mono text-xs leading-6' tabIndex={0}>
        <code>
          {tokens
            ? tokens.map((line) => (
                <span key={line.offset}>
                  {line.tokens.map((token) => (
                    <span key={token.offset} style={{ color: token.color }}>
                      {token.content}
                    </span>
                  ))}
                  {'\n'}
                </span>
              ))
            : props.code}
        </code>
      </pre>
    </CodeBlockFrame>
  )
}

export function CodeExamples(props: {
  origin: string
  protocol?: string
  beginner?: boolean
  chat?: boolean
  image?: boolean
  video?: boolean
  audio?: boolean
  audioTask?: 'speech' | 'transcription'
  onAudioTaskChange?: (task: 'speech' | 'transcription') => void
}) {
  const { t } = useTranslation()
  const [stream, setStream] = useState('minimal')
  const [audioTask, setAudioTask] = useState<'speech' | 'transcription'>(
    'speech'
  )
  const code = buildQuickstart(
    props.origin,
    props.protocol ?? 'openai',
    stream === 'stream'
  )
  if (props.beginner) {
    return <BeginnerApiGuide origin={props.origin} kind='quickstart' />
  }
  if (props.audio) {
    return (
      <Tabs
        value={props.audioTask ?? audioTask}
        onValueChange={(value) => {
          const task = value === 'transcription' ? 'transcription' : 'speech'
          setAudioTask(task)
          props.onAudioTaskChange?.(task)
        }}
        className='min-w-0'
      >
        <TabsList
          aria-label={t('Audio task')}
          className='grid w-full grid-cols-2 group-data-horizontal/tabs:h-auto sm:w-fit'
        >
          <TabsTrigger value='speech' className='h-auto py-2 whitespace-normal'>
            {t('Text → Speech')}
          </TabsTrigger>
          <TabsTrigger
            value='transcription'
            className='h-auto py-2 whitespace-normal'
          >
            {t('Speech → Text')}
          </TabsTrigger>
        </TabsList>
        <p className='text-muted-foreground mt-2 text-sm leading-6'>
          {t(
            'Not every audio model supports both tasks. Check the model type before using it.'
          )}{' '}
          <Link
            to='/docs/$slug'
            params={{ slug: 'models' }}
            className='text-primary underline underline-offset-4'
          >
            {t('View Models')}
          </Link>
        </p>
        {(['speech', 'transcription'] as const).map((task) => (
          <TabsContent key={task} value={task} className='mt-7 min-w-0'>
            <BeginnerApiGuide kind={task} origin={props.origin} />
          </TabsContent>
        ))}
      </Tabs>
    )
  }
  let guideKind: 'chat' | 'image' | 'video' = 'chat'
  if (props.image) guideKind = 'image'
  if (props.video) guideKind = 'video'
  if (props.chat || props.image || props.video) {
    return (
      <BeginnerApiGuide
        key={guideKind}
        origin={props.origin}
        kind={guideKind}
      />
    )
  }
  return (
    <section className='mt-8 min-w-0 space-y-4' id='request-examples'>
      <h2 className='scroll-mt-40 text-xl font-semibold'>
        {t('Request examples')}
      </h2>
      {!props.beginner && (
        <Tabs
          value={stream}
          onValueChange={(value) => setStream(String(value))}
        >
          <TabsList aria-label={t('Request mode')}>
            <TabsTrigger value='minimal'>{t('Minimal request')}</TabsTrigger>
            <TabsTrigger value='stream'>{t('Streaming')}</TabsTrigger>
          </TabsList>
        </Tabs>
      )}
      <Tabs defaultValue='curl'>
        <TabsList aria-label={t('SDK language')}>
          <TabsTrigger value='curl'>cURL</TabsTrigger>
          <TabsTrigger value='python'>Python</TabsTrigger>
          <TabsTrigger value='typescript'>TypeScript</TabsTrigger>
        </TabsList>
        <TabsContent value='curl'>
          <DocsCode code={code.curl} language='bash' />
        </TabsContent>
        <TabsContent value='python'>
          <DocsCode code={code.python} language='python' />
        </TabsContent>
        <TabsContent value='typescript'>
          <DocsCode code={code.typescript} language='typescript' />
        </TabsContent>
      </Tabs>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Keep API keys in server environment variables. Examples do not submit requests from this page.'
        )}
      </p>
    </section>
  )
}

function BeginnerApiGuide(props: {
  origin: string
  kind: 'quickstart' | 'chat' | 'image' | 'video' | 'speech' | 'transcription'
}) {
  const { t } = useTranslation()
  const [language, setLanguage] = useState('curl')
  const article = getArticle(
    {
      quickstart: 'quickstart',
      chat: 'text-chat',
      image: 'image',
      video: 'video',
      speech: 'audio',
      transcription: 'audio',
    }[props.kind]
  )
  if (!article) return null
  let code = buildChatExample(props.origin, t('Hello!'))
  if (props.kind === 'image') {
    code = buildImageExample(props.origin, t('A small red house beside a lake'))
  }
  const videoCode = buildVideoExample(
    props.origin,
    t('A paper boat drifting on a calm lake')
  )
  if (props.kind === 'video') code = videoCode
  if (props.kind === 'speech' || props.kind === 'transcription') {
    code = buildAudioExample(
      props.origin,
      props.kind,
      t('Hello! Welcome to New API.'),
      t('Saved speech.mp3')
    )
  }
  const endpoint = {
    quickstart: 'POST /v1/chat/completions',
    chat: 'POST /v1/chat/completions',
    image: 'POST /v1/images/generations',
    video: 'POST /v1/videos',
    speech: 'POST /v1/audio/speech',
    transcription: 'POST /v1/audio/transcriptions',
  }[props.kind]
  const baseURL = `${props.origin.replace(/\/$/, '')}/v1`
  let install = 'npm install openai'
  if (language === 'python') {
    install = ['video', 'speech', 'transcription'].includes(props.kind)
      ? 'pip install requests'
      : 'pip install openai'
  }
  let responsePreview: object = {
    choices: [
      {
        message: {
          role: 'assistant',
          content: t('Hello! How can I help you?'),
        },
      },
    ],
  }
  if (props.kind === 'image') {
    responsePreview = {
      created: 1720000000,
      data: [{ url: 'https://example.com/generated-image.png' }],
    }
  }
  if (props.kind === 'video') {
    responsePreview = { id: 'task_example', status: 'queued' }
  }
  if (props.kind === 'transcription') {
    responsePreview = { text: t('Hello, this is the transcribed text.') }
  }
  return (
    <div className='min-w-0 space-y-9'>
      {article.sections
        .filter((section) => !section.task || section.task === props.kind)
        .map((section) => (
          <section
            key={section.id}
            id={section.id}
            className='min-w-0 scroll-mt-24'
          >
            {section.id !== 'advanced-options' && (
              <h2 className='mb-3 text-xl font-semibold tracking-tight'>
                <a href={`#${section.id}`} className='hover:underline'>
                  {t(section.title)}
                </a>
              </h2>
            )}
            {section.id === 'endpoint' && (
              <div className='bg-muted/30 mb-3 flex min-w-0 items-center justify-between gap-3 rounded-lg border px-4 py-3'>
                <code className='text-sm break-all'>{endpoint}</code>
                <CopyButton value={endpoint} size='sm' />
              </div>
            )}
            {![
              'you-need',
              'advanced-options',
              'response',
              'result',
              'check-status',
            ].includes(section.id) && (
              <Markdown
                className={
                  section.id === 'how-it-works'
                    ? 'text-sm leading-7 [&_code]:break-all [&_ol]:grid [&_ol]:gap-4 [&_ol]:md:grid-cols-3'
                    : 'text-sm leading-7 [&_table]:w-full [&_table]:text-left [&_td]:py-2 [&_th]:py-2'
                }
              >
                {translateDocSection(section, t)}
              </Markdown>
            )}
            {props.kind === 'quickstart' &&
              ['create-key', 'choose-model'].includes(section.id) && (
                <Button
                  className='mt-3'
                  variant='outline'
                  size='sm'
                  render={
                    <Link
                      to={section.id === 'create-key' ? '/keys' : '/docs/$slug'}
                      params={
                        section.id === 'choose-model'
                          ? { slug: 'models' }
                          : undefined
                      }
                    />
                  }
                >
                  {t(
                    section.id === 'create-key'
                      ? 'Create API Key'
                      : 'View models'
                  )}
                </Button>
              )}
            {section.id === 'check-status' && (
              <div className='mt-3 space-y-3'>
                <div className='bg-muted/30 flex items-center justify-between gap-3 rounded-lg border px-4 py-3'>
                  <code className='text-sm break-all'>
                    GET /v1/videos/{'{task_id}'}
                  </code>
                  <CopyButton value='GET /v1/videos/{task_id}' size='sm' />
                </div>
                <DocsCode code={videoCode.status} language='bash' />
                <DocsCode
                  code={JSON.stringify(
                    { id: 'task_example', status: 'completed', progress: 100 },
                    null,
                    2
                  )}
                  language='json'
                />
                <Markdown className='text-sm leading-7 [&_table]:w-full [&_td]:py-2 [&_th]:py-2'>
                  {translateDocSection(section, t)}
                </Markdown>
              </div>
            )}
            {section.id === 'download-video' && (
              <div className='mt-3 space-y-3'>
                <div className='bg-muted/30 flex items-center justify-between gap-3 rounded-lg border px-4 py-3'>
                  <code className='text-sm break-all'>
                    GET /v1/videos/{'{task_id}'}/content
                  </code>
                  <CopyButton
                    value='GET /v1/videos/{task_id}/content'
                    size='sm'
                  />
                </div>
                <DocsCode code={videoCode.download} language='bash' />
              </div>
            )}
            {section.id === 'you-need' && (
              <dl className='grid min-w-0 grid-cols-[80px_minmax(0,1fr)] gap-x-5 gap-y-3 text-sm sm:grid-cols-[100px_minmax(0,1fr)]'>
                <dt className='text-muted-foreground'>Base URL</dt>
                <dd className='flex min-w-0 items-center gap-3'>
                  <code className='break-all'>{baseURL}</code>
                  <CopyButton value={baseURL} size='sm' />
                </dd>
                <dt className='text-muted-foreground'>API Key</dt>
                <dd className='flex flex-wrap items-center gap-x-4 gap-y-2'>
                  <code>YOUR_API_KEY</code>
                  <Link
                    to='/keys'
                    className='text-primary underline underline-offset-4'
                  >
                    {t('Get API Key')}
                  </Link>
                </dd>
                <dt className='text-muted-foreground'>Model ID</dt>
                <dd className='flex flex-wrap items-center gap-x-4 gap-y-2'>
                  <code>YOUR_MODEL_ID</code>
                  <Link
                    to='/docs/$slug'
                    params={{ slug: 'models' }}
                    className='text-primary underline underline-offset-4'
                  >
                    {t('View Models')}
                  </Link>
                </dd>
                {props.kind === 'speech' && (
                  <>
                    <dt className='text-muted-foreground'>{t('Voice')}</dt>
                    <dd>
                      <code>YOUR_VOICE_ID</code>
                      <p className='text-muted-foreground mt-1 text-xs leading-5'>
                        {t(
                          'Choose a voice supported by your model. Some models choose a default voice.'
                        )}
                      </p>
                    </dd>
                    <dt className='text-muted-foreground'>{t('Text')}</dt>
                    <dd>{t('Text you want to read aloud')}</dd>
                  </>
                )}
                {props.kind === 'transcription' && (
                  <>
                    <dt className='text-muted-foreground'>{t('Audio file')}</dt>
                    <dd>
                      <code>YOUR_AUDIO_FILE</code>
                    </dd>
                  </>
                )}
                {['image', 'video'].includes(props.kind) && (
                  <>
                    <dt className='text-muted-foreground'>Prompt</dt>
                    <dd>
                      {t(
                        props.kind === 'video'
                          ? 'Describe the video you want to create'
                          : 'Describe the image you want to create'
                      )}
                    </dd>
                  </>
                )}
              </dl>
            )}
            {['quick-example', 'create-video'].includes(section.id) && (
              <Tabs
                value={language}
                onValueChange={(value) => setLanguage(String(value))}
                className='mt-4 min-w-0'
              >
                <TabsList aria-label={t('SDK language')}>
                  <TabsTrigger value='curl'>cURL</TabsTrigger>
                  <TabsTrigger value='python'>Python</TabsTrigger>
                  <TabsTrigger value='javascript'>JavaScript</TabsTrigger>
                </TabsList>
                {(['curl', 'python', 'javascript'] as const).map((value) => (
                  <TabsContent
                    key={value}
                    value={value}
                    className='min-w-0 space-y-3'
                  >
                    {value !== 'curl' &&
                      !(
                        ['video', 'speech', 'transcription'].includes(
                          props.kind
                        ) && value === 'javascript'
                      ) && (
                        <div className='flex flex-wrap items-center gap-2 text-xs'>
                          <span className='text-muted-foreground'>
                            {t('Install:')}
                          </span>
                          <code>{install}</code>
                          <CopyButton value={install} size='sm' />
                        </div>
                      )}
                    <DocsCode
                      code={code[value]}
                      language={value === 'curl' ? 'bash' : value}
                    />
                    {value === 'javascript' && (
                      <p className='text-muted-foreground text-sm leading-6'>
                        {t(
                          'This JavaScript example runs in Node.js. Never put an API Key in frontend code that runs in the browser.'
                        )}
                      </p>
                    )}
                  </TabsContent>
                ))}
              </Tabs>
            )}
            {props.kind === 'quickstart' && section.id === 'quick-example' && (
              <Link
                className='text-primary mt-3 block text-sm underline underline-offset-4'
                to='/docs/$slug'
                params={{ slug: 'text-chat' }}
              >
                {t('More options in Chat')}
              </Link>
            )}
            {['response', 'result'].includes(section.id) && (
              <div className='mt-3'>
                {props.kind !== 'speech' && (
                  <DocsCode
                    language='json'
                    code={JSON.stringify(responsePreview, null, 2)}
                  />
                )}
                <Markdown className='mt-3 text-sm leading-7'>
                  {translateDocSection(section, t)}
                </Markdown>
                {props.kind === 'quickstart' && (
                  <div className='mt-3 space-y-3'>
                    <p className='text-muted-foreground text-sm'>
                      {t(
                        'View request history, models and costs in Usage & Logs.'
                      )}
                    </p>
                    <Button
                      variant='outline'
                      size='sm'
                      render={
                        <Link
                          to='/usage-logs/$section'
                          params={{ section: 'common' }}
                        />
                      }
                    >
                      {t('View Usage & Logs')}
                    </Button>
                  </div>
                )}
              </div>
            )}
            {section.id === 'advanced-options' && (
              <Collapsible className='rounded-lg border'>
                <h2>
                  <CollapsibleTrigger className='flex w-full items-center justify-between gap-3 px-4 py-3 text-left text-sm font-medium'>
                    {t('Advanced options')}
                    <ChevronDown aria-hidden='true' className='size-4' />
                  </CollapsibleTrigger>
                </h2>
                <CollapsibleContent className='px-4 pb-4'>
                  <Markdown className='text-sm leading-7 [&_table]:w-full [&_table]:text-left [&_td]:py-2 [&_th]:py-2'>
                    {translateDocSection(section, t)}
                  </Markdown>
                </CollapsibleContent>
              </Collapsible>
            )}
          </section>
        ))}
      {props.kind === 'image' && (
        <Alert className='px-4 py-3'>
          <AlertDescription className='text-sm leading-6'>
            <Markdown>
              {t(
                'Image generation can take longer than chat. If the request times out, check [Usage & Logs](/usage-logs/common) and [Task Logs](/usage-logs/task) before trying again. Some image tasks can continue after a timeout. A `504 task_timeout` can mean the result is still being processed.'
              )}
            </Markdown>
          </AlertDescription>
        </Alert>
      )}
      {props.kind === 'video' && (
        <Alert className='px-4 py-3'>
          <AlertDescription className='text-sm leading-6'>
            <Markdown>
              {t(
                'Video generation can take a few minutes depending on the model. Keep your Task ID to check later. Before resubmitting a slow or failed request, check its status, [Usage & Logs](/usage-logs/common) and [Task Logs](/usage-logs/task).'
              )}
            </Markdown>
          </AlertDescription>
        </Alert>
      )}
      {props.kind === 'speech' && (
        <Alert className='px-4 py-3'>
          <AlertDescription className='text-sm leading-6'>
            {t(
              'These examples use synchronous speech models with MP3 output. Some models return Task IDs and require different request fields. Follow the model instructions for task-based speech.'
            )}
          </AlertDescription>
        </Alert>
      )}
      {['speech', 'transcription'].includes(props.kind) && (
        <p className='text-muted-foreground text-sm'>
          {t('JavaScript examples require Node.js 20 or newer.')}
        </p>
      )}
      <Alert className='px-4 py-3'>
        <AlertTitle>{t('Security note')}</AlertTitle>
        <AlertDescription className='text-sm leading-6'>
          <p>
            {t(
              'Do not share your API Key. In real applications, store it in an environment variable or secret manager instead of source code.'
            )}
          </p>
          {props.kind === 'quickstart' && (
            <p>{t('Never put an API Key in public frontend code.')}</p>
          )}
          <Link
            to='/docs/$slug'
            params={{ slug: 'api-key' }}
            className='text-primary underline underline-offset-4'
          >
            {t('API Key')}
          </Link>
        </AlertDescription>
      </Alert>
    </div>
  )
}
