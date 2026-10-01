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
  translate: (key: string, values?: Record<string, string | number>) => string
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
  for (const endpoint of data.endpoints) {
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
  for (const error of data.errors) {
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
