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
import { execFileSync } from 'node:child_process'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

import { parseDocument } from 'yaml'

export const methods = [
  'get',
  'post',
  'put',
  'patch',
  'delete',
  'head',
  'options',
]
export const normalizePath = (value) =>
  value.replaceAll(/:[^/]+|\*[^/]+|\{[^}]+\}/g, '{}').replace(/\/$/, '')

export function parsePublicSpec(raw) {
  const value = JSON.parse(raw)
  const document = parseDocument(raw, { uniqueKeys: true })
  if (document.errors.length) {
    throw new Error(
      `Duplicate/invalid JSON mapping: ${document.errors[0].message}`
    )
  }
  return value
}

export function resolveSchema(value, spec, seen = new Set()) {
  if (!value || typeof value !== 'object') return value
  if (value.$ref) {
    if (!value.$ref.startsWith('#/')) {
      throw new Error(`External reference: ${value.$ref}`)
    }
    const target = value.$ref
      .slice(2)
      .split('/')
      .reduce(
        (node, key) => node?.[key.replaceAll('~1', '/').replaceAll('~0', '~')],
        spec
      )
    if (!target) throw new Error(`Unresolved reference: ${value.$ref}`)
    if (seen.has(value.$ref)) return { $ref: value.$ref }
    return resolveSchema(target, spec, new Set([...seen, value.$ref]))
  }
  if (Array.isArray(value)) {
    return value.map((item) => resolveSchema(item, spec, seen))
  }
  return Object.fromEntries(
    Object.entries(value).map(([key, item]) => [
      key,
      resolveSchema(item, spec, seen),
    ])
  )
}

function validateExample(value, schema, location) {
  if (value === null && (schema.nullable || !schema.type)) return
  if (schema.anyOf || schema.oneOf) {
    const options = schema.anyOf ?? schema.oneOf
    if (
      options.some((option) => {
        try {
          validateExample(value, option, location)
          return true
        } catch {
          return false
        }
      })
    ) {
      return
    }
    throw new Error(`Example does not match union: ${location}`)
  }
  if (schema.type === 'object') {
    if (!value || typeof value !== 'object' || Array.isArray(value)) {
      throw new Error(`Example object: ${location}`)
    }
    for (const key of schema.required ?? []) {
      if (!(key in value)) {
        throw new Error(`Example required ${location}.${key}`)
      }
    }
    for (const [key, item] of Object.entries(value)) {
      if (schema.properties?.[key]) {
        validateExample(item, schema.properties[key], `${location}.${key}`)
      }
    }
  } else if (schema.type === 'array') {
    if (!Array.isArray(value)) throw new Error(`Example array: ${location}`)
    for (const item of value) {
      validateExample(item, schema.items ?? {}, location)
    }
  } else if (
    schema.type &&
    (typeof value !== (schema.type === 'integer' ? 'number' : schema.type) ||
      (schema.type === 'integer' && !Number.isInteger(value)))
  ) {
    throw new Error(`Example type: ${location}`)
  }
  if (schema.enum && !schema.enum.includes(value)) {
    throw new Error(`Example enum: ${location}`)
  }
  if (
    schema.pattern &&
    typeof value === 'string' &&
    !new RegExp(schema.pattern).test(value)
  ) {
    throw new Error(`Example pattern: ${location}`)
  }
}

function validateMediaExamples(media, location) {
  if ('example' in media) {
    validateExample(media.example, media.schema ?? {}, location)
  }
  for (const [name, example] of Object.entries(media.examples ?? {})) {
    if ('value' in example) {
      validateExample(example.value, media.schema ?? {}, `${location}/${name}`)
    }
  }
}

