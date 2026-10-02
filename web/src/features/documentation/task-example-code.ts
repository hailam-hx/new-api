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
export type MediaExampleKind = 'image' | 'video' | 'speech' | 'transcription'

// These examples target the host routes, not vendor-specific plugin URLs.
export function buildTaskExample(
  baseUrl: string,
  kind: MediaExampleKind
): { curl: string; python: string; javascript: string } {
  const origin = baseUrl.replace(/\/$/, '')
  const shellOrigin = `'${origin.replaceAll("'", "'\\''")}'`
  const curlStart = `(
set -euo pipefail
export NEW_API_BASE_URL=${shellOrigin}
: "\${NEW_API_KEY:?Set NEW_API_KEY securely}"
: "\${NEW_API_MODEL:?Set NEW_API_MODEL to your chosen model ID}"`
  const pythonStart = `import os
import requests

base_url = ${JSON.stringify(origin)}
headers = {"Authorization": "Bearer " + os.environ["NEW_API_KEY"]}
model = os.environ["NEW_API_MODEL"]`
  const jsStart = `const baseURL = ${JSON.stringify(origin)}
const { NEW_API_KEY: key, NEW_API_MODEL: model } = process.env
if (!key || !model) throw new Error('Set NEW_API_KEY and NEW_API_MODEL')
const headers = { Authorization: \`Bearer \${key}\` }
async function request(path, options = {}) {
  const response = await fetch(baseURL + path, { ...options, headers: { ...headers, ...options.headers } })
  if (!response.ok) throw new Error(\`HTTP \${response.status}; check Usage Logs before retrying\`)
  return response
}`
  if (kind === 'transcription') {
    return {
      curl: `${curlStart}
: "\${NEW_API_AUDIO_FILE:?Set NEW_API_AUDIO_FILE to a local audio file}"
curl --fail-with-body "$NEW_API_BASE_URL/v1/audio/transcriptions" \\
  -H "Authorization: Bearer $NEW_API_KEY" \\
  --form-string "model=$NEW_API_MODEL" \\
  --form-string "response_format=json" \\
  -F "file=@$NEW_API_AUDIO_FILE"
)`,
      python: `${pythonStart}
with open(os.environ["NEW_API_AUDIO_FILE"], "rb") as audio:
    response = requests.post(base_url + "/v1/audio/transcriptions", headers=headers,
        data={"model": model, "response_format": "json"}, files={"file": audio}, timeout=600)
response.raise_for_status()
print(response.json()["text"])`,
      javascript: `import { readFile } from 'node:fs/promises'

${jsStart}
if (!process.env.NEW_API_AUDIO_FILE) throw new Error('Set NEW_API_AUDIO_FILE')
const form = new FormData()
form.set('model', model)
form.set('response_format', 'json')
form.set('file', new File([await readFile(process.env.NEW_API_AUDIO_FILE)], process.env.NEW_API_AUDIO_FILE.split('/').pop()))
const response = await request('/v1/audio/transcriptions', { method: 'POST', body: form })
console.log((await response.json()).text)`,
    }
  }
  if (kind === 'speech') {
    return {
      curl: `${curlStart}
: "\${NEW_API_VOICE:?Set NEW_API_VOICE to a voice supported by your model}"
jq -n --arg model "$NEW_API_MODEL" --arg voice "$NEW_API_VOICE" \\
  '{model: $model, input: "Hello! Welcome to New API.", voice: $voice, response_format: "mp3"}' | \\
  curl --fail-with-body "$NEW_API_BASE_URL/v1/audio/speech" \\
    -H "Authorization: Bearer $NEW_API_KEY" -H "Content-Type: application/json" \\
    --data-binary @- -D speech.headers --output speech.response
if grep -qi '^content-type:.*json' speech.headers; then
  cat speech.response
else
  mv speech.response speech.mp3
fi
)`,
      python: `${pythonStart}
response = requests.post(base_url + "/v1/audio/speech", headers=headers,
    json={"model": model, "input": "Hello! Welcome to New API.",
          "voice": os.environ["NEW_API_VOICE"], "response_format": "mp3"}, timeout=600)
response.raise_for_status()
if "json" in response.headers.get("Content-Type", ""):
    print(response.json())
else:
    with open("speech.mp3", "wb") as audio:
        audio.write(response.content)`,
      javascript: `import { writeFile } from 'node:fs/promises'

${jsStart}
const voice = process.env.NEW_API_VOICE
if (!voice) throw new Error('Set NEW_API_VOICE to a supported voice')
const response = await request('/v1/audio/speech', {
  method: 'POST', headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ model, input: 'Hello! Welcome to New API.', voice, response_format: 'mp3' })
})
if (response.headers.get('content-type')?.includes('json')) {
  console.log(await response.json())
} else {
  await writeFile('speech.mp3', new Uint8Array(await response.arrayBuffer()))
}`,
    }
  }
  const path = kind === 'image' ? '/v1/images/generations' : '/v1/videos'
  const prompt =
    kind === 'image'
      ? 'A small red house beside a lake'
      : 'A paper boat floating on a calm lake'
  const curlRequest = `${curlStart}
result=$(jq -n --arg model "$NEW_API_MODEL" \\
  '{model: $model, prompt: "${prompt}"}' | \\
  curl --fail-with-body "$NEW_API_BASE_URL${path}" \\
    -H "Authorization: Bearer $NEW_API_KEY" -H "Content-Type: application/json" \\
    --data-binary @-)`
  const pythonRequest = `${pythonStart}
response = requests.post(base_url + "${path}", headers=headers,
    json={"model": model, "prompt": "${prompt}"}, timeout=600)
response.raise_for_status()
result = response.json()`
  const jsRequest = `${jsStart}
const response = await request('${path}', {
  method: 'POST', headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ model, prompt: '${prompt}' })
})
const result = await response.json()`
  if (kind === 'image') {
    return {
      curl: `${curlRequest}
printf '%s\\n' "$result" | jq .
)`,
      python: `${pythonRequest}
print(result)`,
      javascript: `${jsRequest}
console.log(result)`,
    }
  }
  return {
    curl: `${curlRequest}
task_id=$(printf '%s' "$result" | jq -er '.id')
printf 'Task ID: %s\\n' "$task_id"
encoded_id=$(printf '%s' "$task_id" | jq -sRr @uri)
for attempt in {1..120}; do
  result=$(curl --fail-with-body "$NEW_API_BASE_URL/v1/videos/$encoded_id" \\
    -H "Authorization: Bearer $NEW_API_KEY")
  task_status=$(printf '%s' "$result" | jq -er '.status')
  if [ "$task_status" = completed ]; then
    curl --fail-with-body "$NEW_API_BASE_URL/v1/videos/$encoded_id/content" \\
      -H "Authorization: Bearer $NEW_API_KEY" --output video.mp4
    exit 0
  fi
  if [ "$task_status" = failed ]; then
    printf '%s\\n' "$result" | jq .
    exit 1
  fi
  sleep 5
done
printf 'Still running. Keep task ID %s; check its status later.\\n' "$task_id"
)`,
    python: `import time
from urllib.parse import quote

${pythonRequest}
task_id = result["id"]
print("Task ID:", task_id)
path = "/v1/videos/" + quote(task_id, safe="")
for attempt in range(120):
    response = requests.get(base_url + path, headers=headers, timeout=60)
    response.raise_for_status()
    task = response.json()
    if task["status"] == "failed":
        raise RuntimeError(task.get("error") or "Video generation failed")
    if task["status"] == "completed":
        video = requests.get(base_url + path + "/content", headers=headers, timeout=600)
        video.raise_for_status()
        with open("video.mp4", "wb") as output:
            output.write(video.content)
        break
    time.sleep(5)
else:
    print("Still running. Keep the task ID and check its status later.")`,
    javascript: `import { writeFile } from 'node:fs/promises'

${jsRequest}
if (!result.id) throw new Error('Missing task ID; check Task Logs before retrying')
console.log('Task ID:', result.id)
const path = '/v1/videos/' + encodeURIComponent(result.id)
let completed = false
for (let attempt = 0; attempt < 120; attempt++) {
  const task = await (await request(path)).json()
  if (task.status === 'failed') throw new Error(task.error?.message ?? 'Video generation failed')
  if (task.status === 'completed') {
    const video = await request(path + '/content')
    await writeFile('video.mp4', new Uint8Array(await video.arrayBuffer()))
    completed = true
    break
  }
  await new Promise(resolve => setTimeout(resolve, 5000))
}
if (!completed) console.log('Still running. Keep the task ID and check its status later.')`,
  }
}
