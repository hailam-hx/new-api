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

import type { TaskChannelDiagnostic } from '../types'

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
  if (reason === 'live_canary_requires_input') return t('Input required')
  if (reason === 'live_canary_requires_fixture') return t('Fixture required')
  if (reason === 'live_canary_required') return t('Live canary required')
  if (
    reason === 'PLUGIN_PRICE_NOT_FOUND' ||
    reason === 'MODEL_PRICE_NOT_CONFIGURED'
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
  return reason ?? ''
}

export type ChannelTestExportProps = {
  channelId: number
  channelName: string
  models: string[]
  results: Record<string, ChannelTestResult>
  disabled?: boolean
}

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
    ],
  ]
  for (const model of props.models) {
    const result = props.results[model]
    rows.push([
      props.channelId,
      props.channelName,
      model,
      statuses[result?.status ?? 'idle'],
      result?.responseTime ?? '',
      result?.completedAt ? new Date(result.completedAt).toISOString() : '',
      result?.endpointType ?? '',
      result?.stream ?? '',
      result?.errorCode ?? '',
      result?.error ?? '',
      result?.diagnostic && result.diagnostic.mode !== 'runtime'
        ? 0
        : t('Unknown'),
      result?.diagnostic?.mode ?? '',
      result?.diagnostic?.outcome ?? '',
      result?.diagnostic?.status ?? '',
      result?.diagnostic?.connectivity_tested ?? '',
      result?.diagnostic?.live_generation_tested ?? '',
      result?.diagnostic?.plugin ?? '',
      result?.diagnostic?.checks.find((check) => check.check === 'pricing')
        ?.billing_source ?? '',
      result?.diagnostic ? JSON.stringify(result.diagnostic.checks) : '',
      result?.diagnostic?.connectivity_status ?? '',
      result?.diagnostic?.connectivity_latency_ms ?? '',
      result?.diagnostic?.credential_identity ?? '',
      result?.diagnostic?.model_access ?? '',
      result?.diagnostic?.catalog_model ?? '',
      result?.diagnostic?.checks.find((check) => check.check === 'connectivity')
        ?.evidence ?? '',
      result?.diagnostic?.endpoint ?? '',
      result?.diagnostic?.protocol ?? '',
      result?.diagnostic?.resolved_upstream_model ??
        result?.diagnostic?.mapped_model ??
        '',
      result?.diagnostic ? JSON.stringify(result.diagnostic) : '',
    ])
  }
  const content = rows
    .map((row) =>
      row
        .map((value) => {
          let text = String(value)
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
