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
import type { TFunction } from 'i18next'

import type { ChannelVerificationData, TaskChannelDiagnostic } from '../types'
import {
  redactVerificationEvidence,
  verificationEvidenceRecord,
} from './channel-verification'

export type ChannelTestResult = {
  status:
    | 'idle'
    | 'testing'
    | 'success'
    | 'error'
    | 'pass'
    | 'partial'
    | 'untested'
  diagnostic?: TaskChannelDiagnostic
  responseTime?: number
  completedAt?: number
  error?: string
  errorCode?: string
  endpointType?: string
  stream?: boolean
}

/** Used by both hide and delete failed-model actions and batch counts. */
export function isFailedChannelTest(result?: ChannelTestResult): boolean {
  if (result?.diagnostic) return result.diagnostic.outcome === 'fail'
  return result?.status === 'error'
}

export function taskDiagnosticReasonLabel(
  reason: string | undefined,
  t: TFunction
): string {
  const originalReason = reason
  if (reason === 'UPSTREAM_HTTP_ERROR') return t('Upstream returned an error')
  if (reason === 'DFLOP_DEPLOYMENT_POOL_UNAVAILABLE') {
    return t('DFLOP has no healthy deployment for this model')
  }
  if (reason === 'DFLOP_NO_HEALTHY_CHANNEL') {
    return t('DFLOP has no healthy channel for this model')
  }
  if (
    reason === 'INSUFFICIENT_EVIDENCE' ||
    reason === 'runtime_failure_unclassified'
  ) {
    return t(
      'Upstream result is inconclusive; exact request evidence is required'
    )
  }
  reason = reason?.toLowerCase()
  if (reason === 'live_canary_requires_input') return t('Input required')
  if (reason === 'live_canary_requires_fixture') return t('Fixture required')
  if (reason === 'live_canary_required') return t('Live canary required')
  if (
    reason === 'plugin_price_not_found' ||
    reason === 'model_price_not_configured'
  ) {
    return t('Pricing not ready')
  }
  if (reason === 'credential_structure_not_verified') {
    return t('Credential structure not verified')
  }
  if (reason === 'connectivity_pass') return t('DFLOP connectivity verified')
  if (reason === 'connectivity_unavailable') {
    return t('Safe connectivity check unavailable')
  }
  if (reason === 'connectivity_not_tested') return t('Connectivity not tested')
  if (reason === 'model_access_not_confirmed') {
    return t('Model access not confirmed for this API key')
  }
  if (reason === 'auth_failed') return t('DFLOP authentication failed')
  if (reason === 'upstream_unreachable') return t('DFLOP endpoint unreachable')
  if (reason === 'upstream_rate_limited') return t('DFLOP rate limit reached')
  if (reason === 'upstream_service_error') return t('DFLOP service error')
  if (reason === 'connectivity_response_invalid') {
    return t('Invalid DFLOP catalog response')
  }
  if (
    reason === 'credential_selection_required' ||
    reason === 'credential_selection_invalid'
  ) {
    return t('Select an enabled API key')
  }
  return originalReason ?? ''
}

export type ChannelTestExportProps = {
  channelId: number
  channelName: string
  models: string[]
  results: Record<string, ChannelTestResult>
  verification?: ChannelVerificationData
  disabled?: boolean
}

const verificationExportFields = [
  'evidence_scope',
  'model',
  'operation',
  'fixture_id',
  'config_state',
  'connectivity_state',
  'runtime_state',
  'request_state',
  'generation_state',
  'parser_state',
  'usage_state',
  'historical_runtime_state',
  'historical_reason_code',
  'historical_run_id',
  'current_canary_readiness',
  'fixture_hash',
  'config_hash',
  'warning_codes',
  'current_run_id',
  'billing_state',
  'ledger_state',
  'runtime_evidence_status',
  'reason_code',
  'catalog_hash',
  'pricing_snapshot_hash',
  'billing_expr_hash',
  'idempotency_key_hash',
  'request_id',
  'task_id',
  'trace_id',
  'terminal_status',
  'normalized_usage',
  'provider_usage',
  'provider_unit_count',
  'provider_cost_points',
  'newapi_quota',
  'newapi_cost',
  'wallet_delta',
  'result',
  'correlation_quality',
  'verified_at',
  'run_id',
] as const

