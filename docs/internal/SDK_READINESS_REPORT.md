# Public SDK readiness — Phase 5D/5E

This verifies temporary client generation from `docs/openapi/public.json`. No official HOTX SDK package is published, no generated client is retained in the repository, and no production dependency is added.

| Target | Result | Verified scope |
| --- | --- | --- |
| TypeScript REST | PASS | All 107 operations generated; strict minimal client typecheck; seven injected mock calls (six JSON and one binary). |
| Python REST | PASS | All 107 modules generated; byte compilation; typed sample; seven injected mock calls. Strict whole-package mypy remains blocked by the generator template below. |
| TypeScript SSE | MANUAL_RECIPE | Generated `openapi-fetch` raw `parseAs: 'stream'` transport verified. Manual incremental event decoding, split UTF-8, LF/CRLF, multiline data, comments, event/id, terminal sentinel, incomplete EOF and consumer cancellation verified with in-memory byte streams. |
| Python SSE | MANUAL_RECIPE | Generated HTTP endpoints do not provide incremental SSE event iteration. Manual decoder verified against the same byte fixtures and generator-close cleanup. |
| TypeScript WebSocket | MANUAL_RECIPE | Independent Node WebSocket transport recipe verified against a loopback fixture: authenticated upgrade, create, delta, cancel, terminal event and normal close. HTTP generation does not provide WebSocket methods. |
| Python WebSocket | MANUAL_RECIPE | Independent `websockets` client verified against the loopback fixture with the same create/cancel/close lifecycle. |
| Go | NOT_SUPPORTED | No Go SDK generator or published client is provided by this verification pipeline. |


These are generation and minimal integration results, not a claim that every deployment-dependent provider protocol is fully typed or that every operation was executed.

## Tooling and commands

No public SDK generator or SDK publishing pipeline existed in the baseline. `controller/plugin_protocol_sdk_test.go` tests the plugin protocol, not a published external Python/TypeScript client. The historical Apifox exports in `docs/openapi/relay.json` and `api.json` are not the canonical public SDK inputs.

Tested with Node 22.23.2, Bun 1.4.2, Python 3.14.8, `openapi-typescript` 7.13.0, `openapi-fetch` 0.17.0, TypeScript 5.9.3, `@types/node` 26.6.5, `openapi-python-client` 0.29.1, httpx 0.28.1, mypy 2.4.0, and `openapi-spec-validator` 0.9.0. Phase 5 adds temporary-only `ws` 8.18.3 and `websockets` 15.0.1 for loopback WebSocket fixture verification. Generator and direct compiler versions are pinned by the reusable script; temporary Python transitive dependencies follow the generator's constraints.

Run from repository root:

```sh
node web/scripts/verify-public-sdk.mjs --install
```

This installs tools and generates output in an OS temporary directory, then removes that directory on success or failure. It does not modify `web/package.json`, the repository lockfile, the spec, or generated source in the repository. Package registry downloads are the only external network use.

To reuse an existing temporary tool installation:

```sh
node web/scripts/verify-public-sdk.mjs --tool-root /tmp/hotx-sdk-smoke.hdo1lH
```

The reused directory must contain the pinned `node_modules` and a `venv` containing the Python tools. This run validates the full OpenAPI document with `openapi-spec-validator`, verifies operation ID uniqueness, then runs:

```text
openapi-typescript <public.json> -o schema.d.ts
tsc --strict --noUncheckedIndexedAccess --module NodeNext --moduleResolution NodeNext --target ES2022 --outDir dist smoke.ts
node dist/smoke.js
openapi-python-client generate --path <public.json> --meta none --config python-config.json --output-path hotx_public_client
python -m compileall -q hotx_public_client smoke.py
python -m mypy --follow-imports=silent smoke.py
python smoke.py
```

The script uses the generator's documented `content_type_overrides` configuration to parse declared binary media as bytes (`application/octet-stream`). It applies this only to response schemas with `format: binary`. This preserves the actual public media types and never rewrites the OpenAPI contract. Without this configuration, the tested Python generator silently omits successful video/mp4 and image/* parsers after warning. The binary mock verifies video/mp4 bytes survive parsing into the generated `File.payload`.

