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

import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

import { preflightTaskChannel } from '../../api'
import type { TaskChannelDiagnostic } from '../../types'

export function TaskConnectivityAction(props: {
  channelId: number
  model: string
  operation?: string
  diagnostic?: TaskChannelDiagnostic
  multiKey: boolean
  disabled: boolean
  onComplete: (diagnostic: TaskChannelDiagnostic) => void
}) {
  const { t } = useTranslation()
  const [keyIndex, setKeyIndex] = useState<string | null>(null)
  const mutation = useMutation({
    mutationFn: async () => {
      const response = requireServerSuccess(
        await preflightTaskChannel(
          props.channelId,
          props.model,
          props.operation,
          'connectivity',
          props.multiKey && keyIndex !== null ? Number(keyIndex) : undefined
        )
      )
      if (
        !response.data ||
        response.data.mode !== 'connectivity' ||
        !['pass', 'partial', 'untested', 'fail'].includes(response.data.outcome)
      ) {
        throw new Error(t('Invalid DFLOP catalog response'))
      }
      return response.data
    },
    onSuccess: props.onComplete,
    onError: (error) => handleServerError(error, t('Upstream not tested')),
  })
  if (!props.diagnostic?.connectivity_available) return null
  const indices = props.diagnostic.connectivity_key_indices ?? []
  const selectedValid =
    !props.multiKey || (keyIndex !== null && indices.includes(Number(keyIndex)))
  return (
    <div className='flex items-center gap-2'>
      {props.multiKey && (
        <Combobox
          options={indices.map((index) => ({
            value: String(index),
            label: t('API key index {{index}}', { index }),
          }))}
          value={keyIndex}
          onValueChange={setKeyIndex}
          aria-label={t('Select an enabled API key')}
          placeholder={t('Select an enabled API key')}
          disabled={props.disabled || mutation.isPending}
          className='w-40'
        />
      )}
      <Button
        variant='outline'
        size='sm'
        disabled={props.disabled || mutation.isPending || !selectedValid}
        onClick={() => mutation.mutate()}
      >
        {t('Check DFLOP connectivity')}
      </Button>
    </div>
  )
}
