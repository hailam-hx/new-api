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
import { useMutation } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { CopyButton } from '@/components/copy-button'
import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import { Textarea } from '@/components/ui/textarea'
import { handleServerError } from '@/lib/handle-server-error'

import {
  executeChannelVerification,
  planChannelVerification,
  resumeChannelVerification,
} from '../../api'
import type {
  ChannelVerificationData,
  ChannelVerificationPlan,
} from '../../types'

export function ChannelRuntimeAction(props: {
  channelId: number
  models: string[]
  verification?: ChannelVerificationData
  disabled: boolean
  onComplete: (data: ChannelVerificationData) => void
  onBusyChange: (busy: boolean) => void
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [model, setModel] = useState<string | null>(null)
  const [planJSON, setPlanJSON] = useState('')
  const [manifestJSON, setManifestJSON] = useState('')
  const [confirming, setConfirming] = useState(false)
  const [attempted, setAttempted] = useState(false)
  const [reason, setReason] = useState('')
  let plan: ChannelVerificationPlan | undefined
  let approved:
    | {
        approved: boolean
        signature: string
        expires_at: number
        max_requests: number
        max_total_provider_points: string
        targets: {
          model: string
          protocol: string
          mode: string
          fixture_id: string
          maximum_provider_points: string
        }[]
      }
    | undefined
  try {
    const value = JSON.parse(planJSON) as ChannelVerificationPlan
    if (
      typeof value.plan_version === 'string' &&
      Array.isArray(value.targets) &&
      value.targets.every(
        (target) =>
          target &&
          typeof target.model === 'string' &&
          target.fixture &&
          typeof target.fixture.id === 'string' &&
          typeof target.fixture.protocol === 'string' &&
          typeof target.fixture.mode === 'string'
      )
    ) {
      plan = value
    }
  } catch {
    /* An incomplete import remains non-executable. */
  }
  try {
    const value = JSON.parse(manifestJSON) as typeof approved
    if (
      value &&
      value.approved === true &&
      typeof value.signature === 'string' &&
      value.signature &&
      Array.isArray(value.targets) &&
      value.targets.length > 0 &&
      value.targets.every(
        (target) =>
          target &&
          typeof target.model === 'string' &&
          typeof target.fixture_id === 'string' &&
          typeof target.protocol === 'string' &&
          typeof target.mode === 'string' &&
          typeof target.maximum_provider_points === 'string'
      ) &&
      value.targets.length === value.max_requests &&
      value.expires_at > Date.now() / 1000
    ) {
      approved = value
    }
  } catch {
    /* Server validation remains authoritative. */
  }
  const prepare = useMutation({
    mutationFn: async () => {
      if (!model) throw new Error(t('Select model'))
      return planChannelVerification(props.channelId, model)
    },
    retry: false,
    onSuccess: (data) => {
      setPlanJSON(JSON.stringify(data, null, 2))
      setManifestJSON('')
      setAttempted(false)
      setReason('')
    },
    onError: (error) =>
      handleServerError(error, t('Verification preparation failed')),
  })
  const execute = useMutation({
    mutationFn: async () => {
      if (!plan || !approved) {
        throw new Error(t('Signed authorization required'))
      }
      // Hash the exact imported file bytes, including whitespace. Never sign or
      // infer approval in the browser, and never retry a paid POST.
      const bytes = new TextEncoder().encode(manifestJSON)
      const digest = await crypto.subtle.digest('SHA-256', bytes)
      const hash = Array.from(new Uint8Array(digest), (byte) =>
        byte.toString(16).padStart(2, '0')
      ).join('')
      return executeChannelVerification(
        props.channelId,
        plan,
        manifestJSON,
        hash
      )
    },
    retry: false,
    onMutate: () => {
      setAttempted(true)
      setConfirming(false)
      props.onBusyChange(true)
    },
    onSuccess: (response) => {
      setReason(response.data.execution_reason ?? '')
      props.onComplete(response.data)
    },
    onError: (error) => {
      setReason(
        t(
          'Execution unresolved. Refresh evidence and resume the existing intent; do not submit again.'
        )
      )
      handleServerError(error)
    },
    onSettled: () => props.onBusyChange(false),
  })
  const resume = useMutation({
    mutationFn: async (
      item: NonNullable<ChannelVerificationData['items'][number]>
    ) => {
      const target = plan?.targets.find(
        (target) =>
          target.model === item.model &&
          target.fixture.id === item.fixture_id &&
          target.fixture.protocol === item.protocol &&
          target.fixture.mode === item.mode
      )
      if (!target || !item.id || !item.run_id) {
        throw new Error(t('Matching frozen plan required'))
      }
      return resumeChannelVerification(
        props.channelId,
        item.run_id,
        item.id,
        target.fixture
      )
    },
    retry: false,
    onMutate: () => props.onBusyChange(true),
    onSuccess: (response) => {
      setReason(response.data.execution_reason ?? '')
      props.onComplete(response.data)
    },
    onError: (error) => handleServerError(error),
    onSettled: () => props.onBusyChange(false),
  })
  const busy = prepare.isPending || execute.isPending || resume.isPending
  const canExecute = Boolean(
    plan &&
    approved &&
    approved.targets.every((approvedTarget) =>
      plan?.targets.some(
        (target) =>
          target.model === approvedTarget.model &&
          target.fixture.id === approvedTarget.fixture_id &&
          target.fixture.protocol === approvedTarget.protocol &&
          target.fixture.mode === approvedTarget.mode &&
          !target.blocker &&
          !target.executor_blocker &&
          target.maximum_provider_points ===
            approvedTarget.maximum_provider_points
      )
    ) &&
    !attempted &&
    !busy &&
    !props.disabled
  )
  const recoverable =
    props.verification?.items
      .flatMap((item) =>
        item.historical_evidence ? [item, item.historical_evidence] : [item]
      )
      .filter(
        (item) =>
          item.run_id &&
          item.id &&
          Boolean(
            item.task_id || item.request_id || item.idempotency_key_hash
          ) &&
          item.status !== 'RUNTIME_VERIFIED'
      ) ?? []
  return (
    <>
      <Button
        variant='outline'
        size='sm'
        disabled={props.disabled || busy}
        onClick={() => setOpen(true)}
      >
        {t('Runtime verification')}
      </Button>
      <Dialog
        open={open}
        onOpenChange={(next) => {
          if (!busy) setOpen(next)
        }}
        title={t('Runtime verification')}
        contentClassName='sm:max-w-3xl'
        bodyClassName='space-y-3'
        footer={
          <Button
            variant='outline'
            disabled={busy}
            onClick={() => setOpen(false)}
          >
            {t('Close')}
          </Button>
        }
      >
        <p className='text-muted-foreground text-sm'>
          {t(
            'Prepare is free. Paid execution requires an operator-signed manifest and explicit budget confirmation. Unknown or blocked costs cannot be executed.'
          )}
        </p>
        <div className='flex flex-wrap gap-2'>
          <Combobox
            options={props.models.map((name) => ({ value: name, label: name }))}
            value={model}
            onValueChange={setModel}
            aria-label={t('Select model')}
            placeholder={t('Select model')}
            disabled={busy}
          />
          <Button
            disabled={!model || busy || props.disabled}
            onClick={() => prepare.mutate()}
          >
            {t('Prepare verification plan')}
          </Button>
        </div>
        <label className='block space-y-1'>
          <span>{t('Verification plan JSON')}</span>
          <Textarea
            aria-label={t('Verification plan JSON')}
            value={planJSON}
            disabled={busy}
            onChange={(event) => {
              setPlanJSON(event.target.value)
              setManifestJSON('')
              setReason('')
            }}
          />
        </label>
        {plan && (
          <>
            <CopyButton value={planJSON} variant='outline' size='sm'>
              {t('Copy verification plan')}
            </CopyButton>
            <p className='text-sm'>
              {t('Maximum provider cost')}: {plan.known_maximum_provider_points}{' '}
              {t('Points')} · {t('Requests')}: {plan.planned_posts}
            </p>
            <div className='max-h-48 overflow-auto text-xs'>
              {plan.targets
                .filter(
                  (target) =>
                    target.model === model ||
                    approved?.targets.some(
                      (entry) => entry.model === target.model
                    )
                )
                .map((target) => (
                  <p
                    key={`${target.model}/${target.fixture.id}/${target.fixture.mode}`}
                  >
                    {target.model} · {target.fixture.mode} ·{' '}
                    {target.blocker ||
                      target.executor_blocker ||
                      target.maximum_provider_points ||
                      t('Unknown')}
                  </p>
                ))}
            </div>
          </>
        )}
        <p className='text-muted-foreground text-xs'>
          {t(
            'Copy and save the plan for offline operator approval. Import the signed manifest here; private signing keys must never be entered in the browser.'
          )}
        </p>
        <label className='block space-y-1'>
          <span>{t('Signed authorization JSON')}</span>
          <Textarea
            aria-label={t('Signed authorization JSON')}
            value={manifestJSON}
            disabled={busy || attempted}
            onChange={(event) => setManifestJSON(event.target.value)}
          />
        </label>
        <Button disabled={!canExecute} onClick={() => setConfirming(true)}>
          {t('Review paid execution')}
        </Button>
        {reason && (
          <p role='status' className='text-sm wrap-break-word'>
            {reason}
          </p>
        )}
        {busy && (
          <p role='status'>
            {t('Verification running. Do not submit another request.')}
          </p>
        )}
        {recoverable.map((item) => (
          <Button
            key={item.id}
            variant='outline'
            size='sm'
            disabled={busy || !plan}
            onClick={() => resume.mutate(item)}
          >
            {t('Resume verification (GET only)')}: {item.model} · {item.run_id}/
            {item.id}
          </Button>
        ))}
      </Dialog>
      <ConfirmDialog
        open={confirming}
        onOpenChange={setConfirming}
        title={t('Confirm paid verification')}
        confirmText={t('Run paid verification')}
        disabled={!canExecute}
        handleConfirm={() => execute.mutate()}
        desc={
          <div className='space-y-2'>
            <p>
              {t(
                'This creates paid provider tasks. No automatic retries. Verification states change only after evidence is saved.'
              )}
            </p>
            <p>
              {t('Maximum provider cost')}:{' '}
              {approved?.max_total_provider_points} {t('Points')} ·{' '}
              {t('Requests')}: {approved?.max_requests}
            </p>
            <div className='max-h-48 overflow-auto'>
              {approved?.targets.map((target) => (
                <p key={`${target.model}/${target.fixture_id}/${target.mode}`}>
                  {target.model} · {target.protocol} · {target.mode} ·{' '}
                  {target.maximum_provider_points} {t('Points')}
                </p>
              ))}
            </div>
          </div>
        }
      />
    </>
  )
}
