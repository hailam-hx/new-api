# Public SDK readiness — Phase 4B

This verifies temporary client generation from `docs/openapi/public.json`. No official HOTX SDK package is published, no generated client is retained in the repository, and no production dependency is added.

| Target | Result | Scope |
| --- | --- | --- |
| TypeScript | PASS | All 107 operations generated as typed paths/operation IDs; strict minimal client typecheck and seven mock transport calls pass. |
| Python | PASS, with generator configuration | All 107 endpoint modules generated without warnings; complete package byte compilation, minimal client typecheck, and seven mock transport calls pass. |
| Go | NOT_TESTED | Optional target; no additional generator introduced. |

These are generation and minimal integration results, not a claim that every deployment-dependent provider protocol is fully typed or that every operation was executed.

## Tooling and commands

No public SDK generator or SDK publishing pipeline existed in the baseline. `controller/plugin_protocol_sdk_test.go` tests the plugin protocol, not a published external Python/TypeScript client. The historical Apifox exports in `docs/openapi/relay.json` and `api.json` are not the canonical public SDK inputs.

Tested with Node 22.23.2, Bun 1.4.2, Python 3.14.8, `openapi-typescript` 7.13.0, `openapi-fetch` 0.17.0, TypeScript 5.9.3, `@types/node` 26.6.5, `openapi-python-client` 0.29.1, httpx 0.28.1, mypy 2.4.0, and `openapi-spec-validator` 0.9.0. Generator and direct compiler versions are pinned by the reusable script; temporary Python transitive dependencies follow the generator's constraints.

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

No upstream API requests, paid requests, database access, or live API requests occur.

## Findings and limits

- Historical `relay.json` fails full OpenAPI validation because its `Combination` security scheme is an Apifox-specific `type: combination`. Its duplicate `createImage` ID causes Python image edits generation to be omitted. It also contains stale unsupported file/fine-tune operations. These are reasons to use the new public contract; the archival export is preserved.
- The canonical spec generates all 107 operation IDs in both targets without invalid discriminator, recursive reference, or nullable union generation errors. Read-control DTOs preserve required fields and explicit nullable monetary/expiration values. Existing unique operation IDs are retained; the previously ambiguous duplicate image edit ID is normalized separately in the coverage report.
- TypeScript 7.0.2, initially resolved as latest, is incompatible with the tested generator's compiler API (`ts.factory` is unavailable). TypeScript 5.9.3 is explicitly pinned; no product compiler dependency is changed.
- An additional full-package Python mypy run (`python -m mypy hotx_public_client smoke.py`) reports **12 errors in the generated `client.py`**, across **417 checked files**: generator context-manager methods forward `*tuple[object, ...]` to httpx `__exit__`/`__aexit__`. This is **BLOCKED_BY_GENERATOR_TEMPLATE** for strict whole-package mypy. No generated template is patched and no error suppression is added to generated code. The required minimal typed client sample passes, while byte compilation checks the entire generated package.
- Native/plugin payloads and provider-specific optional nested fields remain open contracts where the implementation is deployment/model dependent. Generation success does not make those schemas exhaustive. Consumers must retain or ignore unknown response fields as appropriate.
- OpenAPI unions for model/input content generate, but provider combinations beyond the minimal examples are not runtime-tested. Nullable scalar correctness is explicitly exercised for the balance contract; it is not inferred from compilation alone.
- Generic clients do not automatically negotiate every success content type. For mixed JSON/binary responses such as audio speech, select the transport/parser using the requested output and actual response Content-Type. The Python generator selects a parser from media entries; generation is not proof that all alternatives are handled dynamically.
- SSE is declared as `text/event-stream`, not an array. Real incremental event parsing requires a streaming transport and is not covered by these buffered JSON/binary smoke calls. WebSocket handshakes and bidirectional traffic require a WebSocket client; generated HTTP methods alone do not implement them. Neither SSE nor WebSocket runtime support is claimed.
- No SDK is published, generated vendor source is not committed, and the smoke is opt-in rather than adding package downloads to the normal frontend build.

## Validation result

Full OpenAPI validation: PASS. TypeScript full generation, strict minimal typecheck, and seven mock calls: PASS. Python full generation with binary configuration, complete byte compilation, minimal sample typecheck, and seven mock calls: PASS. Python strict generated-package mypy: BLOCKED_BY_GENERATOR_TEMPLATE. Changed script lint and formatting: PASS. Go: NOT_TESTED.
