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
import type { PricingModel } from '@/features/pricing/types'

import { translateDocSection, type DocArticle } from './content'
import type reference from './generated/reference.json'

export type SearchResult = { title: string; detail: string; href: string }
export type DocReference = typeof reference

export function filterCatalog(
  models: PricingModel[],
  query: string,
  capability: string,
  vendor: string
): PricingModel[] {
  const search = query.trim().toLowerCase()
  return models.filter((model) => {
    if (vendor && model.vendor_name !== vendor) return false
    if (
      capability &&
      !(model.capabilities ?? []).includes(
        capability as NonNullable<PricingModel['capabilities']>[number]
      ) &&
      !(model.input_modalities ?? []).includes(
        capability as NonNullable<PricingModel['input_modalities']>[number]
      ) &&
      !(model.output_modalities ?? []).includes(
        capability as NonNullable<PricingModel['output_modalities']>[number]
      ) &&
      !(model.supported_endpoint_types ?? []).includes(capability)
    ) {
      return false
    }
    return (
      !search ||
      [
        model.model_name,
        model.vendor_name,
        model.description,
        ...(model.supported_endpoint_types ?? []),
        ...(model.capabilities ?? []),
      ]
        .join(' ')
        .toLowerCase()
        .includes(search)
    )
  })
}

export function searchDocumentation(
  query: string,
  pages: DocArticle[],
  data: DocReference,
  models: PricingModel[],
  translate: (key: string, values?: Record<string, string | number>) => string,
  includeReference = false
): SearchResult[] {
  const terms = query
    .trim()
    .toLowerCase()
    .replaceAll('*', '')
    .split(/\s+/)
    .filter(Boolean)
  if (!terms.length) return []
  const matches = (value: string): boolean =>
    terms.every((term) => value.toLowerCase().includes(term))
  const results: SearchResult[] = []
  for (const article of pages) {
    if (
      matches(
        [
          translate(article.title),
          translate(article.description),
          ...(article.keywords ?? []).map((key) => translate(key)),
          ...article.sections.flatMap((section) => [
            translate(section.title),
            translateDocSection(section, translate),
          ]),
        ].join(' ')
      )
    ) {
      const section = article.sections.find((item) =>
        matches(
          `${translate(item.title)} ${translateDocSection(item, translate)}`
        )
      )
      results.push({
        title: translate(article.title),
        detail: section ? translate(section.title) : translate(article.group),
        href: `${article.slug === 'overview' ? '/docs' : `/docs/${article.slug}`}${section ? `#${section.id}` : ''}`,
      })
    }
  }
  for (const endpoint of includeReference ? data.endpoints : []) {
    if (
      matches(
        `${endpoint.method} ${endpoint.path} ${endpoint.summary} ${endpoint.handler}`
      )
    ) {
      results.push({
        title: `${endpoint.method} ${endpoint.path}`,
        detail: translate('API Reference'),
        href: `/docs/api-reference#${endpointAnchor(endpoint.method, endpoint.path)}`,
      })
    }
  }
  for (const error of includeReference ? data.errors : []) {
    if (matches(`${error.name} ${error.code}`)) {
      results.push({
        title: error.code,
        detail: translate('Error Codes'),
        href: `/docs/error-codes#${error.code.replaceAll(/[^a-z0-9-]/gi, '-')}`,
      })
    }
  }
  for (const model of models) {
    if (
      matches(
        `${model.model_name} ${model.vendor_name ?? ''} ${model.description ?? ''}`
      )
    ) {
      results.push({
        title: model.model_name,
        detail: model.vendor_name ?? translate('Model Catalog'),
        href: `/docs/models/${encodeURIComponent(model.model_name)}`,
      })
    }
  }
  return results.slice(0, 80)
}

export function endpointAnchor(method: string, path: string): string {
  return `${method}-${path}`.toLowerCase().replaceAll(/[^a-z0-9-]/g, '-')
}