Official primary references: [openapi-typescript](https://openapi-ts.dev/introduction), [openapi-fetch usage](https://openapi-ts.dev/openapi-fetch/), and [openapi-python-client configuration](https://github.com/openapi-generators/openapi-python-client#content_type_overrides).

## Actual client methods and fixtures

The generated TypeScript types are used with `createClient<paths>()`; methods are typed HTTP/path calls, not invented resource methods:

```ts
client.GET('/v1/models')
client.GET('/v1/catalog')
client.GET('/v1/key/balance')
client.GET('/v1/key/usage', { params: { query: { start: 1, end: 2 } } })
client.GET('/v1/tasks')
client.GET('/v1/tasks/{key}', { params: { path: { key: 'fixture/task' } } })
client.GET('/v1/videos/{task_id}/content', {
  params: { path: { task_id: 'fixture-video' } },
  parseAs: 'arrayBuffer',
})
```

The generated Python endpoint modules expose `sync`, `sync_detailed`, `asyncio`, and `asyncio_detailed`. The smoke calls `list_models.sync`, `get_developer_catalog.sync`, `get_developer_key_balance.sync`, `get_developer_key_usage.sync`, `get_developer_tasks.sync`, `get_developer_task.sync`, and `get_video_content.sync` with an `AuthenticatedClient` and httpx `MockTransport`.

Each client makes six JSON reads and one binary read entirely through injected mock transports. Assertions protect bearer header propagation, usage query serialization, path escaping (`fixture/task` becomes `fixture%2Ftask`), exact decimal strings, negative recorded net usage, unlimited balance nulls, no-expiration nulls, an omitted terminal pagination cursor, preservation of an unknown catalog property in Python, and binary bytes. TypeScript also proves that omitting the required task path parameter fails typechecking.

No upstream/provider API requests, paid requests, database access, or live HOTX API requests occur. Phase 5 WebSocket tests use a loopback-only fixture server.

## Findings and limits

- Historical `relay.json` fails full OpenAPI validation because its `Combination` security scheme is an Apifox-specific `type: combination`. Its duplicate `createImage` ID causes Python image edits generation to be omitted. It also contains stale unsupported file/fine-tune operations. These are reasons to use the new public contract; the archival export is preserved.
- The canonical spec generates all 107 operation IDs in both targets without invalid discriminator, recursive reference, or nullable union generation errors. Read-control DTOs preserve required fields and explicit nullable monetary/expiration values. Existing unique operation IDs are retained; the previously ambiguous duplicate image edit ID is normalized separately in the coverage report.
- TypeScript 7.0.2, initially resolved as latest, is incompatible with the tested generator's compiler API (`ts.factory` is unavailable). TypeScript 5.9.3 is explicitly pinned; no product compiler dependency is changed.
- An additional full-package Python mypy run (`python -m mypy hotx_public_client smoke.py`) reports **12 errors in the generated `client.py`**, across **428 checked files**: generator context-manager methods forward `*tuple[object, ...]` to httpx `__exit__`/`__aexit__`. This is **BLOCKED_BY_GENERATOR_TEMPLATE** for strict whole-package mypy. No generated template is patched and no error suppression is added to generated code. The required minimal typed client sample passes, while byte compilation checks the entire generated package.
- Native/plugin payloads and provider-specific optional nested fields remain open contracts where the implementation is deployment/model dependent. Generation success does not make those schemas exhaustive. Consumers must retain or ignore unknown response fields as appropriate.
- OpenAPI unions for model/input content generate, but provider combinations beyond the minimal examples are not runtime-tested. Nullable scalar correctness is explicitly exercised for the balance contract; it is not inferred from compilation alone.
- Generic clients do not automatically negotiate every success content type. For mixed JSON/binary responses such as audio speech, select the transport/parser using the requested output and actual response Content-Type. The Python generator selects a parser from media entries; generation is not proof that all alternatives are handled dynamically.
- SSE is declared as `text/event-stream`, not an array. Generated TypeScript raw streaming transport is verified, while event parsing is manual. The Python generator remains buffered. Manual SSE decoders and WebSocket transport recipes live in `web/scripts/sdk-streaming-fixtures.mjs` and are executed by the SDK smoke. These fixture recipes are not published SDK methods, a universal provider capability claim, or a substitute for backend streaming/WebSocket contract tests.
- No SDK is published and generated vendor source is not committed. `.github/workflows/public-sdk.yml` now runs the temporary generation and local fixture smoke on every pull request as well as manual dispatch. It downloads tools from package registries; it makes no provider/upstream API requests.

## Validation result

Full OpenAPI validation: PASS. TypeScript full generation, strict minimal typecheck, and seven mock calls: PASS. Python full generation with binary configuration, complete byte compilation, minimal sample typecheck, and seven mock calls: PASS. Python strict generated-package mypy: BLOCKED_BY_GENERATOR_TEMPLATE. Changed script lint and formatting: PASS. Go: NOT_SUPPORTED by this pipeline.


## Committed contract CI

The canonical compatibility baseline is `docs/openapi/public.json` at the PR merge base. `web/scripts/public-openapi-contract.mjs --base-ref <base SHA>` resolves `git merge-base <base SHA> HEAD` then uses `git show <merge base>:docs/openapi/public.json`. Checkout fetches complete history. Local execution without `--base-ref` compares to committed HEAD. The working-tree `public-contract-baseline.json` is not consulted by the gate, so changing it in a PR cannot hide a regression.

The checker emits `BREAKING`, `POTENTIALLY_BREAKING`, and `ADDITIVE`. Only `BREAKING` fails by default. Potential changes emit review warnings. Explicitly detected breaks include operation deletion, required request additions, request enum removal/restriction, type/nullability changes, required response removal, authentication requirement/scheme changes, and narrowing input bounds. Structural patterns/unions whose containment is unknown are review warnings; this is not a proof of semantic compatibility. New optional fields/parameters/headers and operations pass.

Intentional breaking changes require a reviewed version/migration decision: preserve the old operation contract, add a separately versioned replacement, record the migration/deprecation in developer versioning/changelog docs, and review both committed contracts. A baseline snapshot refresh alone does not approve removal or weakening of the existing public contract. There is no magic environment bypass. If maintaining the old version is impossible, the release remains blocked until maintainers approve a versioned release process; CI is not silently disabled.

Current targeted verification: public contract and recipe test file **25 PASS**; `bun run docs:contract` **107 operations, no compatibility changes** against committed `1fee3b56d8f012ef35b38324ca33ebeb284a1781`; `bun run typecheck` **PASS**. Python strict audit reports exactly **12** `arg-type` errors in generated `client.py` at lines **105, 136, 246, 280**, three each for forwarded arguments to `__exit__` / `__aexit__`. The script permits only this exact known error shape; unexpected errors fail. Full-package strict typing is still **BLOCKED_BY_GENERATOR_TEMPLATE**, without hand-editing generated output.

The loopback fixture exercises cancellation while a response is active. `response.cancel` is a Responses WebSocket message, not a universal HTTP/task cancellation API. Production availability remains channel/model/configuration dependent. Browser consumers must use the source-documented API-key subprotocol rather than Node-only authorization headers. Terminal event variants and unexpected/abrupt closes must follow the backend contract report; the client fixture proves the demonstrated successful lifecycle only.

AsyncAPI companion validation is mandatory in the same public SDK CI workflow: `node web/scripts/verify-public-asyncapi.mjs --install`. Observed local reusable-tool result: **PASS**, AsyncAPI 3.0.0, parser 3.4.0, schema/local references/messages plus five AJV-validated message examples. External references are rejected before resolution. The validator emits one informational newer-version hint, with zero validation errors.


## Executed developer recipe proofs

The existing public-contract test file executes the actual strings returned by `buildDeveloperRecipe`, through Bash or Python. Only HTTP transport is replaced with deterministic JSON/binary fixtures; every unexpected URL/call fails. A temporary `jq` shim always exits 99, proving the critical recipes execute without jq. No displayed request is sent to a live service.

All five flows pass: compatible-model discovery and JSON submission, exact decimal balance guard before one chat submission, usage time-range arguments, one video submission followed by saved task ID and completed binary download, and opaque task pagination terminating on `has_more: false`. Regression cases verify model names containing quotes/dollar signs/backticks remain JSON data, the saved video task ID has a real newline, malformed catalog JSON stops before submission, insufficient balance stops before submission, and failed/unknown task states preserve the saved ID without resubmitting or downloading content. Nine executable recipe cases pass within the **25 PASS** public-contract/recipe test file. Python's standard `json`/`Decimal` modules perform JSON parsing/serialization and monetary comparison; no jq install or new product dependency is needed.

## Phase 5 final canonical-spec verification

After the implemented idempotency header/error documentation, the canonical spec still has **107/107 callable operations**. The committed-baseline classifier reports **13 additive optional-header changes**, zero breaking or potential changes; existing header descriptions and newly documented error statuses also received source review. The latest local run under bundled **Node 24.19** generated both clients successfully; Python strict whole-package typing still reports exactly the known **12** generator-template errors across **428 files**. The documentation contract/recipe test file now has **26 passing tests**, including actual builder execution without jq and conditional idempotency coverage.

The verified reference helpers live in [`sdk-streaming-fixtures.mjs`](../../web/scripts/sdk-streaming-fixtures.mjs): `typescriptStreamingRecipe` and `pythonStreamingRecipe` include incremental decoders and a demonstrated socket lifecycle. They are executable fixture recipes, not published SDK APIs. A production integration must add its own body/event limits, transport timeout policy and protocol-specific terminal handling, and must not reconnect/resubmit paid work after uncertainty. Python uses `websockets`; TypeScript's Node recipe uses `ws`. Those packages exist only in disposable verification tooling, not product dependencies.
