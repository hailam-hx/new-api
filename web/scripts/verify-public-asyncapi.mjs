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
*/
import { execFileSync } from 'node:child_process'
import { mkdtempSync, readFileSync, rmSync } from 'node:fs'
import { createRequire } from 'node:module'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const args = process.argv.slice(2)
let toolRoot
let install = false
let spec = fileURLToPath(
  new URL('../../docs/asyncapi/public.yaml', import.meta.url)
)
for (let index = 0; index < args.length; index++) {
  const argument = args[index]
  if (argument === '--install') {
    install = true
  } else if (argument === '--tool-root' || argument === '--spec') {
    const value = args[++index]
    if (!value || value.startsWith('--')) {
      throw new Error(`Missing value for ${argument}`)
    }
    if (argument === '--tool-root') toolRoot = resolve(value)
    else spec = resolve(value)
  } else {
    throw new Error(`Unknown argument: ${argument}`)
  }
}
if (install === Boolean(toolRoot)) {
  throw new Error(
    'Use exactly one of --install or --tool-root <temporary tools directory>'
  )
}

try {
  if (install) {
    toolRoot = mkdtempSync(join(tmpdir(), 'new-api-asyncapi-'))
    execFileSync('bun', ['add', '--cwd', toolRoot, '@asyncapi/parser@3.4.0'], {
      stdio: 'inherit',
    })
  }
  const require = createRequire(join(toolRoot, 'package.json'))
  const { Parser } = require('@asyncapi/parser')
  const { load } = require('js-yaml')
  const Ajv = require('ajv')
  const source = readFileSync(spec, 'utf8')
  const data = load(source)

  // Inspect references before giving the document to the parser's resolver.
  // The canonical contract is self-contained; CI must never fetch remote refs.
  const pending = [data]
  const visited = new Set()
  while (pending.length > 0) {
    const value = pending.pop()
    if (!value || typeof value !== 'object' || visited.has(value)) continue
    visited.add(value)
    if (
      '$ref' in value &&
      (typeof value.$ref !== 'string' || !value.$ref.startsWith('#/'))
    ) {
      throw new Error('AsyncAPI references must be local document pointers')
    }
    pending.push(...Object.values(value))
  }

  const { document, diagnostics } = await new Parser().parse(source)
  for (const diagnostic of diagnostics) {
    console.log(
      `${diagnostic.severity === 0 ? 'ERROR' : 'INFO'} ${diagnostic.code}: ${diagnostic.message}`
    )
  }
  if (
    !document ||
    diagnostics.some((diagnostic) => diagnostic.severity === 0)
  ) {
    throw new Error(
      'Public AsyncAPI schema, references, operations or examples failed validation'
    )
  }
  // Parser 3.4 validates document structure but does not reject every invalid
  // v3 payload example. Validate these separately using its bundled JSON Schema
  // validator, preserving document-local component references.
  const ajv = new Ajv({ strict: false, allErrors: true })
  let examples = 0
  for (const value of visited) {
    if (!value.payload || !Array.isArray(value.examples)) continue
    const validate = ajv.compile({
      ...value.payload,
      components: data.components,
    })
    for (const example of value.examples) {
      if (!Object.hasOwn(example, 'payload')) continue
      if (!validate(example.payload)) {
        throw new Error(
          `Invalid AsyncAPI payload example: ${ajv.errorsText(validate.errors)}`
        )
      }
      examples++
    }
  }
  console.log(
    `Public AsyncAPI PASS (${data.asyncapi}; parser 3.4.0; schema, refs, messages; ${examples} examples)`
  )
} catch (error) {
  console.error(error instanceof Error ? error.message : String(error))
  process.exitCode = 1
} finally {
  if (install && toolRoot) rmSync(toolRoot, { recursive: true, force: true })
}