export function buildQuickstart(
  baseUrl: string,
  protocol: string,
  stream: boolean
): { curl: string; python: string; typescript: string } {
  const origin = baseUrl.replace(/\/$/, '')
  const selectedProtocol =
    protocol === 'anthropic' || protocol === 'gemini' ? protocol : 'openai'
  const discoveryCurl = `set -euo pipefail\nexport NEW_API_BASE_URL='${origin.replaceAll("'", "'\\''")}'\n: "\${NEW_API_KEY:?Set NEW_API_KEY securely}"\nmodels=$(curl --fail-with-body "$NEW_API_BASE_URL/v1/models" -H "Authorization: Bearer $NEW_API_KEY")\nNEW_API_MODEL=$(printf '%s' "$models" | jq -er --arg requested "\${NEW_API_MODEL:-}" '[.data[] | select((.supported_endpoint_types // []) | index("${selectedProtocol}")) | select($requested == "" or .id == $requested)][0].id // error("No accessible compatible model")')`
  const discoveryPython = `import json\nimport os\nfrom urllib.request import Request, urlopen\n\norigin = ${JSON.stringify(origin)}\nrequest = Request(origin + "/v1/models", headers={"Authorization": "Bearer " + os.environ["NEW_API_KEY"]})\nwith urlopen(request) as response:\n    available = json.load(response)["data"]\nrequested = os.environ.get("NEW_API_MODEL")\nmodel = next((item["id"] for item in available if "${selectedProtocol}" in item.get("supported_endpoint_types", []) and (not requested or item["id"] == requested)), None)\nif model is None:\n    raise ValueError("No accessible compatible model")`
  const discoveryTypescript = `const origin = ${JSON.stringify(origin)}\nconst listing = await fetch(origin + '/v1/models', { headers: { Authorization: \`Bearer \${process.env.NEW_API_KEY}\` } })\nif (!listing.ok) throw new Error(\`Model listing failed: \${listing.status}\`)\nconst available = await listing.json() as { data: { id: string; supported_endpoint_types?: string[] }[] }\nconst model = available.data.find(item => item.supported_endpoint_types?.includes('${selectedProtocol}') && (!process.env.NEW_API_MODEL || item.id === process.env.NEW_API_MODEL))?.id\nif (!model) throw new Error('No accessible compatible model')`
  if (selectedProtocol === 'anthropic') {
    return {
      curl: `${discoveryCurl}\njq -n --arg model "$NEW_API_MODEL" '{model: $model, max_tokens: 256, messages: [{role: "user", content: "Hello"}]${stream ? ', stream: true' : ''}}' |\n  curl --fail-with-body ${stream ? '-N ' : ''}"$NEW_API_BASE_URL/v1/messages" \\\n    -H "x-api-key: $NEW_API_KEY" -H "anthropic-version: 2023-06-01" \\\n    -H "Content-Type: application/json" --data-binary @-`,
      python: `from anthropic import Anthropic\n${discoveryPython}\n\nclient = Anthropic(api_key=os.environ["NEW_API_KEY"], base_url=origin)\n${stream ? 'with client.messages.stream(model=model, max_tokens=256, messages=[{"role": "user", "content": "Hello"}]) as response:\n    for text in response.text_stream:\n        print(text, end="", flush=True)' : 'response = client.messages.create(model=model, max_tokens=256, messages=[{"role": "user", "content": "Hello"}])\nprint(response.content)'}`,
      typescript: `import Anthropic from '@anthropic-ai/sdk'\n\n${discoveryTypescript}\nconst client = new Anthropic({ apiKey: process.env.NEW_API_KEY, baseURL: origin })\nconst response = ${stream ? 'client.messages.stream' : 'await client.messages.create'}({ model, max_tokens: 256, messages: [{ role: 'user', content: 'Hello' }] })\n${stream ? "response.on('text', text => process.stdout.write(text))\nawait response.finalMessage()" : 'console.log(response.content)'}`,
    }
  }
  if (selectedProtocol === 'gemini') {
    return {
      curl: `${discoveryCurl}\nencoded_model=$(printf '%s' "$NEW_API_MODEL" | jq -sRr @uri)\ncurl --fail-with-body ${stream ? '-N ' : ''}"$NEW_API_BASE_URL/v1beta/models/$encoded_model:${stream ? 'streamGenerateContent?alt=sse' : 'generateContent'}" \\\n  -H "x-goog-api-key: $NEW_API_KEY" -H "Content-Type: application/json" \\\n  -d '{"contents":[{"parts":[{"text":"Hello"}]}]}'`,
      python: `from google import genai\nfrom google.genai import types\n${discoveryPython}\n\nclient = genai.Client(api_key=os.environ["NEW_API_KEY"], http_options=types.HttpOptions(base_url=origin, api_version="v1beta"))\n${stream ? 'for chunk in client.models.generate_content_stream(model=model, contents="Hello"):\n    print(chunk.text or "", end="", flush=True)' : 'response = client.models.generate_content(model=model, contents="Hello")\nprint(response.text)'}`,
      typescript: `import { GoogleGenAI } from '@google/genai'\n\n${discoveryTypescript}\nconst client = new GoogleGenAI({ apiKey: process.env.NEW_API_KEY, httpOptions: { baseUrl: origin, apiVersion: 'v1beta' } })\n${stream ? "const response = await client.models.generateContentStream({ model, contents: 'Hello' })\nfor await (const chunk of response) process.stdout.write(chunk.text ?? '')" : "const response = await client.models.generateContent({ model, contents: 'Hello' })\nconsole.log(response.text)"}`,
    }
  }
  return {
    curl: `${discoveryCurl}\njq -n --arg model "$NEW_API_MODEL" '{model: $model, messages: [{role: "user", content: "Hello"}]${stream ? ', stream: true' : ''}}' |\n  curl --fail-with-body ${stream ? '-N ' : ''}"$NEW_API_BASE_URL/v1/chat/completions" \\\n    -H "Authorization: Bearer $NEW_API_KEY" \\\n    -H "Content-Type: application/json" --data-binary @-`,
    python: `import os\nfrom openai import OpenAI\n\nclient = OpenAI(api_key=os.environ["NEW_API_KEY"], base_url=${JSON.stringify(`${origin}/v1`)})\nrequested = os.environ.get("NEW_API_MODEL")\nmodel = next((item.id for item in client.models.list() if "openai" in getattr(item, "supported_endpoint_types", []) and (not requested or item.id == requested)), None)\nif model is None:\n    raise ValueError("No accessible compatible model")\nresponse = client.chat.completions.create(model=model, messages=[{"role": "user", "content": "Hello"}]${stream ? ', stream=True' : ''})\n${stream ? 'for chunk in response:\n    if chunk.choices:\n        print(chunk.choices[0].delta.content or "", end="", flush=True)' : 'print(response.choices[0].message.content)'}`,
    typescript: `import OpenAI from 'openai'\n\nconst client = new OpenAI({ apiKey: process.env.NEW_API_KEY, baseURL: ${JSON.stringify(`${origin}/v1`)} })\nconst available = await client.models.list()\nconst model = available.data.find(item => 'supported_endpoint_types' in item && Array.isArray(item.supported_endpoint_types) && item.supported_endpoint_types.includes('openai') && (!process.env.NEW_API_MODEL || item.id === process.env.NEW_API_MODEL))?.id\nif (!model) throw new Error('No accessible compatible model')\nconst response = await client.chat.completions.create({ model, messages: [{ role: 'user', content: 'Hello' }]${stream ? ', stream: true' : ''} })\n${stream ? "for await (const chunk of response) process.stdout.write(chunk.choices[0]?.delta.content ?? '')" : 'console.log(response.choices[0].message.content)'}`,
  }
}

