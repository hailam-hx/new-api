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
import { useQueryClient } from '@tanstack/react-query'
import { EyeOff } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'
import { handleBatchDisableModels } from '@/features/models/lib/model-actions'
import { invalidateVendorData } from '@/features/models/vendor-api'
import { handleServerError } from '@/lib/handle-server-error'

import { loadChannelModels } from '../../lib/channel-model-visibility'

export function ChannelTestHideFailed({
  models,
  disabled,
  onBusyChange,
}: {
  models: string[]
  disabled: boolean
  onBusyChange: (busy: boolean) => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [confirmedModels, setConfirmedModels] = useState<string[]>([])
  const [busy, setBusy] = useState(false)

  const hideFailedModels = async () => {
    if (busy || disabled || confirmedModels.length === 0) return
    setBusy(true)
    onBusyChange(true)
    try {
      const metadata = await loadChannelModels()
      const names = new Set(confirmedModels)
      const targets = metadata.filter(
        (model) =>
          model.name_rule === 0 &&
          names.has(model.model_name) &&
          model.square_state !== 'hidden'
      )
      if (targets.length > 0) {
        await handleBatchDisableModels(
          targets.filter((model) => model.id > 0).map((model) => model.id),
          queryClient,
          undefined,
          targets.filter((model) => model.id === 0)
        )
      }
      setConfirmedModels([])
    } catch (error: unknown) {
      handleServerError(error, t('Failed to hide model from model square'))
    } finally {
      // Refresh even if a batch had mixed outcomes or an ambiguous network error.
      try {
        await invalidateVendorData(queryClient)
      } finally {
        setBusy(false)
        onBusyChange(false)
      }
    }
  }

  return (
    <>
      <Button
        variant='outline'
        size='sm'
        disabled={disabled || busy || models.length === 0}
        onClick={() => setConfirmedModels([...new Set(models)])}
      >
        <EyeOff data-icon='inline-start' />
        {t('Hide failed models ({{count}})', { count: models.length })}
      </Button>
      <ConfirmDialog
        open={confirmedModels.length > 0}
        onOpenChange={(open) => {
          if (!open && !busy) setConfirmedModels([])
        }}
        title={t('Hide failed models')}
        desc={
          <div className='space-y-2'>
            <p>
              {t(
                'Hide these models from the model square across all channels. Channel configuration and pricing are preserved. You can show them again in model management.'
              )}
            </p>
            <div className='max-h-40 overflow-auto break-all'>
              {confirmedModels.join(', ')}
            </div>
          </div>
        }
        confirmText={t('Hide failed models')}
        isLoading={busy}
        handleConfirm={hideFailedModels}
      />
    </>
  )
}
