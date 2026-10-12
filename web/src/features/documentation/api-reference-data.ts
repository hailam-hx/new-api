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
import reference from './generated/reference.json'

// Deliberate customer subset. Routes and OpenAPI field types come from the
// existing generator; host DTO corrections below supersede stale schemas.
export const apiReferenceGroups = [
  ['models', 'Models'],
  ['chat', 'Chat'],
  ['responses', 'Responses'],
  ['images', 'Image'],
  ['video', 'Video'],
  ['audio', 'Audio'],
] as const

type Field = {
  name: string
  type: string
  required: boolean
  conditional?: boolean
}
type Definition = {
  method: 'GET' | 'POST'
  path: string
  group: string
  description: string
  fields?: string[]
  conditional?: string[]
  required?: string[]
  types?: Record<string, string>
  contentType?: string
  payload?: Record<string, unknown>
  form?: Record<string, string>
  response: Record<string, unknown> | string
  note?: string
  source: string
}
const videoShape = {
  id: 'VIDEO_ID',
  object: 'video',
  model: 'YOUR_MODEL_ID',
  status: 'queued',
  progress: 0,
  created_at: 0,
}
const imageShape = { created: 0, data: [{ url: 'IMAGE_URL' }] }
const responseShape = {
  id: 'resp_RESPONSE_ID',
  object: 'response',
  created_at: 0,
  model: 'YOUR_MODEL_ID',
  status: 'completed',
  output: [
    {
      type: 'message',
      role: 'assistant',
      content: [{ type: 'output_text', text: 'RESPONSE_TEXT' }],
    },
  ],
}
const definitions: Definition[] = [
  {
    method: 'GET',
    path: '/v1/models',
    group: 'models',
    description: 'List accessible models',
    source: 'controller/model.go; relaykit/dto/pricing.go',
    response: {
      success: true,
      object: 'list',
      data: [
        {
          id: 'YOUR_MODEL_ID',
          object: 'model',
          created: 0,
          owned_by: 'PROVIDER',
          supported_endpoint_types: ['openai'],
        },
      ],
    },
    note: 'Model discovery is scoped to your API key; Bearer authentication returns the OpenAI format.',
  },
  {
    method: 'POST',
    path: '/v1/chat/completions',
    group: 'chat',
    description: 'Create a chat completion',
    source:
      'relaykit/dto/openai_request.go; relay/helper/valid_request.go; relaykit/dto/openai_response.go',
    fields: [
      'model',
      'messages',
      'stream',
      'stream_options',
      'temperature',
      'top_p',
      'max_tokens',
      'max_completion_tokens',
      'stop',
      'tools',
      'tool_choice',
      'response_format',
      'seed',
    ],
    required: ['model', 'messages'],
    types: { seed: 'number' },
    contentType: 'application/json',
    payload: {
      model: 'YOUR_MODEL_ID',
      messages: [{ role: 'user', content: 'Hello' }],
    },
    response: {
      id: 'COMPLETION_ID',
      object: 'chat.completion',
      created: 0,
      model: 'YOUR_MODEL_ID',
      choices: [
        {
          index: 0,
          message: { role: 'assistant', content: 'RESPONSE_TEXT' },
          finish_reason: 'stop',
        },
      ],
    },
    note: 'stream=true returns SSE when supported by the model.',
  },
  {
    method: 'POST',
    path: '/v1/responses',
    group: 'responses',
    description: 'Create a response',
    source:
      'relaykit/dto/openai_request.go; relay/helper/valid_request.go; router/task-plugin-protocol-router.go',
    fields: [
      'model',
      'input',
      'instructions',
      'stream',
      'max_output_tokens',
      'temperature',
      'top_p',
      'tools',
      'tool_choice',
      'reasoning',
      'previous_response_id',
      'truncation',
    ],
    required: ['model', 'input'],
    contentType: 'application/json',
    payload: { model: 'YOUR_MODEL_ID', input: 'Hello' },
    response: responseShape,
    note: 'Feature support depends on the channel. Retrieval only supports stored plugin responses.',
  },
  {
    method: 'GET',
    path: '/v1/responses/{response_id}',
    group: 'responses',
    description: 'Retrieve a plugin response',
    source: 'controller/plugin_protocol.go',
    response: responseShape,
    note: 'Feature support depends on the channel. Retrieval only supports stored plugin responses.',
  },
  {
    method: 'POST',
    path: '/v1/images/generations',
    group: 'images',
    description: 'Generate images',
    source:
      'relaykit/dto/openai_image.go; relay/helper/valid_request.go; pkg/jsplugin/routing.go',
    fields: [
      'model',
      'prompt',
      'n',
      'size',
      'quality',
      'response_format',
      'stream',
    ],
    required: ['model', 'prompt'],
    types: {
      model: 'string',
      prompt: 'string',
      n: 'integer',
      size: 'string',
      quality: 'string',
      response_format: 'string',
      stream: 'boolean',
    },
    contentType: 'application/json',
    payload: { model: 'YOUR_MODEL_ID', prompt: 'A small red robot' },
    response: imageShape,
    note: 'Image data contains url or b64_json according to model support and response_format.',
  },
  {
    method: 'POST',
    path: '/v1/images/edits',
    group: 'images',
    description: 'Edit images',
    source:
      'relaykit/dto/openai_image.go; relay/helper/valid_request.go; pkg/jsplugin/routing.go',
    fields: [
      'model',
      'prompt',
      'image',
      'mask',
      'n',
      'size',
      'quality',
      'response_format',
    ],
    required: ['model', 'prompt', 'image'],
    types: {
      model: 'string',
      prompt: 'string',
      image: 'file / file[]',
      mask: 'file',
      n: 'integer',
      size: 'string',
      quality: 'string',
      response_format: 'string',
    },
    contentType: 'multipart/form-data',
    form: {
      model: 'YOUR_MODEL_ID',
      prompt: 'Make the background blue',
      image: '@image.png',
    },
    response: imageShape,
    note: 'Compatible models also accept JSON. Multipart accepts image or image[]; mask is model-dependent.',
  },
  {
    method: 'POST',
    path: '/v1/videos',
    group: 'video',
    description: 'Create a video',
    source:
      'pkg/jsplugin/routing.go; relay/channel/openai/adaptor.go; relaykit/dto/openai_video.go',
    fields: ['model', 'prompt', 'seconds', 'input_reference'],
    required: ['model'],
    conditional: ['prompt'],
    types: { input_reference: 'file' },
    contentType: 'multipart/form-data',
    form: { model: 'YOUR_MODEL_ID', prompt: 'A robot waving' },
    response: videoShape,
    note: 'JSON and multipart are accepted. Model-specific options vary; retrieve status before content.',
  },
  {
    method: 'GET',
    path: '/v1/videos/{task_id}',
    group: 'video',
    description: 'Retrieve video status',
    source: 'relay/relay_task.go; relaykit/dto/openai_video.go',
    response: videoShape,
  },
  {
    method: 'GET',
    path: '/v1/videos/{task_id}/content',
    group: 'video',
    description: 'Retrieve video content',
    source: 'controller/video_proxy.go',
    response: 'video/mp4',
    note: 'Binary video; 400 if unfinished, 404 if missing, 410 if expired.',
  },
  {
    method: 'POST',
    path: '/v1/audio/speech',
    group: 'audio',
    description: 'Generate speech',
    source:
      'relaykit/dto/audio.go; router/task-plugin-protocol-router.go; controller/plugin_protocol_audio_speech.go',
    fields: ['model', 'input', 'voice', 'response_format', 'speed'],
    required: ['model'],
    conditional: ['input', 'voice'],
    contentType: 'application/json',
    payload: { model: 'YOUR_MODEL_ID', input: 'Hello', voice: 'YOUR_VOICE_ID' },
    response: 'audio/mpeg',
    note: 'Speech returns audio bytes. voice-tts-pro returns task JSON; poll /v1/audio/speech/{task_id}.',
  },
  {
    method: 'GET',
    path: '/v1/audio/speech/{task_id}',
    group: 'audio',
    description: 'Retrieve a speech task',
    source: 'controller/plugin_protocol_audio_speech.go; dto/task_plugin.go',
    response: {
      task_id: 'task_TASK_ID',
      platform: 'dflop-tts',
      status: 'QUEUED',
      progress: '0%',
      fail_reason: '',
      created_at: 0,
    },
    note: 'Only voice-tts-pro tasks belonging to your account can be retrieved.',
  },
  {
    method: 'POST',
    path: '/v1/audio/transcriptions',
    group: 'audio',
    description: 'Transcribe audio',
    source:
      'relay/audio_handler.go; relaykit/dto/audio.go; relay/channel/openai/audio.go',
    fields: [
      'model',
      'file',
      'language',
      'prompt',
      'response_format',
      'temperature',
    ],
    required: ['model', 'file'],
    types: { file: 'file' },
    contentType: 'multipart/form-data',
    form: { model: 'YOUR_MODEL_ID', file: '@audio.mp3' },
    response: { text: 'TRANSCRIBED_TEXT' },
    note: 'Default response: JSON text. Other formats depend on the model.',
  },
]
export const publicApiEndpoints = definitions.map((definition) => {
  const route = reference.endpoints.find(
    (endpoint) =>
      endpoint.method === definition.method && endpoint.path === definition.path
  )
  if (!route) {
    throw new Error(
      `Unregistered customer API: ${definition.method} ${definition.path}`
    )
  }
  const body = route.requestBody as {
    content?: Record<
      string,
      {
        schema?: {
          properties?: Record<
            string,
            { type?: string; oneOf?: { type?: string }[] }
          >
        }
      }
    >
  } | null
  const properties =
    Object.values(body?.content ?? {})[0]?.schema?.properties ?? {}
  const fields: Field[] = (definition.fields ?? []).map((name) => {
    const schema = properties[name]
    const type =
      definition.types?.[name] ??
      schema?.type ??
      schema?.oneOf
        ?.map((option) => option.type)
        .filter(Boolean)
        .join(' / ') ??
      'JSON'
    if (!schema && !definition.types?.[name]) {
      throw new Error(`Missing request schema: ${definition.path}/${name}`)
    }
    return {
      name,
      type: type || 'JSON',
      required: definition.required?.includes(name) ?? false,
      conditional: definition.conditional?.includes(name) ?? false,
    }
  })
  for (const name of definition.path.matchAll(/\{([^}]+)\}/g)) {
    fields.push({ name: name[1], type: 'string', required: true })
  }
  const url = `https://hotx-api.com${definition.path}`
  const command = [`curl '${url}'`, "  -H 'Authorization: Bearer YOUR_API_KEY'"]
  if (definition.payload) {
    command.push(
      "  -H 'Content-Type: application/json'",
      `  -d '${JSON.stringify(definition.payload, null, 2)}'`
    )
  }
  if (definition.form) {
    for (const [name, value] of Object.entries(definition.form)) {
      command.push(`  -F '${name}=${value}'`)
    }
  }
  if (typeof definition.response === 'string') {
    command.push(
      `  --output ${definition.group === 'video' ? 'video.mp4' : 'speech.mp3'}`
    )
  }
  const example = command.join(' \\\n')
  return {
    ...definition,
    fields,
    example,
    routeSource: route.source,
    schemaSource: route.schemaSource,
  }
})