export function validatePublicContract(spec, advertised, reference, root) {
  if (!(spec.openapi ?? '').startsWith('3.0.')) {
    throw new Error('Expected OpenAPI 3.0.x')
  }
  resolveSchema(spec, spec)
  const ids = new Set()
  const actual = new Set()
  const expected = new Set(
    advertised
      .filter((endpoint) => endpoint.path !== '/v1/edits')
      .map((endpoint) => `${endpoint.method} ${normalizePath(endpoint.path)}`)
  )
  for (const [url, item] of Object.entries(spec.paths)) {
    if (
      !url.startsWith('/') ||
      url.startsWith('/api/') ||
      url.startsWith('/internal/')
    ) {
      throw new Error(`Non-public path: ${url}`)
    }
    for (const [method, operation] of Object.entries(item)) {
      if (
        ['parameters', 'summary', 'description', 'servers'].includes(method)
      ) {
        continue
      }
      if (!methods.includes(method)) {
        throw new Error(`Unexpected path member: ${method}`)
      }
      const key = `${method.toUpperCase()} ${normalizePath(url)}`
      if (actual.has(key)) {
        throw new Error(`Equivalent duplicate operation: ${key}`)
      }
      actual.add(key)
      if (!expected.has(key)) {
        throw new Error(`Operation outside public allowlist: ${key}`)
      }
      if (!operation.operationId || ids.has(operation.operationId)) {
        throw new Error(
          `Missing/duplicate operationId: ${operation.operationId}`
        )
      }
      ids.add(operation.operationId)
      if (!operation.responses || !Object.keys(operation.responses).length) {
        throw new Error(`No responses: ${key}`)
      }
      if (
        !operation.security?.length ||
        operation.security.some(
          (alternative) => !Object.keys(alternative).length
        )
      ) {
        throw new Error(`Anonymous security: ${key}`)
      }
      for (const alternative of operation.security) {
        for (const scheme of Object.keys(alternative)) {
          if (!spec.components.securitySchemes[scheme]) {
            throw new Error(`Missing security scheme: ${scheme}`)
          }
        }
      }
      const parameters = operationParameters(item, operation, spec)
      for (const match of url.matchAll(/\{([^}]+)\}/g)) {
        if (
          !parameters.some(
            (parameter) =>
              parameter.in === 'path' &&
              parameter.name === match[1] &&
              parameter.required
          )
        ) {
          throw new Error(`Missing required path parameter: ${url}/${match[1]}`)
        }
      }
      const definition = advertised.find(
        (endpoint) =>
          `${endpoint.method} ${normalizePath(endpoint.path)}` === key
      )
      if (definition.nativeRoute) {
        const manifest = fs.readFileSync(
          path.join(root, definition.source),
          'utf8'
        )
        const declared = [
          ...manifest.matchAll(
            /method:\s*["'](GET|POST|HEAD)["'][^}]*?path:\s*["']([^"']+)["']/g
          ),
        ]
        if (
          !declared.some(
            (match) =>
              match[1] === definition.method &&
              normalizePath(match[2]) === normalizePath(url)
          )
        ) {
          throw new Error(`Missing native route: ${key}`)
        }
      } else if (
        !reference.endpoints.some(
          (route) =>
            `${route.method} ${normalizePath(route.path)}` === key &&
            !route.handler.includes('RelayNotImplemented')
        )
      ) {
        throw new Error(`Missing backend route: ${key}`)
      }
      const resolved = resolveSchema(operation, spec)
      for (const [code, response] of Object.entries(resolved.responses)) {
        if (!/^[1-5]\d\d$|^default$/.test(code) || !response.description) {
          throw new Error(`Invalid response: ${key}/${code}`)
        }
        for (const [contentType, media] of Object.entries(
          response.content ?? {}
        )) {
          validateMediaExamples(media, `${key}/${code}/${contentType}`)
        }
      }
      for (const [contentType, media] of Object.entries(
        resolved.requestBody?.content ?? {}
      )) {
        validateMediaExamples(media, `${key}/request/${contentType}`)
      }
    }
  }
  const missing = [...expected].filter((key) => !actual.has(key))
  if (missing.length) {
    throw new Error(`Missing public operations: ${missing.join(', ')}`)
  }
  function validateSchema(value) {
    if (!value || typeof value !== 'object') return
    if (Array.isArray(value)) {
      value.forEach(validateSchema)
      return
    }
    if (value.type === 'array' && !value.items) {
      throw new Error('Array schema missing items')
    }
    if (value.required && value.properties) {
      for (const key of value.required) {
        if (!value.properties[key]) {
          throw new Error(`Required property not defined: ${key}`)
        }
      }
    }
    Object.values(value).forEach(validateSchema)
  }
  validateSchema(spec.components.schemas)
  return {
    operations: actual.size,
    duplicateOperationIds: 0,
    unresolvedRefs: 0,
    missingPublicOperations: 0,
    operationsWithoutBackend: 0,
    internalLeaks: 0,
  }
}

function operationParameters(item, operation, spec) {
  // OpenAPI operation parameters override path-level parameters by name/location.
  const effective = new Map()
  for (const parameter of resolveSchema(
    [...(item.parameters ?? []), ...(operation.parameters ?? [])],
    spec
  )) {
    effective.set(`${parameter.in}/${parameter.name}`, parameter)
  }
  return [...effective.values()]
}

export function contractSnapshot(spec) {
  const snapshot = { version: 1, operations: {} }
  for (const [url, item] of Object.entries(spec.paths)) {
    for (const [method, operation] of Object.entries(item)) {
      if (methods.includes(method)) {
        const resolved = resolveSchema(operation, spec)
        snapshot.operations[`${method.toUpperCase()} ${url}`] = {
          operationId: operation.operationId,
          security: securityContract(operation.security ?? spec.security ?? []),
          parameters: operationParameters(item, operation, spec),
          securitySchemes: Object.fromEntries(
            Object.entries(
              resolveSchema(spec.components?.securitySchemes ?? {}, spec)
            ).map(([name, scheme]) => [
              name,
              Object.fromEntries(
                Object.entries(scheme).filter(
                  ([key]) => key !== 'description' && !key.startsWith('x-')
                )
              ),
            ])
          ),
          requestBody: resolved.requestBody,
          responses: resolved.responses,
        }
      }
    }
  }
  return snapshot
}

function securityContract(security) {
  return security
    .map((alternative) =>
      Object.fromEntries(
        Object.entries(alternative)
          .sort(([a], [b]) => a.localeCompare(b))
          .map(([scheme, scopes]) => [scheme, [...scopes].sort()])
      )
    )
    .sort((a, b) => JSON.stringify(a).localeCompare(JSON.stringify(b)))
}

export function compareContracts(before, after) {
  const changes = []
  function record(kind, at, reason) {
    changes.push({ kind, at, reason })
  }
  function compareSchema(old, next, at, request) {
    if (!old || !next) return
    if (
      old.type !== next.type ||
      old.format !== next.format ||
      Boolean(old.nullable) !== Boolean(next.nullable)
    ) {
      record('BREAKING', at, 'Type, format or nullability changed')
    }
    for (const key of Object.keys(old.properties ?? {})) {
      if (!next.properties?.[key]) {
        record(
          request ? 'POTENTIALLY_BREAKING' : 'BREAKING',
          `${at}.${key}`,
          'Field removed'
        )
      } else {
        compareSchema(
          old.properties[key],
          next.properties[key],
          `${at}.${key}`,
          request
        )
      }
    }
    for (const key of Object.keys(next.properties ?? {})) {
      if (!old.properties?.[key]) {
        record(
          request && next.required?.includes(key) ? 'BREAKING' : 'ADDITIVE',
          `${at}.${key}`,
          'Field added'
        )
      }
    }
    for (const key of next.required ?? []) {
      if (!(old.required ?? []).includes(key)) {
        record(
          request ? 'BREAKING' : 'ADDITIVE',
          `${at}.${key}`,
          'Required field added'
        )
      }
    }
    for (const key of old.required ?? []) {
      if (!(next.required ?? []).includes(key) && !request) {
        record('BREAKING', `${at}.${key}`, 'Response requirement removed')
      }
    }
    if (JSON.stringify(old.enum) !== JSON.stringify(next.enum)) {
      if (
        old.enum?.some((value) => next.enum && !next.enum.includes(value)) ||
        (!old.enum && next.enum)
      ) {
        record('BREAKING', at, 'Enum values removed or accepted input narrowed')
      } else {
        record(
          request ? 'ADDITIVE' : 'POTENTIALLY_BREAKING',
          at,
          'Enum expanded; assess client unknown-value handling'
        )
      }
    }
    for (const key of [
      'minimum',
      'maximum',
      'minLength',
      'maxLength',
      'minItems',
      'maxItems',
      'minProperties',
      'maxProperties',
      'exclusiveMinimum',
      'exclusiveMaximum',
      'multipleOf',
      'uniqueItems',
      'pattern',
      'oneOf',
      'anyOf',
      'allOf',
      'additionalProperties',
    ]) {
      if (JSON.stringify(old[key]) !== JSON.stringify(next[key])) {
        const lower = ['minimum', 'minLength', 'minItems', 'minProperties']
        const upper = ['maximum', 'maxLength', 'maxItems', 'maxProperties']
        const narrowed =
          request &&
          ((lower.includes(key) &&
            next[key] !== undefined &&
            (old[key] === undefined || next[key] > old[key])) ||
            (upper.includes(key) &&
              next[key] !== undefined &&
              (old[key] === undefined || next[key] < old[key])) ||
            (key === 'additionalProperties' &&
              next[key] === false &&
              old[key] !== false) ||
            (key === 'uniqueItems' && next[key] === true && !old[key]) ||
            (['exclusiveMinimum', 'exclusiveMaximum'].includes(key) &&
              next[key] === true &&
              old[key] !== true) ||
            (['oneOf', 'anyOf'].includes(key) &&
              old[key]?.some(
                (branch) =>
                  !next[key]?.some(
                    (value) => JSON.stringify(value) === JSON.stringify(branch)
                  )
              )) ||
            (key === 'allOf' &&
              next[key]?.some(
                (branch) =>
                  !old[key]?.some(
                    (value) => JSON.stringify(value) === JSON.stringify(branch)
                  )
              )) ||
            ([
              'exclusiveMinimum',
              'exclusiveMaximum',
              'multipleOf',
              'pattern',
            ].includes(key) &&
              old[key] === undefined &&
              next[key] !== undefined))
        record(
          narrowed ? 'BREAKING' : 'POTENTIALLY_BREAKING',
          at,
          `Constraint changed: ${key}`
        )
      }
    }
    compareSchema(old.items, next.items, `${at}[]`, request)
  }
  for (const [key, old] of Object.entries(before.operations)) {
    const next = after.operations[key]
    if (!next) {
      record('BREAKING', key, 'Operation removed')
      continue
    }
    if (JSON.stringify(old.security) !== JSON.stringify(next.security)) {
      record('BREAKING', key, 'Security requirements changed')
    }
    for (const scheme of new Set(
      [...old.security, ...next.security].flatMap((entry) => Object.keys(entry))
    )) {
      if (
        JSON.stringify(old.securitySchemes?.[scheme]) !==
        JSON.stringify(next.securitySchemes?.[scheme])
      ) {
        record('BREAKING', key, `Security scheme changed: ${scheme}`)
      }
    }
    for (const parameter of old.parameters) {
      if (
        !next.parameters.some(
          (value) => value.in === parameter.in && value.name === parameter.name
        )
      ) {
        record(
          'BREAKING',
          key,
          `Parameter removed: ${parameter.in}/${parameter.name}`
        )
      }
    }
    if (old.operationId !== next.operationId) {
      record('POTENTIALLY_BREAKING', key, 'SDK operationId renamed')
    }
    for (const parameter of next.parameters) {
      const previous = old.parameters.find(
        (value) => value.in === parameter.in && value.name === parameter.name
      )
      if (!previous) {
        record(
          parameter.required ? 'BREAKING' : 'ADDITIVE',
          key,
          `Parameter added: ${parameter.in}/${parameter.name}`
        )
      } else {
        if (parameter.required && !previous.required) {
          record(
            'BREAKING',
            key,
            `Parameter became required: ${parameter.name}`
          )
        }
        compareSchema(
          previous.schema,
          parameter.schema,
          `${key}/parameter/${parameter.name}`,
          true
        )
      }
    }
    if (next.requestBody?.required && !old.requestBody?.required) {
      record('BREAKING', key, 'Request body became required')
    }
    for (const [type, media] of Object.entries(
      old.requestBody?.content ?? {}
    )) {
      if (!next.requestBody?.content?.[type]) {
        record('BREAKING', key, `Request content type removed: ${type}`)
      } else {
        compareSchema(
          media.schema,
          next.requestBody.content[type].schema,
          `${key}/request/${type}`,
          true
        )
      }
    }
    for (const [code, response] of Object.entries(old.responses)) {
      if (!next.responses[code]) {
        record(
          /^(2\d\d|101)$/.test(code) ? 'BREAKING' : 'POTENTIALLY_BREAKING',
          key,
          `Response status removed: ${code}`
        )
        continue
      }
      for (const header of Object.keys(next.responses[code].headers ?? {})) {
        if (!response.headers?.[header]) {
          record(
            'ADDITIVE',
            `${key}/response/${code}`,
            `Response header added: ${header}`
          )
        }
      }
      for (const [type, media] of Object.entries(response.content ?? {})) {
        if (!next.responses[code].content?.[type]) {
          record('BREAKING', key, `Response content type removed: ${type}`)
        } else {
          compareSchema(
            media.schema,
            next.responses[code].content[type].schema,
            `${key}/response/${code}/${type}`,
            false
          )
        }
      }
    }
  }
  for (const key of Object.keys(after.operations)) {
    if (!before.operations[key]) record('ADDITIVE', key, 'Operation added')
  }
  return changes
}

// PRs compare to their merge base, not a snapshot editable by the same PR.
// With no argument, local verification compares against the committed HEAD.
export function readCommittedContract(root, baseRef = 'HEAD') {
  const commit = execFileSync('git', ['merge-base', baseRef, 'HEAD'], {
    cwd: root,
    encoding: 'utf8',
  }).trim()
  const raw = execFileSync(
    'git',
    ['show', `${commit}:docs/openapi/public.json`],
    { cwd: root, encoding: 'utf8', maxBuffer: 16 * 1024 * 1024 }
  )
  return { commit, snapshot: contractSnapshot(parsePublicSpec(raw)) }
}

if (
  process.argv[1] &&
  path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)
) {
  const root = fileURLToPath(new URL('../../', import.meta.url))
  const spec = parsePublicSpec(
    fs.readFileSync(path.join(root, 'docs/openapi/public.json'), 'utf8')
  )
  const { publicApiEndpoints } =
    await import('../src/features/documentation/api-reference-data.ts')
  const reference = JSON.parse(
    fs.readFileSync(
      path.join(
        root,
        'web/src/features/documentation/generated/reference.json'
      ),
      'utf8'
    )
  )
  console.log(
    JSON.stringify(
      validatePublicContract(spec, publicApiEndpoints, reference, root),
      null,
      2
    )
  )
  const baseIndex = process.argv.indexOf('--base-ref')
  const base = baseIndex < 0 ? 'HEAD' : process.argv[baseIndex + 1]
  if (!base || base.startsWith('-')) {
    throw new Error('Expected committed --base-ref')
  }
  const baseline = readCommittedContract(root, base)
  console.log(`Committed public contract baseline: ${baseline.commit}`)
  const changes = compareContracts(baseline.snapshot, contractSnapshot(spec))
  console.log(JSON.stringify({ compatibility: changes }, null, 2))
  for (const change of changes.filter(
    (entry) => entry.kind === 'POTENTIALLY_BREAKING'
  )) {
    console.warn(`::warning::${change.at}: ${change.reason}`)
  }
  if (changes.some((change) => change.kind === 'BREAKING')) {
    throw new Error(
      'Breaking public contract change: preserve the existing version or introduce a reviewed new version; updating the working-tree snapshot cannot bypass this gate'
    )
  }
}