export function buildFirstRequest(origin: string, stream: boolean) {
  const url = `${origin.replace(/\/$/, '')}/v1`
  const shellEndpoint = `'${`${url}/chat/completions`.replaceAll("'", "'\\''")}'`
  const payload = `{model: $model, messages: [{role: "user", content: "Hello"}]${stream ? ', stream: true}' : '}'}`
  return {
    curl: `: "\${NEW_API_KEY:?Set NEW_API_KEY}" "\${NEW_API_MODEL:?Set NEW_API_MODEL}"
jq -n --arg model "$NEW_API_MODEL" '${payload}' |
  curl --fail-with-body ${stream ? '-N ' : ''}${shellEndpoint} \
    -H "Authorization: Bearer $NEW_API_KEY" \
    -H "Content-Type: application/json" --data-binary @-`,
    python: `import os
from openai import OpenAI

client = OpenAI(api_key=os.environ["NEW_API_KEY"], base_url=${JSON.stringify(url)})
response = client.chat.completions.create(
    model=os.environ["NEW_API_MODEL"],
    messages=[{"role": "user", "content": "Hello"}]${stream ? ',\n    stream=True' : ''}
)
${stream ? 'for chunk in response:\n    if chunk.choices:\n        print(chunk.choices[0].delta.content or "", end="", flush=True)' : 'print(response.choices[0].message.content)'}`,
    javascript: `import OpenAI from 'openai'

const client = new OpenAI({ apiKey: process.env.NEW_API_KEY, baseURL: ${JSON.stringify(url)} })
const response = await client.chat.completions.create({
  model: process.env.NEW_API_MODEL,
  messages: [{ role: 'user', content: 'Hello' }]${stream ? ',\n  stream: true' : ''}
})
${stream ? "for await (const chunk of response) console.log(chunk.choices[0]?.delta?.content ?? '')" : 'console.log(response.choices[0].message.content)'}`,
  }
}

