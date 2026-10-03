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
import type {
  ChannelVerificationItem,
  ChannelVerificationRun,
  VerificationLayerStatus,
} from '../types'

/** Runtime requires accepted submission, valid terminal output, and parser facts. */
export function verificationRuntimeStatus(
  item: ChannelVerificationItem
): VerificationLayerStatus {
  const states = [item.generation_status, item.parser_status]
  for (const state of ['FAIL', 'AMBIGUOUS', 'BLOCKED'] as const) {
    if (states.includes(state)) return state
  }
  return states.every((state) => state === 'PASS') ? 'PASS' : 'NOT_TESTED'
}

export function verificationTimestamp(value?: number | string): string {
  if (value === undefined || value === '' || value === 0) return ''
  const date =
    typeof value === 'number' ? new Date(value * 1000) : new Date(value)
  return Number.isNaN(date.getTime()) ? '' : date.toISOString()
}

/** Defense in depth for evidence returned by older or external verification runs. */
export function redactVerificationEvidence(value: string): string {
  if (!/^[\s]*[[{]/.test(value)) return redactEvidenceText(value)
  try {
    const parsed: unknown = JSON.parse(value)
    return JSON.stringify(redactEvidenceValue(parsed), null, 2)
  } catch {
    return redactEvidenceText(value)
  }
}

function redactEvidenceValue(value: unknown): unknown {
  if (typeof value === 'string') return redactEvidenceText(value)
  if (Array.isArray(value)) return value.map(redactEvidenceValue)
  if (!value || typeof value !== 'object') return value
  return Object.fromEntries(
    Object.entries(value).map(([key, entry]) => {
      const normalized = key.toLowerCase().replaceAll('-', '_')
      const secret =
        /^(?:authorization|proxy_authorization|cookie|set_cookie|api_key|apikey|key|token|access_token|refresh_token|id_token|password|secret|client_secret|private_key|idempotency_key|credential|credentials|signature|signed_url)$/.test(
          normalized
        ) || /(?:^|_)(?:secret|password|api_key|private_key)$/.test(normalized)
      return [key, secret ? '[REDACTED]' : redactEvidenceValue(entry)]
    })
  )
}

function redactEvidenceText(value: string): string {
  return value
    .replaceAll(/\bBearer\s+[^\s"'<>]+/gi, 'Bearer [REDACTED]')
    .replaceAll(/\bsk[-_][a-zA-Z0-9_-]+/g, '[REDACTED]')
    .replaceAll(
      /([?&](?:token|key|api_key|access_token|signature|sig|credential|x-amz-signature|x-amz-credential)=)[^&\s"'<>]*/gi,
      '$1[REDACTED]'
    )
    .replaceAll(/(https?:\/\/)[^/\s@]+@/gi, '$1[REDACTED]@')
}

/** Explicit allowlist prevents additional API fields from leaking into UI/export. */
export function verificationEvidenceRecord(
  item: ChannelVerificationItem,
  run?: ChannelVerificationRun | null,
  scope: 'current' | 'historical' = 'current'
): Record<string, string | number> {
  const evidence: Record<string, string | number> = {
    evidence_scope: scope,
    historical_runtime_state: item.historical_runtime_state ?? '',
    historical_reason_code: item.historical_reason_code ?? '',
    historical_run_id: item.historical_run_id ?? '',
    current_canary_readiness: item.current_canary_readiness ?? '',
    model: item.model,
    protocol: item.protocol,
    operation: item.operation,
    mode: item.mode,
    fixture_id: item.fixture_id,
    endpoint: item.endpoint,
    config_state: item.config_status,
    connectivity_state: item.connectivity_status,
    runtime_state: verificationRuntimeStatus(item),
    request_state: item.request_status,
    generation_state: item.generation_status,
    parser_state: item.parser_status,
    usage_state: item.parser_status,
    fixture_hash: item.fixture_hash ?? '',
    config_hash: item.config_hash ?? '',
    warning_codes: (item.warning_codes ?? []).join(';'),
    current_run_id: item.run_id ?? run?.id ?? '',
    billing_state: item.billing_status,
    ledger_state: item.ledger_status,
    runtime_evidence_status: item.status ?? item.result ?? '',
    reason_code: item.reason_code ?? '',
    catalog_hash: item.catalog_hash ?? run?.catalog_hash ?? '',
    pricing_snapshot_hash: item.pricing_snapshot_hash ?? '',
    billing_expr_hash: item.billing_expr_hash ?? '',
    billing_source: item.billing_source ?? '',
    idempotency_key_hash: item.idempotency_key_hash ?? '',
    request_id: item.request_id ?? '',
    task_id: item.task_id ?? '',
    trace_id: item.trace_id ?? '',
    terminal_status: item.terminal_status ?? '',
    normalized_usage: item.normalized_usage_json ?? '',
    provider_usage: item.provider_usage_json ?? '',
    provider_unit_count: item.provider_unit_count ?? '',
    provider_cost_points: item.provider_cost_points ?? '',
    newapi_quota: item.newapi_quota ?? '',
    newapi_cost: item.newapi_raw_cost ?? '',
    wallet_delta: item.wallet_delta ?? '',
    result: item.result ?? '',
    correlation_quality: item.correlation_quality ?? '',
    verified_at: verificationTimestamp(item.verified_at),
    evidence: item.evidence_json ?? '',
    run_id: item.run_id ?? run?.id ?? '',
  }
  for (const [key, value] of Object.entries(evidence)) {
    if (typeof value === 'string') {
      evidence[key] = redactVerificationEvidence(value)
    }
  }
  return evidence
}
