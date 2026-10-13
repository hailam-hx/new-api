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
// Examples are displayed only; rendering documentation never submits work.
export function buildDeveloperRecipe(
  section: string,
  origin: string
): { language: 'bash' | 'python'; code: string; curl?: string } {
  const baseUrl = origin.replace(/\/$/, '')
  const preamble = `set -euo pipefail\nexport NEW_API_BASE_URL='${baseUrl.replaceAll("'", "'\\''")}'\n: "\${NEW_API_KEY:?Set NEW_API_KEY securely}"\n`
  if (section === 'balance') {
    return {
      language: 'python',
      curl: `${preamble}curl --fail-with-body "$NEW_API_BASE_URL/v1/key/balance" -H "Authorization: Bearer $NEW_API_KEY"`,
      code: 'import json\nimport os\nfrom decimal import Decimal\nfrom urllib.request import Request, urlopen\n\norigin = ORIGIN\nheaders = {"Authorization": "Bearer " + os.environ["NEW_API_KEY"]}\nwith urlopen(Request(origin + "/v1/key/balance", headers=headers)) as response:\n    balance = json.load(response)\n# Set the estimate in the returned unit, including expected request options.\nif balance["unit"] != os.environ["ESTIMATE_UNIT"]:\n    raise ValueError("Estimate unit differs from balance unit")\nestimate = Decimal(os.environ["ESTIMATED_COST"])\nif not estimate.is_finite() or estimate < 0:\n    raise ValueError("Estimate must be finite and nonnegative")\nif not balance["unlimited"]:\n    if balance["remaining"] is None or Decimal(balance["remaining"]) < estimate:\n        raise ValueError("Insufficient key allowance")\n# Submit once; this balance check does not reserve funds.\npayload = {"model": os.environ["NEW_API_MODEL"], "messages": [{"role": "user", "content": "Hello"}]}\nrequest = Request(origin + "/v1/chat/completions", data=json.dumps(payload).encode(), headers={**headers, "Content-Type": "application/json"})\nwith urlopen(request) as response:\n    print(json.load(response))'.replace(
        'ORIGIN',
        JSON.stringify(baseUrl)
      ),
    }
  }
  const examples: Record<string, string> = {
    discover:
      'catalog=$(curl --fail-with-body "$NEW_API_BASE_URL/v1/catalog" -H "Authorization: Bearer $NEW_API_KEY")\nmodel=$(printf \'%s\' "$catalog" | jq -er \'[.models[] | select(any(.endpoints[]; .method == "POST" and .path == "/v1/chat/completions"))][0].id // error("No compatible model")\')\njq -n --arg model "$model" \'{model: $model, messages: [{role: "user", content: "Hello"}]}\' |\n  curl --fail-with-body "$NEW_API_BASE_URL/v1/chat/completions" -H "Authorization: Bearer $NEW_API_KEY" -H \'Content-Type: application/json\' --data-binary @-',
    usage:
      'curl --fail-with-body --get "$NEW_API_BASE_URL/v1/key/usage" \\\n  -H "Authorization: Bearer $NEW_API_KEY" \\\n  --data-urlencode "start=$START_UNIX_SECONDS" \\\n  --data-urlencode "end=$END_UNIX_SECONDS"',
    video:
      ' : "${NEW_API_MODEL:?Select a catalog model supporting POST /v1/videos}"\nsubmission=$(jq -n --arg model "$NEW_API_MODEL" \'{model: $model, prompt: "A calm ocean"}\' |\n  curl --fail-with-body "$NEW_API_BASE_URL/v1/videos" -H "Authorization: Bearer $NEW_API_KEY" -H \'Content-Type: application/json\' --data-binary @-)\ntask_id=$(printf \'%s\' "$submission" | jq -er \'.id\')\nprintf \'%s\\n\' "$task_id" > video-task-id.txt\nfor attempt in {1..60}; do\n  task=$(curl --fail-with-body "$NEW_API_BASE_URL/v1/tasks/$task_id" -H "Authorization: Bearer $NEW_API_KEY")\n  status=$(printf \'%s\' "$task" | jq -er \'.status\')\n  case "$status" in\n    completed)\n      curl --fail-with-body "$NEW_API_BASE_URL/v1/videos/$task_id/content" -H "Authorization: Bearer $NEW_API_KEY" --output video.mp4\n      exit 0 ;;\n    failed) printf \'%s\\n\' "$task"; exit 1 ;;\n    queued|in_progress) sleep 5 ;;\n    *) printf \'Unknown status: %s\\n\' "$status"; exit 1 ;;\n  esac\ndone\nprintf \'Polling stopped; retrieve saved task %s before resubmitting.\\n\' "$task_id"',
    tasks:
      'cursor=\'\'\nfor page in {1..100}; do\n  args=(--data-urlencode \'limit=20\')\n  if [ -n "$cursor" ]; then args+=(--data-urlencode "cursor=$cursor"); fi\n  tasks=$(curl --fail-with-body --get "$NEW_API_BASE_URL/v1/tasks" -H "Authorization: Bearer $NEW_API_KEY" "${args[@]}")\n  printf \'%s\' "$tasks" | jq \'.data\'\n  if [ "$(printf \'%s\' "$tasks" | jq -r \'.has_more\')" != true ]; then break; fi\n  cursor=$(printf \'%s\' "$tasks" | jq -er \'.next_cursor\')\ndone',
  }
  return { language: 'bash', code: preamble + (examples[section] ?? '') }
}
