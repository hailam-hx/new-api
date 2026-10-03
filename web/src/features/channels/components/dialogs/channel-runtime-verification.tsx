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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import {
  StaticDataTable,
  type StaticDataTableColumn,
} from '@/components/data-table'
import { Dialog } from '@/components/dialog'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { handleServerError } from '@/lib/handle-server-error'

import { getChannelVerification, prepareChannelVerification } from '../../api'
import type { ChannelTestResult } from '../../lib/channel-test-export'
import {
  redactVerificationEvidence,
  verificationEvidenceRecord,
  verificationRuntimeStatus,
  verificationTimestamp,
} from '../../lib/channel-verification'
import type {
  ChannelVerificationItem,
  VerificationLayerStatus,
} from '../../types'
import { ChannelTestExport } from './channel-test-export'

type ChannelRuntimeVerificationProps = {
  channelId: number
  channelName: string
  open: boolean
  models: string[]
  results: Record<string, ChannelTestResult>
  disabled?: boolean
}

export function ChannelRuntimeVerification(
  props: ChannelRuntimeVerificationProps
) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [evidenceSection, setEvidenceSection] = useState<
    'all' | 'ids' | 'billing'
  >('all')
  const [selectedId, setSelectedId] = useState<number | null>(null)
  const queryKey = ['channels', props.channelId, 'runtime-verification']
  const query = useQuery({
    queryKey,
    queryFn: async ({ signal }) =>
      (await getChannelVerification(props.channelId, undefined, signal)).data,
    enabled: props.open,
    staleTime: 0,
    retry: false,
    meta: { errorToast: false },
  })
  const prepare = useMutation({
    mutationFn: async (model?: string) =>
      (await prepareChannelVerification(props.channelId, model)).data,
    retry: false,
    onSuccess: async (data) => {
      setSelectedId(null)
      queryClient.setQueryData(queryKey, data)
      await queryClient.invalidateQueries({ queryKey })
    },
    onError: (error) =>
      handleServerError(error, t('Verification preparation failed')),
  })
  const currentSelectedItem = query.data?.items
    .flatMap((item) =>
      item.historical_evidence ? [item, item.historical_evidence] : [item]
    )
    .find((item) => item.id === selectedId)
  const runId = currentSelectedItem?.run_id ?? query.data?.run?.id
  const evidenceQuery = useQuery({
    queryKey: [...queryKey, 'run', runId],
    queryFn: async ({ signal }) =>
      (await getChannelVerification(props.channelId, runId, signal)).data,
    enabled: props.open && selectedId !== null && runId !== undefined,
    staleTime: 0,
    retry: false,
    meta: { errorToast: false },
  })
  const selectedItem = evidenceQuery.data?.items.find(
    (item) => item.id === selectedId
  )
  const evidence = selectedItem
    ? JSON.stringify(
        Object.fromEntries(
          Object.entries(
            verificationEvidenceRecord(
              selectedItem,
              evidenceQuery.data?.run,
              query.data?.items.some(
                (item) => item.historical_evidence?.id === selectedId
              )
                ? 'historical'
                : 'current'
            )
          ).filter(
            ([key]) =>
              evidenceSection === 'all' ||
              (evidenceSection === 'ids'
                ? [
                    'model',
                    'protocol',
                    'mode',
                    'request_id',
                    'task_id',
                    'trace_id',
                    'correlation_quality',
                    'run_id',
                    'evidence_scope',
                  ].includes(key)
                : [
                    'model',
                    'billing_state',
                    'usage_state',
                    'ledger_state',
                    'billing_expr_hash',
                    'pricing_snapshot_hash',
                    'provider_usage',
                    'normalized_usage',
                    'provider_cost_points',
                    'newapi_quota',
                    'wallet_delta',
                    'evidence',
                    'run_id',
                    'evidence_scope',
                  ].includes(key))
          )
        ),
        null,
        2
      )
    : ''
  const isBusy = props.disabled || prepare.isPending || query.isFetching
  const columns: StaticDataTableColumn<ChannelVerificationItem>[] = [
    {
      id: 'model',
      header: t('Model'),
      className: 'min-w-52',
      cell: (item) => (
        <div className='space-y-1'>
          <p className='font-medium'>{item.model}</p>
          <p className='text-muted-foreground text-xs'>
            {[item.protocol, item.operation, item.mode]
              .filter(Boolean)
              .join(' · ')}
          </p>
          {item.historical_runtime_state ===
            'HISTORICAL_RUNTIME_UNRECOVERABLE' && (
            <p className='text-muted-foreground text-xs'>
              {t('Historical result unrecoverable')}:{' '}
              {item.historical_reason_code}
            </p>
          )}
          {item.current_canary_readiness === 'READY_FOR_FRESH_CANARY' && (
            <p className='text-muted-foreground text-xs'>
              {t('Fresh verification required')}
            </p>
          )}
        </div>
      ),
    },
    {
      id: 'config',
      header: t('Config'),
      cell: (item) => (
        <VerificationBadge label={t('Config')} status={item.config_status} />
      ),
    },
    {
      id: 'connectivity',
      header: t('Connectivity'),
      cell: (item) => (
        <VerificationBadge
          label={t('Connectivity')}
          status={item.connectivity_status}
        />
      ),
    },
    {
      id: 'request',
      header: t('Request'),
      cell: (item) => (
        <VerificationBadge label={t('Request')} status={item.request_status} />
      ),
    },
    {
      id: 'usage',
      header: t('Usage'),
      cell: (item) => (
        <VerificationBadge label={t('Usage')} status={item.parser_status} />
      ),
    },
    {
      id: 'runtime',
      header: t('Runtime'),
      cell: (item) => (
        <VerificationBadge
          label={t('Runtime')}
          status={verificationRuntimeStatus(item)}
        />
      ),
    },
    {
      id: 'billing',
      header: t('Billing'),
      cell: (item) => (
        <VerificationBadge label={t('Billing')} status={item.billing_status} />
      ),
    },
    {
      id: 'ledger',
      header: t('Ledger'),
      cell: (item) => (
        <VerificationBadge label={t('Ledger')} status={item.ledger_status} />
      ),
    },
    {
      id: 'evidence',
      header: t('Runtime evidence'),
      className: 'min-w-80',
      cell: (item) => {
        const layers = [
          [t('Config'), item.config_status],
          [t('Connectivity'), item.connectivity_status],
          [t('Request'), item.request_status],
          [t('Generation'), item.generation_status],
          [t('Parser'), item.parser_status],
          [t('Billing'), item.billing_status],
          [t('Ledger'), item.ledger_status],
        ]
        const openLayers = layers
          .filter(([, state]) => state !== 'PASS')
          .map(([label, state]) => `${label}: ${state}`)
          .join(' · ')
        return (
          <div className='space-y-1 text-xs'>
            <p className='font-mono wrap-break-word'>
              {item.status ?? item.result}
            </p>
            {item.reason_code && (
              <p className='font-mono wrap-break-word'>
                {redactVerificationEvidence(item.reason_code)}
              </p>
            )}
            {openLayers && (
              <p className='text-muted-foreground whitespace-normal'>
                {t('Open layers: {{layers}}', { layers: openLayers })}
              </p>
            )}
            <p>
              {t('Last verified at')}:{' '}
              {verificationTimestamp(item.verified_at) || t('Not verified')}
            </p>
            <p className='font-mono wrap-break-word'>
              {redactVerificationEvidence(item.endpoint)}
            </p>
            <p className='wrap-break-word'>
              {t('Catalog hash')}:{' '}
              <span className='font-mono'>
                {item.catalog_hash ?? query.data?.run?.catalog_hash ?? '-'}
              </span>
            </p>
            <p className='wrap-break-word'>
              {t('Billing source')}:{' '}
              {redactVerificationEvidence(item.billing_source ?? '') || '-'}
            </p>
            {item.request_id && (
              <p className='font-mono wrap-break-word'>
                {t('Request ID')}: {redactVerificationEvidence(item.request_id)}
              </p>
            )}
            {item.task_id && (
              <p className='font-mono wrap-break-word'>
                {t('Task ID')}: {redactVerificationEvidence(item.task_id)}
              </p>
            )}
            {item.historical_evidence && (
              <div className='text-muted-foreground space-y-1 border-t pt-2'>
                <p>
                  {t('Historical verification: {{status}}', {
                    status:
                      item.historical_evidence.status ??
                      item.historical_evidence.result,
                  })}
                </p>
                <p>
                  {t('Last verified at')}:{' '}
                  {verificationTimestamp(
                    item.historical_evidence.verified_at
                  ) || t('Not verified')}
                </p>
                <p className='wrap-break-word'>
                  {t('Catalog hash')}:{' '}
                  <span className='font-mono'>
                    {item.historical_evidence.catalog_hash || '-'}
                  </span>
                </p>
              </div>
            )}
          </div>
        )
      },
    },
    {
      id: 'actions',
      header: t('Actions'),
      className: 'min-w-44',
      cell: (item) => (
        <div className='flex flex-col gap-1'>
          <Button
            variant='ghost'
            size='sm'
            disabled={
              item.id === undefined || !(item.run_id ?? query.data?.run?.id)
            }
            aria-label={t('View evidence for {{model}}', { model: item.model })}
            aria-haspopup='dialog'
            onClick={() => {
              setEvidenceSection('all')
              setSelectedId(item.id ?? null)
            }}
          >
            {t('View evidence')}
          </Button>
          <Button
            variant='ghost'
            size='sm'
            disabled={item.id === undefined}
            onClick={() => {
              setEvidenceSection('ids')
              setSelectedId(item.id ?? null)
            }}
          >
            {t('View exact IDs')}
          </Button>
          <Button
            variant='ghost'
            size='sm'
            disabled={item.id === undefined}
            onClick={() => {
              setEvidenceSection('billing')
              setSelectedId(item.id ?? null)
            }}
          >
            {t('View billing reconciliation')}
          </Button>
          {item.historical_evidence && (
            <Button
              variant='ghost'
              size='sm'
              disabled={
                item.historical_evidence.id === undefined ||
                !item.historical_evidence.run_id
              }
              aria-label={t('View previous evidence for {{model}}', {
                model: item.model,
              })}
              aria-haspopup='dialog'
              onClick={() => (
                setEvidenceSection('all'),
                setSelectedId(item.historical_evidence?.id ?? null)
              )}
            >
              {t('View previous evidence')}
            </Button>
          )}
          <Button
            variant='outline'
            size='sm'
            disabled={isBusy}
            aria-label={t('Re-run verification for {{model}}', {
              model: item.model,
            })}
            onClick={() => prepare.mutate(item.model)}
          >
            {t('Re-run verification')}
          </Button>
        </div>
      ),
    },
  ]
  return (
    <section className='space-y-3' aria-label={t('Persisted verification')}>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <h3 className='text-sm font-medium'>{t('Current evidence')}</h3>
        <Button
          variant='outline'
          size='sm'
          disabled={isBusy}
          onClick={() => prepare.mutate(undefined)}
        >
          {t('Re-run verification')}
        </Button>
      </div>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Re-run checks configuration and free connectivity only. Paid generation requires an approved authorization manifest.'
        )}
      </p>
      {query.data?.run && (
        <p className='text-muted-foreground text-xs wrap-break-word'>
          {t('Catalog hash')}:{' '}
          <span className='font-mono'>{query.data.run.catalog_hash}</span>
        </p>
      )}
      {query.isPending && (
        <LoadingState
          className='min-h-0 py-4'
          message={t('Loading verification evidence...')}
        />
      )}
      {query.isError && (
        <ErrorState
          className='min-h-0 p-3'
          title={t('Failed to load verification evidence')}
          onRetry={() => {
            void query.refetch()
          }}
        />
      )}
      {!query.isPending && !query.isError && (
        <div className='max-h-80 overflow-auto'>
          <StaticDataTable
            data={query.data?.items ?? []}
            columns={columns}
            getRowKey={(item, index) => item.id ?? index}
            tableClassName='w-max min-w-full'
            containerProps={{
              role: 'region',
              'aria-label': t('Verification layers'),
            }}
            emptyContent={t('No persisted verification evidence')}
          />
        </div>
      )}
      <ChannelTestExport
        channelId={props.channelId}
        channelName={props.channelName}
        models={props.models}
        results={props.results}
        verification={query.isError ? undefined : query.data}
        disabled={isBusy || query.isError}
      />
      <Dialog
        open={props.open && selectedId !== null}
        onOpenChange={(open) => {
          if (!open) setSelectedId(null)
        }}
        title={t('Verification evidence: {{model}}', {
          model: selectedItem?.model ?? currentSelectedItem?.model ?? '',
        })}
        contentClassName='sm:max-w-3xl'
        bodyClassName='space-y-3'
        footer={
          <Button variant='outline' onClick={() => setSelectedId(null)}>
            {t('Close')}
          </Button>
        }
      >
        {evidenceQuery.isPending && (
          <LoadingState message={t('Loading verification evidence...')} />
        )}
        {evidenceQuery.isError && (
          <ErrorState
            title={t('Failed to load verification evidence')}
            onRetry={() => {
              void evidenceQuery.refetch()
            }}
          />
        )}
        {!evidenceQuery.isPending &&
          !evidenceQuery.isError &&
          !selectedItem && (
            <p className='text-muted-foreground text-sm'>
              {t('No persisted verification evidence')}
            </p>
          )}
        {!evidenceQuery.isError && selectedItem && (
          <>
            <CopyButton
              value={evidence}
              size='sm'
              variant='outline'
              aria-label={t('Copy verification evidence')}
            >
              {t('Copy verification evidence')}
            </CopyButton>
            <pre className='bg-muted max-h-[60vh] overflow-auto rounded-md p-3 text-xs wrap-break-word whitespace-pre-wrap'>
              {evidence}
            </pre>
          </>
        )}
      </Dialog>
    </section>
  )
}

function VerificationBadge(props: {
  label: string
  status: VerificationLayerStatus
}) {
  let variant: 'success' | 'danger' | 'warning' | 'neutral' = 'neutral'
  if (props.status === 'PASS') variant = 'success'
  if (props.status === 'FAIL') variant = 'danger'
  if (props.status === 'BLOCKED' || props.status === 'AMBIGUOUS') {
    variant = 'warning'
  }
  return (
    <StatusBadge
      label={`${props.label}: ${props.status}`}
      variant={variant}
      copyable={false}
    />
  )
}