export function buildAPIKeyExample(origin: string) {
  const url = `${origin.replace(/\/$/, '')}/v1/models`
  return `curl '${url.replaceAll("'", "'\\''")}' \\
  -H "Authorization: Bearer YOUR_API_KEY"`
}

export function buildChatExample(origin: string, greeting = 'Hello!') {
  const baseURL = `${origin.replace(/\/$/, '')}/v1`
  const payload = JSON.stringify(
    {
      model: 'YOUR_MODEL_ID',
      messages: [{ role: 'user', content: greeting }],
    },
    null,
    2
  )
  return {
    curl: `curl '${`${baseURL}/chat/completions`.replaceAll("'", "'\\''")}' \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '${payload.replaceAll("'", "'\\''")}'`,
    python: `from openai import OpenAI

client = OpenAI(
    api_key="YOUR_API_KEY",
    base_url=${JSON.stringify(baseURL)}
)

response = client.chat.completions.create(
    model="YOUR_MODEL_ID",
    messages=[
        {
            "role": "user",
            "content": ${JSON.stringify(greeting)}
        }
    ]
)

print(response.choices[0].message.content)`,
    javascript: `import OpenAI from "openai";

const client = new OpenAI({
  apiKey: "YOUR_API_KEY",
  baseURL: ${JSON.stringify(baseURL)}
});

const response = await client.chat.completions.create({
  model: "YOUR_MODEL_ID",
  messages: [
    {
      role: "user",
      content: ${JSON.stringify(greeting)}
    }
  ]
});

console.log(response.choices[0].message.content);`,
  }
}

export function buildImageExample(
  origin: string,
  prompt = 'A small red house beside a lake'
) {
  const baseURL = `${origin.replace(/\/$/, '')}/v1`
  const payload = JSON.stringify(
    {
      model: 'YOUR_MODEL_ID',
      prompt,
    },
    null,
    2
  )
  return {
    curl: `curl '${`${baseURL}/images/generations`.replaceAll("'", "'\\''")}' \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '${payload.replaceAll("'", "'\\''")}'`,
    python: `from openai import OpenAI

client = OpenAI(
    api_key="YOUR_API_KEY",
    base_url=${JSON.stringify(baseURL)},
    max_retries=0,
    timeout=600
)

response = client.images.generate(
    model="YOUR_MODEL_ID",
    prompt=${JSON.stringify(prompt)}
)

print(response)`,
    javascript: `import OpenAI from "openai";

const client = new OpenAI({
  apiKey: "YOUR_API_KEY",
  baseURL: ${JSON.stringify(baseURL)},
  maxRetries: 0,
  timeout: 600000
});

const response = await client.images.generate({
  model: "YOUR_MODEL_ID",
  prompt: ${JSON.stringify(prompt)}
});

console.log(response);`,
  }
}

