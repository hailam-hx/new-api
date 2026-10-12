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

// Public client contracts. Backend DTOs and plugin presenters take precedence
// over generated OpenAPI schemas; deployment-dependent routes are marked below.
export const apiReferenceGroups = [
  ['models', 'Models API'],
  ['chat', 'Text APIs'],
  ['responses', 'Responses API'],
  ['anthropic', 'Anthropic Messages'],
  ['gemini', 'Gemini Native'],
  ['images', 'Image API'],
  ['video', 'Video API'],
  ['audio', 'Audio API'],
  ['utilities', 'AI Utilities'],
  ['realtime', 'Realtime API'],
  ['tasks', 'Task API'],
  ['plugin-routes', 'Native Plugin APIs'],
  ['midjourney', 'Midjourney API'],
  ['billing', 'Billing APIs'],
] as const

type Field = {
  name: string
  type: string
  required: boolean
  conditional?: boolean
}
type Definition = {
  method: 'GET' | 'POST' | 'HEAD'
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
  response: Record<string, unknown> | unknown[] | string
  note?: string
  source: string
  deploymentDependent?: boolean
  headers?: Record<string, string>
  query?: Record<string, string>
  transport?: 'websocket'
  nativeRoute?: boolean
  responseFields?: Field[]
  responseTypes?: Record<string, string>
  responseAlternatives?: {
    description: string
    response: Record<string, unknown>
    responseFields: Field[]
  }[]
  schemaNote?: string
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
    types: {
      model: 'string',
      prompt: 'string',
      seconds: 'integer / string',
      input_reference: 'file',
    },
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
    fields: ['model', 'input', 'voice', 'response_format', 'speed', 'async'],
    types: { async: 'boolean' },
    headers: { 'Idempotency-Key': 'YOUR_IDEMPOTENCY_KEY' },
    responseAlternatives: [
      {
        description: 'Asynchronous voice-tts-pro task response',
        response: {
          id: 'task_TASK_ID',
          task_id: 'task_TASK_ID',
          status: 'queued',
          model: 'voice-tts-pro',
          created_at: 0,
        },
        responseFields: ['id', 'task_id', 'status', 'model', 'created_at'].map(
          (name) => ({
            name,
            type: name === 'created_at' ? 'integer' : 'string',
            required: true,
          })
        ),
      },
    ],
    required: ['model'],
    conditional: ['input', 'voice', 'async'],
    contentType: 'application/json',
    payload: { model: 'YOUR_MODEL_ID', input: 'Hello', voice: 'YOUR_VOICE_ID' },
    response: 'audio/mpeg',
    note: 'Speech normally returns audio bytes. For the enabled voice-tts-pro plugin, send async=true and an Idempotency-Key header containing 1 to 200 printable ASCII characters without spaces. This form returns task JSON; poll /v1/audio/speech/{task_id}. input is required; voice is optional for this plugin.',
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

const additionalDefinitions: Definition[] = [
  {
    method: 'GET',
    path: '/v1/models/{model}',
    group: 'models',
    description: 'Retrieve model metadata',
    source: 'controller/model.go',
    response: {
      id: 'YOUR_MODEL_ID',
      object: 'model',
      created: 0,
      owned_by: 'PROVIDER',
    },
    note: 'Returns OpenAI metadata by default, or Anthropic metadata with x-api-key and anthropic-version headers. An unknown model returns an error object with HTTP 200.',
  },
  {
    method: 'GET',
    path: '/v1beta/models',
    group: 'models',
    description: 'List accessible models',
    source: 'controller/model.go',
    response: {
      models: [
        {
          name: 'models/YOUR_MODEL_ID',
          displayName: 'YOUR_MODEL_ID',
        },
      ],
    },
    note: 'Model discovery is scoped to your API key; response format follows the selected protocol.',
  },
  {
    method: 'GET',
    path: '/v1beta/openai/models',
    group: 'models',
    description: 'List accessible models',
    source: 'controller/model.go',
    response: {
      success: true,
      object: 'list',
      data: [
        {
          id: 'YOUR_MODEL_ID',
          object: 'model',
          created: 0,
          owned_by: 'PROVIDER',
        },
      ],
    },
    note: 'Model discovery is scoped to your API key; response format follows the selected protocol.',
  },
  {
    method: 'POST',
    path: '/v1/completions',
    group: 'chat',
    description: 'Create a text completion',
    source: 'relaykit/dto/openai_request.go; relaykit/dto/openai_response.go',
    response: {
      id: 'COMPLETION_ID',
      object: 'text_completion',
      created: 0,
      model: 'YOUR_MODEL_ID',
      choices: [
        {
          index: 0,
          text: 'RESPONSE_TEXT',
          finish_reason: 'stop',
        },
      ],
    },
    fields: [
      'model',
      'prompt',
      'stream',
      'temperature',
      'top_p',
      'max_tokens',
      'stop',
      'suffix',
      'n',
    ],
    required: ['model', 'prompt'],
    types: {
      model: 'string',
      prompt: 'string / array',
      stream: 'boolean',
      temperature: 'number',
      top_p: 'number',
      max_tokens: 'integer',
      stop: 'string / array',
      suffix: 'string',
      n: 'integer',
    },
    contentType: 'application/json',
    payload: {
      model: 'YOUR_MODEL_ID',
      prompt: 'Hello',
    },
    note: 'stream=true returns SSE when supported by the model.',
  },
  {
    method: 'POST',
    path: '/v1/responses/compact',
    group: 'responses',
    description: 'Compact a response conversation',
    source:
      'relaykit/dto/openai_responses_compaction_request.go; relaykit/dto/openai_compaction.go',
    response: {
      id: 'COMPACTION_ID',
      object: 'response.compaction',
      created_at: 0,
      output: [
        {
          type: 'compaction',
          encrypted_content: 'OPAQUE_CONTENT',
        },
      ],
    },
    fields: [
      'model',
      'input',
      'instructions',
      'previous_response_id',
      'tools',
      'reasoning',
    ],
    required: ['model'],
    types: {
      model: 'string',
      input: 'string / array',
      instructions: 'JSON',
      previous_response_id: 'string',
      tools: 'array',
      reasoning: 'object',
    },
    contentType: 'application/json',
    payload: {
      model: 'YOUR_MODEL_ID',
      input: [
        {
          role: 'user',
          content: 'Hello',
        },
      ],
    },
    note: 'Compaction is non-streaming and requires channel support. Output items and opaque content depend on the upstream provider.',
  },
  {
    method: 'POST',
    path: '/v1/messages',
    group: 'anthropic',
    description: 'Create an Anthropic message',
    source: 'relaykit/dto/claude.go; relay/helper/valid_request.go',
    response: {
      id: 'MESSAGE_ID',
      type: 'message',
      role: 'assistant',
      model: 'YOUR_MODEL_ID',
      content: [
        {
          type: 'text',
          text: 'RESPONSE_TEXT',
        },
      ],
      stop_reason: 'end_turn',
      usage: {
        input_tokens: 0,
        output_tokens: 0,
      },
    },
    fields: [
      'model',
      'messages',
      'max_tokens',
      'system',
      'stream',
      'temperature',
      'top_p',
      'top_k',
      'stop_sequences',
      'tools',
      'tool_choice',
      'thinking',
    ],
    required: ['model', 'messages'],
    types: {
      model: 'string',
      messages: 'array',
      max_tokens: 'integer',
      system: 'string / array',
      stream: 'boolean',
      temperature: 'number',
      top_p: 'number',
      top_k: 'integer',
      stop_sequences: 'array',
      tools: 'array',
      tool_choice: 'object',
      thinking: 'object',
    },
    contentType: 'application/json',
    headers: {
      'anthropic-version': '2023-06-01',
    },
    payload: {
      model: 'YOUR_MODEL_ID',
      max_tokens: 1024,
      messages: [
        {
          role: 'user',
          content: 'Hello',
        },
      ],
    },
    note: 'Bearer authentication is accepted; Anthropic clients may use x-api-key. Set max_tokens according to the selected model. stream=true returns Anthropic SSE events when supported. Token counting is not implemented.',
  },
  {
    method: 'POST',
    path: '/v1/models/{path}',
    group: 'gemini',
    description: 'Call a Gemini native action',
    source:
      'relaykit/dto/gemini.go; relay/helper/valid_request.go; relay/channel/gemini/adaptor.go',
    response: {
      candidates: [
        {
          content: {
            role: 'model',
            parts: [
              {
                text: 'RESPONSE_TEXT',
              },
            ],
          },
          finishReason: 'STOP',
        },
      ],
    },
    fields: [
      'contents',
      'generationConfig',
      'systemInstruction',
      'safetySettings',
      'tools',
      'toolConfig',
      'cachedContent',
      'content',
      'requests',
    ],
    types: {
      contents: 'array',
      generationConfig: 'object',
      systemInstruction: 'object',
      safetySettings: 'array',
      tools: 'array',
      toolConfig: 'object',
      cachedContent: 'string',
      content: 'object',
      requests: 'array',
    },
    conditional: ['contents', 'content', 'requests'],
    contentType: 'application/json',
    payload: {
      contents: [
        {
          role: 'user',
          parts: [
            {
              text: 'Hello',
            },
          ],
        },
      ],
    },
    note: 'Replace {path} with YOUR_MODEL_ID:generateContent, YOUR_MODEL_ID:streamGenerateContent, YOUR_MODEL_ID:embedContent or YOUR_MODEL_ID:batchEmbedContents. Generation uses contents, embedding uses content, batch embedding uses requests. Embedding responses contain embedding.values or embeddings[].values. Streaming uses SSE; :countTokens is unavailable on /v1beta. Bearer or x-goog-api-key authentication is accepted.',
  },
  {
    method: 'POST',
    path: '/v1beta/models/{path}',
    group: 'gemini',
    description: 'Call a Gemini native action',
    source:
      'relaykit/dto/gemini.go; relay/helper/valid_request.go; relay/channel/gemini/adaptor.go',
    response: {
      candidates: [
        {
          content: {
            role: 'model',
            parts: [
              {
                text: 'RESPONSE_TEXT',
              },
            ],
          },
          finishReason: 'STOP',
        },
      ],
    },
    fields: [
      'contents',
      'generationConfig',
      'systemInstruction',
      'safetySettings',
      'tools',
      'toolConfig',
      'cachedContent',
      'content',
      'requests',
    ],
    types: {
      contents: 'array',
      generationConfig: 'object',
      systemInstruction: 'object',
      safetySettings: 'array',
      tools: 'array',
      toolConfig: 'object',
      cachedContent: 'string',
      content: 'object',
      requests: 'array',
    },
    conditional: ['contents', 'content', 'requests'],
    contentType: 'application/json',
    payload: {
      contents: [
        {
          role: 'user',
          parts: [
            {
              text: 'Hello',
            },
          ],
        },
      ],
    },
    note: 'Replace {path} with YOUR_MODEL_ID:generateContent, YOUR_MODEL_ID:streamGenerateContent, YOUR_MODEL_ID:embedContent or YOUR_MODEL_ID:batchEmbedContents. Generation uses contents, embedding uses content, batch embedding uses requests. Embedding responses contain embedding.values or embeddings[].values. Streaming uses SSE; :countTokens is unavailable on /v1beta. Bearer or x-goog-api-key authentication is accepted.',
  },
  {
    method: 'POST',
    path: '/v1/engines/{model}/embeddings',
    group: 'gemini',
    description: 'Create an engine embedding',
    source: 'relay/gemini_handler.go; relaykit/dto/gemini.go',
    response: { embedding: { values: [0.1, 0.2] } },
    fields: ['content', 'taskType', 'title', 'outputDimensionality'],
    required: ['content'],
    types: {
      content: 'object',
      taskType: 'string',
      title: 'string',
      outputDimensionality: 'integer',
    },
    contentType: 'application/json',
    payload: { content: { parts: [{ text: 'Hello' }] } },
    note: 'Legacy Gemini-compatible route. Request conversion and response format depend on the selected channel; prefer the native embedContent action.',
  },
  {
    method: 'POST',
    path: '/v1/edits',
    group: 'images',
    description: 'Inspect the registered legacy edits route',
    source:
      'router/relay-router.go; relay/constant/relay_mode.go; relay/helper/valid_request.go; controller/relay.go; relay/compatible_handler.go',
    response: {},
    contentType: 'application/json',
    note: 'This registered route currently fails because its image request DTO is dispatched to the text handler. It is not a usable image-edit API. Use /v1/images/edits instead; no successful response schema is claimed here.',
    schemaNote:
      'This registered route currently fails because its image request DTO is dispatched to the text handler. It is not a usable image-edit API. Use /v1/images/edits instead; no successful response schema is claimed here.',
    responseFields: [],
  },
  {
    method: 'POST',
    path: '/v1/embeddings',
    group: 'utilities',
    description: 'Create embeddings',
    source: 'relaykit/dto/embedding.go; relay/helper/valid_request.go',
    response: {
      object: 'list',
      model: 'YOUR_MODEL_ID',
      data: [
        {
          object: 'embedding',
          index: 0,
          embedding: [0.1, 0.2],
        },
      ],
      usage: {
        prompt_tokens: 0,
        total_tokens: 0,
      },
    },
    fields: ['model', 'input', 'encoding_format', 'dimensions', 'user'],
    required: ['model', 'input'],
    types: {
      model: 'string',
      input: 'string / array',
      encoding_format: 'string',
      dimensions: 'integer',
      user: 'string',
    },
    contentType: 'application/json',
    payload: {
      model: 'YOUR_MODEL_ID',
      input: 'Hello',
    },
    note: 'Input forms, dimensions and encoding formats depend on the model.',
  },
  {
    method: 'POST',
    path: '/v1/rerank',
    group: 'utilities',
    description: 'Rank documents by relevance',
    source: 'relaykit/dto/rerank.go',
    response: {
      results: [
        {
          index: 0,
          relevance_score: 0.9,
        },
      ],
      usage: {
        total_tokens: 0,
      },
    },
    fields: [
      'model',
      'query',
      'documents',
      'top_n',
      'return_documents',
      'max_chunk_per_doc',
      'overlap_tokens',
    ],
    required: ['model', 'query', 'documents'],
    types: {
      model: 'string',
      query: 'string',
      documents: 'array',
      top_n: 'integer',
      return_documents: 'boolean',
      max_chunk_per_doc: 'integer',
      overlap_tokens: 'integer',
    },
    contentType: 'application/json',
    payload: {
      model: 'YOUR_MODEL_ID',
      query: 'Hello',
      documents: ['Hello world', 'Goodbye'],
    },
    note: 'return_documents includes document data when supported; chunking options depend on the channel.',
  },
  {
    method: 'POST',
    path: '/v1/moderations',
    group: 'utilities',
    description: 'Moderate content',
    source:
      'relaykit/dto/openai_request.go; relay/helper/valid_request.go; relay/channel/openai/relay-openai.go',
    response: {
      id: 'MODERATION_ID',
      model: 'YOUR_MODEL_ID',
      results: [
        {
          flagged: false,
          categories: {},
          category_scores: {},
        },
      ],
    },
    fields: ['model', 'input'],
    required: ['input'],
    types: {
      model: 'string',
      input: 'string / array',
    },
    contentType: 'application/json',
    payload: {
      model: 'YOUR_MODEL_ID',
      input: 'Hello',
    },
    note: 'The default model is text-moderation-latest when omitted. Categories and scores are provider-defined.',
  },
  {
    method: 'POST',
    path: '/v1/alpha/search',
    group: 'utilities',
    description: 'Run a standalone web search',
    source:
      'relaykit/dto/alpha_search_request.go; relay/alpha_search_handler.go',
    response: {},
    schemaNote:
      'The upstream search response is forwarded unchanged; this route does not define a universal response schema.',
    fields: ['model', 'id', 'stream'],
    required: ['model'],
    types: {
      model: 'string',
      id: 'string',
      stream: 'boolean',
    },
    contentType: 'application/json',
    payload: {
      model: 'YOUR_MODEL_ID',
    },
    note: 'Provider-specific JSON fields are forwarded with the original body. This route is non-streaming; the search payload and returned result structure require upstream channel support.',
  },
  {
    method: 'GET',
    path: '/v1/responses',
    group: 'realtime',
    description: 'Connect to Responses WebSocket',
    source:
      'controller/responses_websocket.go; relay/responses_websocket.go; middleware/auth.go',
    response: {
      type: 'response.completed',
      response: {
        id: 'RESPONSE_ID',
        status: 'completed',
      },
    },
    transport: 'websocket',
    query: {},
    note: 'Use a WebSocket client with Authorization: Bearer YOUR_API_KEY. Browser clients can pass responses or realtime plus openai-insecure-api-key.YOUR_API_KEY subprotocols. Responses accepts response.create with model/input at top level or inside response; one request may run at a time. response.cancel controls the active request only; upstream event support varies.',
  },
  {
    method: 'GET',
    path: '/v1/realtime',
    group: 'realtime',
    description: 'Connect to Realtime WebSocket',
    source:
      'relaykit/dto/realtime.go; relay/channel/openai/relay_realtime.go; middleware/auth.go',
    response: {
      type: 'session.created',
      session: {},
    },
    transport: 'websocket',
    query: {
      model: 'YOUR_MODEL_ID',
    },
    note: 'Use a WebSocket client with the model query parameter and Authorization: Bearer YOUR_API_KEY. Browser clients can pass realtime plus openai-insecure-api-key.YOUR_API_KEY subprotocols. Client events include session.update, conversation.item.create, input_audio_buffer.append and response.create; server events include session.created, response.audio.delta and response.done. Event support depends on the Realtime model.',
  },
  {
    method: 'POST',
    path: '/v1/tasks/{key}',
    group: 'tasks',
    description: 'Submit a plugin task',
    source:
      'router/task-router.go; middleware/task_plugin.go; controller/relay.go',
    response: {
      id: 'task_TASK_ID',
      task_id: 'task_TASK_ID',
      status: 'queued',
      model: 'YOUR_MODEL_ID',
      created_at: 0,
    },
    fields: ['model', 'prompt', 'action', 'metadata'],
    required: ['model'],
    types: {
      model: 'string',
      prompt: 'string',
      action: 'string',
      metadata: 'object',
    },
    contentType: 'application/json',
    payload: {
      model: 'YOUR_MODEL_ID',
      prompt: 'Hello',
    },
    note: 'On submission, {key} is the enabled plugin key. Request fields and submission response are defined by that plugin; this is a core-field example, not a universal schema.',
    deploymentDependent: true,
  },
  {
    method: 'GET',
    path: '/v1/tasks/{key}',
    group: 'tasks',
    description: 'Retrieve a task',
    source: 'controller/task.go',
    response: {
      task_id: 'task_TASK_ID',
      platform: 'PLUGIN_KEY',
      status: 'QUEUED',
      progress: '0%',
      fail_reason: '',
      created_at: 0,
      finished_at: 0,
    },
    note: 'On read routes, {key} is the public task ID, not the plugin key. Retrieval is restricted to tasks belonging to your account.',
  },
  {
    method: 'GET',
    path: '/v1/tasks/{key}/artifacts',
    group: 'tasks',
    description: 'List task artifacts',
    source: 'controller/task.go',
    response: {
      task_id: 'task_TASK_ID',
      artifacts: [
        {
          key: 'ARTIFACT_KEY',
          type: 'video',
          mime_type: 'video/mp4',
          content_url: 'ARTIFACT_CONTENT_URL',
        },
      ],
    },
    note: 'Artifacts are available only when the task and plugin provide them. Use the returned content_url; it may include a scoped access token.',
  },
  {
    method: 'GET',
    path: '/v1/tasks/{key}/artifacts/{artifact_key}/content',
    group: 'tasks',
    description: 'Retrieve artifact content',
    source: 'controller/task.go; middleware/auth.go',
    response: 'application/octet-stream',
    note: 'Content type follows the artifact. HEAD returns headers without a body. Authenticate with your API key or the scoped access token in the returned content_url; missing, unfinished or expired artifacts return an error.',
  },
  {
    method: 'HEAD',
    path: '/v1/tasks/{key}/artifacts/{artifact_key}/content',
    group: 'tasks',
    description: 'Inspect artifact content headers',
    source: 'controller/task.go; middleware/auth.go',
    response: 'application/octet-stream',
    note: 'Content type follows the artifact. HEAD returns headers without a body. Authenticate with your API key or the scoped access token in the returned content_url; missing, unfinished or expired artifacts return an error.',
  },
  {
    method: 'GET',
    path: '/dashboard/billing/subscription',
    group: 'billing',
    description: 'Read legacy billing limits',
    source: 'controller/billing.go',
    response: {
      object: 'billing_subscription',
      has_payment_method: true,
      soft_limit_usd: 0,
      hard_limit_usd: 0,
      system_hard_limit_usd: 0,
      access_until: 0,
    },
    note: 'Requires an active inference API key. Scope is token or account according to site settings. Legacy USD-named fields follow the site quota display type; they are not always USD. Errors may be returned with HTTP 200.',
  },
  {
    method: 'GET',
    path: '/v1/dashboard/billing/subscription',
    group: 'billing',
    description: 'Read legacy billing limits',
    source: 'controller/billing.go',
    response: {
      object: 'billing_subscription',
      has_payment_method: true,
      soft_limit_usd: 0,
      hard_limit_usd: 0,
      system_hard_limit_usd: 0,
      access_until: 0,
    },
    note: 'Requires an active inference API key. Scope is token or account according to site settings. Legacy USD-named fields follow the site quota display type; they are not always USD. Errors may be returned with HTTP 200.',
  },
  {
    method: 'GET',
    path: '/dashboard/billing/usage',
    group: 'billing',
    description: 'Read legacy billing usage',
    source: 'controller/billing.go',
    response: {
      object: 'list',
      total_usage: 0,
    },
    note: 'Requires an active inference API key. Scope is token or account according to site settings. total_usage is the site display amount multiplied by 100, not always USD cents; date filtering is not implemented.',
  },
  {
    method: 'GET',
    path: '/v1/dashboard/billing/usage',
    group: 'billing',
    description: 'Read legacy billing usage',
    source: 'controller/billing.go',
    response: {
      object: 'list',
      total_usage: 0,
    },
    note: 'Requires an active inference API key. Scope is token or account according to site settings. total_usage is the site display amount multiplied by 100, not always USD cents; date filtering is not implemented.',
  },
  {
    method: 'GET',
    path: '/api/usage/token/',
    group: 'billing',
    description: 'Read API key usage',
    source: 'controller/token.go; middleware/auth.go',
    response: {
      code: true,
      message: 'ok',
      data: {
        object: 'token_usage',
        name: 'KEY_NAME',
        total_granted: 0,
        total_used: 0,
        total_available: 0,
        unlimited_quota: false,
        model_limits: {},
        model_limits_enabled: false,
        expires_at: 0,
      },
    },
    note: 'Use the Bearer header. Values are raw quota units; expires_at is Unix seconds, or 0 without expiry. Read-only authentication allows expired or exhausted keys, but rejects disabled keys and banned accounts.',
  },

  {
    method: 'GET',
    path: '/{mode}/mj/image/{id}',
    group: 'midjourney',
    description: 'Retrieve a Midjourney image',
    source: 'dto/midjourney.go; relay/mjproxy_handler.go',
    response: 'image/*',
    note: 'Public image proxy registered before API-key authentication. The task must resolve to an available upstream image.',
  },
  {
    method: 'POST',
    path: '/{mode}/mj/insight-face/swap',
    group: 'midjourney',
    description: 'Swap faces in images',
    source: 'dto/midjourney.go; relay/mjproxy_handler.go',
    response: {
      code: 1,
      description: 'SUBMISSION_DESCRIPTION',
      properties: {},
      result: 'TASK_ID',
    },
    fields: ['sourceBase64', 'targetBase64'],
    required: ['sourceBase64', 'targetBase64'],
    types: {
      sourceBase64: 'string',
      targetBase64: 'string',
    },
    contentType: 'application/json',
    payload: {
      sourceBase64: 'BASE64_IMAGE',
      targetBase64: 'BASE64_IMAGE',
    },
    note: 'Requires a compatible Midjourney channel. Fields and operation support vary by upstream; the shared DTO is not a guarantee that every field works for every action. {mode}, when present, selects the upstream mode.',
  },
  {
    method: 'POST',
    path: '/{mode}/mj/submit/action',
    group: 'midjourney',
    description: 'Submit a Midjourney operation',
    source: 'dto/midjourney.go; relay/mjproxy_handler.go',
    response: {
      code: 1,
      description: 'SUBMISSION_DESCRIPTION',
      properties: {},
      result: 'TASK_ID',
    },
    fields: ['customId', 'taskId'],
    types: {
      customId: 'string',
      taskId: 'string',
    },
    contentType: 'application/json',
    payload: {
      taskId: 'TASK_ID',
    },
    note: 'Requires a compatible Midjourney channel. Fields and operation support vary by upstream; the shared DTO is not a guarantee that every field works for every action. {mode}, when present, selects the upstream mode.',
  },
  {
    method: 'POST',
    path: '/{mode}/mj/submit/blend',
    group: 'midjourney',
    description: 'Submit a Midjourney operation',
    source: 'dto/midjourney.go; relay/mjproxy_handler.go',
    response: {
      code: 1,
      description: 'SUBMISSION_DESCRIPTION',
      properties: {},
      result: 'TASK_ID',
    },
    fields: ['base64Array', 'botType'],
    types: {
      base64Array: 'array',
      botType: 'string',
    },
    contentType: 'application/json',
    payload: {
      base64Array: ['BASE64_IMAGE'],
    },
    note: 'Requires a compatible Midjourney channel. Fields and operation support vary by upstream; the shared DTO is not a guarantee that every field works for every action. {mode}, when present, selects the upstream mode.',
  },
  {
    method: 'POST',
    path: '/{mode}/mj/submit/change',
    group: 'midjourney',
    description: 'Submit a Midjourney operation',
    source: 'dto/midjourney.go; relay/mjproxy_handler.go',
    response: {
      code: 1,
      description: 'SUBMISSION_DESCRIPTION',
      properties: {},
      result: 'TASK_ID',
    },
    fields: ['taskId', 'action', 'index'],
    required: ['taskId', 'action', 'index'],
    types: {
      taskId: 'string',
      action: 'string',
      index: 'integer',
    },
    contentType: 'application/json',
    payload: {
      taskId: 'TASK_ID',
      action: 'UPSCALE',
      index: 1,
    },
    note: 'Requires a compatible Midjourney channel. Fields and operation support vary by upstream; the shared DTO is not a guarantee that every field works for every action. {mode}, when present, selects the upstream mode.',
  },
  {
    method: 'POST',
    path: '/{mode}/mj/submit/describe',
    group: 'midjourney',
    description: 'Submit a Midjourney operation',
    source: 'dto/midjourney.go; relay/mjproxy_handler.go',
    response: {
      code: 1,
      description: 'SUBMISSION_DESCRIPTION',
      properties: {},
      result: 'TASK_ID',
    },
    fields: ['base64Array', 'botType'],
    types: {
      base64Array: 'array',
      botType: 'string',
    },
    contentType: 'application/json',
    payload: {
      base64Array: ['BASE64_IMAGE'],
    },
    note: 'Requires a compatible Midjourney channel. Fields and operation support vary by upstream; the shared DTO is not a guarantee that every field works for every action. {mode}, when present, selects the upstream mode.',
  },
  {
    method: 'POST',
    path: '/{mode}/mj/submit/edits',
    group: 'midjourney',
    description: 'Submit a Midjourney operation',
    source: 'dto/midjourney.go; relay/mjproxy_handler.go',
    response: {
      code: 1,
      description: 'SUBMISSION_DESCRIPTION',
      properties: {},
      result: 'TASK_ID',
    },
    fields: ['prompt', 'base64Array', 'maskBase64'],
    types: {
      prompt: 'string',
      base64Array: 'array',
      maskBase64: 'string',
    },
    contentType: 'application/json',
    payload: {
      prompt: 'Hello',
    },
    note: 'Requires a compatible Midjourney channel. Fields and operation support vary by upstream; the shared DTO is not a guarantee that every field works for every action. {mode}, when present, selects the upstream mode.',
  },
  {
    method: 'POST',
    path: '/{mode}/mj/submit/imagine',
    group: 'midjourney',
    description: 'Submit a Midjourney operation',
    source: 'dto/midjourney.go; relay/mjproxy_handler.go',
    response: {
      code: 1,
      description: 'SUBMISSION_DESCRIPTION',
      properties: {},
      result: 'TASK_ID',
    },
    fields: ['prompt', 'botType', 'state', 'notifyHook'],
    required: ['prompt'],
    types: {
      prompt: 'string',
      botType: 'string',
      state: 'string',
      notifyHook: 'string',
    },
    contentType: 'application/json',
    payload: {
      prompt: 'A small red robot',
    },
    note: 'Requires a compatible Midjourney channel. Fields and operation support vary by upstream; the shared DTO is not a guarantee that every field works for every action. {mode}, when present, selects the upstream mode.',
  },
  {
    method: 'POST',
    path: '/{mode}/mj/submit/modal',
    group: 'midjourney',
    description: 'Submit a Midjourney operation',
    source: 'dto/midjourney.go; relay/mjproxy_handler.go',
    response: {
      code: 1,
      description: 'SUBMISSION_DESCRIPTION',
      properties: {},
      result: 'TASK_ID',
    },
    fields: ['taskId', 'prompt', 'maskBase64'],
    types: {
      taskId: 'string',
      prompt: 'string',
      maskBase64: 'string',
    },
    contentType: 'application/json',
    payload: {
      taskId: 'TASK_ID',
    },
    note: 'Requires a compatible Midjourney channel. Fields and operation support vary by upstream; the shared DTO is not a guarantee that every field works for every action. {mode}, when present, selects the upstream mode.',
  },
  {
    method: 'POST',
    path: '/{mode}/mj/submit/shorten',
    group: 'midjourney',
    description: 'Submit a Midjourney operation',
    source: 'dto/midjourney.go; relay/mjproxy_handler.go',
    response: {
      code: 1,
      description: 'SUBMISSION_DESCRIPTION',
      properties: {},
      result: 'TASK_ID',
    },
    fields: ['prompt', 'botType'],
    types: {
      prompt: 'string',
      botType: 'string',
    },
    contentType: 'application/json',
    payload: {
      prompt: 'Hello',
    },
    note: 'Requires a compatible Midjourney channel. Fields and operation support vary by upstream; the shared DTO is not a guarantee that every field works for every action. {mode}, when present, selects the upstream mode.',
  },
  {
    method: 'POST',
    path: '/{mode}/mj/submit/simple-change',
    group: 'midjourney',
    description: 'Submit a Midjourney operation',
    source: 'dto/midjourney.go; relay/mjproxy_handler.go',
    response: {
      code: 1,
      description: 'SUBMISSION_DESCRIPTION',
      properties: {},
      result: 'TASK_ID',
    },
    fields: ['content'],
    required: ['content'],
    types: {
      content: 'string',
    },
    contentType: 'application/json',
    payload: {
      content: 'CHANGE_INSTRUCTION',
    },
    note: 'Requires a compatible Midjourney channel. Fields and operation support vary by upstream; the shared DTO is not a guarantee that every field works for every action. {mode}, when present, selects the upstream mode.',
  },
  {
    method: 'POST',
    path: '/{mode}/mj/submit/upload-discord-images',
    group: 'midjourney',
    description: 'Submit a Midjourney operation',
    source: 'dto/midjourney.go; relay/mjproxy_handler.go',
    response: {
      code: 1,
      description: 'SUBMISSION_DESCRIPTION',
      result: ['IMAGE_URL'],
    },
    fields: ['base64Array'],
    types: {
      base64Array: 'array',
    },
    contentType: 'application/json',
    payload: {
      base64Array: ['BASE64_IMAGE'],
    },
    note: 'Requires a compatible Midjourney channel. Fields and operation support vary by upstream; the shared DTO is not a guarantee that every field works for every action. {mode}, when present, selects the upstream mode.',
  },
  {
    method: 'POST',
    path: '/{mode}/mj/submit/video',
    group: 'midjourney',
    description: 'Submit a Midjourney operation',
    source: 'dto/midjourney.go; relay/mjproxy_handler.go',
    response: {
      code: 1,
      description: 'SUBMISSION_DESCRIPTION',
      properties: {},
      result: 'TASK_ID',
    },
    fields: ['taskId', 'prompt'],
    types: {
      taskId: 'string',
      prompt: 'string',
    },
    contentType: 'application/json',
    payload: {
      taskId: 'TASK_ID',
    },
    note: 'Requires a compatible Midjourney channel. Fields and operation support vary by upstream; the shared DTO is not a guarantee that every field works for every action. {mode}, when present, selects the upstream mode.',
  },
  {
    method: 'GET',
    path: '/{mode}/mj/task/{id}/fetch',
    group: 'midjourney',
    description: 'Retrieve a Midjourney task',
    source: 'dto/midjourney.go; relay/mjproxy_handler.go',
    response: {
      id: 'TASK_ID',
      action: 'IMAGINE',
      status: 'SUCCESS',
      progress: '100%',
      imageUrl: 'IMAGE_URL',
      failReason: '',
    },
    note: 'Requires a compatible Midjourney channel. Fields and operation support vary by upstream; the shared DTO is not a guarantee that every field works for every action. {mode}, when present, selects the upstream mode.',
  },
  {
    method: 'GET',
    path: '/{mode}/mj/task/{id}/image-seed',
    group: 'midjourney',
    description: 'Retrieve a Midjourney image seed',
    source: 'dto/midjourney.go; relay/mjproxy_handler.go',
    response: {
      code: 1,
      description: 'SUBMISSION_DESCRIPTION',
      properties: {},
      result: 'TASK_ID',
    },
    note: 'Requires a compatible Midjourney channel. Fields and operation support vary by upstream; the shared DTO is not a guarantee that every field works for every action. {mode}, when present, selects the upstream mode.',
  },
  {
    method: 'POST',
    path: '/{mode}/mj/task/list-by-condition',
    group: 'midjourney',
    description: 'Retrieve Midjourney tasks by ID',
    source: 'dto/midjourney.go; relay/mjproxy_handler.go',
    response: [
      {
        id: 'TASK_ID',
        action: 'IMAGINE',
        status: 'SUCCESS',
        progress: '100%',
        imageUrl: 'IMAGE_URL',
        failReason: '',
      },
    ],
    fields: ['ids'],
    types: {
      ids: 'array',
    },
    contentType: 'application/json',
    payload: {
      ids: ['TASK_ID'],
    },
    note: 'Requires a compatible Midjourney channel. Fields and operation support vary by upstream; the shared DTO is not a guarantee that every field works for every action. {mode}, when present, selects the upstream mode.',
  },
  {
    method: 'GET',
    path: '/mj/image/{id}',
    group: 'midjourney',
    description: 'Retrieve a Midjourney image',
    source: 'dto/midjourney.go; relay/mjproxy_handler.go',
    response: 'image/*',
    note: 'Public image proxy registered before API-key authentication. The task must resolve to an available upstream image.',
  },
  {
    method: 'POST',
    path: '/mj/insight-face/swap',
    group: 'midjourney',
    description: 'Swap faces in images',
    source: 'dto/midjourney.go; relay/mjproxy_handler.go',
    response: {
      code: 1,
      description: 'SUBMISSION_DESCRIPTION',
      properties: {},
      result: 'TASK_ID',
    },
    fields: ['sourceBase64', 'targetBase64'],
    required: ['sourceBase64', 'targetBase64'],
    types: {
      sourceBase64: 'string',
      targetBase64: 'string',
    },
    contentType: 'application/json',
    payload: {
      sourceBase64: 'BASE64_IMAGE',
      targetBase64: 'BASE64_IMAGE',
    },
    note: 'Requires a compatible Midjourney channel. Fields and operation support vary by upstream; the shared DTO is not a guarantee that every field works for every action. {mode}, when present, selects the upstream mode.',
  },
  {
    method: 'POST',
    path: '/mj/submit/action',
    group: 'midjourney',
    description: 'Submit a Midjourney operation',
    source: 'dto/midjourney.go; relay/mjproxy_handler.go',
    response: {
      code: 1,
      description: 'SUBMISSION_DESCRIPTION',
      properties: {},
      result: 'TASK_ID',
    },
    fields: ['customId', 'taskId'],
    types: {
      customId: 'string',
      taskId: 'string',
    },
    contentType: 'application/json',
    payload: {
      taskId: 'TASK_ID',
    },
    note: 'Requires a compatible Midjourney channel. Fields and operation support vary by upstream; the shared DTO is not a guarantee that every field works for every action. {mode}, when present, selects the upstream mode.',
  },
  {
    method: 'POST',
    path: '/mj/submit/blend',
    group: 'midjourney',
    description: 'Submit a Midjourney operation',
    source: 'dto/midjourney.go; relay/mjproxy_handler.go',
    response: {
      code: 1,
      description: 'SUBMISSION_DESCRIPTION',
      properties: {},
      result: 'TASK_ID',
    },
    fields: ['base64Array', 'botType'],
    types: {
      base64Array: 'array',
      botType: 'string',
    },
    contentType: 'application/json',
    payload: {
      base64Array: ['BASE64_IMAGE'],
    },
    note: 'Requires a compatible Midjourney channel. Fields and operation support vary by upstream; the shared DTO is not a guarantee that every field works for every action. {mode}, when present, selects the upstream mode.',
  },
  {
    method: 'POST',
    path: '/mj/submit/change',
    group: 'midjourney',
    description: 'Submit a Midjourney operation',
    source: 'dto/midjourney.go; relay/mjproxy_handler.go',
    response: {
      code: 1,
      description: 'SUBMISSION_DESCRIPTION',
      properties: {},
      result: 'TASK_ID',
    },
    fields: ['taskId', 'action', 'index'],
    required: ['taskId', 'action', 'index'],
    types: {
      taskId: 'string',
      action: 'string',
      index: 'integer',
    },
    contentType: 'application/json',
    payload: {
      taskId: 'TASK_ID',
      action: 'UPSCALE',
      index: 1,
    },
    note: 'Requires a compatible Midjourney channel. Fields and operation support vary by upstream; the shared DTO is not a guarantee that every field works for every action. {mode}, when present, selects the upstream mode.',
  },
  {
    method: 'POST',
    path: '/mj/submit/describe',
    group: 'midjourney',
    description: 'Submit a Midjourney operation',
    source: 'dto/midjourney.go; relay/mjproxy_handler.go',
    response: {
      code: 1,
      description: 'SUBMISSION_DESCRIPTION',
      properties: {},
      result: 'TASK_ID',
    },
    fields: ['base64Array', 'botType'],
    types: {
      base64Array: 'array',
      botType: 'string',
    },
    contentType: 'application/json',
    payload: {
      base64Array: ['BASE64_IMAGE'],
    },
    note: 'Requires a compatible Midjourney channel. Fields and operation support vary by upstream; the shared DTO is not a guarantee that every field works for every action. {mode}, when present, selects the upstream mode.',
  },
  {
    method: 'POST',
    path: '/mj/submit/edits',
    group: 'midjourney',
    description: 'Submit a Midjourney operation',
    source: 'dto/midjourney.go; relay/mjproxy_handler.go',
    response: {
      code: 1,
      description: 'SUBMISSION_DESCRIPTION',
      properties: {},
      result: 'TASK_ID',
    },
    fields: ['prompt', 'base64Array', 'maskBase64'],
    types: {
      prompt: 'string',
      base64Array: 'array',
      maskBase64: 'string',
    },
    contentType: 'application/json',
    payload: {
      prompt: 'Hello',
    },
    note: 'Requires a compatible Midjourney channel. Fields and operation support vary by upstream; the shared DTO is not a guarantee that every field works for every action. {mode}, when present, selects the upstream mode.',
  },
  {
    method: 'POST',
    path: '/mj/submit/imagine',
    group: 'midjourney',
    description: 'Submit a Midjourney operation',
    source: 'dto/midjourney.go; relay/mjproxy_handler.go',
    response: {
      code: 1,
      description: 'SUBMISSION_DESCRIPTION',
      properties: {},
      result: 'TASK_ID',
    },
    fields: ['prompt', 'botType', 'state', 'notifyHook'],
    required: ['prompt'],
    types: {
      prompt: 'string',
      botType: 'string',
      state: 'string',
      notifyHook: 'string',
    },
    contentType: 'application/json',
    payload: {
      prompt: 'A small red robot',
    },
    note: 'Requires a compatible Midjourney channel. Fields and operation support vary by upstream; the shared DTO is not a guarantee that every field works for every action. {mode}, when present, selects the upstream mode.',
  },
  {
    method: 'POST',
    path: '/mj/submit/modal',
    group: 'midjourney',
    description: 'Submit a Midjourney operation',
    source: 'dto/midjourney.go; relay/mjproxy_handler.go',
    response: {
      code: 1,
      description: 'SUBMISSION_DESCRIPTION',
      properties: {},
      result: 'TASK_ID',
    },
    fields: ['taskId', 'prompt', 'maskBase64'],
    types: {
      taskId: 'string',
      prompt: 'string',
      maskBase64: 'string',
    },
    contentType: 'application/json',
    payload: {
      taskId: 'TASK_ID',
    },
    note: 'Requires a compatible Midjourney channel. Fields and operation support vary by upstream; the shared DTO is not a guarantee that every field works for every action. {mode}, when present, selects the upstream mode.',
  },
  {
    method: 'POST',
    path: '/mj/submit/shorten',
    group: 'midjourney',
    description: 'Submit a Midjourney operation',
    source: 'dto/midjourney.go; relay/mjproxy_handler.go',
    response: {
      code: 1,
      description: 'SUBMISSION_DESCRIPTION',
      properties: {},
      result: 'TASK_ID',
    },
    fields: ['prompt', 'botType'],
    types: {
      prompt: 'string',
      botType: 'string',
    },
    contentType: 'application/json',
    payload: {
      prompt: 'Hello',
    },
    note: 'Requires a compatible Midjourney channel. Fields and operation support vary by upstream; the shared DTO is not a guarantee that every field works for every action. {mode}, when present, selects the upstream mode.',
  },
  {
    method: 'POST',
    path: '/mj/submit/simple-change',
    group: 'midjourney',
    description: 'Submit a Midjourney operation',
    source: 'dto/midjourney.go; relay/mjproxy_handler.go',
    response: {
      code: 1,
      description: 'SUBMISSION_DESCRIPTION',
      properties: {},
      result: 'TASK_ID',
    },
    fields: ['content'],
    required: ['content'],
    types: {
      content: 'string',
    },
    contentType: 'application/json',
    payload: {
      content: 'CHANGE_INSTRUCTION',
    },
    note: 'Requires a compatible Midjourney channel. Fields and operation support vary by upstream; the shared DTO is not a guarantee that every field works for every action. {mode}, when present, selects the upstream mode.',
  },
  {
    method: 'POST',
    path: '/mj/submit/upload-discord-images',
    group: 'midjourney',
    description: 'Submit a Midjourney operation',
    source: 'dto/midjourney.go; relay/mjproxy_handler.go',
    response: {
      code: 1,
      description: 'SUBMISSION_DESCRIPTION',
      result: ['IMAGE_URL'],
    },
    fields: ['base64Array'],
    types: {
      base64Array: 'array',
    },
    contentType: 'application/json',
    payload: {
      base64Array: ['BASE64_IMAGE'],
    },
    note: 'Requires a compatible Midjourney channel. Fields and operation support vary by upstream; the shared DTO is not a guarantee that every field works for every action. {mode}, when present, selects the upstream mode.',
  },
  {
    method: 'POST',
    path: '/mj/submit/video',
    group: 'midjourney',
    description: 'Submit a Midjourney operation',
    source: 'dto/midjourney.go; relay/mjproxy_handler.go',
    response: {
      code: 1,
      description: 'SUBMISSION_DESCRIPTION',
      properties: {},
      result: 'TASK_ID',
    },
    fields: ['taskId', 'prompt'],
    types: {
      taskId: 'string',
      prompt: 'string',
    },
    contentType: 'application/json',
    payload: {
      taskId: 'TASK_ID',
    },
    note: 'Requires a compatible Midjourney channel. Fields and operation support vary by upstream; the shared DTO is not a guarantee that every field works for every action. {mode}, when present, selects the upstream mode.',
  },
  {
    method: 'GET',
    path: '/mj/task/{id}/fetch',
    group: 'midjourney',
    description: 'Retrieve a Midjourney task',
    source: 'dto/midjourney.go; relay/mjproxy_handler.go',
    response: {
      id: 'TASK_ID',
      action: 'IMAGINE',
      status: 'SUCCESS',
      progress: '100%',
      imageUrl: 'IMAGE_URL',
      failReason: '',
    },
    note: 'Requires a compatible Midjourney channel. Fields and operation support vary by upstream; the shared DTO is not a guarantee that every field works for every action. {mode}, when present, selects the upstream mode.',
  },
  {
    method: 'GET',
    path: '/mj/task/{id}/image-seed',
    group: 'midjourney',
    description: 'Retrieve a Midjourney image seed',
    source: 'dto/midjourney.go; relay/mjproxy_handler.go',
    response: {
      code: 1,
      description: 'SUBMISSION_DESCRIPTION',
      properties: {},
      result: 'TASK_ID',
    },
    note: 'Requires a compatible Midjourney channel. Fields and operation support vary by upstream; the shared DTO is not a guarantee that every field works for every action. {mode}, when present, selects the upstream mode.',
  },
  {
    method: 'POST',
    path: '/mj/task/list-by-condition',
    group: 'midjourney',
    description: 'Retrieve Midjourney tasks by ID',
    source: 'dto/midjourney.go; relay/mjproxy_handler.go',
    response: [
      {
        id: 'TASK_ID',
        action: 'IMAGINE',
        status: 'SUCCESS',
        progress: '100%',
        imageUrl: 'IMAGE_URL',
        failReason: '',
      },
    ],
    fields: ['ids'],
    types: {
      ids: 'array',
    },
    contentType: 'application/json',
    payload: {
      ids: ['TASK_ID'],
    },
    note: 'Requires a compatible Midjourney channel. Fields and operation support vary by upstream; the shared DTO is not a guarantee that every field works for every action. {mode}, when present, selects the upstream mode.',
  },
]

const videoCreate = definitions.find((entry) => entry.path === '/v1/videos')
const videoRetrieve = definitions.find(
  (entry) => entry.path === '/v1/videos/{task_id}'
)
const videoContent = definitions.find(
  (entry) => entry.path === '/v1/videos/{task_id}/content'
)
const transcription = definitions.find(
  (entry) => entry.path === '/v1/audio/transcriptions'
)
if (!videoCreate || !videoRetrieve || !videoContent || !transcription) {
  throw new Error('Missing primary public API contract')
}
additionalDefinitions.push(
  { ...videoCreate, path: '/v1/videos/generations' },
  { ...videoCreate, path: '/v1/video/generations' },
  { ...videoRetrieve, path: '/v1/video/generations/{task_id}' },
  {
    ...videoContent,
    method: 'HEAD',
    description: 'Inspect video content headers',
    response: 'video/mp4',
  },
  {
    ...videoCreate,
    path: '/v1/videos/{video_id}/remix',
    description: 'Remix a video',
    fields: ['model', 'prompt'],
    types: { model: 'string', prompt: 'string' },
    source: 'router/video-router.go; relay/channel/openai/adaptor.go',
    note: 'Requires a compatible video channel and a source video belonging to your account. Request fields depend on the selected provider.',
  },
  {
    ...transcription,
    path: '/v1/audio/translations',
    description: 'Translate audio',
    fields: ['model', 'file', 'prompt', 'response_format', 'temperature'],
    source: 'relay/audio_handler.go; relaykit/dto/audio.go',
    note: 'Translates audio to English when supported by the model. Response formats depend on the channel.',
  }
)

const nativeDefinitions: Definition[] = [
  {
    method: 'POST',
    path: '/ali/api/v1/services/aigc/multimodal-generation/generation',
    group: 'plugin-routes',
    description: 'Call a native plugin operation',
    source: 'plugins/tasks/alibaba/plugin.js',
    deploymentDependent: true,
    nativeRoute: true,
    note: 'Available only when this plugin is enabled and a compatible channel and model are configured. Native request and response fields follow this plugin and its provider; core fields shown here are not a complete provider schema.',
    response: {
      request_id: 'REQUEST_ID',
      output: {
        task_id: 'task_TASK_ID',
        task_status: 'PENDING',
      },
    },
    schemaNote:
      'Native image results may return the complete provider image payload instead of a pending task.',
    fields: ['model', 'input', 'parameters'],
    types: {
      model: 'string',
      input: 'object',
      parameters: 'object',
    },
    payload: {
      model: 'YOUR_MODEL_ID',
      input: {
        prompt: 'Hello',
      },
      parameters: {},
    },
    contentType: 'application/json',
    required: ['model'],
  },
  {
    method: 'POST',
    path: '/ali/api/v1/services/aigc/image-generation/generation',
    group: 'plugin-routes',
    description: 'Call a native plugin operation',
    source: 'plugins/tasks/alibaba/plugin.js',
    deploymentDependent: true,
    nativeRoute: true,
    note: 'Available only when this plugin is enabled and a compatible channel and model are configured. Native request and response fields follow this plugin and its provider; core fields shown here are not a complete provider schema.',
    response: {
      request_id: 'REQUEST_ID',
      output: {
        task_id: 'task_TASK_ID',
        task_status: 'PENDING',
      },
    },
    schemaNote:
      'Native image results may return the complete provider image payload instead of a pending task.',
    fields: ['model', 'input', 'parameters'],
    types: {
      model: 'string',
      input: 'object',
      parameters: 'object',
    },
    payload: {
      model: 'YOUR_MODEL_ID',
      input: {
        prompt: 'Hello',
      },
      parameters: {},
    },
    contentType: 'application/json',
    required: ['model'],
  },
  {
    method: 'POST',
    path: '/ali/api/v1/services/aigc/text2image/image-synthesis',
    group: 'plugin-routes',
    description: 'Call a native plugin operation',
    source: 'plugins/tasks/alibaba/plugin.js',
    deploymentDependent: true,
    nativeRoute: true,
    note: 'Available only when this plugin is enabled and a compatible channel and model are configured. Native request and response fields follow this plugin and its provider; core fields shown here are not a complete provider schema.',
    response: {
      request_id: 'REQUEST_ID',
      output: {
        task_id: 'task_TASK_ID',
        task_status: 'PENDING',
      },
    },
    schemaNote:
      'Native image results may return the complete provider image payload instead of a pending task.',
    fields: ['model', 'input', 'parameters'],
    types: {
      model: 'string',
      input: 'object',
      parameters: 'object',
    },
    payload: {
      model: 'YOUR_MODEL_ID',
      input: {
        prompt: 'Hello',
      },
      parameters: {},
    },
    contentType: 'application/json',
    required: ['model'],
  },
  {
    method: 'POST',
    path: '/ali/api/v1/services/aigc/image2image/image-synthesis',
    group: 'plugin-routes',
    description: 'Call a native plugin operation',
    source: 'plugins/tasks/alibaba/plugin.js',
    deploymentDependent: true,
    nativeRoute: true,
    note: 'Available only when this plugin is enabled and a compatible channel and model are configured. Native request and response fields follow this plugin and its provider; core fields shown here are not a complete provider schema.',
    response: {
      request_id: 'REQUEST_ID',
      output: {
        task_id: 'task_TASK_ID',
        task_status: 'PENDING',
      },
    },
    schemaNote:
      'Native image results may return the complete provider image payload instead of a pending task.',
    fields: ['model', 'input', 'parameters'],
    types: {
      model: 'string',
      input: 'object',
      parameters: 'object',
    },
    payload: {
      model: 'YOUR_MODEL_ID',
      input: {
        prompt: 'Hello',
      },
      parameters: {},
    },
    contentType: 'application/json',
    required: ['model'],
  },
  {
    method: 'POST',
    path: '/ali/api/v1/services/aigc/video-generation/video-synthesis',
    group: 'plugin-routes',
    description: 'Call a native plugin operation',
    source: 'plugins/tasks/alibaba/plugin.js',
    deploymentDependent: true,
    nativeRoute: true,
    note: 'Available only when this plugin is enabled and a compatible channel and model are configured. Native request and response fields follow this plugin and its provider; core fields shown here are not a complete provider schema.',
    response: {
      request_id: 'REQUEST_ID',
      output: {
        task_id: 'task_TASK_ID',
        task_status: 'PENDING',
      },
    },
    schemaNote:
      'Native image results may return the complete provider image payload instead of a pending task.',
    fields: ['model', 'input', 'parameters'],
    types: {
      model: 'string',
      input: 'object',
      parameters: 'object',
    },
    payload: {
      model: 'YOUR_MODEL_ID',
      input: {
        prompt: 'Hello',
      },
      parameters: {},
    },
    contentType: 'application/json',
    required: ['model'],
  },
  {
    method: 'POST',
    path: '/ali/api/v1/services/aigc/image2video/video-synthesis',
    group: 'plugin-routes',
    description: 'Call a native plugin operation',
    source: 'plugins/tasks/alibaba/plugin.js',
    deploymentDependent: true,
    nativeRoute: true,
    note: 'Available only when this plugin is enabled and a compatible channel and model are configured. Native request and response fields follow this plugin and its provider; core fields shown here are not a complete provider schema.',
    response: {
      request_id: 'REQUEST_ID',
      output: {
        task_id: 'task_TASK_ID',
        task_status: 'PENDING',
      },
    },
    schemaNote:
      'Native image results may return the complete provider image payload instead of a pending task.',
    fields: ['model', 'input', 'parameters'],
    types: {
      model: 'string',
      input: 'object',
      parameters: 'object',
    },
    payload: {
      model: 'YOUR_MODEL_ID',
      input: {
        prompt: 'Hello',
      },
      parameters: {},
    },
    contentType: 'application/json',
    required: ['model'],
  },
  {
    method: 'GET',
    path: '/ali/api/v1/tasks/{task_id}',
    group: 'plugin-routes',
    description: 'Read a native plugin task',
    source: 'plugins/tasks/alibaba/plugin.js',
    deploymentDependent: true,
    nativeRoute: true,
    note: 'Available only when this plugin is enabled and a compatible channel and model are configured. Native request and response fields follow this plugin and its provider; core fields shown here are not a complete provider schema.',
    response: {
      output: {
        task_id: 'task_TASK_ID',
        task_status: 'PENDING',
      },
    },
  },
  {
    method: 'POST',
    path: '/dflop-image/v1/images/generations',
    group: 'plugin-routes',
    description: 'Call a native plugin operation',
    source: 'plugins/tasks/dflop-image/plugin.js',
    deploymentDependent: true,
    nativeRoute: true,
    note: 'Available only when this plugin is enabled and a compatible channel and model are configured. Native request and response fields follow this plugin and its provider; core fields shown here are not a complete provider schema.',
    response: {
      id: 'task_TASK_ID',
      model: 'YOUR_MODEL_ID',
      status: 'queued',
    },
    schemaNote:
      'Completed native image results use the provider image payload; pending results use id, model and status.',
    fields: ['model', 'prompt', 'image', 'image_urls', 'n', 'size'],
    types: {
      model: 'string',
      prompt: 'string',
      image: 'string / array',
      image_urls: 'array',
      n: 'integer',
      size: 'string',
    },
    payload: {
      model: 'YOUR_MODEL_ID',
      prompt: 'Hello',
    },
    contentType: 'application/json',
    required: ['model'],
  },
  {
    method: 'POST',
    path: '/dflop-image/v1/images/edits',
    group: 'plugin-routes',
    description: 'Call a native plugin operation',
    source: 'plugins/tasks/dflop-image/plugin.js',
    deploymentDependent: true,
    nativeRoute: true,
    note: 'Available only when this plugin is enabled and a compatible channel and model are configured. Native request and response fields follow this plugin and its provider; core fields shown here are not a complete provider schema.',
    response: {
      id: 'task_TASK_ID',
      model: 'YOUR_MODEL_ID',
      status: 'queued',
    },
    schemaNote:
      'Completed native image results use the provider image payload; pending results use id, model and status.',
    fields: ['model', 'prompt', 'image', 'image_urls', 'n', 'size'],
    types: {
      model: 'string',
      prompt: 'string',
      image: 'string / array',
      image_urls: 'array',
      n: 'integer',
      size: 'string',
    },
    payload: {
      model: 'YOUR_MODEL_ID',
      prompt: 'Hello',
    },
    contentType: 'application/json',
    required: ['model'],
  },
  {
    method: 'GET',
    path: '/dflop-image/v1/images/generations/{task_id}',
    group: 'plugin-routes',
    description: 'Read a native plugin task',
    source: 'plugins/tasks/dflop-image/plugin.js',
    deploymentDependent: true,
    nativeRoute: true,
    note: 'Available only when this plugin is enabled and a compatible channel and model are configured. Native request and response fields follow this plugin and its provider; core fields shown here are not a complete provider schema.',
    response: {
      id: 'task_TASK_ID',
      model: 'YOUR_MODEL_ID',
      status: 'running',
    },
  },
  {
    method: 'POST',
    path: '/dflop/v1/videos/generations',
    group: 'plugin-routes',
    description: 'Call a native plugin operation',
    source: 'plugins/tasks/dflop-media/plugin.js',
    deploymentDependent: true,
    nativeRoute: true,
    note: 'Available only when this plugin is enabled and a compatible channel and model are configured. Native request and response fields follow this plugin and its provider; core fields shown here are not a complete provider schema.',
    response: {},
    schemaNote:
      'The native presenter returns the persisted provider result. Required inputs differ for video, music, voice cloning and avatar models; no universal response schema is defined.',
    fields: ['model', 'prompt', 'content', 'duration'],
    types: {
      model: 'string',
      prompt: 'string',
      content: 'array',
      duration: 'number',
    },
    payload: {
      model: 'YOUR_MODEL_ID',
      prompt: 'Hello',
    },
    contentType: 'application/json',
    required: ['model'],
  },
  {
    method: 'GET',
    path: '/dflop/v1/videos/generations/{task_id}',
    group: 'plugin-routes',
    description: 'Read a native plugin task',
    source: 'plugins/tasks/dflop-media/plugin.js',
    deploymentDependent: true,
    nativeRoute: true,
    note: 'Available only when this plugin is enabled and a compatible channel and model are configured. Native request and response fields follow this plugin and its provider; core fields shown here are not a complete provider schema.',
    response: {},
    schemaNote:
      'The native presenter returns the persisted provider result; no universal response schema is defined.',
  },
  {
    method: 'POST',
    path: '/dflop/v1/music/generations',
    group: 'plugin-routes',
    description: 'Call a native plugin operation',
    source: 'plugins/tasks/dflop-media/plugin.js',
    deploymentDependent: true,
    nativeRoute: true,
    note: 'Available only when this plugin is enabled and a compatible channel and model are configured. Native request and response fields follow this plugin and its provider; core fields shown here are not a complete provider schema.',
    response: {},
    schemaNote:
      'The native presenter returns the persisted provider result. Required inputs differ for video, music, voice cloning and avatar models; no universal response schema is defined.',
    fields: ['model', 'prompt', 'content', 'duration'],
    types: {
      model: 'string',
      prompt: 'string',
      content: 'array',
      duration: 'number',
    },
    payload: {
      model: 'YOUR_MODEL_ID',
      prompt: 'Hello',
    },
    contentType: 'application/json',
    required: ['model'],
  },
  {
    method: 'GET',
    path: '/dflop/v1/music/generations/{task_id}',
    group: 'plugin-routes',
    description: 'Read a native plugin task',
    source: 'plugins/tasks/dflop-media/plugin.js',
    deploymentDependent: true,
    nativeRoute: true,
    note: 'Available only when this plugin is enabled and a compatible channel and model are configured. Native request and response fields follow this plugin and its provider; core fields shown here are not a complete provider schema.',
    response: {},
    schemaNote:
      'The native presenter returns the persisted provider result; no universal response schema is defined.',
  },
  {
    method: 'POST',
    path: '/dflop/v1/audio/voices',
    group: 'plugin-routes',
    description: 'Call a native plugin operation',
    source: 'plugins/tasks/dflop-media/plugin.js',
    deploymentDependent: true,
    nativeRoute: true,
    note: 'Available only when this plugin is enabled and a compatible channel and model are configured. Native request and response fields follow this plugin and its provider; core fields shown here are not a complete provider schema.',
    response: {},
    schemaNote:
      'The native presenter returns the persisted provider result. Required inputs differ for video, music, voice cloning and avatar models; no universal response schema is defined.',
    fields: ['model', 'prompt', 'content', 'duration'],
    types: {
      model: 'string',
      prompt: 'string',
      content: 'array',
      duration: 'number',
    },
    payload: {
      model: 'YOUR_MODEL_ID',
      prompt: 'Hello',
    },
    contentType: 'application/json',
    required: ['model'],
  },
  {
    method: 'GET',
    path: '/dflop/v1/audio/voices/{task_id}',
    group: 'plugin-routes',
    description: 'Read a native plugin task',
    source: 'plugins/tasks/dflop-media/plugin.js',
    deploymentDependent: true,
    nativeRoute: true,
    note: 'Available only when this plugin is enabled and a compatible channel and model are configured. Native request and response fields follow this plugin and its provider; core fields shown here are not a complete provider schema.',
    response: {},
    schemaNote:
      'The native presenter returns the persisted provider result; no universal response schema is defined.',
  },
  {
    method: 'POST',
    path: '/dflop/v1/videos/avatars',
    group: 'plugin-routes',
    description: 'Call a native plugin operation',
    source: 'plugins/tasks/dflop-media/plugin.js',
    deploymentDependent: true,
    nativeRoute: true,
    note: 'Available only when this plugin is enabled and a compatible channel and model are configured. Native request and response fields follow this plugin and its provider; core fields shown here are not a complete provider schema.',
    response: {},
    schemaNote:
      'The native presenter returns the persisted provider result. Required inputs differ for video, music, voice cloning and avatar models; no universal response schema is defined.',
    fields: ['model', 'prompt', 'content', 'duration'],
    types: {
      model: 'string',
      prompt: 'string',
      content: 'array',
      duration: 'number',
    },
    payload: {
      model: 'YOUR_MODEL_ID',
      prompt: 'Hello',
    },
    contentType: 'application/json',
    required: ['model'],
  },
  {
    method: 'GET',
    path: '/dflop/v1/videos/avatars/{task_id}',
    group: 'plugin-routes',
    description: 'Read a native plugin task',
    source: 'plugins/tasks/dflop-media/plugin.js',
    deploymentDependent: true,
    nativeRoute: true,
    note: 'Available only when this plugin is enabled and a compatible channel and model are configured. Native request and response fields follow this plugin and its provider; core fields shown here are not a complete provider schema.',
    response: {},
    schemaNote:
      'The native presenter returns the persisted provider result; no universal response schema is defined.',
  },
  {
    method: 'POST',
    path: '/doubao/api/v3/contents/generations/tasks',
    group: 'plugin-routes',
    description: 'Call a native plugin operation',
    source: 'plugins/tasks/doubao/plugin.js',
    deploymentDependent: true,
    nativeRoute: true,
    note: 'Available only when this plugin is enabled and a compatible channel and model are configured. Native request and response fields follow this plugin and its provider; core fields shown here are not a complete provider schema.',
    response: {
      id: 'task_TASK_ID',
    },
    fields: ['model', 'content', 'duration'],
    types: {
      model: 'string',
      content: 'array',
      duration: 'integer',
    },
    payload: {
      model: 'YOUR_MODEL_ID',
      content: [
        {
          type: 'text',
          text: 'Hello',
        },
      ],
    },
    contentType: 'application/json',
    required: ['model'],
  },
  {
    method: 'GET',
    path: '/doubao/api/v3/contents/generations/tasks/{task_id}',
    group: 'plugin-routes',
    description: 'Read a native plugin task',
    source: 'plugins/tasks/doubao/plugin.js',
    deploymentDependent: true,
    nativeRoute: true,
    note: 'Available only when this plugin is enabled and a compatible channel and model are configured. Native request and response fields follow this plugin and its provider; core fields shown here are not a complete provider schema.',
    response: {
      id: 'task_TASK_ID',
      status: 'queued',
    },
  },
  {
    method: 'POST',
    path: '/doubao/api/v3/images/generations',
    group: 'plugin-routes',
    description: 'Call a native plugin operation',
    source: 'plugins/tasks/doubao/plugin.js',
    deploymentDependent: true,
    nativeRoute: true,
    note: 'Available only when this plugin is enabled and a compatible channel and model are configured. Native request and response fields follow this plugin and its provider; core fields shown here are not a complete provider schema.',
    response: {
      data: [
        {
          url: 'IMAGE_URL',
        },
      ],
    },
    schemaNote:
      'Native image generation returns the provider JSON once complete; stream must be omitted or false.',
    fields: ['model', 'prompt', 'image', 'size', 'stream'],
    types: {
      model: 'string',
      prompt: 'string',
      image: 'string / array',
      size: 'string',
      stream: 'boolean',
    },
    payload: {
      model: 'YOUR_MODEL_ID',
      prompt: 'Hello',
    },
    contentType: 'application/json',
    required: ['model'],
  },
  {
    method: 'POST',
    path: '/jimeng/',
    group: 'plugin-routes',
    description: 'Call a native plugin operation',
    source: 'plugins/tasks/jimeng/plugin.js',
    deploymentDependent: true,
    nativeRoute: true,
    note: 'Available only when this plugin is enabled and a compatible channel and model are configured. Native request and response fields follow this plugin and its provider; core fields shown here are not a complete provider schema.',
    response: {},
    query: {
      Action: 'CVSync2AsyncSubmitTask',
    },
    schemaNote:
      'Action=CVSync2AsyncSubmitTask creates a task with req_key; Action=CVSync2AsyncGetResult reads the public task_id. Response uses the plugin native presenter.',
    fields: ['req_key', 'prompt', 'task_id'],
    types: {
      req_key: 'string',
      prompt: 'string',
      task_id: 'string',
    },
    payload: {
      req_key: 'YOUR_MODEL_ID',
      prompt: 'Hello',
    },
    contentType: 'application/json',
  },
  {
    method: 'POST',
    path: '/kling/v1/videos/text2video',
    group: 'plugin-routes',
    description: 'Call a native plugin operation',
    source: 'plugins/tasks/kling/plugin.js',
    deploymentDependent: true,
    nativeRoute: true,
    note: 'Available only when this plugin is enabled and a compatible channel and model are configured. Native request and response fields follow this plugin and its provider; core fields shown here are not a complete provider schema.',
    response: {
      code: 0,
      data: {
        task_id: 'task_TASK_ID',
        task_status: 'submitted',
      },
    },
    conditional: ['model_name', 'model'],
    fields: ['model_name', 'model', 'prompt', 'image', 'duration'],
    types: {
      model_name: 'string',
      model: 'string',
      prompt: 'string',
      image: 'string',
      duration: 'string / integer',
    },
    payload: {
      model_name: 'YOUR_MODEL_ID',
      prompt: 'Hello',
    },
    contentType: 'application/json',
  },
  {
    method: 'POST',
    path: '/kling/v1/videos/image2video',
    group: 'plugin-routes',
    description: 'Call a native plugin operation',
    source: 'plugins/tasks/kling/plugin.js',
    deploymentDependent: true,
    nativeRoute: true,
    note: 'Available only when this plugin is enabled and a compatible channel and model are configured. Native request and response fields follow this plugin and its provider; core fields shown here are not a complete provider schema.',
    response: {
      code: 0,
      data: {
        task_id: 'task_TASK_ID',
        task_status: 'submitted',
      },
    },
    conditional: ['model_name', 'model'],
    fields: ['model_name', 'model', 'prompt', 'image', 'duration'],
    types: {
      model_name: 'string',
      model: 'string',
      prompt: 'string',
      image: 'string',
      duration: 'string / integer',
    },
    payload: {
      model_name: 'YOUR_MODEL_ID',
      prompt: 'Hello',
    },
    contentType: 'application/json',
  },
  {
    method: 'GET',
    path: '/kling/v1/videos/text2video/{task_id}',
    group: 'plugin-routes',
    description: 'Read a native plugin task',
    source: 'plugins/tasks/kling/plugin.js',
    deploymentDependent: true,
    nativeRoute: true,
    note: 'Available only when this plugin is enabled and a compatible channel and model are configured. Native request and response fields follow this plugin and its provider; core fields shown here are not a complete provider schema.',
    response: {
      code: 0,
      data: {
        task_id: 'task_TASK_ID',
        task_status: 'submitted',
      },
    },
  },
  {
    method: 'GET',
    path: '/kling/v1/videos/image2video/{task_id}',
    group: 'plugin-routes',
    description: 'Read a native plugin task',
    source: 'plugins/tasks/kling/plugin.js',
    deploymentDependent: true,
    nativeRoute: true,
    note: 'Available only when this plugin is enabled and a compatible channel and model are configured. Native request and response fields follow this plugin and its provider; core fields shown here are not a complete provider schema.',
    response: {
      code: 0,
      data: {
        task_id: 'task_TASK_ID',
        task_status: 'submitted',
      },
    },
  },
  {
    method: 'POST',
    path: '/suno/submit/{action}',
    group: 'plugin-routes',
    description: 'Call a native plugin operation',
    source: 'plugins/tasks/sunoapi/plugin.js',
    deploymentDependent: true,
    nativeRoute: true,
    note: 'Available only when this plugin is enabled and a compatible channel and model are configured. Native request and response fields follow this plugin and its provider; core fields shown here are not a complete provider schema. Replace {action} with MUSIC or LYRICS.',
    response: {
      code: 'success',
      message: '',
      data: 'task_TASK_ID',
    },
    fields: ['prompt', 'make_instrumental'],
    types: {
      prompt: 'string',
      make_instrumental: 'boolean',
    },
    payload: {
      prompt: 'Hello',
    },
    contentType: 'application/json',
  },
  {
    method: 'POST',
    path: '/suno/fetch',
    group: 'plugin-routes',
    description: 'Call a native plugin operation',
    source: 'plugins/tasks/sunoapi/plugin.js',
    deploymentDependent: true,
    nativeRoute: true,
    note: 'Available only when this plugin is enabled and a compatible channel and model are configured. Native request and response fields follow this plugin and its provider; core fields shown here are not a complete provider schema.',
    response: {
      code: 'success',
      message: '',
      data: [],
    },
    fields: ['ids'],
    types: {
      ids: 'array',
    },
    payload: {
      ids: ['task_TASK_ID'],
    },
    contentType: 'application/json',
  },
  {
    method: 'GET',
    path: '/suno/fetch/{task_id}',
    group: 'plugin-routes',
    description: 'Read a native plugin task',
    source: 'plugins/tasks/sunoapi/plugin.js',
    deploymentDependent: true,
    nativeRoute: true,
    note: 'Available only when this plugin is enabled and a compatible channel and model are configured. Native request and response fields follow this plugin and its provider; core fields shown here are not a complete provider schema.',
    response: {
      code: 'success',
      message: '',
      data: {
        task_id: 'task_TASK_ID',
        status: 'QUEUED',
        progress: '0%',
        data: null,
      },
    },
  },
]

for (const definition of nativeDefinitions) {
  if (definition.path.startsWith('/ali/')) {
    definition.responseTypes = { output: 'object' }
  }
  if (definition.method !== 'POST') continue
  if (definition.path.startsWith('/dflop-image/')) {
    definition.required = ['model', 'prompt']
    if (definition.path.endsWith('/edits')) {
      definition.conditional = ['image', 'image_urls']
      definition.payload = { ...definition.payload, image: 'IMAGE_URL' }
    }
  }
  if (definition.path === '/doubao/api/v3/contents/generations/tasks') {
    definition.required = ['model', 'content']
  }
}

const nestedResponseFields: Record<string, Field[]> = {
  '/v1/models': [
    { name: 'data[].id', type: 'string', required: false },
    { name: 'data[].supported_endpoint_types', type: 'array', required: false },
  ],
  '/v1beta/openai/models': [
    { name: 'data[].id', type: 'string', required: false },
  ],
  '/v1beta/models': [
    { name: 'models[].name', type: 'string', required: false },
    { name: 'models[].displayName', type: 'string', required: false },
  ],
  '/v1/chat/completions': [
    { name: 'choices[].index', type: 'integer', required: false },
    { name: 'choices[].message', type: 'object', required: false },
    { name: 'choices[].finish_reason', type: 'string / null', required: false },
    { name: 'usage', type: 'object', required: false },
  ],
  '/v1/completions': [
    { name: 'choices[].index', type: 'integer', required: false },
    { name: 'choices[].text', type: 'string', required: false },
    { name: 'choices[].finish_reason', type: 'string / null', required: false },
    { name: 'usage', type: 'object', required: false },
  ],
  '/v1/responses': [
    { name: 'output[].type', type: 'string', required: false },
    { name: 'output[].content', type: 'array', required: false },
    { name: 'usage', type: 'object', required: false },
  ],
  '/v1/responses/{response_id}': [
    { name: 'output[].type', type: 'string', required: false },
    { name: 'output[].content', type: 'array', required: false },
    { name: 'usage', type: 'object', required: false },
  ],
  '/v1/responses/compact': [
    { name: 'output[].type', type: 'string', required: false },
    { name: 'usage', type: 'object', required: false },
  ],
  '/v1/messages': [
    { name: 'content[].type', type: 'string', required: false },
    { name: 'usage.input_tokens', type: 'integer', required: false },
    { name: 'usage.output_tokens', type: 'integer', required: false },
  ],
  '/v1/engines/{model}/embeddings': [
    { name: 'embedding.values', type: 'number[]', required: false },
  ],
  '/v1/images/generations': [
    { name: 'data[].url', type: 'string', required: false },
    { name: 'data[].b64_json', type: 'string', required: false },
    { name: 'data[].revised_prompt', type: 'string', required: false },
  ],
  '/v1/images/edits': [
    { name: 'data[].url', type: 'string', required: false },
    { name: 'data[].b64_json', type: 'string', required: false },
    { name: 'data[].revised_prompt', type: 'string', required: false },
  ],
}

const responseFieldTypes: Record<string, string> = {
  id: 'string',
  object: 'string',
  created: 'integer',
  created_at: 'integer',
  completed_at: 'integer',
  finished_at: 'integer',
  model: 'string',
  status: 'string',
  progress: 'integer / string',
  output: 'array',
  content: 'array',
  stop_reason: 'string / null',
  usage: 'object',
  data: 'array / object / string',
  choices: 'array',
  success: 'boolean',
  models: 'array',
  nextPageToken: 'string / null',
  candidates: 'array',
  embedding: 'object',
  embeddings: 'array',
  results: 'array',
  text: 'string',
  task_id: 'string',
  platform: 'string',
  fail_reason: 'string',
  artifacts: 'array',
  code: 'integer / string / boolean',
  message: 'string',
  description: 'string',
  properties: 'object',
  result: 'string / array',
  action: 'string',
  imageUrl: 'string',
  failReason: 'string',
  request_id: 'string',
  type: 'string',
  response: 'object',
  session: 'object',
  has_payment_method: 'boolean',
  soft_limit_usd: 'number',
  hard_limit_usd: 'number',
  system_hard_limit_usd: 'number',
  access_until: 'integer',
  total_usage: 'number',
}

export const publicApiEndpoints = [
  ...definitions,
  ...additionalDefinitions,
  ...nativeDefinitions,
].map((definition) => {
  const route = reference.endpoints.find(
    (endpoint) =>
      endpoint.method === definition.method && endpoint.path === definition.path
  )
  if (!route && !definition.nativeRoute) {
    throw new Error(
      `Unregistered customer API: ${definition.method} ${definition.path}`
    )
  }
  const body = route?.requestBody as {
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
  const query = new URLSearchParams(definition.query).toString()
  const command = [
    `curl '${url}${query ? `?${query}` : ''}'`,
    "  -H 'Authorization: Bearer YOUR_API_KEY'",
  ]
  if (definition.method === 'HEAD') command.push('  --head')
  if (definition.method === 'POST' && !definition.payload && !definition.form) {
    command.push('  -X POST')
  }
  for (const [name, value] of Object.entries(definition.headers ?? {})) {
    command.push(`  -H '${name}: ${value}'`)
  }
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
  if (typeof definition.response === 'string' && definition.method !== 'HEAD') {
    let outputFile = 'artifact.bin'
    if (definition.group === 'video') outputFile = 'video.mp4'
    if (definition.group === 'audio') outputFile = 'speech.mp3'
    command.push(`  --output ${outputFile}`)
  }
  const example =
    definition.transport === 'websocket'
      ? `wss://hotx-api.com${definition.path}${query ? `?${query}` : ''}\nAuthorization: Bearer YOUR_API_KEY\n\n${JSON.stringify(definition.path === '/v1/responses' ? { type: 'response.create', model: 'YOUR_MODEL_ID', input: 'Hello' } : { type: 'response.create' }, null, 2)}`
      : command.join(' \\\n')
  const nestedFields =
    definition.transport === 'websocket'
      ? []
      : (nestedResponseFields[definition.path] ?? [])
  return {
    ...definition,
    responseFields: definition.responseFields ?? [
      ...Object.keys(
        Array.isArray(definition.response) ||
          typeof definition.response === 'string'
          ? {}
          : definition.response
      ).map((name) => ({
        name,
        type:
          definition.responseTypes?.[name] ??
          responseFieldTypes[name] ??
          'JSON',
        required: false,
      })),
      ...(typeof definition.response === 'string' ? [] : nestedFields),
    ],
    schemaNote:
      definition.schemaNote ??
      'Core response fields only; optional fields and nested data vary by model and provider. The example represents one successful response.',
    fields,
    example,
    routeSource: route?.source ?? definition.source,
    schemaSource: route?.schemaSource ?? definition.source,
  }
})
