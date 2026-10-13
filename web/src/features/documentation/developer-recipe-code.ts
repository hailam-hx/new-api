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
    discover: `catalog=$(curl --fail-with-body "$NEW_API_BASE_URL/v1/catalog" -H "Authorization: Bearer $NEW_API_KEY")
model=$(printf '%s' "$catalog" | python3 -c 'import json,sys; models=json.load(sys.stdin)["models"]; print(next(m["id"] for m in models if any(e["method"] == "POST" and e["path"] == "/v1/chat/completions" for e in m["endpoints"])))')
python3 -c 'import json,sys; print(json.dumps({"model":sys.argv[1],"messages":[{"role":"user","content":"Hello"}]}))' "$model" |
  curl --fail-with-body "$NEW_API_BASE_URL/v1/chat/completions" -H "Authorization: Bearer $NEW_API_KEY" -H 'Content-Type: application/json' --data-binary @-`,
    usage: `curl --fail-with-body --get "$NEW_API_BASE_URL/v1/key/usage" \\
  -H "Authorization: Bearer $NEW_API_KEY" \\
  --data-urlencode "start=$START_UNIX_SECONDS" \\
  --data-urlencode "end=$END_UNIX_SECONDS"`,
    video: `: "\${NEW_API_MODEL:?Select a catalog model supporting POST /v1/videos}"
submission=$(python3 -c 'import json,sys; print(json.dumps({"model":sys.argv[1],"prompt":"A calm ocean"}))' "$NEW_API_MODEL" |
  curl --fail-with-body "$NEW_API_BASE_URL/v1/videos" -H "Authorization: Bearer $NEW_API_KEY" -H 'Content-Type: application/json' --data-binary @-)
task_id=$(printf '%s' "$submission" | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')
printf '%s\\n' "$task_id" > video-task-id.txt
for attempt in {1..60}; do
  task=$(curl --fail-with-body "$NEW_API_BASE_URL/v1/tasks/$task_id" -H "Authorization: Bearer $NEW_API_KEY")
  status=$(printf '%s' "$task" | python3 -c 'import json,sys; print(json.load(sys.stdin)["status"])')
  case "$status" in
    completed)
      curl --fail-with-body "$NEW_API_BASE_URL/v1/videos/$task_id/content" -H "Authorization: Bearer $NEW_API_KEY" --output video.mp4
      exit 0 ;;
    failed) printf '%s\\n' "$task"; exit 1 ;;
    queued|in_progress) sleep 5 ;;
    *) printf 'Unknown status: %s\\n' "$status"; exit 1 ;;
  esac
done
printf 'Polling stopped; retrieve saved task %s before resubmitting.\\n' "$task_id"`,
    tasks: `cursor=''
for page in {1..100}; do
  args=(--data-urlencode 'limit=20')
  if [ -n "$cursor" ]; then args+=(--data-urlencode "cursor=$cursor"); fi
  tasks=$(curl --fail-with-body --get "$NEW_API_BASE_URL/v1/tasks" -H "Authorization: Bearer $NEW_API_KEY" "\${args[@]}")
  printf '%s' "$tasks" | python3 -c 'import json,sys; print(json.dumps(json.load(sys.stdin)["data"]))'
  if [ "$(printf '%s' "$tasks" | python3 -c 'import json,sys; print("true" if json.load(sys.stdin)["has_more"] else "false")')" != true ]; then break; fi
  cursor=$(printf '%s' "$tasks" | python3 -c 'import json,sys; cursor=json.load(sys.stdin)["next_cursor"]; assert isinstance(cursor,str) and cursor; print(cursor)')
done`,
  }
  return { language: 'bash', code: preamble + (examples[section] ?? '') }
}