export function buildVideoExample(
  origin: string,
  prompt = 'A paper boat drifting on a calm lake'
) {
  const baseURL = `${origin.replace(/\/$/, '')}/v1`
  const payload = JSON.stringify({ model: 'YOUR_MODEL_ID', prompt }, null, 2)
  const headers = '  -H "Authorization: Bearer YOUR_API_KEY"'
  return {
    curl: `curl '${`${baseURL}/videos`.replaceAll("'", "'\\''")}' \\
${headers} \\
  -H "Content-Type: application/json" \\
  -d '${payload.replaceAll("'", "'\\''")}'`,
    python: `import requests

response = requests.post(
    ${JSON.stringify(`${baseURL}/videos`)},
    headers={
        "Authorization": "Bearer YOUR_API_KEY",
        "Content-Type": "application/json"
    },
    json={
        "model": "YOUR_MODEL_ID",
        "prompt": ${JSON.stringify(prompt)}
    },
    timeout=600
)
response.raise_for_status()
result = response.json()
print(result)`,
    javascript: `const response = await fetch(
  ${JSON.stringify(`${baseURL}/videos`)},
  {
    method: "POST",
    headers: {
      "Authorization": "Bearer YOUR_API_KEY",
      "Content-Type": "application/json"
    },
    body: JSON.stringify({
      model: "YOUR_MODEL_ID",
      prompt: ${JSON.stringify(prompt)}
    })
  }
);

const result = await response.json();
console.log(result);`,
    status: `curl '${`${baseURL}/videos/TASK_ID`.replaceAll("'", "'\\''")}' \\
${headers}`,
    download: `curl '${`${baseURL}/videos/TASK_ID/content`.replaceAll("'", "'\\''")}' \\
${headers} \\
  --output video.mp4`,
  }
}

export function buildAudioExample(
  origin: string,
  task: 'speech' | 'transcription',
  input = 'Hello! Welcome to New API.',
  saved = 'Saved speech.mp3'
) {
  const baseURL = `${origin.replace(/\/$/, '')}/v1`
  if (task === 'transcription') {
    return {
      curl: `curl '${`${baseURL}/audio/transcriptions`.replaceAll("'", "'\\''")}' \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -F "file=@YOUR_AUDIO_FILE" \\
  -F "model=YOUR_MODEL_ID"`,
      python: `import requests

with open("YOUR_AUDIO_FILE", "rb") as audio:
    response = requests.post(
        ${JSON.stringify(`${baseURL}/audio/transcriptions`)},
        headers={"Authorization": "Bearer YOUR_API_KEY"},
        data={"model": "YOUR_MODEL_ID"},
        files={"file": audio},
        timeout=600
    )
response.raise_for_status()
result = response.json()
print(result["text"])`,
      javascript: `import { readFile } from "node:fs/promises";

const form = new FormData();
form.set("model", "YOUR_MODEL_ID");
form.set("file", new Blob([await readFile("YOUR_AUDIO_FILE")]), "audio.mp3");

const response = await fetch(
  ${JSON.stringify(`${baseURL}/audio/transcriptions`)},
  {
    method: "POST",
    headers: { "Authorization": "Bearer YOUR_API_KEY" },
    body: form
  }
);
const result = await response.json();
console.log(result.text);`,
    }
  }
  const payload = JSON.stringify(
    {
      model: 'YOUR_MODEL_ID',
      input,
      voice: 'YOUR_VOICE_ID',
      response_format: 'mp3',
    },
    null,
    2
  )
  return {
    curl: `curl '${`${baseURL}/audio/speech`.replaceAll("'", "'\\''")}' \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '${payload.replaceAll("'", "'\\''")}' \\
  --output speech.mp3`,
    python: `import requests

response = requests.post(
    ${JSON.stringify(`${baseURL}/audio/speech`)},
    headers={
        "Authorization": "Bearer YOUR_API_KEY",
        "Content-Type": "application/json"
    },
    json={
        "model": "YOUR_MODEL_ID",
        "input": ${JSON.stringify(input)},
        "voice": "YOUR_VOICE_ID",
        "response_format": "mp3"
    },
    timeout=600
)
response.raise_for_status()
with open("speech.mp3", "wb") as file:
    file.write(response.content)
print(${JSON.stringify(saved)})`,
    javascript: `import { writeFile } from "node:fs/promises";

const response = await fetch(
  ${JSON.stringify(`${baseURL}/audio/speech`)},
  {
    method: "POST",
    headers: {
      "Authorization": "Bearer YOUR_API_KEY",
      "Content-Type": "application/json"
    },
    body: JSON.stringify({
      model: "YOUR_MODEL_ID",
      input: ${JSON.stringify(input)},
      voice: "YOUR_VOICE_ID",
      response_format: "mp3"
    })
  }
);
const audio = await response.arrayBuffer();
await writeFile("speech.mp3", Buffer.from(audio));
console.log(${JSON.stringify(saved)});`,
  }
}