export function createChannelTestCSV(
  props: ChannelTestExportProps,
  t: TFunction
): string {
  const statuses = {
    idle: t('Not tested'),
    testing: t('Testing'),
    success: t('Success'),
    error: t('Failed'),
    pass: t('Preflight valid'),
    partial: t('Partial verification'),
    untested: t('Upstream not tested'),
  }
  const rows: (string | number | boolean)[][] = [
    [
      t('Channel ID'),
      t('Name'),
      t('Model'),
      t('Status'),
      t('Response time (s)'),
      t('Completed at (UTC)'),
      t('Endpoint Type'),
      t('Stream Mode'),
      t('Reason code'),
      t('Details'),
      t('Test cost'),
      'mode',
      'outcome',
      'diagnostic_status',
      'connectivity_tested',
      'live_generation_tested',
      'plugin',
      'billing_source',
      'checks',
      'connectivity_status',
      'connectivity_latency_ms',
      'credential_identity',
      'model_access',
      'catalog_model',
      'evidence',
      'endpoint',
      'protocol',
      'canonical_model',
      'diagnostic',
      ...verificationExportFields,
    ],
  ]
  const persistedItems = props.verification?.items ?? []
  const allModels = [
    ...new Set([...props.models, ...persistedItems.map((item) => item.model)]),
  ]
  for (const model of allModels) {
    const result = props.results[model]
    const items = persistedItems
      .filter((item) => item.model === model)
      .flatMap((item) => [
        { item, scope: 'current' as const },
        ...(item.historical_evidence
          ? [{ item: item.historical_evidence, scope: 'historical' as const }]
          : []),
      ])
    for (const { item, scope } of items.length
      ? items
      : [{ item: undefined, scope: 'current' as const }]) {
      const evidence = item
        ? verificationEvidenceRecord(item, props.verification?.run, scope)
        : undefined
      rows.push([
        props.channelId,
        props.channelName,
        model,
        evidence?.runtime_evidence_status ?? statuses[result?.status ?? 'idle'],
        result?.responseTime ?? '',
        result?.completedAt ? new Date(result.completedAt).toISOString() : '',
        result?.endpointType ?? '',
        result?.stream ?? '',
        result?.errorCode ?? '',
        result?.error ?? '',
        result?.diagnostic && result.diagnostic.mode !== 'runtime' ? 0 : '',
        evidence?.mode ?? result?.diagnostic?.mode ?? '',
        result?.diagnostic?.outcome ?? '',
        result?.diagnostic?.status ?? '',
        result?.diagnostic?.connectivity_tested ?? '',
        result?.diagnostic?.live_generation_tested ?? '',
        result?.diagnostic?.plugin ?? '',
        evidence?.billing_source ??
          result?.diagnostic?.checks.find((check) => check.check === 'pricing')
            ?.billing_source ??
          '',
        result?.diagnostic ? JSON.stringify(result.diagnostic.checks) : '',
        result?.diagnostic?.connectivity_status ?? '',
        result?.diagnostic?.connectivity_latency_ms ?? '',
        result?.diagnostic?.credential_identity ?? '',
        result?.diagnostic?.model_access ?? '',
        result?.diagnostic?.catalog_model ?? '',
        evidence?.evidence ??
          result?.diagnostic?.checks.find(
            (check) => check.check === 'connectivity'
          )?.evidence ??
          '',
        evidence?.endpoint ?? result?.diagnostic?.endpoint ?? '',
        evidence?.protocol ?? result?.diagnostic?.protocol ?? '',
        result?.diagnostic?.resolved_upstream_model ??
          result?.diagnostic?.mapped_model ??
          '',
        result?.diagnostic ? JSON.stringify(result.diagnostic) : '',
        ...verificationExportFields.map((field) => evidence?.[field] ?? ''),
      ])
    }
  }
  const content = rows
    .map((row) =>
      row
        .map((value) => {
          let text = redactVerificationEvidence(String(value))
          // Upstream/model text must remain literal when opened in spreadsheet apps.
          if (/^[\s]*[=+\-@]/.test(text) || /^[\t\r\n]/.test(text)) {
            text = `'${text}`
          }
          return `"${text.replaceAll('"', '""')}"`
        })
        .join(',')
    )
    .join('\r\n')
  return `\uFEFF${content}\r\n`
}
