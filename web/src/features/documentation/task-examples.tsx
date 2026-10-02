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
import { useTranslation } from 'react-i18next'

import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

import { DocsCode } from './code-examples'
import { buildTaskExample, type MediaExampleKind } from './task-example-code'

function MediaCodeTabs(props: { origin: string; kind: MediaExampleKind }) {
  const { t } = useTranslation()
  const code = buildTaskExample(props.origin, props.kind)
  return (
    <Tabs defaultValue='curl' className='min-w-0'>
      <TabsList aria-label={t('Example language')}>
        <TabsTrigger value='curl'>cURL</TabsTrigger>
        <TabsTrigger value='python'>Python</TabsTrigger>
        <TabsTrigger value='javascript'>JavaScript</TabsTrigger>
      </TabsList>
      <TabsContent value='curl'>
        <DocsCode code={code.curl} language='bash' />
      </TabsContent>
      <TabsContent value='python'>
        <DocsCode code={code.python} language='python' />
      </TabsContent>
      <TabsContent value='javascript'>
        <DocsCode code={code.javascript} language='javascript' />
      </TabsContent>
    </Tabs>
  )
}

export function TaskExamples(props: {
  kind: 'image' | 'video' | 'audio'
  origin: string
}) {
  const { t } = useTranslation()
  return (
    <section id='request-examples' className='mt-8 min-w-0 space-y-4'>
      <h2 className='scroll-mt-40 text-xl font-semibold'>
        {t('Request examples')}
      </h2>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Set NEW_API_KEY securely and set NEW_API_MODEL to the model ID you chose in Models. Check that it supports this API.'
        )}
      </p>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Run these examples on your computer or server. cURL needs jq; Python needs pip install requests; JavaScript needs Node.js 20 or newer.'
        )}
      </p>
      {props.kind === 'audio' ? (
        <Tabs defaultValue='speech' className='min-w-0'>
          <TabsList aria-label={t('Audio task')}>
            <TabsTrigger value='speech'>{t('Text-to-Speech')}</TabsTrigger>
            <TabsTrigger value='transcription'>
              {t('Speech-to-Text')}
            </TabsTrigger>
          </TabsList>
          <TabsContent value='speech' className='space-y-4'>
            <p className='text-muted-foreground text-sm'>
              {t(
                'Choose a speech model and set NEW_API_VOICE to a voice it supports. A normal response saves speech.mp3. If the model returns task JSON, keep its task ID and check GET /v1/audio/speech/{task_id} for the result.'
              )}
            </p>
            <MediaCodeTabs origin={props.origin} kind='speech' />
          </TabsContent>
          <TabsContent value='transcription' className='space-y-4'>
            <p className='text-muted-foreground text-sm'>
              {t(
                'Choose a transcription model and set NEW_API_AUDIO_FILE to your local audio file path. The response contains the recognized text. Supported file formats and upload limits depend on the model.'
              )}
            </p>
            <MediaCodeTabs origin={props.origin} kind='transcription' />
          </TabsContent>
        </Tabs>
      ) : (
        <MediaCodeTabs origin={props.origin} kind={props.kind} />
      )}
      {props.kind === 'image' && (
        <p className='text-muted-foreground text-sm'>
          {t(
            'The response contains image URLs or base64 image data. Open a returned URL or decode b64_json to save the image. Keep the full response if you need help.'
          )}
        </p>
      )}
      {props.kind === 'video' && (
        <p className='text-muted-foreground text-sm'>
          {t(
            'Create the task, keep its ID, check its status, then download video.mp4 after completion. These examples stop waiting after about ten minutes. Keep the same task ID and check again later instead of creating another task.'
          )}
        </p>
      )}
      <Collapsible>
        <CollapsibleTrigger className='focus-visible:ring-ring rounded-md py-2 text-sm font-medium underline underline-offset-4 focus-visible:ring-2 focus-visible:outline-none'>
          {t('Advanced options')}
        </CollapsibleTrigger>
        <CollapsibleContent className='text-muted-foreground pt-2 text-sm'>
          {props.kind === 'image' &&
            t(
              'Image requests can include n, size, quality and response_format when supported by the chosen model. Start with the defaults; larger images or additional images can cost more.'
            )}
          {props.kind === 'video' &&
            t(
              'Duration, resolution and reference images depend on the video model. Add only the fields and values supported by your chosen model; there is no shared aspect-ratio setting for every provider.'
            )}
          {props.kind === 'audio' &&
            t(
              'Speech requests can include response_format and speed when the model supports them. Transcription uses multipart file uploads. Do not manually set the multipart Content-Type boundary.'
            )}
        </CollapsibleContent>
      </Collapsible>
      <p className='text-muted-foreground text-xs'>
        {t(
          'If generation times out, check Usage Logs and Task Logs before sending the request again. A task may still finish and incur a charge.'
        )}
      </p>
    </section>
  )
}
