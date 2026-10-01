import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { ThemedToken } from 'shiki'

import { CodeBlockFrame } from '@/components/ai-elements/code-block'
import { CopyButton } from '@/components/copy-button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
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
import { useTheme } from '@/context/theme-provider'

import { buildQuickstart } from './lib'

export function DocsCode(props: {
  code: string
  language: 'bash' | 'python' | 'typescript' | 'json'
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

export function CodeExamples(props: { origin: string; protocol?: string }) {
  const { t } = useTranslation()
  const [stream, setStream] = useState('minimal')
  const code = buildQuickstart(
    props.origin,
    props.protocol ?? 'openai',
    stream === 'stream'
  )
  return (
    <section className='mt-8 min-w-0 space-y-4' id='request-examples'>
      <h2 className='scroll-mt-40 text-xl font-semibold'>
        {t('Request examples')}
      </h2>
      <Tabs value={stream} onValueChange={(value) => setStream(String(value))}>
        <TabsList aria-label={t('Request mode')}>
          <TabsTrigger value='minimal'>{t('Minimal request')}</TabsTrigger>
          <TabsTrigger value='stream'>{t('Streaming')}</TabsTrigger>
        </TabsList>
      </Tabs>
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
